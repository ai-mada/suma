package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

const cursorLifecycleFixture191 = "suma-owned-lifecycle191\n"
const cursorLifecycleVersion191 = "Cursor owned lifecycle fixture191"

// The reviewed package contains our native Go test host, not Cursor or Node.
// Like a foreground ACP service, it remains alive after its RPCs and stdin end.
func init() {
	if len(os.Args) < 3 || filepath.Base(os.Args[0]) != cursorNodeName() {
		return
	}
	entry, err := os.ReadFile(os.Args[1])
	if err != nil || !strings.HasPrefix(string(entry), cursorLifecycleFixture191) {
		return
	}
	mode := strings.TrimPrefix(string(entry), cursorLifecycleFixture191)
	if len(os.Args) == 4 && os.Args[3] == "--version" {
		fmt.Println(cursorLifecycleVersion191)
		os.Exit(0)
	}
	if os.Getenv("CURSOR_API_KEY") != "owned-lifecycle-key191" || os.Args[len(os.Args)-1] != "acp" {
		os.Exit(101)
	}
	emit := func(v any) { _ = json.NewEncoder(os.Stdout).Encode(v) }
	models := []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": "fixed191", "options": []any{map[string]any{"value": "fixed191", "name": "Fixed"}}}}
	scanner := bufio.NewScanner(os.Stdin)
	var activePrompt json.RawMessage
	for scanner.Scan() {
		var p struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &p) != nil {
			os.Exit(102)
		}
		var result any
		switch p.Method {
		case "initialize":
			result = map[string]any{"protocolVersion": 1, "agentCapabilities": map[string]any{}, "authMethods": []any{}}
		case "session/new":
			result = map[string]any{"sessionId": "owned-lifecycle191", "configOptions": models}
		case "session/set_config_option":
			result = map[string]any{"configOptions": models}
		case "session/cancel":
			// Standard Cancel is a notification; it has no response.
			if len(p.ID) != 0 {
				os.Exit(103)
			}
			if len(activePrompt) != 0 {
				emit(map[string]any{"jsonrpc": "2.0", "id": activePrompt, "result": map[string]any{"stopReason": "cancelled"}})
			}
			continue
		case "session/prompt":
			if mode == "cancelled_context" {
				activePrompt = p.ID
				emit(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "owned-lifecycle191", "update": map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "owned-cancel-ready191"}}}})
				continue
			}
			if mode == "changed_package" {
				_ = os.WriteFile(os.Args[1], []byte("changed after RPC started"), 0600)
			}
			if mode == "rpc_error" {
				emit(map[string]any{"jsonrpc": "2.0", "id": p.ID, "error": map[string]any{"code": -32603, "message": "owned failure"}})
				continue
			}
			if os.WriteFile("lifecycle191.txt", []byte("owned candidate191\n"), 0600) != nil {
				os.Exit(104)
			}
			for _, args := range [][]string{{"add", "lifecycle191.txt"}, {"-c", "user.name=OwnedFixture", "-c", "user.email=fixture@example.test", "commit", "-m", "owned lifecycle candidate191"}} {
				if exec.Command("git", args...).Run() != nil {
					os.Exit(105)
				}
			}
			result = map[string]any{"stopReason": "end_turn"}
		default:
			os.Exit(106)
		}
		wire, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": p.ID, "result": result})
		if p.Method == "session/prompt" && mode == "truncated" {
			wire = append(append(wire, '\n'), []byte(`{"jsonrpc":`)...)
			_, _ = os.Stdout.Write(wire)
		} else {
			_, _ = os.Stdout.Write(append(wire, '\n'))
		}
		if p.Method == "session/prompt" && mode == "natural_nonzero" {
			os.Exit(7)
		}
	}
	time.Sleep(time.Minute)
	os.Exit(107)
}

func cursorLifecycleSpec191(t *testing.T, mode string) CommandSpec {
	t.Helper()
	t.Setenv("LOCAL_CURSOR_LIFECYCLE191", "owned-lifecycle-key191")
	root := t.TempDir()
	exe, _ := os.Executable()
	content, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, cursorNodeName()), content, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(root, "index.js"), []byte(cursorLifecycleFixture191+mode), 0600); err != nil {
		t.Fatal(err)
	}
	spec, err := BuildCursorACPCommand(root, cursorLifecycleVersion191, "fixed191", "LOCAL_CURSOR_LIFECYCLE191")
	if err != nil {
		t.Fatal(err)
	}
	return spec
}

func TestCursorPostRPCStopPreservesNativeExitAndRejectsUnsafeStacks(t *testing.T) {
	for _, mode := range []string{"execution", "discovery", "truncated", "rpc_error", "changed_package", "cancelled_context", "natural_nonzero"} {
		t.Run(mode, func(t *testing.T) {
			spec := cursorLifecycleSpec191(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
			defer cancel()
			if mode == "discovery" {
				_, sha, err := commandExecutable(spec)
				if err != nil {
					t.Fatal(err)
				}
				result, err := DiscoverACPModels(ctx, ACPModelDiscoveryInput{AdapterID: "cursor-cli", Executable: spec.Executable, ExecutableSHA256: sha, ReviewedVersion: cursorLifecycleVersion191, CredentialEnvRef: spec.ACP.CredentialEnvRef})
				if runtime.GOOS != "windows" {
					if err != nil || result.State == "ready" || result.Catalogue != nil {
						t.Fatal("unproved Unix containment became ready", err)
					}
					return
				}
				if err != nil || result.State != "ready" || result.Catalogue == nil || !result.Evidence.HostStoppedAfterRPC || result.Evidence.HostProcessExitCode == nil || *result.Evidence.HostProcessExitCode == 0 || !result.Evidence.SessionCancelRequested || !result.Evidence.SessionCancelWritten || !result.Evidence.HostProcessTreeStopped {
					t.Fatalf("catalogue RPC and native service stop were conflated: %s/%s %v", result.State, result.ErrorCode, err)
				}
				return
			}
			repo, base := gitFixture(t)
			cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"cursor191": spec}, ToolInputClient: &Client{ProjectID: "owned191"}}
			cfg.ToolInventory = describeForTest(t, cfg)
			if len(cfg.ToolInventory) != 1 || !cfg.ToolInventory[0].Available {
				t.Fatal("owned package unavailable", cfg.ToolInventory)
			}
			p := cfg.ToolInventory[0].Profile
			a := Assignment{RunID: "owned191-" + mode, NodeID: "develop", Attempt: 1, Fence: 1, DispatchID: "dispatch191", WorkerID: "worker191", Type: "agent", Command: []string{"cursor191"}, Config: map[string]any{"goal": "Owned lifecycle check", "tool_profile_id": "owned191", "tool_profile_sha256": p.ProfileSHA256, "acp_execution": p.ACPExecution}}
			j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
			if err != nil {
				t.Fatal(err)
			}
			defer j.Close()
			if err = j.Accept(a); err != nil {
				t.Fatal(err)
			}
			cfg.Journal = j
			if mode == "cancelled_context" {
				cfg.ToolProgress = func(e tc.Event) error {
					if strings.Contains(e.Message, "owned-cancel-ready191") {
						cancel()
					}
					return nil
				}
			}
			result := Execute(ctx, a, cfg)
			if mode != "execution" {
				if result.ExitCode == 0 || !result.OutcomeUnknown && mode != "cancelled_context" {
					t.Fatal("unsafe protocol/service became success", result.Error)
				}
				if result.ToolEvidence != nil && result.ToolEvidence.ACPExecution.HostStoppedAfterRPC {
					t.Fatal("unsafe stop received post-RPC evidence")
				}
				return
			}
			if runtime.GOOS != "windows" {
				if result.ExitCode == 0 || !result.OutcomeUnknown {
					t.Fatal("unproved Unix containment became success")
				}
				return
			}
			if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.Stopped || result.ToolEvidence == nil {
				t.Fatalf("successful RPC still rejected a proven host stop: exit=%d unknown=%t %s", result.ExitCode, result.OutcomeUnknown, result.Error)
			}
			proof := result.ToolEvidence.ACPExecution
			if !proof.HostStoppedAfterRPC || proof.HostProcessExitCode == nil || *proof.HostProcessExitCode == 0 || !proof.HostProcessTreeStopped || !proof.PromptCompleted || result.ToolEvidence.Terminal != "completed" {
				t.Fatal("native exit was replaced by a synthetic zero")
			}
			if head, _ := result.WorkspaceEvidence["head_commit"].(string); head == "" || head == base || result.WorkspaceEvidence["clean"] != true {
				t.Fatal("actual Git candidate not captured")
			}
			copy := cloneACPEvidence(proof)
			*copy.HostProcessExitCode = 0
			if *proof.HostProcessExitCode == 0 {
				t.Fatal("host exit evidence alias escaped clone")
			}
		})
	}
}
