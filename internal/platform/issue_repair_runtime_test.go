package platform

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

// This journey uses real isolated Git worktrees, Node processes, captured JUnit
// and HTTP Runner callbacks. The fixture owner explicitly exercises the review
// protocol; this is not evidence of a real customer's human sign-off or an AI
// vendor call. Three processes suffice: reproduce, repair, independent retest.
func TestIssueRepairActualWorkspaceIndependentRegressionAndReopen(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.e.AttemptAccounting = f.s
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for this actual issue-repair integration gate", err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("Git is required for this actual issue-repair integration gate", err)
	}
	repo := t.TempDir()
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	const broken = "module.exports=(a,b)=>a-b;\n"
	workspaceGitTestWrite(t, repo, "sum.cjs", broken)
	workspaceGitTestCommand(t, repo, "add", ".gitignore", "sum.cjs")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: retain the original arithmetic failure")
	base := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	// All commands are administrator-owned literals. No shell or model chooses
	// paths, executables, Git flags, report locations or commit instructions.
	const repairScript = `const fs=require('node:fs'),cp=require('node:child_process'),path=require('node:path');const privateGit=path.resolve('.cache','issue-repair119-git'),hooks=path.join(privateGit,'empty-hooks'),config=path.join(privateGit,'empty.gitconfig');fs.mkdirSync(hooks,{recursive:true,mode:0o700});fs.writeFileSync(config,'',{flag:'wx',mode:0o600});fs.writeFileSync('sum.cjs','module.exports=(a,b)=>a+b;\n');const git=process.argv[1],flags=['-c','core.hooksPath='+hooks,'-c','user.name=Issue repair fixture','-c','user.email=issue-repair@example.invalid','-c','commit.gpgsign=false','-c','core.autocrlf=false'];const env={...process.env,GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:config,GIT_TERMINAL_PROMPT:'0'};for(const args of [['add','--','sum.cjs'],['commit','-m','fixture: repair exact arithmetic candidate']])cp.execFileSync(git,[...flags,...args],{env,stdio:'pipe'});console.log('Owned deterministic repair committed; independent verification remains required.');`
	const testScript = `const fs=require('node:fs');const passed=require('./sum.cjs')(2,3)===5;fs.writeFileSync(process.argv[1],'<testsuite name="issue119" tests="1" failures="'+(passed?'0':'1')+'"><testcase classname="arithmetic" name="adds">'+(passed?'':'<failure message="sum(2,3) must equal 5"/>')+'</testcase></testsuite>');process.exitCode=passed?0:1;`
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"repair119": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", repairScript, git}}}}
	verifier := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: base, Commands: map[string]runner.CommandSpec{"regression119": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", testScript, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	developerProfile, developerToken := issueRepairRuntimeTool(t, f, "repair-worker-119", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "independent-worker-119", &verifier)
	control := httptest.NewServer(f.handler)
	defer control.Close()
	developerClient := &runner.Client{BaseURL: control.URL, Token: developerToken, ProjectID: f.project}
	testClient := &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	caseDefinition := QualityTestCase{Key: "sum-regression", Title: "Adding two integers returns their sum", Description: "Retain the original defect selector across repair cycles", Preconditions: "Use the exact isolated source revision", Steps: []string{"Evaluate sum(2,3)", "Capture the matching JUnit case"}, ExpectedResult: "The result is 5", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "arithmetic", Name: "adds"}}
	qualityNode := func(candidate bool) map[string]any {
		config := map[string]any{"tool_profile_id": testProfile.ID, "tool_profile_sha256": testProfile.Profile.ProfileSHA256}
		out := map[string]any{"id": "regression", "type": "test", "command": []string{"regression119"}, "config": config}
		if candidate {
			config["candidate_from"] = "repair"
			out["depends_on"] = []string{"repair"}
		}
		return out
	}
	start := func(task Record, name string, nodes []any) string {
		workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": name, "nodes": nodes}), 201)
		run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "workflow_version": workflow["version"], "task_id": task.ID, "expected_version": task.Version}), 201)
		return run["id"].(string)
	}
	// Reproduce from the real buggy baseline, not fabricated WorkspaceEvidence.
	originalTask := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Reproduce arithmetic defect", "description": "Owned deterministic source fixture", "acceptance_criteria": []string{"sum(2,3) returns 5"}}), 201))
	originalPlan := issueRepairRuntimePlan(t, f, originalTask, caseDefinition)
	originalRun := start(originalTask, "Reproduce original failed case", []any{qualityNode(false)})
	a, err := testClient.Claim(ctx, testProfile.WorkerID, []string{"command:regression119"})
	if err != nil || a == nil || a.RunID != originalRun {
		t.Fatal("actual failing baseline claim is missing", err)
	}
	failed := runner.Execute(ctx, *a, verifier)
	failed.OperationID = newID()
	if failed.ExitCode != 1 || failed.OutcomeUnknown || failed.TestReport == nil || failed.WorkspaceEvidence["head_commit"] != base || failed.WorkspaceEvidence["clean"] != true {
		t.Fatalf("actual failure was not captured: exit=%d error=%s evidence=%+v", failed.ExitCode, failed.Error, failed.WorkspaceEvidence)
	}
	if err = testClient.Complete(ctx, failed); err != nil {
		t.Fatal("real failed JUnit callback was rejected", err)
	}
	originalExecution := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+originalPlan.ID+"/executions/"+originalRun+"?project_id="+f.project, nil, 200))
	if originalExecution.State != "failed" || originalExecution.Failed != 1 || len(originalExecution.Cases) != 1 || originalExecution.Cases[0].CaseKey != caseDefinition.Key {
		t.Fatalf("original failed case identity was lost: %+v", originalExecution)
	}
	issue := issueRepairRuntimeDecode[Record](t, f.call("POST", "/test-plans/"+originalPlan.ID+"/executions/"+originalRun+"/defects", TestPlanDefectRequest{ProjectID: f.project, ExpectedVersion: originalPlan.Version, CaseKey: caseDefinition.Key, Reason: "Retain the actual baseline failure", IdempotencyKey: newID()}, 201))
	prepare := func(current Record) IssueRepairView {
		return issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", "/issues/"+issue.ID+"/repair/prepare", IssueRepairPrepareRequest{ProjectID: f.project, ExpectedIssueVersion: current.Version, Title: "Repair the exact arithmetic failure", Description: "Keep the source case, independently verify the committed candidate", OwnerID: "", AcceptanceCriteria: []string{"sum(2,3) returns 5"}, Reason: "Explicit fixture repair preparation", IdempotencyKey: newID()}, 201))
	}
	prepared := prepare(issue)
	if prepared.Current == nil || prepared.Task == nil || prepared.Issue.Status != "in_progress" {
		t.Fatal("preparation did not establish the protected task and cycle", prepared)
	}
	repairTask := *prepared.Task
	regression := issueRepairRuntimePlan(t, f, repairTask, caseDefinition)
	bound := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", "/issues/"+issue.ID+"/repair/regression", IssueRepairBindRequest{ProjectID: f.project, ExpectedIssueVersion: prepared.Issue.Version, ExpectedRepairVersion: prepared.Current.Version, ExpectedPlanVersion: regression.Version, PlanID: regression.ID, CaseKeys: []string{caseDefinition.Key}, Reason: "Bind the same original case and selector", IdempotencyKey: newID()}, 200))
	if bound.Current == nil || bound.Current.Regression == nil || bound.Current.Regression.PlanSHA256 != regression.Published.SHA256 || bound.Current.Regression.PlanRevision != regression.Published.Revision {
		t.Fatal("regression binding did not retain its exact publication", bound)
	}
	// Both commands use their current approved platform profiles. The producer
	// remains an explicit deterministic command, never an impersonated AI tool.
	repairRun := start(repairTask, "Repair and independently verify the exact candidate", []any{map[string]any{"id": "repair", "type": "command", "command": []string{"repair119"}, "config": map[string]any{"tool_profile_id": developerProfile.ID, "tool_profile_sha256": developerProfile.Profile.ProfileSHA256}}, qualityNode(true)})
	producer, err := developerClient.Claim(ctx, developerProfile.WorkerID, []string{"command:repair119"})
	if err != nil || producer == nil || producer.RunID != repairRun || producer.NodeID != "repair" {
		t.Fatal("actual repair command was not claimed", err)
	}
	produced := runner.Execute(ctx, *producer, developer)
	produced.OperationID = newID()
	candidate, _ := produced.WorkspaceEvidence["head_commit"].(string)
	if produced.ExitCode != 0 || produced.Error != "" || produced.OutcomeUnknown || candidate == "" || candidate == base || produced.WorkspaceEvidence["clean"] != true || produced.ToolEvidence == nil || produced.ToolEvidence.Profile.ProfileSHA256 != developerProfile.Profile.ProfileSHA256 {
		t.Fatalf("actual committed repair failed: exit=%d error=%s output=%q evidence=%+v", produced.ExitCode, produced.Error, produced.Output[:min(len(produced.Output), 4000)], produced.WorkspaceEvidence)
	}
	if err = developerClient.Complete(ctx, produced); err != nil {
		t.Fatal("actual producer result callback was rejected", err)
	}
	independent, err := testClient.Claim(ctx, testProfile.WorkerID, []string{"command:regression119"})
	if err != nil || independent == nil || independent.RunID != repairRun || independent.CandidateCommit != candidate || independent.CandidateFrom != "repair" || independent.WorkerID == producer.WorkerID {
		t.Fatal("independent test did not claim the scheduler-pinned candidate", independent, err)
	}
	verified := runner.Execute(ctx, *independent, verifier)
	verified.OperationID = newID()
	if verified.ExitCode != 0 || verified.Error != "" || verified.OutcomeUnknown || verified.TestReport == nil || verified.WorkspaceEvidence["base_commit"] != candidate || verified.WorkspaceEvidence["head_commit"] != candidate || verified.WorkspaceEvidence["clean"] != true || verified.WorkspaceEvidence["attempt_key"] == produced.WorkspaceEvidence["attempt_key"] {
		t.Fatalf("independent real candidate verification failed: exit=%d error=%s evidence=%+v", verified.ExitCode, verified.Error, verified.WorkspaceEvidence)
	}
	if err = testClient.Complete(ctx, verified); err != nil {
		t.Fatal("actual independent JUnit callback was rejected", err)
	}
	execution := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+regression.ID+"/executions/"+repairRun+"?project_id="+f.project, nil, 200))
	if execution.State != "passed" || !execution.BasisCurrent || execution.Passed != 1 || !reflect.DeepEqual(execution.SourceCommits, []string{candidate}) || len(execution.Cases) != 1 || execution.Cases[0].CaseKey != caseDefinition.Key || len(execution.Cases[0].Evidence) != 1 || execution.Cases[0].Evidence[0].WorkerID != independent.WorkerID {
		t.Fatalf("passing regression facts differ from the real candidate: %+v", execution)
	}
	read := func() IssueRepairView {
		return issueRepairRuntimeDecode[IssueRepairView](t, f.call("GET", "/issues/"+issue.ID+"/repair?project_id="+f.project, nil, 200))
	}
	beforeReview := read()
	if beforeReview.CanClose || beforeReview.Task == nil {
		t.Fatal("a passing command bypassed the formal review gate", beforeReview)
	}
	f.call("POST", "/tasks/"+repairTask.ID+"/review", f.body(map[string]any{"expected_version": beforeReview.Task.Version, "run_id": repairRun, "decision": "accepted", "reason": "Protocol fixture owner explicitly accepts the independently tested candidate; no real customer sign-off is claimed"}), 200)
	reviewed := read()
	if !reviewed.CanClose || reviewed.Current == nil || reviewed.Task == nil || reviewed.Task.Status != "accepted" {
		t.Fatal("current independent evidence and explicit review did not unlock closure", reviewed)
	}
	closed := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", "/issues/"+issue.ID+"/repair/close", IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: reviewed.Issue.Version, ExpectedRepairVersion: reviewed.Current.Version, RunID: repairRun, Reason: "Close against current review and exact independent regression", IdempotencyKey: newID()}, 200))
	if closed.Issue.Status != "closed" || closed.Current == nil || closed.Current.Closure == nil || closed.Current.Closure.RunID != repairRun || closed.Current.Closure.PlanSHA256 != regression.Published.SHA256 || !reflect.DeepEqual(closed.Current.Closure.SourceCommits, []string{candidate}) {
		t.Fatal("closure lost current exact evidence", closed)
	}
	reopened := issueRepairRuntimeDecode[IssueRepairView](t, f.call("POST", "/issues/"+issue.ID+"/repair/reopen", IssueRepairReopenRequest{ProjectID: f.project, ExpectedIssueVersion: closed.Issue.Version, ExpectedRepairVersion: closed.Current.Version, Reason: "Owned fixture: record a subsequent recurrence without rewriting prior acceptance", IdempotencyKey: newID()}, 200))
	if reopened.Cycle != 2 || reopened.Current != nil || reopened.HistoryTotal != 1 || !reopened.HistoryComplete || len(reopened.History) != 1 || reopened.History[0].Closure == nil || reopened.History[0].Closure.RunID != repairRun {
		t.Fatal("reopen rewrote the historical closure", reopened)
	}
	next := prepare(reopened.Issue)
	if next.Current == nil || next.Current.Cycle != 2 || next.Task == nil || next.Task.ID == repairTask.ID || next.Task.RunID != "" || next.Current.RequirementID != closed.Current.RequirementID || len(next.History) != 1 || next.History[0].Closure == nil || next.History[0].Closure.RunID != repairRun {
		t.Fatal("new repair cycle reused an executed task or forked canonical intake", next)
	}
	if head := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD"); head != base {
		t.Fatal("actual repair changed the administrator checkout HEAD")
	}
	if source, e := os.ReadFile(filepath.Join(repo, "sum.cjs")); e != nil || string(source) != broken {
		t.Fatal("actual repair changed the administrator checkout content", e)
	}
	receipt := map[string]any{"kind": "actual_os_http_issue_repair", "project_id": f.project, "issue_id": issue.ID, "original_task_id": originalTask.ID, "original_run_id": originalRun, "repair_task_id": repairTask.ID, "repair_run_id": repairRun, "next_task_id": next.Task.ID, "cycle": next.Cycle, "base_commit": base, "candidate_commit": candidate, "producer_worker": producer.WorkerID, "test_worker": independent.WorkerID, "producer_workspace_sha256": produced.WorkspaceEvidence["evidence_sha256"], "test_workspace_sha256": verified.WorkspaceEvidence["evidence_sha256"], "report_sha256": verified.TestReport.SHA256, "plan_id": regression.ID, "plan_revision": regression.Published.Revision, "plan_sha256": regression.Published.SHA256, "case_key": caseDefinition.Key, "source_commits": execution.SourceCommits, "actual_node_processes": 3, "model_calls": 0, "customer_human_signoff": false, "administrator_checkout_unchanged": true}
	raw, _ := json.Marshal(receipt)
	t.Logf("issue_repair_runtime_receipt=%s", raw)
	issueRepairRuntimeHold(t, f, issue.ID, receipt)
}

func issueRepairRuntimeDecode[T any](t *testing.T, value any) (out T) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func issueRepairRuntimeTool(t *testing.T, f *fixture, worker string, cfg *runner.ExecConfig) (ToolProfile, string) {
	t.Helper()
	ctx := context.Background()
	inventory, err := runner.DescribeTools(ctx, *cfg)
	if err != nil || len(inventory) != 1 || !inventory[0].Available {
		t.Fatalf("actual command inventory unavailable: %+v %v", inventory, err)
	}
	credential := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": worker, "profiles": []any{}, "deployment": false}), 201)
	token := credential["token"].(string)
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: worker, Name: "Owned deterministic issue-repair command", Profile: inventory[0].Profile, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: inventory}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, Action: "enable", ExpectedVersion: profile.Version, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	cfg.ToolInventory = inventory
	return profile, token
}

func issueRepairRuntimePlan(t *testing.T, f *fixture, task Record, item QualityTestCase) TestPlanView {
	t.Helper()
	requirementVersion := 0
	if task.RequirementID != "" {
		requirement, err := f.s.Get(context.Background(), f.actor, f.project, "requirements", task.RequirementID)
		if err != nil {
			t.Fatal(err)
		}
		requirementVersion = requirement.Version
	}
	plan := issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans", TestPlanCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ExpectedRequirementVersion: requirementVersion, Title: "Exact arithmetic regression", Description: "Real independent JUnit retains the original case selector", Cases: []QualityTestCase{item}, IdempotencyKey: newID()}, 201))
	return issueRepairRuntimeDecode[TestPlanView](t, f.call("POST", "/test-plans/"+plan.ID+"/commands", TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Action: "publish", Reason: "Explicitly publish the fixed regression contract", IdempotencyKey: newID()}, 200))
}

// Optional read-only UI hold reuses the actual completed journey's database.
// It never installs a second dispatcher or creates a synthetic passing report.
// Public readiness/receipts contain no login details; the fixture-only account
// remains in the owned ignored cache, outside every served web directory.
func issueRepairRuntimeHold(t *testing.T, f *fixture, issue string, receipt map[string]any) {
	t.Helper()
	dir, web := os.Getenv("SUMA_ISSUE_REPAIR_UI_DIR"), os.Getenv("SUMA_ISSUE_REPAIR_WEB_DIR")
	if dir == "" && web == "" {
		return
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".cache", "e2e", "issue-repair-119"))
	if err != nil {
		t.Fatal(err)
	}
	dir, err = filepath.Abs(dir)
	if err != nil || !under(root, dir) || dir == root {
		t.Fatal("issue-repair GUI receipt directory must be a private child of the exact owned cache")
	}
	web, err = filepath.Abs(web)
	if err != nil || !under(root, web) || web == root || under(web, dir) || under(dir, web) {
		t.Fatal("issue-repair GUI assets and private receipts must be separate children of the owned cache")
	}
	if _, err = os.Stat(filepath.Join(web, "index.html")); err != nil {
		t.Fatal("final issue-repair GUI web entry must exist", err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{dir, web} {
		resolved, e := filepath.EvalSymlinks(path)
		if e != nil || !under(resolvedRoot, resolved) {
			t.Fatal("issue-repair GUI path resolves outside its exact owned cache", e)
		}
	}
	for _, name := range []string{"login.local.json", "login.ready", "stop.request", "result.public.json", "fixture.sqlite"} {
		if _, e := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(e) {
			t.Fatal("issue-repair GUI receipt directory must be fresh", name)
		}
	}
	server, origin, port := modelSelectionGUIServer(t, f, web, "issue119.localhost")
	t.Cleanup(server.Close)
	counts := func() map[string]int {
		out := modelSelectionGUICounts(t, f)
		for _, table := range []string{"suma_issues", "suma_tasks", "suma_requirements", "suma_issue_repairs", "suma_test_plan_reports"} {
			out[table] = expertCount(t, f, table)
		}
		return out
	}
	seed := counts()
	state := map[string]any{"status": "ready", "kind": "actual_os_http_issue_repair", "origin": origin, "receipt": receipt, "seed_counts": seed}
	t.Cleanup(func() {
		server.Close()
		final := counts()
		state["final_counts"] = final
		if !reflect.DeepEqual(seed, final) {
			state["status"] = "read_only_guard_failed"
			t.Error("read-only issue-repair browser inspection changed execution, review or repair facts")
		}
		// VACUUM copies only this test-owned fixture. Keep the database outside
		// the served asset directory for evidence after TempDir cleanup.
		database := filepath.Join(dir, "fixture.sqlite")
		if _, e := f.s.DB.Exec("VACUUM INTO '" + strings.ReplaceAll(database, "'", "''") + "'"); e != nil {
			t.Error("retain owned issue-repair database", e)
		} else if bytes, e := os.ReadFile(database); e != nil {
			t.Error("hash retained owned issue-repair database", e)
		} else {
			state["database_sha256"] = fmt.Sprintf("%x", sha256.Sum256(bytes))
			state["database_bytes"] = len(bytes)
		}
		modelSelectionGUIJSON(t, filepath.Join(dir, "result.public.json"), state)
	})
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.local.json"), map[string]any{"kind": "actual_os_http_issue_repair", "allowed_actions": []string{"read", "refresh"}, "origin": origin, "base_url": server.URL, "email": "owner@example.test", "password": "test-password-strong-42", "project_id": f.project, "user_id": f.user.ID, "issue_id": issue, "receipt": receipt})
	modelSelectionGUIJSON(t, filepath.Join(dir, "web.files.public.json"), modelSelectionGUIWebFiles(t, web))
	modelSelectionGUIJSON(t, filepath.Join(dir, "login.ready"), map[string]any{"status": "ready", "kind": "actual_os_http_issue_repair", "origin": origin, "port": port, "seed_counts": seed})
	t.Logf("actual issue-repair GUI ready: %s; read/refresh only; private fixture login stays in the owned cache", origin)
	// Keep the optional inspection window below go test's default timeout.
	// A live handle, rather than a readiness file, remains the liveness proof.
	timer := time.NewTimer(5 * time.Minute)
	defer timer.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	state["stop_reason"] = "timeout"
	waiting := true
	for waiting {
		select {
		case <-timer.C:
			waiting = false
		case <-ticker.C:
			if _, e := os.Stat(filepath.Join(dir, "stop.request")); e == nil {
				state["stop_reason"] = "explicit_stop"
				waiting = false
			} else if !os.IsNotExist(e) {
				t.Fatal(fmt.Errorf("read owned GUI stop signal: %w", e))
			}
		}
	}
	state["status"] = "closed_without_new_execution"
}
