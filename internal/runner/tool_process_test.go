package runner

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// This executable fixture speaks a deliberately small declared protocol. It is
// not a Codex/model invocation; only transport and process semantics are tested.
func TestMain(m *testing.M) {
	if codexAppServerFixtureProcess() {
		return
	}
	if os.Getenv("SUMA_TOOL_FIXTURE") == "1" {
		if len(os.Args) == 2 && os.Args[1] == "--version" {
			fmt.Println("codex-cli fixture-transport-only")
			os.Exit(0)
		}
		if len(os.Args) == 3 && os.Args[1] == "exec" && os.Args[2] == "--help" {
			fmt.Println("fixture exec --json --sandbox")
			os.Exit(0)
		}
		if len(os.Args) > 2 && os.Args[1] == "exec" && os.Args[2] == "--json" {
			input, _ := io.ReadAll(os.Stdin)
			if strings.TrimSpace(string(input)) != "inspect actual fixture" {
				os.Exit(21)
			}
			mode := os.Getenv("SUMA_TOOL_MODE")
			if mode == "stderr-terminal-only" {
				fmt.Fprintln(os.Stderr, `{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}`)
				os.Exit(0)
			}
			if mode == "terminal-only" {
				fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":0,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0}}`)
				os.Exit(0)
			}
			fmt.Println(`{"type":"thread.started","thread_id":"fixture-session"}`)
			fmt.Println(`{"type":"turn.started"}`)
			fmt.Fprintln(os.Stderr, "diagnostic token=fixture-secret")
			if mode == "blocked" {
				time.Sleep(time.Minute)
				os.Exit(0)
			}
			if mode == "malformed" {
				fmt.Println(`{"broken":`)
				os.Exit(0)
			}
			if mode == "missing" {
				os.Exit(0)
			}
			if mode == "failed" {
				fmt.Println(`{"type":"turn.failed","error":{"message":"actual fixture failure"}}`)
				os.Exit(0)
			}
			fmt.Println(`{"type":"item.completed","item":{"id":"message-1","type":"agent_message","text":"actual fixture result"}}`)
			if mode == "legacy-three-counters" {
				fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":3,"cached_input_tokens":0,"output_tokens":2}}`)
				os.Exit(0)
			}
			fmt.Println(`{"type":"turn.completed","usage":{"input_tokens":3,"cached_input_tokens":0,"output_tokens":2,"reasoning_output_tokens":0}}`)
			if mode == "conflicting" {
				fmt.Println(`{"type":"turn.failed","error":{"message":"conflict"}}`)
			}
			if mode == "bad-exit" {
				os.Exit(9)
			}
			os.Exit(0)
		}
		os.Exit(22)
	}
	if acpRunnerFixtureProcess() {
		return
	}
	os.Exit(m.Run())
}

func fixtureToolConfig(t *testing.T, mode string) ExecConfig {
	t.Helper()
	exe, _ := os.Executable()
	cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"fixture": {Executable: exe, Adapter: "codex", Protocol: "codex-jsonl-v1", ReadOnly: true, Input: "goal", Args: []string{"exec", "--json", "--sandbox", "read-only", "-"}, Env: map[string]string{"SUMA_TOOL_FIXTURE": "1", "SUMA_TOOL_MODE": mode}}}, Secrets: []string{"fixture-secret"}}
	cfg.ToolInventory = describeForTest(t, cfg)
	if len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available {
		t.Fatalf("fixture probe: %+v", cfg.ToolInventory)
	}
	return cfg
}
func fixtureToolAssignment(cfg ExecConfig) Assignment {
	return Assignment{Type: "agent", Command: []string{"fixture"}, Config: map[string]any{"goal": "inspect actual fixture", "tool_profile_id": "fixed-fixture", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256}}
}

func TestActualStructuredProcessNeedsExitAndWholeRunTerminal(t *testing.T) {
	for _, mode := range []string{"completed", "legacy-three-counters", "failed", "missing", "malformed", "conflicting", "bad-exit", "stderr-terminal-only", "terminal-only"} {
		t.Run(mode, func(t *testing.T) {
			cfg := fixtureToolConfig(t, mode)
			events := []tc.Event{}
			cfg.ToolProgress = func(e tc.Event) error { events = append(events, e); return nil }
			result := Execute(context.Background(), fixtureToolAssignment(cfg), cfg)
			if result.ToolEvidence == nil {
				t.Fatalf("missing typed result: %+v", result)
			}
			if mode == "completed" || mode == "legacy-three-counters" {
				if result.ExitCode != 0 || result.Error != "" || result.ToolEvidence.Terminal != "completed" || result.ToolEvidence.SessionID != "fixture-session" || len(events) < 3 {
					t.Fatalf("actual successful fixture: %+v", result)
				}
			} else if result.ExitCode == 0 || result.Error == "" {
				t.Fatalf("invented success: %+v", result)
			}
			if strings.Contains(result.Output, "fixture-secret") {
				t.Fatal("stderr secret escaped")
			}
		})
	}
}

func TestActualNativeProcessRequiresAssignedRequestIdentityAndStartedPhase(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable for native JSONL process fixture")
	}
	entry := filepath.Join(t.TempDir(), "native-id-fixture.js")
	script := `if(process.argv[2]==='--version'){console.log('suma-agent-runtime fixture-identity-only');process.exit(0)}if(process.argv[2]==='--help'){console.log('JSONL SUMA_AGENT_TOKEN');process.exit(0)}let raw='';process.stdin.on('data',b=>raw+=String(b));process.stdin.on('end',()=>{const r=JSON.parse(raw),mode=process.env.SUMA_ID_FIXTURE;const id=mode==='foreign'?'another-attempt':r.request_id;const emit=e=>console.log(JSON.stringify(mode==='missing-id'?e:{...e,request_id:id}));if(mode==='bare-error'){console.log(JSON.stringify({type:'error',message:'missing identity'}));return}if(mode!=='missing-start')emit({type:'started'});emit({type:'completed',output:'fixture identity result'});if(mode==='after-terminal')emit({type:'tool_started',call_id:'late'});});`
	if err = os.WriteFile(entry, []byte(script), 0600); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"valid", "missing-id", "foreign", "missing-start", "bare-error", "after-terminal"} {
		t.Run(mode, func(t *testing.T) {
			cfg := ExecConfig{AllowExec: true, Workspace: t.TempDir(), Commands: map[string]CommandSpec{"native": {Executable: node, Adapter: "suma", Protocol: "suma-jsonl-v1", Args: []string{entry}, Input: "agent-jsonl", ReadOnly: true, Env: map[string]string{"SUMA_ID_FIXTURE": mode}}}}
			cfg.ToolInventory = describeForTest(t, cfg)
			if !cfg.ToolInventory[0].Available {
				t.Fatalf("fixture unavailable %+v", cfg.ToolInventory)
			}
			a := Assignment{Type: "agent", Command: []string{"native"}, Config: map[string]any{"goal": "identity fixture", "tool_profile_sha256": cfg.ToolInventory[0].Profile.ProfileSHA256}}
			r := Execute(context.Background(), a, cfg)
			if mode == "valid" {
				if r.ExitCode != 0 || r.ToolEvidence == nil || r.ToolEvidence.Terminal != "completed" || r.ToolEvidence.SessionID != digest([]byte(key(a))) {
					t.Fatalf("valid native fixture %+v", r)
				}
			} else if r.ExitCode == 0 || r.ToolEvidence == nil || r.ToolEvidence.Terminal == "completed" {
				t.Fatalf("forged native completion %+v", r)
			}
		})
	}
}
func TestStructuredProcessCancellationRetainsActualStopFact(t *testing.T) {
	cfg := fixtureToolConfig(t, "blocked")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	result := Execute(ctx, fixtureToolAssignment(cfg), cfg)
	if result.ExitCode == 0 || result.ToolEvidence == nil || result.ToolEvidence.Terminal == "completed" {
		t.Fatalf("invented cancelled completion: %+v", result)
	}
	if runtime.GOOS == "windows" && (!result.Stopped || result.OutcomeUnknown) {
		t.Fatalf("confirmed Windows stop was lost: %+v", result)
	}
}

func TestNativeAgentEnvironmentNeverForwardsRunnerBearer(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "runner-private")
	t.Setenv("SUMA_AGENT_TOKEN", "agent-private")
	env, secrets, err := commandEnvironment(CommandSpec{Adapter: "suma", EnvFrom: []string{"SUMA_RUNNER_TOKEN", "SUMA_AGENT_TOKEN"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range env {
		if strings.HasPrefix(strings.ToUpper(s), "SUMA_RUNNER_TOKEN=") {
			t.Fatal("Runner bearer forwarded to native agent")
		}
	}
	if !strings.Contains(strings.Join(env, "\n"), "SUMA_AGENT_TOKEN=agent-private") || !strings.Contains(strings.Join(secrets, "\n"), "agent-private") {
		t.Fatal("agent credential absent or not redacted")
	}
}
