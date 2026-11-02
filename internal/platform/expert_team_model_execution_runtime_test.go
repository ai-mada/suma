package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/runner"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// One representative end-to-end execution. Authentication, fixed preview,
// claim, the native child/MCP connection and the original usage ledger all run
// for real; only the locally owned model protocol response is deterministic.
func TestExpertTeamModelSelectionActualRuntimeAndUsageLedger(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	holdDir := os.Getenv("SUMA_EXPERT_MODEL_UI_DIR")
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
		if body["model"] != "owned-model-040" || body["max_completion_tokens"] != float64(100) || body["stream"] != false {
			t.Error("actual provider request differs from approved model parameters")
		}
		calls++
		requestSHA = hash(raw)
		reply, _ := json.Marshal(map[string]any{"id": "owned-model-response-040", "model": "owned-model-040", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "Owned protocol fixture; no source changes or formal acceptance claimed."}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 23, "completion_tokens": 17}})
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
	deployment := modelcontract.Deployment{ID: "owned-deployment-040", Revision: 1, Protocol: modelcontract.ProviderProtocol, Model: "owned-model-040", EndpointOrigin: provider.URL, EndpointConfigSHA256: endpointSHA, ContextWindow: 8192}
	deployment.SHA256 = modelcontract.DeploymentDigest(deployment)
	secondDeployment := deployment
	secondDeployment.ID, secondDeployment.Model = "owned-deployment-040-b", "owned-model-040-b"
	secondDeployment.SHA256 = modelcontract.DeploymentDigest(secondDeployment)
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
	t.Setenv("SUMA_OWNED_MODEL_040", "owned-model-protocol-fixture")
	cfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"native-model-040": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{bundle}, Input: "agent-jsonl", ModelExecutionProtocol: modelcontract.Protocol, NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifest, Checks: []string{}}}}, ModelDeployments: []runner.ModelDeploymentSpec{{Deployment: deployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_MODEL_040"}, {Deployment: secondDeployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_MODEL_040"}}}
	inventory, err := runner.DescribeTools(ctx, cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatalf("actual runtime inventory unavailable: %+v %v", inventory, err)
	}
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "actual-model-worker-040", "profiles": []any{}, "deployment": false}), 201)
	token := worker["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "actual-model-worker-040", Name: "Actual native model runtime", Profile: inventory[0].Profile, IdempotencyKey: newID()})
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
	team, err := f.s.SaveTeam(ctx, f.actor, "", map[string]any{"name": "Owned model expert team", "status": "active", "expected_version": float64(0), "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	teamID := team["id"].(string)
	resource, err := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Owned model coding expert", "kind": "agent", "status": "active"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.SaveTeamMember(ctx, f.actor, teamID, map[string]any{"resource_kind": "agent", "resource_id": resource.ID, "project_id": f.project, "role": "member", "expected_version": float64(0), "idempotency_key": newID()}); err != nil {
		t.Fatal(err)
	}
	role := ExpertTeamRoleRequest{Key: "developer", Title: "Development", ResourceID: resource.ID, ExpectedResourceVersion: resource.Version, WorkflowID: published.ID, WorkflowVersion: published.Version, AgentNodeID: "develop", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, Knowledge: []ProjectKnowledgeReference{}}
	configuration, err := f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, teamID, ExpertTeamSaveRequest{ProjectID: f.project, Roles: []ExpertTeamRoleRequest{role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, teamID, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: configuration.Version, Action: "publish", Reason: "Explicitly review the fixed native resource, tool and workflow", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	tokenBudget, costBudget := int64(20000), int64(20000)
	selection := modelcontract.Selection{DeploymentID: deployment.ID, DeploymentRevision: deployment.Revision, DeploymentSHA256: deployment.SHA256, Parameters: modelcontract.Parameters{MaxSteps: 2, MaxOutputTokens: 100}, Budget: &modelcontract.Budget{TokenLimit: &tokenBudget, CostLimitMicros: &costBudget, Currency: "CNY"}}
	created := f.call("POST", "/expert-team-plans", f.body(map[string]any{"team_id": teamID, "expected_configuration_version": configuration.Version, "items": []ExpertTeamPlanItemRequest{{TaskID: task["id"].(string), ExpectedTaskVersion: int(task["version"].(float64)), RoleKey: role.Key, ModelExecutionSelection: &selection}}}), 201)
	var plan ExpertTeamPlan
	raw, _ := json.Marshal(created)
	if json.Unmarshal(raw, &plan) != nil || len(plan.Items) != 1 || plan.Items[0].Plan.ModelExecution == nil {
		t.Fatal("expert item preview did not preserve its native model binding")
	}
	binding := *plan.Items[0].Plan.ModelExecution
	if modelcontract.ValidateBinding(binding) != nil || binding.Rate == nil || binding.Rate.ID != rate.ID || binding.Budget == nil || binding.Budget.TokenLimit == nil || binding.Budget.CostLimitMicros == nil || *binding.Budget.TokenLimit != tokenBudget || *binding.Budget.CostLimitMicros != costBudget {
		t.Fatal("expert preview changed the exact model, parameters, budget or quote")
	}
	startedWire := f.call("POST", "/expert-team-plans/"+plan.ID+"/start", f.body(map[string]any{"expected_version": plan.Version, "acknowledge_dispatch": true}), 200)
	var started ExpertTeamPlan
	raw, _ = json.Marshal(startedWire)
	if json.Unmarshal(raw, &started) != nil || len(started.Items) != 1 || started.Items[0].RunID == "" || started.Items[0].Plan.ModelExecution == nil || started.Items[0].Plan.ModelExecution.BindingSHA256 != binding.BindingSHA256 {
		t.Fatal("expert Start changed the fixed item model binding")
	}
	run := map[string]any{"id": started.Items[0].RunID}
	serverConfig := &Server{Service: f.s, Engine: f.e, WebDir: t.TempDir()}
	control := httptest.NewUnstartedServer(serverConfig.Handler())
	if holdDir != "" {
		holdDir, err = filepath.Abs(holdDir)
		if err != nil || !under(filepath.Join(root, ".cache", "e2e", "expert-model-stage-040"), holdDir) {
			t.Fatal("expert UI hold must use its owned stage directory")
		}
		webDir, e := filepath.Abs(os.Getenv("SUMA_EXPERT_MODEL_WEB_DIR"))
		if e != nil || !under(filepath.Join(root, ".cache", "e2e", "expert-model-stage-040"), webDir) {
			t.Fatal("expert UI web must use its owned stage directory")
		}
		if err = os.MkdirAll(holdDir, 0700); err != nil {
			t.Fatal(err)
		}
		listener, e := net.Listen("tcp4", "127.0.0.1:59219")
		if e != nil {
			t.Fatal(e)
		}
		_ = control.Listener.Close()
		control.Listener = listener
		serverConfig.WebDir, serverConfig.PublicURL, serverConfig.PublicURLSource = webDir, "http://127.0.0.1:59219", "configured"
		serverConfig.AllowedOrigins = []string{"http://expertmodel040.localhost:59219", "http://127.0.0.1:59219"}
		control.Config.Handler = serverConfig.Handler()
	}
	control.Start()
	defer control.Close()
	f.handler = control.Config.Handler
	client := &runner.Client{BaseURL: control.URL, Token: token, ProjectID: f.project}
	assignment, err := client.Claim(ctx, profile.WorkerID, []string{"command:" + profile.Profile.CommandID})
	if err != nil || assignment == nil || assignment.RunID != run["id"] {
		t.Fatal("real HTTP claim did not return the frozen assignment", err)
	}
	cfg.ToolInventory = inventory
	result := runner.Execute(ctx, *assignment, cfg)
	result.OperationID = newID()
	if result.ExitCode != 0 || result.Error != "" || result.OutcomeUnknown || result.ToolEvidence == nil || result.ToolEvidence.UsageState != tc.UsageComplete || result.ToolEvidence.ModelExecution == nil {
		t.Fatalf("actual native process failed: exit=%d error=%s unknown=%t output=%s gateway_evidence=%s", result.ExitCode, result.Error, result.OutcomeUnknown, result.Output, stringify(result.ToolEvidence))
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
	if holdDir != "" {
		previewTask := f.call("POST", "/tasks", f.body(map[string]any{"title": "Expert model GUI preview only", "description": "Prepare a per-item model selection without starting another run", "acceptance_criteria": []string{"This GUI preview does not execute a model or accept a task"}}), 201)
		login, _ := json.Marshal(map[string]any{"base_url": control.URL, "browser_origin": "http://expertmodel040.localhost:59219", "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "team_id": teamID, "configuration_version": configuration.Version, "task_id": previewTask["id"], "task_version": previewTask["version"], "role_key": role.Key, "tool_profile_id": profile.ID, "tool_profile_version": profile.Version, "expert_plan_id": plan.ID, "run_id": assignment.RunID})
		if err = os.WriteFile(filepath.Join(holdDir, "login.local.json"), login, 0600); err != nil {
			t.Fatal(err)
		}
		ready, _ := json.Marshal(map[string]any{"port": 59219, "project_id": f.project, "team_id": teamID, "configuration_version": configuration.Version, "task_id": previewTask["id"], "role_key": role.Key, "tool_profile_id": profile.ID, "tool_profile_version": profile.Version, "expert_plan_id": plan.ID, "run_id": assignment.RunID, "binding_sha256": binding.BindingSHA256, "owned_model_calls": calls, "reported_tokens": accounting.ReportedTokens, "estimated_cost_micros": accounting.EstimatedCostMicros, "formal_tests": 0})
		if err = os.WriteFile(filepath.Join(holdDir, "ready.local.json"), ready, 0600); err != nil {
			t.Fatal(err)
		}
		stopPath := os.Getenv("SUMA_EXPERT_MODEL_STOP")
		if stopPath == "" {
			stopPath = filepath.Join(holdDir, "stop.request")
		}
		stopPath, err = filepath.Abs(stopPath)
		if err != nil || !under(holdDir, stopPath) {
			t.Fatal("expert UI stop must use its owned hold directory")
		}
		t.Log("owned expert model UI hold ready on port 59219")
		until, nextInventory := time.Now().Add(15*time.Minute), time.Now().Add(30*time.Second)
		for time.Now().Before(until) {
			if _, err = os.Stat(stopPath); err == nil {
				break
			}
			if time.Now().After(nextInventory) {
				fresh, e := runner.DescribeTools(ctx, cfg)
				if e != nil {
					t.Fatal(e)
				}
				scope, e := runner.InventoryScopeSHA256(client.BaseURL, client.ProjectID, profile.WorkerID)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = client.ToolInventory(ctx, runner.InventoryReport{WorkerID: profile.WorkerID, OperationID: newID(), Items: fresh, ScopeSHA256: scope}); e != nil {
					t.Fatal(e)
				}
				nextInventory = time.Now().Add(30 * time.Second)
			}
			time.Sleep(100 * time.Millisecond)
		}
		guiTask, e := f.s.Get(ctx, f.actor, f.project, "tasks", previewTask["id"].(string))
		if e != nil || guiTask.RunID != "" || calls != 1 || expertCount(t, f, "dag_runs") != 1 {
			t.Fatal("GUI preview started extra work or repeated model execution")
		}
		if err = os.WriteFile(filepath.Join(holdDir, "closed.local.json"), ready, 0600); err != nil {
			t.Fatal(err)
		}
	}
	retained, _ := json.Marshal(map[string]any{"run_id": assignment.RunID, "node_id": assignment.NodeID, "attempt": assignment.Attempt, "fence": assignment.Fence, "profile_sha256": profile.Profile.ProfileSHA256, "expert_plan_id": plan.ID, "team_id": teamID, "role_key": role.Key, "binding": binding, "model_execution": receipt, "usage_attempt_id": accounting.ID, "reported_tokens": accounting.ReportedTokens, "estimated_cost_micros": accounting.EstimatedCostMicros, "currency": "CNY", "state": accounting.State, "held_tokens": accounting.HeldTokens, "held_cost_micros": accounting.HeldCostMicros})
	t.Logf("model_execution_ledger_receipt=%s", retained)
	t.Logf("one current-actor expert item selection/preview/start, one real HTTP claim, one actual native bundle/MCP process, one owned model protocol response, exact request/response/binding hashes, 40 tokens / 57 CNY micros settled; independent formal task tests and acceptance remain pending")
}
