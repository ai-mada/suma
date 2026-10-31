package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ac "github.com/ai-mada/suma/internal/acpcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Only the package's test executable speaks this controlled ACP agent. It is
// not Qoder, does not log in or call a model, and never appears in user inventory.
func acpRunnerFixtureProcess() bool {
	if len(os.Args) != 2 {
		return false
	}
	if os.Args[1] == "--version" {
		fmt.Println("Qoder fixture protocol only 170")
		os.Exit(0)
	}
	if os.Args[1] != "--acp" {
		return false
	}
	if os.Getenv("SUMA_RUNNER_TOKEN") != "" || os.Getenv("SUMA_POOL_TOKEN") != "" {
		os.Exit(61)
	}
	emit := func(value any) {
		if json.NewEncoder(os.Stdout).Encode(value) != nil {
			os.Exit(62)
		}
	}
	response := func(id json.RawMessage, result any) {
		emit(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
	}
	options := func(model string) []any {
		return []any{map[string]any{"id": "model", "name": "Model", "category": "model", "type": "select", "currentValue": model, "options": []any{map[string]any{"value": "performance", "name": "Performance"}, map[string]any{"value": "wrong-ack", "name": "Wrong ACK fixture"}}}}
	}
	update := func(value any) {
		emit(map[string]any{"jsonrpc": "2.0", "method": "session/update", "params": map[string]any{"sessionId": "runner-session170", "update": value}})
	}
	var prompt json.RawMessage
	var goal string
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024), 64<<10)
	for scanner.Scan() {
		var packet map[string]json.RawMessage
		if json.Unmarshal(scanner.Bytes(), &packet) != nil {
			os.Exit(63)
		}
		var method string
		_ = json.Unmarshal(packet["method"], &method)
		switch method {
		case "initialize":
			response(packet["id"], map[string]any{"protocolVersion": 1, "agentInfo": map[string]any{"name": "owned runner protocol fixture", "version": "170"}, "authMethods": []any{}, "agentCapabilities": map[string]any{}})
		case "session/new":
			response(packet["id"], map[string]any{"sessionId": "runner-session170", "configOptions": options("performance")})
		case "session/set_config_option":
			var p struct {
				Value string `json:"value"`
			}
			_ = json.Unmarshal(packet["params"], &p)
			selected := p.Value
			if selected == "wrong-ack" {
				selected = "performance"
			}
			response(packet["id"], map[string]any{"configOptions": options(selected)})
		case "session/prompt":
			prompt = append(json.RawMessage{}, packet["id"]...)
			var p struct {
				Prompt []struct{ Text string } `json:"prompt"`
			}
			_ = json.Unmarshal(packet["params"], &p)
			for _, part := range p.Prompt {
				goal += part.Text
			}
			if goal == "cancel" {
				update(map[string]any{"sessionUpdate": "agent_message_chunk", "content": map[string]any{"type": "text", "text": "cancel-ready"}})
				continue
			}
			update(map[string]any{"sessionUpdate": "tool_call", "toolCallId": "write170", "title": "Change one isolated file", "kind": "edit", "status": "pending"})
			emit(map[string]any{"jsonrpc": "2.0", "id": "permission170", "method": "session/request_permission", "params": map[string]any{"sessionId": "runner-session170", "toolCall": map[string]any{"toolCallId": "write170", "title": "Change one isolated file", "kind": "edit", "rawInput": map[string]any{"path": "file.txt", "content": "candidate170"}}, "options": []any{map[string]any{"optionId": "once", "name": "Allow once", "kind": "allow_once"}, map[string]any{"optionId": "reject", "name": "Reject once", "kind": "reject_once"}}}})
		case "session/cancel":
			if len(prompt) > 0 {
				response(prompt, map[string]any{"stopReason": "cancelled"})
			}
		case "":
			if string(packet["id"]) != `"permission170"` {
				continue
			}
			if !strings.Contains(string(packet["result"]), `"once"`) {
				response(prompt, map[string]any{"stopReason": "refusal"})
				continue
			}
			if os.WriteFile("file.txt", []byte("candidate170\n"), 0600) != nil {
				os.Exit(64)
			}
			for _, args := range [][]string{{"add", "file.txt"}, {"-c", "user.name=ACPFixture", "-c", "user.email=acp@example.test", "commit", "-m", "controlled isolated candidate"}} {
				if exec.Command("git", args...).Run() != nil {
					os.Exit(65)
				}
			}
			update(map[string]any{"sessionUpdate": "tool_call_update", "toolCallId": "write170", "status": "completed"})
			response(prompt, map[string]any{"stopReason": "end_turn"})
		default:
			os.Exit(66)
		}
	}
	os.Exit(0)
	return true
}

func acpProcessFixture(t *testing.T, model, goal string) (ExecConfig, Assignment, *Journal) {
	t.Helper()
	repo, base := gitFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "file.txt"), []byte("user draft retained\n"), 0600); err != nil {
		t.Fatal(err)
	}
	exe, _ := os.Executable()
	spec := acpRunnerSpec()
	spec.Executable = exe
	spec.ACP.ReviewedVersion = "Qoder fixture protocol only 170"
	spec.ACP.Model = model
	cfg := ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]CommandSpec{"qoder170": spec, "independent170": {Adapter: "command", Protocol: "text-v1", Executable: exe, Args: []string{"-test.run=^TestACPIndependentCandidateHelper$", "--"}, Env: map[string]string{"SUMA_ACP_CANDIDATE_HELPER": "1"}, TestEvidence: true}}}
	cfg.ToolInventory = describeForTest(t, cfg)
	var profile tc.Profile
	for _, item := range cfg.ToolInventory {
		if !item.Available {
			t.Fatalf("controlled inventory unavailable: %s", item.Reason)
		}
		if item.Profile.CommandID == "qoder170" {
			profile = item.Profile
		}
	}
	a := Assignment{RunID: "run170-" + goal, NodeID: "develop", Attempt: 1, Fence: 1, DispatchID: "dispatch170-" + goal, WorkerID: "acp-developer170", Type: "agent", Command: []string{"qoder170"}, Config: map[string]any{"goal": goal, "tool_profile_id": "fixture170", "tool_profile_sha256": profile.ProfileSHA256, "acp_execution": profile.ACPExecution}}
	j, err := OpenJournal(filepath.Join(t.TempDir(), "journal"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = j.Close() })
	if err = j.Accept(a); err != nil {
		t.Fatal(err)
	}
	cfg.Journal = j
	return cfg, a, j
}

func TestACPProcessCandidateAndIndependentSameCommitTest(t *testing.T) {
	t.Setenv("SUMA_RUNNER_TOKEN", "platform-not-forwarded170")
	cfg, a, j := acpProcessFixture(t, "performance", "develop")
	var progressCount atomic.Int64
	cfg.ToolProgress = func(event tc.Event) error {
		progressCount.Add(1)
		return tc.ValidateEvent(event)
	}
	var permissionCount int
	cfg.ToolPermission = func(_ context.Context, request ac.PermissionRequest) (ac.PermissionDecision, error) {
		permissionCount++
		j.mu.Lock()
		_, persisted := j.entries[key(a)].ACP.Requests[request.IdempotencyKey]
		j.mu.Unlock()
		if !persisted {
			t.Error("SDK permission reached callback before its durable request")
		}
		return ac.PermissionDecision{OptionID: "once", Kind: "allow_once", ActorID: "human170", DecidedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
	}
	result := Execute(context.Background(), a, cfg)
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.ACPExecution == nil {
		t.Fatalf("controlled ACP process failed: exit=%d error=%s", result.ExitCode, result.Error)
	}
	proof := result.ToolEvidence.ACPExecution
	if !proof.ModelSelectionAcknowledged || proof.SelectedModel != "performance" || !proof.PromptCompleted || !proof.ToolsComplete || !proof.HostProcessStopped || !proof.HostProcessTreeStopped || permissionCount != 1 || progressCount.Load() < 5 {
		t.Fatal("ACP successful process omitted its exact model/tool/permission/host evidence")
	}
	head, _ := result.WorkspaceEvidence["head_commit"].(string)
	if head == "" || head == cfg.BaseCommit || result.WorkspaceEvidence["clean"] != true {
		t.Fatal("ACP child did not produce a genuine clean new commit")
	}
	user, _ := os.ReadFile(filepath.Join(cfg.Repository, "file.txt"))
	if string(user) != "user draft retained\n" {
		t.Fatal("ACP execution altered the user checkout")
	}
	test := Assignment{RunID: a.RunID, NodeID: "test", Attempt: 1, Fence: 1, WorkerID: "independent-test170", Type: "test", Command: []string{"independent170"}, Config: map[string]any{"candidate_from": a.NodeID}, CandidateCommit: head, CandidateFrom: a.NodeID, CandidateAttempt: a.Attempt, CandidateFence: a.Fence, CandidateEvidenceSHA256: result.WorkspaceEvidence["evidence_sha256"].(string)}
	verified := Execute(context.Background(), test, cfg)
	if verified.ExitCode != 0 || verified.WorkspaceEvidence["base_commit"] != head || verified.WorkspaceEvidence["head_commit"] != head || !strings.Contains(verified.Output, "independent candidate170 verified") {
		t.Fatalf("independent worker did not test the exact developer commit: %s", verified.Error)
	}
}

func TestACPProcessModelACKMismatchNeverPrompts(t *testing.T) {
	cfg, a, _ := acpProcessFixture(t, "wrong-ack", "no-prompt")
	result := Execute(context.Background(), a, cfg)
	if result.ExitCode == 0 || !result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.ACPExecution == nil || result.ToolEvidence.ACPExecution.ModelSelectionAcknowledged || result.ToolEvidence.ACPExecution.PromptStarted {
		t.Fatal("missing exact model ACK was replaced by a default Prompt or success")
	}
}

func TestACPProcessCancelKeepsUnknownAndGenuineHostStop(t *testing.T) {
	cfg, a, _ := acpProcessFixture(t, "performance", "cancel")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg.ToolProgress = func(event tc.Event) error {
		if strings.Contains(event.Message, "cancel-ready") {
			cancel()
		}
		return nil
	}
	result := Execute(ctx, a, cfg)
	if !result.OutcomeUnknown || !result.Stopped || result.ExitCode == 0 || result.ToolEvidence == nil || result.ToolEvidence.ACPExecution == nil {
		t.Fatalf("ACP cancel became success or lost process evidence: stopped=%t unknown=%t exit=%d error=%q proof=%+v", result.Stopped, result.OutcomeUnknown, result.ExitCode, result.Error, result.ToolEvidence)
	}
	proof := result.ToolEvidence.ACPExecution
	if !proof.CancelRequested || !proof.HostProcessStopped || !proof.HostProcessTreeStopped || result.ToolEvidence.Terminal != "unknown" {
		t.Fatal("cancel intent, RPC and host stop were not kept separate")
	}
}

func TestACPIndependentCandidateHelper(t *testing.T) {
	if os.Getenv("SUMA_ACP_CANDIDATE_HELPER") != "1" {
		return
	}
	bytes, err := os.ReadFile("file.txt")
	if err != nil || string(bytes) != "candidate170\n" {
		os.Exit(67)
	}
	fmt.Println("independent candidate170 verified")
	os.Exit(0)
}
