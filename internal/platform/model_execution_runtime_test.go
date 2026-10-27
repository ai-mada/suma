package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/runner"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// One representative end-to-end execution. Authentication, fixed preview,
// claim, the native child/MCP connection and the original usage ledger all run
// for real; only the locally owned model protocol response is deterministic.
func TestModelExecutionCurrentPreviewActualRuntimeAndUsageLedger(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	calls := 0
	requestSHA, responseSHA := "", ""
	hash := func(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		var body map[string]any
		if err != nil || json.Unmarshal(raw, &body) != nil || r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer owned-model-protocol-fixture" {
			t.Error("unexpected actual provider request")
			w.WriteHeader(400)
			return
		}
		if body["model"] != "owned-model-036" || body["max_completion_tokens"] != float64(100) || body["stream"] != false {
			t.Error("actual provider request differs from approved model parameters")
		}
		calls++
		requestSHA = hash(raw)
		reply, _ := json.Marshal(map[string]any{"id": "owned-model-response-036", "model": "owned-model-036", "choices": []any{map[string]any{"message": map[string]any{"content": "Owned protocol fixture; no source changes or formal acceptance claimed."}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 23, "completion_tokens": 17}})
		responseSHA = hash(reply)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}))
	defer provider.Close()
	endpoint := provider.URL + "/v1"
	endpointSHA, err := modelcontract.EndpointDigest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deployment := modelcontract.Deployment{ID: "owned-deployment-036", Revision: 1, Protocol: modelcontract.ProviderProtocol, Model: "owned-model-036", EndpointOrigin: provider.URL, EndpointConfigSHA256: endpointSHA, ContextWindow: 8192}
	deployment.SHA256 = modelcontract.DeploymentDigest(deployment)
	repo := t.TempDir()
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "source.txt", "Immutable source fixture\n")
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "source.txt")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: immutable native model base")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node22 is required for this explicit native integration gate", err)
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "packages", "agent-runtime", "dist", "suma-agent.bundle.mjs")
	manifest := filepath.Join(root, "packages", "agent-runtime", "dist", "suma-agent.manifest.json")
	if _, err = os.Stat(bundle); err != nil {
		t.Fatal("build the native runtime before this explicit integration gate", err)
	}
	t.Setenv("SUMA_OWNED_MODEL_036", "owned-model-protocol-fixture")
	cfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"native-model-036": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{bundle}, Input: "agent-jsonl", ModelExecutionProtocol: modelcontract.Protocol, NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifest, Checks: []string{}}}}, ModelDeployments: []runner.ModelDeploymentSpec{{Deployment: deployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_MODEL_036"}}}
	inventory, err := runner.DescribeTools(ctx, cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatalf("actual runtime inventory unavailable: %+v %v", inventory, err)
	}
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "actual-model-worker-036", "profiles": []any{}, "deployment": false}), 201)
	token := worker["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "actual-model-worker-036", Name: "Actual native model runtime", Profile: inventory[0].Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: inventory})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	rate := modelRateForTest(t, f, profile, deployment, 0)
	workflow, _, _ := taskDispatchBuildFixture()
	workflow.Version = 1
	published, err := f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
	if err != nil {
		t.Fatal(err)
	}
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "原生模型准确选择", "description": "一次自有协议验收", "acceptance_criteria": []string{"独立候选测试与人工验收仍须另行完成"}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "原生模型实际执行", "description": "核对准确模型、参数和用量", "requirement_id": requirement["id"], "acceptance_criteria": []string{"不以模型输出代替正式验收"}}), 201)
	selection := modelcontract.Selection{DeploymentID: deployment.ID, DeploymentRevision: deployment.Revision, DeploymentSHA256: deployment.SHA256, Parameters: modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}}
	plan := f.call("POST", "/task-dispatch-plans", f.body(map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "workflow_id": published.ID, "workflow_version": published.Version, "agent_node_id": "develop", "tool_profile_id": profile.ID, "expected_tool_version": profile.Version, "model_execution_selection": selection}), 201)
	var binding modelcontract.Binding
	raw, _ := json.Marshal(plan["model_execution"])
	if json.Unmarshal(raw, &binding) != nil || modelcontract.ValidateBinding(binding) != nil || binding.Rate == nil || binding.Rate.ID != rate.ID {
		t.Fatal("preview did not freeze the exact model and rate")
	}
	run := f.call("POST", "/task-dispatch-plans/"+plan["id"].(string)+"/start", f.body(map[string]any{"expected_version": plan["version"]}), 201)
	control := httptest.NewServer(f.handler)
	defer control.Close()
	client := &runner.Client{BaseURL: control.URL, Token: token, ProjectID: f.project}
	assignment, err := client.Claim(ctx, profile.WorkerID, []string{"command:" + profile.Profile.CommandID})
	if err != nil || assignment == nil || assignment.RunID != run["id"] {
		t.Fatal("real HTTP claim did not return the frozen assignment", err)
	}
	cfg.ToolInventory = inventory
	result := runner.Execute(ctx, *assignment, cfg)
	result.OperationID = newID()
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.UsageState != tc.UsageComplete || result.ToolEvidence.ModelExecution == nil {
		t.Fatalf("actual native process failed: exit=%d error=%s unknown=%t output=%s", result.ExitCode, result.Error, result.OutcomeUnknown, result.Output)
	}
	receipt := result.ToolEvidence.ModelExecution
	if calls != 1 || receipt.BindingSHA256 != binding.BindingSHA256 || receipt.DeploymentSHA256 != deployment.SHA256 || receipt.ParametersSHA256 != binding.ParametersSHA256 || receipt.Model != deployment.Model || len(receipt.Requests) != 1 || receipt.Requests[0].RequestSHA256 != requestSHA || receipt.Requests[0].ResponseSHA256 != responseSHA || receipt.Requests[0].ProviderModel != deployment.Model {
		t.Fatalf("real process receipt does not match actual HTTP: %+v", receipt)
	}
	if err = client.Complete(ctx, result); err != nil {
		t.Fatal("original result callback rejected actual runtime evidence", err)
	}
	attempts, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil || len(attempts.Items) != 1 {
		t.Fatal("actual model accounting missing", err)
	}
	accounting := attempts.Items[0]
	if accounting.State != "settled" || accounting.Model != deployment.Model || accounting.Rate == nil || accounting.Rate.ID != rate.ID || accounting.ReportedTokens == nil || *accounting.ReportedTokens != 40 || accounting.EstimatedCostMicros == nil || *accounting.EstimatedCostMicros != 57 || accounting.HeldTokens != 0 || accounting.HeldCostMicros == nil || *accounting.HeldCostMicros != 0 {
		t.Fatalf("actual native usage did not settle against frozen rate: %+v", accounting)
	}
	if head := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD"); head != base {
		t.Fatal("actual model journey changed administrator source")
	}
	retained, _ := json.Marshal(map[string]any{"run_id": assignment.RunID, "node_id": assignment.NodeID, "attempt": assignment.Attempt, "fence": assignment.Fence, "profile_sha256": profile.Profile.ProfileSHA256, "binding": binding, "model_execution": receipt, "usage_attempt_id": accounting.ID, "reported_tokens": accounting.ReportedTokens, "estimated_cost_micros": accounting.EstimatedCostMicros, "currency": "CNY", "state": accounting.State, "held_tokens": accounting.HeldTokens, "held_cost_micros": accounting.HeldCostMicros})
	t.Logf("model_execution_ledger_receipt=%s", retained)
	t.Logf("one current-actor preview/start, one real HTTP claim, one actual native bundle/MCP process, one owned model protocol response, exact request/response/binding hashes, 40 tokens / 57 CNY micros settled; independent formal task tests and acceptance remain pending")
}
