package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	mc "github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

func dispatchRecommendationRuntimeHash(raw []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

// The actual native bundle connects to the Go gateway and official MCP host.
// Three locally owned model-protocol responses drive one analysis and two code
// turns. They are not supplier calls or a customer's human sign-off.
func TestDispatchRecommendationActualGatewayAdoptionIndependentDelivery(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for the actual recommendation journey", err)
	}
	repo := t.TempDir()
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	const original = "module.exports=(a,b)=>a-b;\n"
	workspaceGitTestWrite(t, repo, "sum.cjs", original)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "sum.cjs")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: immutable recommendation source")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(root, "packages", "agent-runtime", "dist", "suma-agent.bundle.mjs")
	manifest := filepath.Join(root, "packages", "agent-runtime", "dist", "suma-agent.manifest.json")
	if _, err = os.Stat(bundle); err != nil {
		t.Fatal("build the current native runtime before the explicit integration journey", err)
	}
	steps, output := 4, int64(512)
	choice := DispatchRecommendationChoice{MaxSteps: &steps, MaxOutputTokens: &output, Reason: "The current native model can use the pinned development check and independent candidate test within the fixed budget."}
	var mu sync.Mutex
	calls := map[string]int{}
	requestHashes, responseHashes := map[string][]string{}, map[string][]string{}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var body map[string]any
		if readErr != nil || json.Unmarshal(raw, &body) != nil || r.Method != "POST" || r.URL.Path != "/v1/chat/completions" {
			t.Error("unexpected owned protocol request")
			w.WriteHeader(400)
			return
		}
		model, _ := body["model"].(string)
		mu.Lock()
		defer mu.Unlock()
		calls[model]++
		turn := calls[model]
		if body["stream"] != false || r.Header.Get("Authorization") != "Bearer owned-"+model {
			t.Error("the Go gateway changed the provider credential/protocol")
			w.WriteHeader(400)
			return
		}
		message := map[string]any{}
		finish := "stop"
		switch model {
		case "analysis122":
			if turn != 1 || body["max_completion_tokens"] != float64(2048) || body["tool_choice"] != "none" || body["tools"] != nil || choice.CandidateID == "" {
				t.Error("analysis must be one bounded tool-free request")
				w.WriteHeader(400)
				return
			}
			proposal := DispatchRecommendationProposal{SchemaVersion: 1, Summary: "Use the current approved native candidate", Rationale: "One immutable current model candidate satisfies the exact code task, independent test and hard-budget constraints.", Recommendations: []DispatchRecommendationChoice{choice}, Questions: []string{}}
			message["content"] = stringify(proposal)
		case "coding122":
			if body["max_completion_tokens"] != float64(512-3*(turn-1)) || body["tool_choice"] != nil {
				t.Error("coding did not use the chosen cumulative output limit")
				w.WriteHeader(400)
				return
			}
			if turn == 1 {
				alias := func(name string) string {
					return "tool_" + dispatchRecommendationRuntimeHash([]byte("workspace." + name))[:32]
				}
				message["content"] = nil
				message["tool_calls"] = []any{map[string]any{"id": "owned-write122", "type": "function", "function": map[string]any{"name": alias("write"), "arguments": stringify(map[string]any{"path": "sum.cjs", "content": "module.exports=(a,b)=>a+b;\n", "expected_sha256": dispatchRecommendationRuntimeHash([]byte(original))})}}, map[string]any{"id": "owned-check122", "type": "function", "function": map[string]any{"name": alias("check"), "arguments": stringify(map[string]any{"check_id": "dev-check122"})}}}
				finish = "tool_calls"
			} else if turn == 2 {
				message["content"] = "Changed sum.cjs and ran the administrator-pinned development check. Independent testing and formal Review remain required."
			} else {
				t.Error("unexpected extra coding model turn")
				w.WriteHeader(400)
				return
			}
		default:
			t.Error("an unselected model reached the provider")
			w.WriteHeader(400)
			return
		}
		reply, _ := json.Marshal(map[string]any{"id": "owned-response-" + model, "model": model, "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3}})
		requestHashes[model] = append(requestHashes[model], dispatchRecommendationRuntimeHash(raw))
		responseHashes[model] = append(responseHashes[model], dispatchRecommendationRuntimeHash(reply))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}))
	defer provider.Close()
	endpoint := provider.URL + "/v1"
	endpointSHA, err := mc.EndpointDigest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deployment := func(id, model string) mc.Deployment {
		d := mc.Deployment{ID: id, Revision: 1, Protocol: mc.ProviderProtocol, Model: model, EndpointOrigin: provider.URL, EndpointConfigSHA256: endpointSHA, ContextWindow: 32768}
		d.SHA256 = mc.DeploymentDigest(d)
		return d
	}
	analysisDeployment, codingDeployment := deployment("analysis-deployment122", "analysis122"), deployment("coding-deployment122", "coding122")
	t.Setenv("SUMA_OWNED_ANALYSIS122", "owned-analysis122")
	t.Setenv("SUMA_OWNED_CODING122", "owned-coding122")
	analysisCfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Commands: map[string]runner.CommandSpec{"recommend122": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{bundle}, Input: "agent-jsonl", ReadOnly: true, DispatchRecommendation: true, ModelExecutionProtocol: mc.Protocol}}, ModelDeployments: []runner.ModelDeploymentSpec{{Deployment: analysisDeployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_ANALYSIS122"}}}
	const check = `if(require('./sum.cjs')(2,3)!==5)throw Error('sum(2,3) must equal 5');`
	developerCfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"native-code122": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{bundle}, Input: "agent-jsonl", ModelExecutionProtocol: mc.Protocol, NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifest, Checks: []string{"dev-check122"}}}, "dev-check122": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", check}}}, ModelDeployments: []runner.ModelDeploymentSpec{{Deployment: codingDeployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_CODING122"}}}
	const verify = `const fs=require('node:fs');const ok=require('./sum.cjs')(2,3)===5;fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="routing122" name="sum">'+(ok?'':'<failure message="sum(2,3) must equal 5"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	testCfg := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"verify122": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", verify, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	analysisProfile, analysisToken := issueRepairRuntimeTool(t, f, "analysis-worker122", &analysisCfg)
	developerProfile, developerToken := dispatchRecommendationRuntimeCodeTool(t, f, &developerCfg)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "independent-worker122", &testCfg)
	analysisRate := modelRateForTest(t, f, analysisProfile, analysisDeployment, 0)
	codingRate := modelRateForTest(t, f, developerProfile, codingDeployment, 0)
	requirement := f.call("POST", "/requirements", f.body(map[string]any{"title": "Arithmetic correctness", "description": "Fix the exact current implementation through an explicitly reviewed recommendation.", "acceptance_criteria": []string{"sum(2,3) returns 5 on the independently tested candidate"}}), 201)
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Use the approved arithmetic candidate", "description": "Change sum.cjs while preserving the administrator checkout.", "requirement_id": requirement["id"], "acceptance_criteria": []string{"sum(2,3) returns 5 on the independently tested candidate"}}), 201))
	qualityPlan := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "arithmetic122", Title: "Independent candidate arithmetic", Description: "Test the exact host-sealed candidate", Preconditions: "Claim candidate_from develop", Steps: []string{"Evaluate sum(2,3)", "Keep the original JUnit selector"}, ExpectedResult: "The sum equals 5", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "routing122", Name: "sum"}})
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Recommended native code with independent fixed candidate regression", "nodes": []any{map[string]any{"id": "develop", "type": "agent", "command": []string{"native-code122"}, "config": map[string]any{"goal": "Correct sum.cjs using only the provided CAS tools and run dev-check122."}}, map[string]any{"id": "verify", "type": "test", "command": []string{"verify122"}, "depends_on": []string{"develop"}, "config": map[string]any{"candidate_from": "develop", "tool_profile_id": testProfile.ID, "tool_profile_sha256": testProfile.Profile.ProfileSHA256}}}}), 201)
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	tokenBudget, costBudget := int64(200000), int64(200000)
	options := DispatchRecommendationOptionsRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: workflow["id"].(string), WorkflowVersion: int(workflow["version"].(float64)), AgentNodeID: "develop", Knowledge: []ProjectKnowledgeReference{}, Constraints: DispatchRecommendationConstraints{RequireHardBudget: true, Budget: &mc.Budget{TokenLimit: &tokenBudget, CostLimitMicros: &costBudget, Currency: "CNY"}, MaxSteps: 4, MaxOutputTokens: 512}}
	// Alias rejection happens at transport, without losing the valid input or
	// creating an analysis run. Domain checks then resolve the original source.
	invalidOptions := issueRepairRuntimeDecode[map[string]any](t, options)
	invalidOptions["Task_ID"] = invalidOptions["task_id"]
	delete(invalidOptions, "task_id")
	f.call("POST", "/dispatch-recommendations/options", invalidOptions, 400)
	source := issueRepairRuntimeDecode[DispatchRecommendationSource](t, f.call("POST", "/dispatch-recommendations/options", options, 200))
	if len(source.Candidates) != 1 || source.Candidates[0].ToolProfileID != developerProfile.ID || source.Candidates[0].Model != codingDeployment.Model || source.Candidates[0].CostState != "quoted" || source.Candidates[0].BudgetMode != "platform_enforced" {
		t.Fatal("the recommendation did not freeze the real quoted model candidate", source)
	}
	mu.Lock()
	choice.CandidateID = source.Candidates[0].ID
	mu.Unlock()
	selection := mc.Selection{DeploymentID: analysisDeployment.ID, DeploymentRevision: analysisDeployment.Revision, DeploymentSHA256: analysisDeployment.SHA256, Parameters: mc.Parameters{MaxSteps: 1, MaxOutputTokens: 2048}}
	recommendation := issueRepairRuntimeDecode[DispatchRecommendation](t, f.call("POST", "/dispatch-recommendations", DispatchRecommendationCreateRequest{DispatchRecommendationOptionsRequest: options, SourceSHA256: source.SHA256, Goal: "Recommend the current authorized model and bounded parameters, then wait for an explicit human dispatch preview.", Locale: "en-US", AnalysisToolProfileID: analysisProfile.ID, ExpectedAnalysisToolVersion: analysisProfile.Version, AnalysisModelExecutionSelection: &selection, AcknowledgeModelUse: true, IdempotencyKey: newID()}, 201))
	control := httptest.NewServer(f.handler)
	defer control.Close()
	analysisClient := &runner.Client{BaseURL: control.URL, Token: analysisToken, ProjectID: f.project}
	a, err := analysisClient.Claim(ctx, analysisProfile.WorkerID, []string{"command:recommend122"})
	if err != nil || a == nil || a.RunID != recommendation.RunID {
		t.Fatal("the original analysis was not claimed", err)
	}
	analyzed := runner.Execute(ctx, *a, analysisCfg)
	analyzed.OperationID = newID()
	if analyzed.ExitCode != 0 || analyzed.Error != "" || analyzed.OutcomeUnknown || analyzed.ToolEvidence == nil || analyzed.ToolEvidence.ModelExecution == nil || analyzed.WorkspaceEvidence != nil && len(analyzed.WorkspaceEvidence) > 0 {
		t.Fatalf("actual readonly analysis failed: exit=%d error=%s output=%q", analyzed.ExitCode, analyzed.Error, analyzed.Output[:min(len(analyzed.Output), 4000)])
	}
	if err = analysisClient.Complete(ctx, analyzed); err != nil {
		t.Fatal("analysis result did not persist", err)
	}
	recommendation = issueRepairRuntimeDecode[DispatchRecommendation](t, f.call("GET", "/dispatch-recommendations/"+recommendation.ID+"?project_id="+f.project, nil, 200))
	if recommendation.State != "ready" || !recommendation.BasisCurrent || recommendation.Proposal == nil || len(recommendation.Proposal.Recommendations) != 1 || recommendation.OriginalOutputSHA256 != dispatchRecommendationRuntimeHash([]byte(analyzed.Output)) {
		t.Fatal("the real single analysis did not produce the original reviewable proposal", recommendation)
	}
	beforeAdopt := expertCount(t, f, "dag_runs")
	adopt := issueRepairRuntimeDecode[DispatchRecommendationAdoption](t, f.call("POST", "/dispatch-recommendations/"+recommendation.ID+"/adopt", DispatchRecommendationAdoptRequest{ProjectID: f.project, ExpectedVersion: recommendation.Version, SourceSHA256: source.SHA256, Choice: recommendation.Proposal.Recommendations[0], Reason: "Owned fixture owner explicitly adopts this exact candidate into a separate dispatch preview.", IdempotencyKey: newID()}, 201))
	if expertCount(t, f, "dag_runs") != beforeAdopt || adopt.Plan.ModelExecution == nil || adopt.Plan.ModelExecution.Deployment != codingDeployment || adopt.Plan.ModelExecution.Parameters.MaxSteps != steps || adopt.Plan.ModelExecution.Parameters.MaxOutputTokens != output {
		t.Fatal("adoption bypassed preview or changed the selected model parameters", adopt)
	}
	run := f.call("POST", "/task-dispatch-plans/"+adopt.Plan.ID+"/start", f.body(map[string]any{"expected_version": adopt.Plan.Version}), 201)
	developerClient := &runner.Client{BaseURL: control.URL, Token: developerToken, ProjectID: f.project}
	b, err := developerClient.Claim(ctx, developerProfile.WorkerID, []string{"command:native-code122"})
	if err != nil || b == nil || b.RunID != run["id"] {
		t.Fatal("the approved original plan was not claimed", err)
	}
	journal, err := runner.OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	if err = journal.Accept(*b); err != nil {
		t.Fatal(err)
	}
	developerCfg.Journal = journal
	hostEvents := []wo.Event{}
	developerCfg.WorkspaceProgress = func(event wo.Event) error {
		if err := journal.RecordWorkspaceProgress(*b, event); err != nil {
			return err
		}
		if err := developerClient.Progress(ctx, runner.Progress{Assignment: *b, OperationID: event.EventID, HostOperation: &event}); err != nil {
			return err
		}
		if err := journal.AckProgress(event.EventID); err != nil {
			return err
		}
		hostEvents = append(hostEvents, wo.CloneEvent(event))
		return nil
	}
	produced := runner.Execute(ctx, *b, developerCfg)
	produced.OperationID = newID()
	if produced.ExitCode != 0 || produced.Error != "" || produced.OutcomeUnknown || produced.ToolEvidence == nil || produced.WorkspaceEvidence["clean"] != true {
		t.Fatalf("actual native code/check/seal failed: exit=%d error=%s output=%q", produced.ExitCode, produced.Error, produced.Output[:min(len(produced.Output), 4000)])
	}
	candidate, _ := produced.WorkspaceEvidence["head_commit"].(string)
	if candidate == "" || candidate == base {
		t.Fatal("the native host did not seal a new candidate")
	}
	if len(hostEvents) != 6 || len(journal.PendingProgress()) != 0 {
		t.Fatal("the actual write/check/seal host facts were not durably acknowledged", hostEvents)
	}
	if err = developerClient.Complete(ctx, produced); err != nil {
		t.Fatal("native candidate callback rejected", err)
	}
	testClient := &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	c, err := testClient.Claim(ctx, testProfile.WorkerID, []string{"command:verify122"})
	if err != nil || c == nil || c.RunID != b.RunID || c.WorkerID == b.WorkerID || c.CandidateFrom != "develop" || c.CandidateCommit != candidate {
		t.Fatal("independent worker did not receive the exact sealed candidate", err)
	}
	tested := runner.Execute(ctx, *c, testCfg)
	tested.OperationID = newID()
	if tested.ExitCode != 0 || tested.Error != "" || tested.TestReport == nil || tested.WorkspaceEvidence["base_commit"] != candidate || tested.WorkspaceEvidence["head_commit"] != candidate {
		t.Fatalf("actual independent JUnit failed: exit=%d error=%s", tested.ExitCode, tested.Error)
	}
	if err = testClient.Complete(ctx, tested); err != nil {
		t.Fatal(err)
	}
	quality := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+qualityPlan.ID+"/executions/"+b.RunID+"?project_id="+f.project, nil, 200))
	if !quality.BasisCurrent || quality.State != "passed" || quality.Passed != 1 || len(quality.SourceCommits) != 1 || quality.SourceCommits[0] != candidate {
		t.Fatal("formal quality did not use the exact independently tested candidate", quality)
	}
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": task.Version, "run_id": b.RunID, "decision": "accepted", "reason": "Owned protocol fixture owner explicitly accepts the exact independently tested candidate; no customer human sign-off is claimed."}), 200)
	if workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base {
		t.Fatal("the actual native journey changed the administrator checkout")
	}
	if raw, err := os.ReadFile(filepath.Join(repo, "sum.cjs")); err != nil || string(raw) != original {
		t.Fatal("administrator source was changed", err)
	}
	mu.Lock()
	if calls["analysis122"] != 1 || calls["coding122"] != 2 {
		t.Fatal("unexpected actual model request counts", calls)
	}
	for model, evidence := range map[string]*mc.Evidence{"analysis122": analyzed.ToolEvidence.ModelExecution, "coding122": produced.ToolEvidence.ModelExecution} {
		if evidence == nil || !evidence.GatewayClosed || len(evidence.Requests) != calls[model] {
			t.Fatal("actual gateway receipt incomplete", model)
		}
		for i, request := range evidence.Requests {
			if request.RequestSHA256 != requestHashes[model][i] || request.ResponseSHA256 != responseHashes[model][i] || request.ProviderModel != model || request.UsageState != tc.UsageComplete {
				t.Fatal("the gateway receipt differs from the actual owned HTTP", model, i)
			}
		}
	}
	mu.Unlock()
	attempts, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil {
		t.Fatal(err)
	}
	settled := 0
	for _, attempt := range attempts.Items {
		if attempt.Model == "analysis122" || attempt.Model == "coding122" {
			expectedTokens, expectedCost, rateID := int64(10), int64(13), analysisRate.ID
			if attempt.Model == "coding122" {
				expectedTokens, expectedCost, rateID = 20, 26, codingRate.ID
			}
			if attempt.State != "settled" || attempt.Rate == nil || attempt.Rate.ID != rateID || attempt.ReportedTokens == nil || *attempt.ReportedTokens != expectedTokens || attempt.EstimatedCostMicros == nil || *attempt.EstimatedCostMicros != expectedCost || attempt.HeldTokens != 0 {
				t.Fatal("actual analysis/coding budget ledger did not settle its frozen quote", attempt)
			}
			settled++
		}
	}
	if settled != 2 {
		t.Fatal("analysis and coding must have separate authoritative usage attempts", settled)
	}
	receipt := map[string]any{"stage": "T-ROUTING-122", "recommendation_id": recommendation.ID, "analysis_run_id": a.RunID, "task_id": task.ID, "plan_id": adopt.Plan.ID, "run_id": b.RunID, "source_sha256": source.SHA256, "candidate_id": choice.CandidateID, "base_commit": base, "candidate_commit": candidate, "analysis_model_requests": 1, "coding_model_requests": 2, "host_operation_records": len(hostEvents), "independent_worker": c.WorkerID, "junit_sha256": tested.TestReport.SHA256, "customer_human_signoff": false}
	t.Logf("dispatch_recommendation_actual_receipt=%s", stringify(receipt))
	dispatchRecommendationRuntimeHold(t, f, repo, base, candidate, receipt)
}

// The root profile's hash includes its pinned check closure. Describe both
// commands together, then publish only the chosen native execution identity.
func dispatchRecommendationRuntimeCodeTool(t *testing.T, f *fixture, cfg *runner.ExecConfig) (ToolProfile, string) {
	t.Helper()
	ctx := context.Background()
	inventory, err := runner.DescribeTools(ctx, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	var actual *tc.Observation
	for i := range inventory {
		if inventory[i].Profile.CommandID == "native-code122" {
			actual = &inventory[i]
		}
	}
	if actual == nil || !actual.Available {
		t.Fatal("actual native code/check inventory unavailable", inventory)
	}
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "coding-worker122", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "coding-worker122", Name: "Current native code with pinned check", Profile: actual.Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ToolInventory = inventory
	return profile, token
}

// Optional browser inspection serves this completed real journey. It retains
// the unique SQLite database and both committed source revisions before the
// temporary repository is removed. The browser may only read and refresh.
func dispatchRecommendationRuntimeHold(t *testing.T, f *fixture, repo, base, candidate string, receipt map[string]any) {
	t.Helper()
	dir, web := os.Getenv("SUMA_DISPATCH_RECOMMENDATION_UI_DIR"), os.Getenv("SUMA_DISPATCH_RECOMMENDATION_WEB_DIR")
	if dir == "" && web == "" {
		return
	}
	if dir == "" || web == "" {
		t.Fatal("both owned recommendation GUI directories are required")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".cache", "e2e", "dispatch-recommendation-122"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil || dir == root || !under(root, dir) {
		t.Fatal("recommendation receipts must be inside the exact owned cache")
	}
	web, err = filepath.Abs(web)
	if err != nil || web == root || !under(root, web) || under(web, dir) || under(dir, web) {
		t.Fatal("recommendation private receipts and web assets must be separate owned directories")
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("build the final recommendation UI before the explicit GUI hold", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, web} {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !under(resolvedRoot, resolved) {
			t.Fatal("GUI paths must resolve inside the exact owned cache")
		}
	}
	for _, name := range []string{"login.local.json", "login.ready", "stop.request", "result.public.json", "fixture.sqlite", "base-source.tar", "candidate-source.tar", "source.manifest.public.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatal("recommendation GUI receipts must be fresh", name)
		}
	}
	server, origin, port := modelSelectionGUIServer(t, f, web, "recommend122.localhost")
	counts := func() map[string]int {
		out := modelSelectionGUICounts(t, f)
		for _, name := range []string{"suma_tasks", "suma_requirements", "suma_test_plan_reports", "dag_events", "suma_dispatch_recommendations", "suma_dispatch_recommendation_outcomes"} {
			out[name] = expertCount(t, f, name)
		}
		return out
	}
	protected := func() string {
		var payload string
		if err := f.s.DB.QueryRow(`SELECT payload FROM suma_dispatch_recommendations WHERE id=?`, receipt["recommendation_id"]).Scan(&payload); err != nil {
			t.Fatal(err)
		}
		task, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", receipt["task_id"].(string))
		if err != nil {
			t.Fatal(err)
		}
		return dispatchRecommendationRuntimeHash([]byte(payload + stringify(task)))
	}
	seed, seedSHA := counts(), protected()
	state := map[string]any{"status": "ready", "kind": "actual_gateway_recommendation_delivery", "origin": origin, "receipt": receipt, "seed_counts": seed, "seed_protected_sha256": seedSHA, "model_calls": 3, "customer_human_signoff": false}
	t.Cleanup(func() {
		server.Close()
		final, finalSHA := counts(), protected()
		state["final_counts"], state["final_protected_sha256"] = final, finalSHA
		if !reflect.DeepEqual(seed, final) || seedSHA != finalSHA {
			state["status"] = "read_only_guard_failed"
			t.Error("read-only GUI changed recommendation/delivery facts")
		}
		archives := []map[string]any{}
		for _, source := range []struct{ name, revision string }{{"base-source.tar", base}, {"candidate-source.tar", candidate}} {
			path := filepath.Join(dir, source.name)
			workspaceGitTestCommand(t, repo, "archive", "--format=tar", "--output="+path, source.revision)
			metadata := projectRiskSourceArchive(t, path)
			metadata["revision"] = source.revision
			archives = append(archives, metadata)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "source.manifest.public.json"), map[string]any{"kind": "owned_committed_source_archives", "archives": archives})
		state["source_archives"] = archives
		database := filepath.Join(dir, "fixture.sqlite")
		if _, err := f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(database, "'", "''") + "'"); err != nil {
			t.Error("retain actual recommendation fixture database", err)
		} else if raw, err := os.ReadFile(database); err != nil {
			t.Error(err)
		} else {
			state["database_sha256"], state["database_bytes"] = fmt.Sprintf("%x", sha256.Sum256(raw)), len(raw)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"kind": "actual_gateway_recommendation_delivery", "allowed_actions": []string{"read", "refresh"}, "origin": origin, "base_url": server.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "user_id": f.user.ID, "receipt": receipt})
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "kind": "actual_gateway_recommendation_delivery", "origin": origin, "port": port, "seed_counts": seed, "model_calls": 3})
	t.Logf("actual recommendation GUI ready: %s; read/refresh only", origin)
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	state["stop_reason"] = "timeout"
	for waiting := true; waiting; {
		select {
		case <-timer.C:
			waiting = false
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop.request")); err == nil {
				state["stop_reason"] = "explicit_stop"
				waiting = false
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
		}
	}
	state["status"] = "closed_without_new_execution"
}
