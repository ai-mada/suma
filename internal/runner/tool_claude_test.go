package runner

import (
	"encoding/json"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"strings"
	"testing"
)

func claude158TestSpec() CommandSpec {
	return CommandSpec{Adapter: "claude", Protocol: "claude-jsonl-v1", Executable: "claude", Input: "goal", Args: []string{"-p", "--input-format", "text", "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--safe-mode", "--restricted", "--settings", `{"disableAllHooks":true,"permissions":{"blockReadsOutsideWorkingDirectories":true,"blockWritesOutsideWorkingDirectories":true}}`, "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--permission-mode", "dontAsk", "--permission-prompts", "none", "--tools", "Read,Edit,Write,Glob,Grep,Bash", "--allowedTools", "Read,Edit,Write,Glob,Grep,Bash(git status *),Bash(git diff *),Bash(git add *),Bash(git commit *)", "--model", "claude-sonnet-5"}}
}

const claude158Init = `{"type":"system","subtype":"init","session_id":"claude-session","model":"claude-sonnet-5","mcp_servers":[],"permissionMode":"dontAsk"}` + "\n"
const claude158Success = `{"type":"result","subtype":"success","is_error":false,"session_id":"claude-session","terminal_reason":"completed","permission_denials":[],"result":"done","usage":{"input_tokens":4,"output_tokens":2},"modelUsage":{"claude-sonnet-5":{"inputTokens":7,"outputTokens":3}}}` + "\n"

func claude158Stream(events *[]tc.Event) *toolStream {
	p := structuredProfile(tc.ClaudeProtocol)
	p.Adapter = "claude"
	p.ToolVersion = tc.ClaudeCLIVersion
	p.ReadOnly = false
	p.ClaudeExecution, _ = localClaudeCapability(claude158TestSpec())
	return newToolStream(p, []string{"private-secret"}, func(e tc.Event) error {
		if events != nil {
			*events = append(*events, e)
		}
		return nil
	})
}
func TestClaudeCLI158ForegroundTypedFactsAndUsage(t *testing.T) {
	var events []tc.Event
	s := claude158Stream(&events)
	body := claude158Init + `{"type":"assistant","session_id":"claude-session","message":{"id":"m1","role":"assistant","content":[{"type":"text","text":"中文 private-secret"},{"type":"thinking","thinking":"private-thought"},{"type":"tool_use","id":"read1","name":"Read","input":{"file_path":"private-tool-input"}}],"usage":{"input_tokens":3,"output_tokens":0}}}` + "\n" + `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"read1","content":"private-tool-output"}]}}` + "\n" + claude158Success
	for _, b := range []byte(body) {
		_, _ = s.Write([]byte{b})
	}
	e, err := s.finish()
	if err != nil || e.Terminal != "completed" || e.ClaudeExecution == nil || e.ClaudeExecution.AcknowledgedModel != "claude-sonnet-5" || e.ClaudeExecution.ResultTerminalReason != "completed" || e.UsageState != tc.UsagePartial || e.Usage == nil || *e.Usage.InputTokens != 7 || *e.Usage.OutputTokens != 3 {
		t.Fatalf("missing exact Claude foreground facts: %+v %v", e, err)
	}
	if err = tc.ValidateEvidence(*e); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(events)
	for _, private := range []string{"private-secret", "private-thought", "private-tool-input", "private-tool-output"} {
		if strings.Contains(string(raw), private) {
			t.Fatalf("private vendor payload persisted: %s", private)
		}
	}
}
func TestClaudeCLI158RejectsUnconfirmedExecution(t *testing.T) {
	for name, body := range map[string]string{
		"missing_init":             claude158Success,
		"changed_model":            strings.Replace(claude158Init, "claude-sonnet-5", "claude-other-5", 1) + claude158Success,
		"changed_session":          claude158Init + strings.ReplaceAll(claude158Success, "claude-session", "other"),
		"external_mcp":             strings.Replace(claude158Init, `"mcp_servers":[]`, `"mcp_servers":[{"name":"other"}]`, 1) + claude158Success,
		"unexpected_tool":          strings.Replace(claude158Init, `"permissionMode":"dontAsk"`, `"permissionMode":"dontAsk","tools":["Agent"]`, 1) + claude158Success,
		"assistant_without_result": claude158Init + `{"type":"assistant","message":{"content":[{"type":"text","text":"success"}]}}` + "\n",
		"deferred":                 claude158Init + strings.Replace(claude158Success, `"result":"done"`, `"result":"done","deferred_tool_use":{"id":"wait"}`, 1),
		"queued":                   claude158Init + strings.Replace(claude158Success, `"result":"done"`, `"result":"done","queued_turn_count":1`, 1),
		"incomplete_reason":        claude158Init + strings.Replace(claude158Success, `"terminal_reason":"completed"`, `"terminal_reason":"background_requested"`, 1),
		"subagent":                 claude158Init + `{"type":"assistant","parent_tool_use_id":"child","message":{"content":[]}}` + "\n" + claude158Success,
		"background":               claude158Init + `{"type":"system","subtype":"task_started","task_id":"child"}` + "\n" + claude158Success,
		"permission_bridge":        claude158Init + `{"type":"control_request","request":{"subtype":"can_use_tool","input":{"secret":"do-not-save"}}}` + "\n" + claude158Success,
		"pending_tool":             claude158Init + `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"pending","name":"Read","input":{}}]}}` + "\n" + claude158Success,
		"duplicate_result":         claude158Init + claude158Success + claude158Success,
	} {
		t.Run(name, func(t *testing.T) {
			s := claude158Stream(nil)
			aborted := false
			s.abort = func() { aborted = true }
			_, _ = s.Write([]byte(body))
			e, err := s.finish()
			if err == nil || e.Terminal != "unknown" || !aborted {
				t.Fatalf("uncertain Claude execution became success: %+v %v", e, err)
			}
		})
	}
}
func TestClaudeCLI158PermissionDenialIsNotSuccess(t *testing.T) {
	s := claude158Stream(nil)
	_, _ = s.Write([]byte(claude158Init + strings.Replace(claude158Success, `"permission_denials":[]`, `"permission_denials":[{"tool_name":"Bash","tool_input":{"private":"never-save"}}]`, 1)))
	e, err := s.finish()
	if err != nil || e.Terminal != "failed" || e.ClaudeExecution.PermissionDenialCount != 1 {
		t.Fatalf("permission denial lost: %+v %v", e, err)
	}
}
func TestClaudeCLI158CrashRetainsKnownPartialUsage(t *testing.T) {
	s := claude158Stream(nil)
	message := `{"type":"assistant","message":{"id":"one","content":[],"usage":{"input_tokens":5,"cache_read_input_tokens":2,"output_tokens":0}}}` + "\n"
	crash := `{"type":"result","subtype":"error_during_execution","is_error":true,"session_id":"claude-session","permission_denials":[],"usage":{"input_tokens":0,"output_tokens":0},"modelUsage":{}}` + "\n"
	_, _ = s.Write([]byte(claude158Init + message + message + crash))
	e, err := s.finish()
	if err != nil || e.Terminal != "failed" || e.UsageState != tc.UsagePartial || e.Usage == nil || *e.Usage.InputTokens != 5 || e.Usage.OutputTokens != nil {
		t.Fatalf("crash fabricated zero or doubled usage: %+v %v", e, err)
	}
}

func TestClaudeCLI158ControlledConfiguration(t *testing.T) {
	spec := claude158TestSpec()
	if err := ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"claude-develop": spec}}); err != nil {
		t.Fatalf("controlled native Claude profile unavailable: %v", err)
	}
	for name, change := range map[string]func(*CommandSpec){
		"resume": func(s *CommandSpec) { s.Args = append(s.Args, "--resume", "other-session") },
		"bypass": func(s *CommandSpec) {
			for i, v := range s.Args {
				if v == "dontAsk" {
					s.Args[i] = "bypassPermissions"
				}
			}
		},
		"shell_launcher": func(s *CommandSpec) { s.Executable = "claude.cmd" },
		"node_launcher":  func(s *CommandSpec) { s.Executable = "node.exe" },
		"broad_tool_approval": func(s *CommandSpec) {
			for i, v := range s.Args {
				if strings.Contains(v, "Bash(git status") {
					s.Args[i] = "Bash"
				}
			}
		},
		"mcp_discovery": func(s *CommandSpec) {
			for i, v := range s.Args {
				if v == `{"mcpServers":{}}` {
					s.Args[i] = `{"mcpServers":{"other":{"command":"arbitrary"}}}`
				}
			}
		},
		"alias_model":      func(s *CommandSpec) { s.Args[len(s.Args)-1] = "sonnet" },
		"auth_environment": func(s *CommandSpec) { s.EnvFrom = []string{"ANTHROPIC_API_KEY"} },
		"home_override":    func(s *CommandSpec) { s.Env = map[string]string{"HOME": "different-actor-home"} },
		"formal_test":      func(s *CommandSpec) { s.TestEvidence = true },
	} {
		t.Run(name, func(t *testing.T) {
			value := spec
			value.Args = append([]string{}, spec.Args...)
			change(&value)
			if ValidateToolConfiguration(ExecConfig{Commands: map[string]CommandSpec{"claude-develop": value}}) == nil {
				t.Fatal("unsafe or unrelated Claude capability accepted")
			}
		})
	}
}
