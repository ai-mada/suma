package runner

import (
	"encoding/json"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const codebuddyTestInit = `{"type":"system","subtype":"init","session_id":"cb-session"}` + "\n"
const codebuddyTestSuccess = `{"type":"result","subtype":"success","is_error":false,"session_id":"cb-session","result":"done","usage":{"input_tokens":4,"output_tokens":2,"cache_read_input_tokens":1,"cache_creation_input_tokens":0}}` + "\n"

func codebuddyTestStream(events *[]tc.Event) *toolStream {
	p := structuredProfile("codebuddy-jsonl-v1")
	p.Adapter = "codebuddy"
	p.ReadOnly = false
	return newToolStream(p, []string{"private-secret"}, func(e tc.Event) error {
		if events != nil {
			*events = append(*events, e)
		}
		return nil
	})
}

func TestCodebuddyStreamForegroundTerminalAndSelectedFacts(t *testing.T) {
	var events []tc.Event
	s := codebuddyTestStream(&events)
	body := codebuddyTestInit +
		`{"type":"assistant","session_id":"cb-session","message":{"id":"msg-1","role":"assistant","content":[{"type":"text","text":"中文 private-secret"},{"type":"tool_use","id":"tool-1","name":"Read","input":{"password":"never-persist"}}],"usage":{"input_tokens":3,"output_tokens":0}}}` + "\n" +
		`{"type":"user","session_id":"cb-session","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tool-1","content":"do-not-persist-tool-output"}]}}` + "\n" + codebuddyTestSuccess
	for _, b := range []byte(body) {
		_, _ = s.Write([]byte{b})
	}
	e, err := s.finish()
	if err != nil || e.Terminal != "completed" || e.SessionID != "cb-session" || e.UsageState != tc.UsageComplete || e.Usage == nil || *e.Usage.InputTokens != 4 || *e.Usage.OutputTokens != 2 || *e.Usage.CachedInputTokens != 1 {
		t.Fatalf("CodeBuddy completion/usage: %+v %v", e, err)
	}
	raw, _ := json.Marshal(events)
	for _, secret := range []string{"private-secret", "never-persist", "do-not-persist-tool-output"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("private unselected data retained: %s", secret)
		}
	}
	if !strings.Contains(string(raw), "中文") || !strings.Contains(string(raw), "activity_started") || !strings.Contains(string(raw), "activity_completed") {
		t.Fatal("selected progress facts missing")
	}
}

func TestCodebuddyStreamRequiresWholeRunTerminalAndIdentity(t *testing.T) {
	for name, body := range map[string]string{
		"assistant-only":   codebuddyTestInit + `{"type":"assistant","message":{"content":[{"type":"text","text":"done"}]}}` + "\n",
		"result-only":      codebuddyTestSuccess,
		"missing-bool":     codebuddyTestInit + `{"type":"result","subtype":"success","session_id":"cb-session"}` + "\n",
		"unknown-subtype":  codebuddyTestInit + `{"type":"result","subtype":"new-success","is_error":false,"session_id":"cb-session"}` + "\n",
		"changed-session":  codebuddyTestInit + strings.ReplaceAll(codebuddyTestSuccess, "cb-session", "foreign"),
		"duplicate-init":   codebuddyTestInit + codebuddyTestInit + codebuddyTestSuccess,
		"duplicate-result": codebuddyTestInit + codebuddyTestSuccess + codebuddyTestSuccess,
		"late-log":         codebuddyTestInit + codebuddyTestSuccess + `{"type":"log","log":{"message":"late"}}` + "\n",
		"duplicate-key":    codebuddyTestInit + `{"type":"result","subtype":"success","is_error":true,"is_error":false,"session_id":"cb-session"}` + "\n",
		"oversize":         codebuddyTestInit + strings.Repeat("x", toolLineLimit+1),
		"pending-tool":     codebuddyTestInit + `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"pending","name":"Bash","input":{}}]}}` + "\n" + codebuddyTestSuccess,
	} {
		t.Run(name, func(t *testing.T) {
			s := codebuddyTestStream(nil)
			_, _ = s.Write([]byte(body))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" {
				t.Fatalf("uncertified run completed: %+v %v", e, err)
			}
		})
	}
}

func TestCodebuddyStreamDoesNotApproveOrOutliveBackgroundWork(t *testing.T) {
	for name, frame := range map[string]string{
		"permission":              `{"type":"control_request","request_id":"permission","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"secret-command"}}}`,
		"waiting":                 `{"type":"waiting_input","message":"secret-question"}`,
		"background-start":        `{"type":"system","subtype":"task_started","task_id":"bg"}`,
		"background-progress":     `{"type":"system","subtype":"task_progress","task_id":"bg"}`,
		"background-notification": `{"type":"system","subtype":"task_notification","task_id":"bg","status":"completed"}`,
		"background-tool":         `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"bg","name":"Bash","input":{"run_in_background":true}}]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			var events []tc.Event
			s := codebuddyTestStream(&events)
			aborted := false
			s.abort = func() { aborted = true }
			_, _ = s.Write([]byte(codebuddyTestInit + frame + "\n" + codebuddyTestSuccess))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" || !aborted {
				t.Fatalf("unattended/background request not stopped: %+v %v", e, err)
			}
			if name == "permission" || name == "waiting" {
				found := false
				for _, event := range events {
					found = found || event.Kind == "waiting_input"
				}
				if !found {
					t.Fatal("waiting-input fact missing")
				}
			}
		})
	}
}

func TestCodebuddyStreamFailureAndUsageAccounting(t *testing.T) {
	for name, frame := range map[string]string{
		"is-error":          `{"type":"result","subtype":"success","is_error":true,"session_id":"cb-session","result":"failed"}`,
		"turn-limit":        `{"type":"result","subtype":"error_max_turns","is_error":false,"session_id":"cb-session"}`,
		"permission-denied": `{"type":"result","subtype":"success","is_error":false,"session_id":"cb-session","permission_denials":[{"tool_name":"Edit","tool_input":{"secret":"not-retained"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := codebuddyTestStream(nil)
			_, _ = s.Write([]byte(codebuddyTestInit + frame + "\n"))
			e, err := s.finish()
			if err != nil || e.Terminal != "failed" {
				t.Fatalf("failure lost: %+v %v", e, err)
			}
		})
	}
	t.Run("model-totals-override-assistant", func(t *testing.T) {
		s := codebuddyTestStream(nil)
		_, _ = s.Write([]byte(codebuddyTestInit +
			`{"type":"assistant","message":{"id":"same","content":[],"usage":{"input_tokens":999,"output_tokens":0}}}` + "\n" +
			`{"type":"result","subtype":"success","is_error":false,"session_id":"cb-session","modelUsage":{"a":{"inputTokens":2,"outputTokens":3,"cacheReadInputTokens":1},"b":{"inputTokens":4,"outputTokens":5,"cacheReadInputTokens":2}}}` + "\n"))
		e, err := s.finish()
		if err != nil || e.Usage == nil || *e.Usage.InputTokens != 6 || *e.Usage.OutputTokens != 8 || *e.Usage.CachedInputTokens != 3 || e.UsageState != tc.UsageComplete {
			t.Fatalf("model totals: %+v %v", e, err)
		}
	})
	t.Run("assistant-subtotal-deduplicated-partial", func(t *testing.T) {
		s := codebuddyTestStream(nil)
		frame := `{"type":"assistant","message":{"id":"same","content":[],"usage":{"input_tokens":7,"output_tokens":0,"cache_creation_input_tokens":2}}}` + "\n"
		_, _ = s.Write([]byte(codebuddyTestInit + frame + frame + `{"type":"result","subtype":"success","is_error":false,"session_id":"cb-session"}` + "\n"))
		e, err := s.finish()
		if err != nil || e.Usage == nil || *e.Usage.InputTokens != 7 || e.Usage.OutputTokens != nil || e.UsageState != tc.UsagePartial {
			t.Fatalf("assistant subtotal: %+v %v", e, err)
		}
	})
	t.Run("partial-models-remain-partial", func(t *testing.T) {
		s := codebuddyTestStream(nil)
		_, _ = s.Write([]byte(codebuddyTestInit + `{"type":"result","subtype":"success","is_error":false,"session_id":"cb-session","modelUsage":{"a":{"inputTokens":2},"b":{"outputTokens":3}}}` + "\n"))
		e, err := s.finish()
		if err != nil || e.Usage == nil || *e.Usage.InputTokens != 2 || *e.Usage.OutputTokens != 3 || e.UsageState != tc.UsagePartial {
			t.Fatalf("incomplete model accounting certified complete: %+v %v", e, err)
		}
	})
	for name, usage := range map[string]string{
		"negative":           `"usage":{"input_tokens":-1,"output_tokens":1}`,
		"fraction":           `"usage":{"input_tokens":1.5,"output_tokens":1}`,
		"null":               `"usage":null`,
		"unsafe":             `"usage":{"input_tokens":9007199254740992,"output_tokens":1}`,
		"aggregate-overflow": `"modelUsage":{"a":{"inputTokens":9007199254740991,"outputTokens":0},"b":{"inputTokens":1,"outputTokens":0}}`,
	} {
		t.Run(name, func(t *testing.T) {
			s := codebuddyTestStream(nil)
			_, _ = s.Write([]byte(codebuddyTestInit + `{"type":"result","subtype":"success","is_error":false,"session_id":"cb-session",` + usage + "}\n"))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" {
				t.Fatalf("invalid usage certified: %+v %v", e, err)
			}
		})
	}
}
