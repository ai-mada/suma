package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	mc "github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

// This opt-in journey uses real Git, Node, native SDK/MCP, HTTP accounting,
// bundle transfer to a different repository, exact-candidate JUnit and the
// review API. The three model responses are owned deterministic protocol data;
// the fixture owner's API review is not a customer's human acceptance.
func TestTaskDiscussionDevelopmentActualNativeAndIndependentQuality(t *testing.T) {
	cli, evidence := os.Getenv("SUMA_DISCUSSION_DEVELOPMENT_CLI"), os.Getenv("SUMA_DISCUSSION_DEVELOPMENT_EVIDENCE_DIR")
	if cli == "" {
		t.Skip("an explicitly selected existing native bundle is required")
	}
	ctx := context.Background()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	cli, err = filepath.Abs(cli)
	if err != nil || !under(filepath.Join(root, ".cache", "e2e", "reply-128", "native"), cli) {
		t.Fatal("use the verified isolated T128 native bundle", err)
	}
	rawBundle, err := os.ReadFile(cli)
	if err != nil {
		t.Fatal(err)
	}
	hash := func(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
	var manifest struct {
		EntrySHA256 string `json:"entry_sha256"`
	}
	manifestPath := filepath.Join(filepath.Dir(cli), "suma-agent.manifest.json")
	workbenchRetainedJSON(t, manifestPath, &manifest)
	if manifest.EntrySHA256 != hash(rawBundle) {
		t.Fatal("bundle manifest does not bind actual bytes")
	}
	evidence = workbenchRetainedDirectory(t, evidence, filepath.Join(root, ".cache", "e2e", "terminal-129"), true)
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	public := map[string]any{"stage": "T129", "kind": "actual_discussion_native_candidate_quality", "status": "started", "bundle_sha256": hash(rawBundle), "commercial_model_calls": 0, "customer_human_signoff": false}
	modelSelectionGUIJSON(t, filepath.Join(evidence, "attempt.started.public.json"), public)
	defer func() {
		if t.Failed() {
			public["status"] = "failed_original_attempt_preserved"
		}
		dbPath := filepath.Join(evidence, "fixture.sqlite")
		if _, snapshotErr := f.s.DB.ExecContext(ctx, `VACUUM INTO ?`, dbPath); snapshotErr != nil {
			t.Errorf("retain original DB: %v", snapshotErr)
		} else if b, e := os.ReadFile(dbPath); e == nil {
			public["database_sha256"], public["database_bytes"] = hash(b), len(b)
		}
		modelSelectionGUIJSON(t, filepath.Join(evidence, "result.public.json"), public)
	}()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	const original = "module.exports=(a,b)=>a-b;\n"
	const fixed = "module.exports=(a,b)=>a+b;\n"
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	workspaceGitTestWrite(t, repo, "sum.cjs", original)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "sum.cjs")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: discussion development base")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	consumerRepo := filepath.Join(t.TempDir(), "independent-repository")
	workspaceGitTestCommand(t, filepath.Dir(consumerRepo), "clone", "--no-hardlinks", repo, consumerRepo)
	workspaceGitTestWrite(t, evidence, "source.original.cjs", original)
	workspaceGitTestWrite(t, evidence, "source.fixed.cjs", fixed)
	var mu sync.Mutex
	calls := 0
	transcripts := []map[string]any{}
	writeAlias := "tool_" + hash([]byte("workspace.write"))[:32]
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, readErr := io.ReadAll(io.LimitReader(r.Body, (2<<20)+1))
		var body map[string]any
		if readErr != nil || json.Unmarshal(raw, &body) != nil || r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer owned-discussion129" || body["model"] != "discussion-development129" || body["stream"] != false {
			t.Error("unexpected owned model protocol request")
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		calls++
		turn := calls
		mu.Unlock()
		message := map[string]any{"role": "assistant"}
		finish := "stop"
		switch turn {
		case 1:
			if body["tool_choice"] != "none" || body["max_completion_tokens"] != float64(512) {
				t.Error("read-only reply did not use explicit selected parameters")
			}
			message["content"] = `{"schema_version":1,"reply":"审阅当前任务后，在独立工作树修复加法；保留独立同候选测试和人工验收，尚未执行。"}`
		case 2:
			if body["max_completion_tokens"] != float64(1000) || !strings.Contains(string(raw), "reviewed_discussion_development_json") || !strings.Contains(string(raw), "reviewed-goal-129") {
				t.Error("native request lost the reviewed handoff or selected budget")
			}
			tools, _ := body["tools"].([]any)
			found := false
			for _, rawTool := range tools {
				tool, _ := rawTool.(map[string]any)
				fn, _ := tool["function"].(map[string]any)
				if fn["name"] == writeAlias {
					found = true
				}
			}
			if !found {
				t.Error("actual host did not advertise the exact canonical workspace write capability")
			}
			args := stringify(map[string]any{"path": "sum.cjs", "content": fixed, "expected_sha256": hash([]byte(original))})
			message["tool_calls"] = []any{map[string]any{"id": "owned-write129", "type": "function", "function": map[string]any{"name": writeAlias, "arguments": args}}}
			finish = "tool_calls"
		case 3:
			if !strings.Contains(string(raw), "owned-write129") {
				t.Error("native follow-up lost its original tool call")
			}
			message["content"] = "The actual workspace write returned. Independent candidate tests and explicit human acceptance remain required."
		default:
			t.Error("unexpected extra model request")
			w.WriteHeader(500)
			return
		}
		reply, _ := json.Marshal(map[string]any{"id": fmt.Sprintf("owned-discussion129-%d", turn), "model": "discussion-development129", "choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": finish}}, "usage": map[string]any{"prompt_tokens": 7, "completion_tokens": 3}})
		mu.Lock()
		transcripts = append(transcripts, map[string]any{"turn": turn, "request_json": string(raw), "response_json": string(reply), "request_sha256": hash(raw), "response_sha256": hash(reply)})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(reply)
	}))
	defer provider.Close()
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		public["model_calls"] = calls
		modelSelectionGUIJSON(t, filepath.Join(evidence, "model-http.public.json"), transcripts)
	}()
	endpoint := provider.URL + "/v1"
	endpointSHA, err := mc.EndpointDigest(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	deployment := mc.Deployment{ID: "discussion-development129", Revision: 1, Protocol: mc.ProviderProtocol, Model: "discussion-development129", EndpointOrigin: provider.URL, EndpointConfigSHA256: endpointSHA, ContextWindow: 8192}
	deployment.SHA256 = mc.DeploymentDigest(deployment)
	t.Setenv("SUMA_OWNED_DISCUSSION129", "owned-discussion129")
	modelDeployments := []runner.ModelDeploymentSpec{{Deployment: deployment, BaseURL: endpoint, APIKeyEnvRef: "SUMA_OWNED_DISCUSSION129"}}
	replyConfig := runner.ExecConfig{AllowExec: true, Workspace: repo, WorktreeRoot: filepath.Join(repo, ".cache", "discussion-replies129"), Commands: map[string]runner.CommandSpec{"discussion129": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{cli}, Input: "agent-jsonl", ReadOnly: true, TaskConversationReply: true, ModelExecutionProtocol: mc.Protocol}}, ModelDeployments: modelDeployments}
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, WorktreeRoot: filepath.Join(repo, ".cache", "native-development129"), Commands: map[string]runner.CommandSpec{"native129": {Adapter: "suma", Protocol: "suma-jsonl-v1", Executable: node, Args: []string{cli}, Input: "agent-jsonl", ModelExecutionProtocol: mc.Protocol, NativeWorkspace: &runner.NativeWorkspacePolicy{RuntimeManifest: manifestPath, Checks: []string{}}}}, ModelDeployments: modelDeployments}
	const testScript = `const fs=require('node:fs');const passed=require('./sum.cjs')(2,3)===5;fs.writeFileSync(process.argv[1],'<testsuite name="discussion129" tests="1" failures="'+(passed?'0':'1')+'"><testcase classname="arithmetic" name="adds">'+(passed?'':'<failure message="sum(2,3) must equal 5"/>')+'</testcase></testsuite>');process.exitCode=passed?0:1;`
	verifier := runner.ExecConfig{AllowExec: true, Workspace: consumerRepo, Repository: consumerRepo, BaseCommit: base, WorktreeRoot: filepath.Join(consumerRepo, ".cache", "independent-quality129"), Commands: map[string]runner.CommandSpec{"verify129": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", testScript, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	replyProfile, replyToken := issueRepairRuntimeTool(t, f, "discussion-worker129", &replyConfig)
	devProfile, devToken := issueRepairRuntimeTool(t, f, "native-developer129", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "independent-verifier129", &verifier)
	modelRateForTest(t, f, replyProfile, deployment, 0)
	modelRateForTest(t, f, devProfile, deployment, 0)
	control := httptest.NewServer(f.handler)
	defer control.Close()
	replyClient := &runner.Client{BaseURL: control.URL, Token: replyToken, ProjectID: f.project}
	devClient := &runner.Client{BaseURL: control.URL, Token: devToken, ProjectID: f.project}
	testClient := &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	developer.Artifacts, verifier.Artifacts = devClient, testClient
	requirement := issueRepairRuntimeDecode[Record](t, f.call("POST", "/requirements", f.body(map[string]any{"title": "Reviewed arithmetic development", "description": "Use actual workspace evidence, independent tests and explicit review.", "acceptance_criteria": []string{"sum(2,3) returns 5"}}), 201))
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Discussion to native code development", "description": "Repair arithmetic without bypassing independent verification.", "requirement_id": requirement.ID, "acceptance_criteria": []string{"sum(2,3) returns 5"}}), 201))
	caseDefinition := QualityTestCase{Key: "sum-regression129", Title: "Adding two integers returns their sum", Description: "Keep exact original selector across the reviewed development handoff", Preconditions: "Use the original fixed source", Steps: []string{"Evaluate sum(2,3)", "Capture matching JUnit"}, ExpectedResult: "Result is 5", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "arithmetic", Name: "adds"}}
	quality := issueRepairRuntimePlan(t, f, task, caseDefinition)
	qualityNode := func(candidate bool) dag.Node {
		config := map[string]any{"tool_profile_id": testProfile.ID, "tool_profile_sha256": testProfile.Profile.ProfileSHA256}
		n := dag.Node{ID: "verify", Name: "Independent exact-candidate JUnit", Type: "test", Command: []string{"verify129"}, Config: config}
		if candidate {
			config["candidate_from"] = "develop"
			n.DependsOn = []string{"develop"}
		}
		return n
	}
	baselineWorkflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Reproduce original arithmetic defect", Nodes: []dag.Node{qualityNode(false)}})
	if err != nil {
		t.Fatal(err)
	}
	baselineRun := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/runs", f.body(map[string]any{"workflow_id": baselineWorkflow.ID, "workflow_version": baselineWorkflow.Version, "task_id": task.ID, "expected_version": task.Version}), 201))
	baseline, err := testClient.Claim(ctx, testProfile.WorkerID, []string{"command:verify129"})
	if err != nil || baseline == nil || baseline.RunID != baselineRun.ID {
		t.Fatal("actual original failure claim missing", err)
	}
	failed := runner.Execute(ctx, *baseline, verifier)
	failed.OperationID = newID()
	if failed.ExitCode != 1 || failed.OutcomeUnknown || failed.TestReport == nil {
		t.Fatal("actual original failing report missing", failed.Error)
	}
	if err = testClient.Complete(ctx, failed); err != nil {
		t.Fatal(err)
	}
	issue := issueRepairRuntimeDecode[Record](t, f.call("POST", "/test-plans/"+quality.ID+"/executions/"+baselineRun.ID+"/defects", TestPlanDefectRequest{ProjectID: f.project, ExpectedVersion: quality.Version, CaseKey: caseDefinition.Key, Reason: "Retain the actual baseline failure for the reviewed development handoff", IdempotencyKey: newID()}, 201))
	intakeBody := f.body(map[string]any{"expected_version": issue.Version, "title": "Follow-up requirement for the exact failing arithmetic case"})
	intake := issueRepairRuntimeDecode[Record](t, f.call("POST", "/issues/"+issue.ID+"/intake", intakeBody, 201))
	intakeAgain := issueRepairRuntimeDecode[Record](t, f.call("POST", "/issues/"+issue.ID+"/intake", intakeBody, 201))
	if intakeAgain.ID != intake.ID {
		t.Fatal("issue intake duplicated the original requirement")
	}
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	note := issueRepairRuntimeDecode[TaskConversationMessage](t, f.call("POST", "/task-conversations", taskConversationRequest(f, task.ID, task.Version, "请审阅实际失败并给出修复计划，保留独立测试和人工验收。"), 201))
	tokenLimit, costLimit := int64(40000), int64(40000)
	selection := func(steps int, output int64) *mc.Selection {
		return &mc.Selection{DeploymentID: deployment.ID, DeploymentRevision: deployment.Revision, DeploymentSHA256: deployment.SHA256, Parameters: mc.Parameters{MaxSteps: steps, MaxOutputTokens: output}, Budget: &mc.Budget{TokenLimit: &tokenLimit, CostLimitMicros: &costLimit, Currency: "CNY"}}
	}
	replyRequest := TaskConversationReplyCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ExpectedLastSequence: note.Sequence, ReplyToSequence: note.Sequence, ContextMessageLimit: 20, ToolProfileID: replyProfile.ID, ExpectedToolVersion: replyProfile.Version, ModelExecutionSelection: selection(1, 512), Locale: "zh-CN", AcknowledgeModelUse: true, IdempotencyKey: newID()}
	reply := issueRepairRuntimeDecode[TaskConversationReply](t, f.call("POST", "/task-conversation-replies", replyRequest, 201))
	readAssignment, err := replyClient.Claim(ctx, replyProfile.WorkerID, []string{"command:discussion129"})
	if err != nil || readAssignment == nil || readAssignment.RunID != reply.RunID {
		t.Fatal("actual reply assignment missing", err)
	}
	readResult := runner.Execute(ctx, *readAssignment, replyConfig)
	readResult.OperationID = newID()
	if readResult.ExitCode != 0 || readResult.Error != "" || readResult.OutcomeUnknown {
		t.Fatal("actual readonly reply failed", readResult.Error)
	}
	if err = replyClient.Complete(ctx, readResult); err != nil {
		t.Fatal(err)
	}
	reply, err = f.s.GetTaskConversationReply(ctx, f.e, f.actor, f.project, reply.ID)
	if err != nil || reply.State != "completed" || reply.MessageID == "" {
		t.Fatal("actual assistant reply was not published", reply, err)
	}
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Reviewed native code and independent quality", Nodes: []dag.Node{{ID: "develop", Type: "agent", Command: []string{"native129"}, Config: map[string]any{"goal": "Implement the human-reviewed engineering objective inside the permitted attempt workspace. Preserve independent candidate testing and human review."}}, qualityNode(true)}})
	if err != nil {
		t.Fatal(err)
	}
	request := TaskDiscussionDevelopmentCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ReplyID: reply.ID, ExpectedReplyVersion: reply.Version, WorkflowID: workflow.ID, WorkflowVersion: workflow.Version, AgentNodeID: "develop", ToolProfileID: devProfile.ID, ExpectedToolVersion: devProfile.Version, ModelExecutionSelection: selection(2, 1000), ReviewedGoal: "reviewed-goal-129: repair sum.cjs to add the two input integers using a checked workspace write; preserve independent verification.", AcceptanceCriteria: []string{"The fixed sum(2,3) equals 5", "Test the exact sealed candidate on the independent worker", "Require explicit review before task acceptance"}, AcknowledgeExecution: true, IdempotencyKey: newID()}
	plan := issueRepairRuntimeDecode[TaskDiscussionDevelopmentPlan](t, f.call("POST", "/task-discussion-development-plans", request, 201))
	start := TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}
	run := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/task-discussion-development-plans/"+plan.ID+"/start", start, 201))
	if replay := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/task-discussion-development-plans/"+plan.ID+"/start", start, 201)); replay.ID != run.ID {
		t.Fatal("handoff replay created another execution")
	}
	assignment, err := devClient.Claim(ctx, devProfile.WorkerID, []string{"command:native129"})
	if err != nil || assignment == nil || assignment.RunID != run.ID || assignment.NodeID != "develop" {
		t.Fatal("actual native development assignment missing", err)
	}
	journal, err := runner.OpenJournal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	if err = journal.Accept(*assignment); err != nil {
		t.Fatal(err)
	}
	developer.Journal = journal
	hostEvents := []wo.Event{}
	developer.WorkspaceProgress = func(event wo.Event) error {
		if e := journal.RecordWorkspaceProgress(*assignment, event); e != nil {
			return e
		}
		if e := devClient.Progress(ctx, runner.Progress{Assignment: *assignment, OperationID: event.EventID, HostOperation: &event}); e != nil {
			return e
		}
		if e := journal.AckProgress(event.EventID); e != nil {
			return e
		}
		hostEvents = append(hostEvents, wo.CloneEvent(event))
		return nil
	}
	produced := runner.Execute(ctx, *assignment, developer)
	if produced.OperationID == "" {
		produced.OperationID = newID()
	}
	candidate, _ := produced.WorkspaceEvidence["head_commit"].(string)
	if produced.ExitCode != 0 || produced.Error != "" || produced.OutcomeUnknown || candidate == "" || candidate == base || produced.WorkspaceEvidence["clean"] != true || produced.ToolEvidence == nil || produced.ToolEvidence.ModelExecution == nil || len(produced.ToolEvidence.ModelExecution.Requests) != 2 || produced.WorkspaceEvidence["candidate_bundle"] == nil {
		t.Fatalf("actual native write/seal/transfer failed: exit=%d unknown=%t error=%s", produced.ExitCode, produced.OutcomeUnknown, produced.Error)
	}
	if len(hostEvents) < 4 || len(journal.PendingProgress()) != 0 {
		t.Fatal("actual workspace write and seal progress were not acknowledged", len(hostEvents))
	}
	if err = devClient.Complete(ctx, produced); err != nil {
		t.Fatal(err)
	}
	var transferred cc.Binding
	if err = json.Unmarshal([]byte(stringify(produced.WorkspaceEvidence["candidate_bundle"])), &transferred); err != nil || !transferred.ValidSealed() {
		t.Fatal("actual sealed candidate binding missing", err)
	}
	var storedPath string
	if err = f.s.DB.QueryRow(`SELECT storage_path FROM suma_candidate_transfers WHERE id=?`, transferred.TransferID).Scan(&storedPath); err != nil {
		t.Fatal(err)
	}
	bundleBytes, err := f.s.readCandidateBundle(candidateTransfer{binding: transferred, path: storedPath})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(evidence, "candidate.bundle"), bundleBytes, 0600); err != nil {
		t.Fatal(err)
	}
	public["candidate_bundle_sha256"], public["candidate_bundle_bytes"] = hash(bundleBytes), len(bundleBytes)
	beforeTest, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": beforeTest.Version, "run_id": run.ID, "decision": "accepted", "reason": "Do not accept before independent exact-candidate testing"}), f.e); err == nil {
		t.Fatal("native completion bypassed independent quality")
	}
	independent, err := testClient.Claim(ctx, testProfile.WorkerID, []string{"command:verify129"})
	if err != nil || independent == nil || independent.RunID != run.ID || independent.CandidateCommit != candidate || independent.WorkerID == assignment.WorkerID {
		t.Fatal("independent worker did not receive exact sealed candidate", err)
	}
	verified := runner.Execute(ctx, *independent, verifier)
	verified.OperationID = newID()
	if verified.ExitCode != 0 || verified.Error != "" || verified.OutcomeUnknown || verified.TestReport == nil || verified.WorkspaceEvidence["head_commit"] != candidate || verified.WorkspaceEvidence["base_commit"] != candidate || verified.WorkspaceEvidence["clean"] != true || verified.WorkspaceEvidence["attempt_key"] == produced.WorkspaceEvidence["attempt_key"] {
		t.Fatal("actual separate-repository independent verification failed", verified.Error)
	}
	if err = testClient.Complete(ctx, verified); err != nil {
		t.Fatal(err)
	}
	modelSelectionGUIJSON(t, filepath.Join(evidence, "test-report.public.json"), verified.TestReport)
	execution := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+quality.ID+"/executions/"+run.ID+"?project_id="+f.project, nil, 200))
	if execution.State != "passed" || !execution.BasisCurrent || execution.Passed != 1 || len(execution.SourceCommits) != 1 || execution.SourceCommits[0] != candidate {
		t.Fatal("formal quality differs from real independent candidate", execution)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil || current.Status == "accepted" {
		t.Fatal("test report automatically accepted task", current, err)
	}
	accepted := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Owned local fixture owner explicitly reviews the independently tested candidate; no customer sign-off is claimed"}), 200))
	if accepted.Status != "accepted" || accepted.RunID != run.ID {
		t.Fatal("explicit review did not retain exact run", accepted)
	}
	if workspaceGitTestCommand(t, repo, "rev-parse", "HEAD") != base || workspaceGitTestCommand(t, consumerRepo, "rev-parse", "HEAD") != base {
		t.Fatal("administrator checkouts were changed")
	}
	page, err := f.s.ListUsageAttempts(ctx, f.actor, f.project, "", "", 100, "")
	if err != nil {
		t.Fatal("actual reply/development usage missing", page, err)
	}
	tokens, cost := int64(0), int64(0)
	modelAttempts := 0
	for _, item := range page.Items {
		// Bound non-model test commands also have usage rows. Their unpriced
		// or unreported state must not be presented as provider settlement.
		if item.ModelExecution == nil {
			continue
		}
		modelAttempts++
		if item.State != "settled" || item.ReportedTokens == nil || item.EstimatedCostMicros == nil || item.HeldTokens != 0 || item.HeldCostMicros == nil || *item.HeldCostMicros != 0 {
			t.Fatal("model usage did not settle against original budget", item)
		}
		tokens += *item.ReportedTokens
		cost += *item.EstimatedCostMicros
	}
	mu.Lock()
	actualCalls := calls
	mu.Unlock()
	if actualCalls != 3 || modelAttempts != 2 || tokens != 30 || cost != 39 {
		t.Fatal("actual model calls or estimated totals differ", actualCalls, tokens, cost)
	}
	public["status"] = "actual_native_handoff_independent_quality_and_issue_intake_passed"
	public["base_commit"], public["candidate_commit"], public["task_id"], public["reply_id"], public["handoff_plan_id"], public["run_id"] = base, candidate, task.ID, reply.ID, plan.ID, run.ID
	public["initial_failed_run_id"], public["issue_id"], public["intake_requirement_id"], public["formal_test_plan_id"] = baselineRun.ID, issue.ID, intake.ID, quality.ID
	public["producer_worker"], public["independent_worker"], public["separate_repository_import"] = assignment.WorkerID, independent.WorkerID, true
	public["formal_quality_state"], public["task_status"], public["model_calls"], public["reported_tokens"], public["estimated_cost_micros"] = execution.State, accepted.Status, actualCalls, tokens, cost
	public["model_usage_attempts"], public["other_usage_records"] = modelAttempts, len(page.Items)-modelAttempts
	public["source_sha256"], public["bundle_sha256"], public["test_report_sha256"] = plan.SourceSHA256, hash(rawBundle), verified.TestReport.SHA256
	public["workspace_host_events"], public["developer_evidence"], public["independent_evidence"] = hostEvents, produced.WorkspaceEvidence, verified.WorkspaceEvidence
	public["administrator_checkouts_unchanged"], public["accepted_by_fixture_owner_api"], public["automatic_customer_acceptance"] = true, true, false
}
