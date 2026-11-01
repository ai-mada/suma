package runner

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	cu "github.com/ai-mada/suma/internal/cursorcontract"
	delivery "github.com/ai-mada/suma/internal/inputdelivery"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	ic "github.com/ai-mada/suma/internal/toolinputcontract"
)

type cursorBridgeWriter190 func([]byte) (int, error)

func (f cursorBridgeWriter190) Write(b []byte) (int, error) { return f(b) }

// Production ACP driver, Runner bridge, Client and real Journal communicate
// with an owned HTTP/stdio fixture. No native host or provider is started.
func TestCursorInputBridgeReplaysDurableFactsBeforeResult(t *testing.T) {
	a, profile, _, template := cursorToolInputFixture190(t)
	a.Config["goal"] = "Continue after an explicit answer and plan decision"
	// Compare the complete transported representation: nested config values
	// become JSON objects at this HTTP boundary, rather than Go struct pointers.
	var transported Assignment
	assignmentBytes, _ := json.Marshal(a)
	_ = json.Unmarshal(assignmentBytes, &transported)
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	records := map[string]ic.ToolInput{}
	opens, polls, observations := 0, 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer owned190" {
			t.Error("original scoped identity changed")
		}
		var body struct {
			ProjectID   string               `json:"project_id"`
			Assignment  Assignment           `json:"assignment"`
			Request     ic.ToolInputRequest  `json:"request"`
			InputID     string               `json:"input_id"`
			Observation delivery.Observation `json:"observation"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.ProjectID != template.ProjectID || !samePermissionValue(body.Assignment, transported) {
			t.Error("original source scope changed")
		}
		mu.Lock()
		defer mu.Unlock()
		var v ic.ToolInput
		switch r.URL.Path {
		case "/api/v1/runner/tool-inputs":
			opens++
			v = template
			v.ID = "bridge190-" + body.Request.Cursor.RequestID.Kind
			v.Request = body.Request
			decision := cu.CursorInputDecision{Outcome: "rejected", Reason: "Revise the plan"}
			if body.Request.Cursor.Questions != nil {
				decision = cu.CursorInputDecision{Outcome: "answered", Answers: []cu.CursorInputSelection{{QuestionID: "scope190", OptionIDs: []string{"http", "cli"}}}}
			}
			answer := *template.Answer
			answer.Cursor = &decision
			answer.AnswerSHA256, _ = cu.AnswerDigest(*body.Request.Cursor, decision)
			v.Answer = &answer
			records[v.ID] = v
		case "/api/v1/runner/tool-inputs/poll":
			polls++
			v = records[body.InputID]
		case "/api/v1/runner/tool-inputs/observations":
			observations++
			v = records[body.InputID]
			copy := body.Observation
			v.Observation = &copy
			v.Version++
			if copy.ExecutionClosed {
				v.State = "closed"
			}
			records[v.ID] = v
		default:
			t.Error("unexpected current input route", r.URL.Path)
		}
		if ic.ValidateInput(v) != nil {
			t.Error("fixture produced invalid scoped input")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, Token: "owned190", ProjectID: template.ProjectID}
	cfg := ExecConfig{Journal: j, ToolInputClient: client}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	b, err := newACPExecutionBridge(ctx, a, cfg, profile, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	packets := make(chan []byte, 32)
	copied := make(chan struct{})
	fixtureDone := make(chan struct{})
	go func() {
		defer close(copied)
		_, _ = b.CopyInput(cursorBridgeWriter190(func(raw []byte) (int, error) {
			select {
			case packets <- append([]byte{}, raw...):
				return len(raw), nil
			case <-ctx.Done():
				return 0, ctx.Err()
			}
		}))
	}()
	go func() {
		defer close(fixtureDone)
		var prompt json.RawMessage
		answers := 0
		send := func(v any) { raw, _ := json.Marshal(v); _, _ = b.Write(append(raw, '\n')) }
		result := func(id json.RawMessage, value any) { send(map[string]any{"jsonrpc": "2.0", "id": id, "result": value}) }
		models := []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": profile.ACPExecution.Model, "options": []any{map[string]any{"value": profile.ACPExecution.Model, "name": "Fixed"}}}}
		for {
			select {
			case raw := <-packets:
				var p struct {
					ID     json.RawMessage `json:"id"`
					Method string          `json:"method"`
					Result json.RawMessage `json:"result"`
				}
				if json.Unmarshal(raw, &p) != nil {
					t.Error("invalid SDK frame")
					return
				}
				switch p.Method {
				case "initialize":
					result(p.ID, map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}})
				case "session/new":
					result(p.ID, map[string]any{"sessionId": "bridge-session190", "configOptions": models})
				case "session/set_config_option":
					result(p.ID, map[string]any{"configOptions": models})
				case "session/prompt":
					prompt = append(json.RawMessage(nil), p.ID...)
					send(map[string]any{"jsonrpc": "2.0", "id": 190, "method": cu.AskQuestionMethod, "params": map[string]any{"toolCallId": "ask190", "questions": []any{map[string]any{"id": "scope190", "prompt": "Keep both interfaces?", "allowMultiple": true, "options": []any{map[string]any{"id": "cli", "label": "CLI"}, map[string]any{"id": "http", "label": "HTTP"}}}}}})
				case "":
					answers++
					if answers == 1 {
						if !strings.Contains(string(p.Result), `"selectedOptionIds":["cli","http"]`) {
							t.Error("native option IDs were not reconstructed from saved decision")
						}
						send(map[string]any{"jsonrpc": "2.0", "id": "190", "method": cu.CreatePlanMethod, "params": map[string]any{"toolCallId": "plan190", "plan": "# Review this plan", "todos": []any{}, "isProject": false}})
					} else {
						if !strings.Contains(string(p.Result), `"outcome":"rejected"`) {
							t.Error("plan rejection became RPC error or approval")
						}
						result(prompt, map[string]any{"stopReason": "end_turn"})
					}
				default:
					t.Error("unexpected native method", p.Method)
					return
				}
			case <-copied:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	select {
	case <-copied:
	case <-ctx.Done():
		t.Fatal("owned SDK input bridge did not settle")
	}
	<-fixtureDone
	result := b.driver.Finish()
	if result.Terminal != "completed" || len(result.Evidence.UserInputs) != 2 {
		t.Fatal("controlled bridge failed", result.Error)
	}
	if err = ReplayToolInputObservations(ctx, client, j); err != nil {
		t.Fatal("durable facts did not replay before Result", err)
	}
	if len(j.pendingToolInputEvents()) != 0 {
		t.Fatal("input facts remained unacknowledged")
	}
	mu.Lock()
	defer mu.Unlock()
	if opens != 2 || polls != 4 || observations < 6 {
		t.Fatal("original Open/Answer/beforeWrite/replay chain changed", opens, polls, observations)
	}
	for _, record := range records {
		if record.Observation == nil || !record.Observation.ReplyWritten || !record.Observation.ExecutionClosed || record.Observation.RequestResolved {
			t.Fatal("native write or execution closure was fabricated")
		}
	}
	if result.Evidence.HostProcessStopped || result.Evidence.HostProcessTreeStopped {
		t.Fatal("stdio fixture invented host process evidence")
	}
}

var _ io.Writer = cursorBridgeWriter190(nil)

func TestCursorInputResultJournalKeepsImmutableEvidence(t *testing.T) {
	a, p, r, v := cursorToolInputFixture190(t)
	j, _ := savedToolInput187(t, a, p, r, v)
	s := r.Cursor
	o := delivery.Observation{Sequence: 1, ParamsSHA256: s.ParamsSHA256, ExecutionClosed: true}
	proof := ac.Evidence{ConfigurationSHA256: s.ConfigurationSHA256, RequestedModel: p.ACPExecution.Model, SelectedModel: p.ACPExecution.Model, ModelSelectionAcknowledged: true, SessionID: s.SessionID, ProtocolVersion: 1, PromptStarted: true, UserInputs: []cu.CursorInputEvidence{{InputID: v.ID, RequestID: s.RequestID, PromptID: s.PromptID, SessionID: s.SessionID, Method: s.Method, ConfigurationSHA256: s.ConfigurationSHA256, ParamsSHA256: s.ParamsSHA256, Kind: "questions", QuestionCount: 1, Observation: &o}}}
	evidence := &tc.Evidence{Profile: p, Terminal: "unknown", UsageState: tc.UsageUnreported, SessionID: s.SessionID, ACPExecution: &proof}
	if tc.ValidateEvidence(*evidence) != nil {
		t.Fatal("invalid owned unknown evidence")
	}
	if err := j.Finish(Result{Assignment: a, ExitCode: -1, OutcomeUnknown: true, ToolEvidence: evidence}); err != nil {
		t.Fatal(err)
	}
	proof.UserInputs[0].InputID = "caller-mutated"
	first := j.Pending()
	if len(first) != 1 || first[0].ToolEvidence.ACPExecution.UserInputs[0].InputID != v.ID {
		t.Fatal("caller changed saved input evidence")
	}
	first[0].ToolEvidence.ACPExecution.UserInputs[0].Observation.Sequence = 99
	if j.Pending()[0].ToolEvidence.ACPExecution.UserInputs[0].Observation.Sequence != 1 {
		t.Fatal("pending result view changed durable delivery facts")
	}
}
