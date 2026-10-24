package runner

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func describeForTest(t *testing.T, cfg ExecConfig) []tc.Observation {
	t.Helper()
	items, err := DescribeTools(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return items
}

func TestToolInventoryUsesActualCommandAndExecutionPermission(t *testing.T) {
	exe, _ := os.Executable()
	cfg := ExecConfig{Commands: map[string]CommandSpec{"local": {Executable: exe, ReadOnly: true}, "missing": {Executable: filepath.Join(t.TempDir(), "not-installed")}}}
	observations := describeForTest(t, cfg)
	if len(observations) != 2 || len(ToolCapabilities(observations)) != 0 {
		t.Fatalf("disabled execution advertised: %+v", observations)
	}
	cfg.AllowExec = true
	observations = describeForTest(t, cfg)
	if len(ToolCapabilities(observations)) != 1 || ToolCapabilities(observations)[0] != "command:local" {
		t.Fatalf("wrong available capabilities: %+v", observations)
	}
	for _, o := range observations {
		if o.Profile.CommandID == "local" {
			if o.Profile.Adapter != "command" || o.Profile.Protocol != "text-v1" || o.Profile.ToolVersion != "local-command" || len(o.Profile.ExecutableSHA256) != 64 || len(o.Profile.ProfileSHA256) != 64 {
				t.Fatalf("profile not probed: %+v", o)
			}
		}
	}
}

func TestToolProfileDriftRefusesBeforeCommandLaunch(t *testing.T) {
	exe, _ := os.Executable()
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"local": {Executable: exe, ReadOnly: true}}}
	cfg.ToolInventory = describeForTest(t, cfg)
	cfg.Commands["local"] = CommandSpec{Executable: exe, ReadOnly: true, Args: []string{"-test.run=TestAgentInputHelper", "--"}, Env: map[string]string{"SUMA_INPUT_HELPER": "1"}, Input: "agent-jsonl"}
	r := Execute(context.Background(), Assignment{Type: "agent", Command: []string{"local"}, Config: map[string]any{"goal": "inspect project"}}, cfg)
	if r.ExitCode == 0 || !strings.Contains(r.Error, "profile") {
		t.Fatalf("drift launched: %+v", r)
	}
}

func TestToolInventoryDoesNotAdvertiseMissingIsolationOrUnsupportedTerminal(t *testing.T) {
	exe, _ := os.Executable()
	items := describeForTest(t, ExecConfig{AllowExec: true, Commands: map[string]CommandSpec{"write": {Executable: exe}, "open": {Executable: "missing-opencode", Adapter: "opencode", Protocol: "opencode-jsonl-v1"}}})
	for _, o := range items {
		if o.Available {
			t.Fatalf("unexecutable profile advertised: %+v", o)
		}
		if o.Profile.CommandID == "write" && !strings.Contains(o.Reason, "repository") {
			t.Fatal(o.Reason)
		}
		if o.Profile.CommandID == "open" && !strings.Contains(o.Reason, "whole-run") {
			t.Fatal(o.Reason)
		}
	}
}

func structuredProfile(protocol string) tc.Profile {
	adapter := "codex"
	if protocol == "suma-jsonl-v1" {
		adapter = "suma"
	}
	return tc.Profile{CommandID: "agent", Adapter: adapter, Protocol: protocol, ProfileSHA256: strings.Repeat("a", 64), ExecutableSHA256: strings.Repeat("b", 64), ToolVersion: "fixture", ReadOnly: true}
}

func TestCodexStreamHandlesFragmentedSafeFactsAndDoesNotRetainToolArguments(t *testing.T) {
	events := []tc.Event{}
	stream := newToolStream(structuredProfile("codex-jsonl-v1"), []string{"private-secret"}, func(e tc.Event) error { events = append(events, e); return nil })
	body := []byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-1\"}\n{\"type\":\"turn.started\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"item-1\",\"type\":\"mcp_tool_call\",\"status\":\"completed\",\"arguments\":{\"password\":\"never-persist\"}}}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"message-1\",\"type\":\"agent_message\",\"text\":\"中文 private-secret\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":4,\"cached_input_tokens\":2,\"output_tokens\":3,\"reasoning_output_tokens\":0}}\n")
	for _, b := range body {
		if _, err := stream.Write([]byte{b}); err != nil {
			t.Fatal(err)
		}
	}
	evidence, err := stream.finish()
	if err != nil || evidence.Terminal != "completed" || evidence.SessionID != "thread-1" || *evidence.Usage.InputTokens != 4 {
		t.Fatalf("terminal %+v %v", evidence, err)
	}
	raw, _ := json.Marshal(events)
	if strings.Contains(string(raw), "private-secret") || strings.Contains(string(raw), "never-persist") || !strings.Contains(string(raw), "中文") {
		t.Fatalf("unsafe/lost facts: %s", raw)
	}
}

func TestStructuredStreamRequiresConsistentTerminal(t *testing.T) {
	prefix := "{\"type\":\"thread.started\",\"thread_id\":\"session\"}\n{\"type\":\"turn.started\"}\n"
	for name, body := range map[string]string{"missing": prefix, "conflicting": prefix + "{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":0,\"cached_input_tokens\":0,\"output_tokens\":0,\"reasoning_output_tokens\":0}}\n{\"type\":\"turn.failed\",\"error\":{\"message\":\"failed\"}}\n", "malformed": "{broken}\n", "oversize": strings.Repeat("x", (64<<10)+1)} {
		t.Run(name, func(t *testing.T) {
			s := newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
			_, _ = s.Write([]byte(body))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" {
				t.Fatalf("unsafe terminal: %+v %v", e, err)
			}
		})
	}
	s := newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
	_, _ = s.Write([]byte("{\"type\":\"error\",\"message\":\"vendor failed\"}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":0,\"cached_input_tokens\":0,\"output_tokens\":0,\"reasoning_output_tokens\":0}}\n"))
	e, _ := s.finish()
	if e.Terminal == "completed" {
		t.Fatal("fatal vendor error overwritten")
	}
	s = newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
	_, _ = s.Write([]byte(prefix + "{\"type\":\"item.completed\",\"item\":{\"id\":\"i\",\"type\":\"error\",\"message\":\"recoverable item\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":0,\"cached_input_tokens\":0,\"output_tokens\":0,\"reasoning_output_tokens\":0}}\n"))
	e, err := s.finish()
	if err != nil || e.Terminal != "completed" {
		t.Fatalf("nonfatal item misclassified %+v %v", e, err)
	}
}

func TestToolOutputRedactsQuotedCredentialsAndForwardedSecretValues(t *testing.T) {
	t.Setenv("SUMA_AGENT_PASSWORD", "private-forwarded-value")
	_, secrets, err := commandEnvironment(CommandSpec{EnvFrom: []string{"SUMA_AGENT_PASSWORD"}})
	if err != nil {
		t.Fatal(err)
	}
	message := progressText(`{"api_key":"unknown-key","password":"unknown-password"} private-forwarded-value`, secrets)
	if strings.Contains(message, "unknown-key") || strings.Contains(message, "unknown-password") || strings.Contains(message, "private-forwarded-value") {
		t.Fatalf("credential was retained: %s", message)
	}
}

func TestNativeEntryBytesChangeProfileEvenWithSamePathVersionAndInterpreter(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node runtime unavailable for actual entry drift probe")
	}
	entry := filepath.Join(t.TempDir(), "native-fixture.js")
	content := `if(process.argv[2]==='--version')console.log('suma-agent-runtime fixture-entry-test');else if(process.argv[2]==='--help')console.log('JSONL SUMA_AGENT_TOKEN');else {process.stdin.resume();process.stdin.on('end',()=>console.log(JSON.stringify({type:'completed',output:'fixture only'})));}`
	if err = os.WriteFile(entry, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"native": {Executable: node, Adapter: "suma", Protocol: "suma-jsonl-v1", Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true}}}
	cfg.ToolInventory = describeForTest(t, cfg)
	if !cfg.ToolInventory[0].Available {
		t.Fatalf("probe unavailable: %+v", cfg.ToolInventory)
	}
	first := cfg.ToolInventory[0].Profile
	if err = os.WriteFile(entry, []byte(content+"\n// actual entry byte drift\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second := describeForTest(t, cfg)[0].Profile
	if first.ExecutableSHA256 != second.ExecutableSHA256 || first.ToolVersion != second.ToolVersion {
		t.Fatal("fixture did not hold interpreter/version constant")
	}
	if first.ProfileSHA256 == second.ProfileSHA256 {
		t.Fatal("same-path JavaScript entry drift was not bound to profile")
	}
	r := Execute(context.Background(), Assignment{Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "entry drift must not run", "tool_profile_sha256": first.ProfileSHA256}}, cfg)
	if r.ExitCode == 0 || !strings.Contains(r.Error, "profile") {
		t.Fatalf("changed entry ran: %+v", r)
	}
}

func TestStructuredProtocolsRejectForeignIdentityAndOutOfOrderTerminal(t *testing.T) {
	for name, body := range map[string]string{
		"missing-request-id": `{"type":"started"}` + "\n" + `{"type":"completed","output":"unbound"}` + "\n",
		"foreign-request-id": `{"type":"started","request_id":"foreign"}` + "\n" + `{"type":"completed","request_id":"foreign","output":"unbound"}` + "\n",
		"missing-start":      `{"type":"completed","request_id":"expected","output":"unbound"}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "expected")
			_, _ = s.Write([]byte(body))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" {
				t.Fatalf("native identity/order accepted: %+v %v", e, err)
			}
		})
	}
	for name, body := range map[string]string{
		"terminal-only":   `{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}` + "\n",
		"no-turn-start":   `{"type":"thread.started","thread_id":"thread"}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}` + "\n",
		"duplicate-start": `{"type":"thread.started","thread_id":"thread"}` + "\n" + `{"type":"turn.started"}` + "\n" + `{"type":"turn.started"}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}` + "\n",
		"after-terminal":  `{"type":"thread.started","thread_id":"thread"}` + "\n" + `{"type":"turn.started"}` + "\n" + `{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}` + "\n" + `{"type":"item.completed","item":{"id":"message","type":"agent_message","text":"late"}}` + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			s := newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
			_, _ = s.Write([]byte(body))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" {
				t.Fatalf("Codex order accepted: %+v %v", e, err)
			}
		})
	}
}

func TestCodexPresetRejectsPromptResumeAndIsolationOverrides(t *testing.T) {
	exe, _ := os.Executable()
	for name, args := range map[string][]string{
		"literal-prompt":    {"exec", "--json", "--sandbox", "read-only", "literal task"},
		"resume":            {"exec", "resume", "--json", "--sandbox", "read-only", "-"},
		"cwd":               {"exec", "--json", "--sandbox", "read-only", "--cd", "elsewhere", "-"},
		"config":            {"exec", "--json", "--sandbox", "read-only", "-c", "sandbox_mode=danger-full-access", "-"},
		"bypass":            {"exec", "--json", "--sandbox", "read-only", "--dangerously-bypass-approvals-and-sandbox", "-"},
		"readonly-mismatch": {"exec", "--json", "--sandbox", "workspace-write", "-"},
	} {
		t.Run(name, func(t *testing.T) {
			items := describeForTest(t, ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"tool": {Executable: exe, Adapter: "codex", Protocol: "codex-jsonl-v1", Input: "goal", ReadOnly: true, Args: args, Env: map[string]string{"SUMA_TOOL_FIXTURE": "1"}}}})
			if items[0].Available {
				t.Fatalf("unsafe preset advertised: %+v", items[0])
			}
		})
	}
}

func TestCodexUsageDoesNotTurnMissingCountersIntoReportedZero(t *testing.T) {
	prefix := `{"type":"thread.started","thread_id":"s"}` + "\n" + `{"type":"turn.started"}` + "\n"
	for _, usage := range []string{`{}`, `{"cached_input_tokens":0,"output_tokens":2}`, `{"input_tokens":1,"output_tokens":2}`, `{"input_tokens":1,"cached_input_tokens":0}`} {
		s := newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
		_, _ = s.Write([]byte(prefix + `{"type":"turn.completed","usage":` + usage + "}\n"))
		e, err := s.finish()
		if err == nil || e.Terminal == "completed" {
			t.Fatalf("missing official counters became zeros: %+v %v", e, err)
		}
	}
}

func TestCodexUsageSupportsOfficialThreeCounterAndExtendedShapes(t *testing.T) {
	prefix := `{"type":"thread.started","thread_id":"s"}` + "\n" + `{"type":"turn.started"}` + "\n"
	for _, extra := range []string{"", `,"reasoning_output_tokens":3`, `,"reasoning_output_tokens":3,"cache_write_input_tokens":4`} {
		t.Run(extra, func(t *testing.T) {
			s := newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
			_, _ = s.Write([]byte(prefix + `{"type":"turn.completed","usage":{"input_tokens":5,"cached_input_tokens":2,"output_tokens":7` + extra + "}}\n"))
			e, err := s.finish()
			if err != nil || e.Terminal != "completed" || e.Usage == nil || *e.Usage.InputTokens != 5 || *e.Usage.CachedInputTokens != 2 || *e.Usage.OutputTokens != 7 {
				t.Fatalf("valid official usage shape rejected: %+v %v", e, err)
			}
			wire, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"reasoning_output_tokens", "cache_write_input_tokens"} {
				if !strings.Contains(extra, name) && strings.Contains(string(wire), name) {
					t.Fatalf("unreported split counter fabricated on wire: %s", wire)
				}
			}
			if strings.Contains(extra, "reasoning_output_tokens") && *e.Usage.ReasoningOutputTokens != 3 {
				t.Fatal("reported reasoning usage lost")
			}
			if strings.Contains(extra, "cache_write_input_tokens") && *e.Usage.CacheWriteInputTokens != 4 {
				t.Fatal("reported cache-write usage lost")
			}
		})
	}
}

func TestCodexUsageRejectsInvalidPresentCounters(t *testing.T) {
	prefix := `{"type":"thread.started","thread_id":"s"}` + "\n" + `{"type":"turn.started"}` + "\n"
	for _, name := range []string{"input_tokens", "cached_input_tokens", "output_tokens", "reasoning_output_tokens", "cache_write_input_tokens"} {
		for _, value := range []any{nil, "1", -1, 1.5, true, json.Number("9223372036854775808")} {
			m := map[string]any{"input_tokens": 5, "cached_input_tokens": 2, "output_tokens": 7}
			m[name] = value
			usage, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			s := newToolStream(structuredProfile("codex-jsonl-v1"), nil, nil)
			_, _ = s.Write([]byte(prefix + `{"type":"turn.completed","usage":` + string(usage) + "}\n"))
			e, err := s.finish()
			if err == nil || e.Terminal == "completed" {
				t.Fatalf("invalid %s=%v accepted: %+v %v", name, value, e, err)
			}
		}
	}
}

func TestSumaMissingUsageIsUnknownRatherThanReportedZero(t *testing.T) {
	prefix := `{"type":"started","request_id":"expected"}` + "\n"
	for _, body := range []string{`{"type":"usage","request_id":"expected","step":0,"usage_reported":false,"usage_state":"unreported"}`} {
		s := newToolStream(structuredProfile("suma-jsonl-v1"), nil, nil, "expected")
		_, _ = s.Write([]byte(prefix + `{"type":"request_started","request_id":"expected","step":0,"output_allowance":5}` + "\n" + body + "\n" + `{"type":"completed","request_id":"expected","output":"fixture"}` + "\n"))
		e, err := s.finish()
		if err != nil || e.Terminal != "completed" || e.Usage != nil {
			t.Fatalf("unknown native usage fabricated %+v %v", e, err)
		}
	}
}

func TestCommandConfigurationRejectsTyposDuplicatesAndUnknownFields(t *testing.T) {
	for _, raw := range []string{`{"tool":{"executable":"node","adpater":"suma"}}`, `{"tool":{"executable":"one"},"tool":{"executable":"two"}}`, `{"tool":{"executable":"node","read_only":true,"unsupported":true}}`, `{} {}`} {
		if _, err := DecodeCommands([]byte(raw)); err == nil {
			t.Fatal("invalid command config accepted")
		}
	}
}
