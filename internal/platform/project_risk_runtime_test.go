package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/runner"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

// Only generated source fixtures are used. These are deterministic local
// commands, not AI execution or evidence of a customer's human sign-off.
type actualFixedCandidateEvidence struct {
	Repository             string `json:"-"`
	BaseCommit             string `json:"base_commit"`
	CandidateCommit        string `json:"candidate_commit"`
	RunID                  string `json:"run_id"`
	SourceNodeID           string `json:"source_node_id"`
	TaskID                 string `json:"task_id"`
	PlanID                 string `json:"plan_id"`
	PlanRevision           int    `json:"plan_revision"`
	PlanSHA256             string `json:"plan_sha256"`
	ProducerWorker         string `json:"producer_worker"`
	TestWorker             string `json:"test_worker"`
	ProducerEvidenceSHA256 string `json:"producer_evidence_sha256"`
	TestEvidenceSHA256     string `json:"test_evidence_sha256"`
	ReportSHA256           string `json:"report_sha256"`
}

// The planning proposal uses the existing owned protocol fixture, not a model
// call. Its HTTP callback creates the real protected PlanningAlert. Mitigation
// then uses two real OS processes, exact-candidate JUnit and formal Review.
func TestProjectRiskActualAlertAdoptionMitigationReviewAndReopen(t *testing.T) {
	h := newIntelligentFixture(t)
	f := h.f
	owner := issueRepairRuntimeDecode[Record](t, f.call("POST", "/members", f.body(map[string]any{"title": "Risk accountable owner", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}}), 201))
	analysis := h.create(t)
	h.complete(t, h.result(h.claim(t), h.output()))
	analysis = h.get(t, analysis.ID)
	alerts := issueRepairRuntimeDecode[PlanningAlertPage](t, f.call("GET", "/planning/alerts?project_id="+f.project+"&state=open&limit=20", nil, 200))
	if analysis.State != "ready" || alerts.Total != 1 || len(alerts.Items) != 1 || alerts.Items[0].AnalysisID != analysis.ID || alerts.Items[0].RunID != analysis.RunID {
		t.Fatal("real planning outcome did not produce its protected alert", analysis, alerts)
	}
	alert := alerts.Items[0]
	created := issueRepairRuntimeDecode[ProjectRiskView](t, f.call("POST", "/risks", ProjectRiskCreateRequest{ProjectID: f.project, ProjectRiskFields: ProjectRiskFields{Title: "Mitigate the original arithmetic delivery risk", Detail: "Owned protocol planning alert; code mitigation and independent evidence are still required", Links: []ProjectRiskReference{}}, SourceAlertID: alert.ID, ExpectedAlertVersion: alert.Version, Reason: "Explicit fixture owner adopts the actual alert", IdempotencyKey: newID()}, 201))
	if created.Created == nil || !*created.Created || created.Risk.Source == nil || created.Risk.Source.AlertID != alert.ID || created.Risk.Source.AnalysisID != analysis.ID || created.Risk.State != "open" || created.Risk.Probability != nil || created.Risk.Impact != nil {
		t.Fatal("explicit alert adoption lost its source or invented an assessment", created)
	}
	read := func() ProjectRiskView {
		return issueRepairRuntimeDecode[ProjectRiskView](t, f.call("GET", "/risks/"+created.Risk.ID+"?project_id="+f.project, nil, 200))
	}
	save := func(current ProjectRiskView, fields ProjectRiskFields) ProjectRiskView {
		return issueRepairRuntimeDecode[ProjectRiskView](t, f.call("PATCH", "/risks/"+created.Risk.ID, ProjectRiskSaveRequest{ProjectID: f.project, ExpectedVersion: current.Risk.Version, ProjectRiskFields: fields, Reason: "Explicitly assess owner, response and linked mitigation scope", IdempotencyKey: newID()}, 200))
	}
	command := func(current ProjectRiskView, action string, review *ProjectRiskReviewInput) ProjectRiskView {
		request := ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: current.Risk.Version, Action: action, Review: review, Reason: "Owned protocol fixture owner explicitly decides " + action, IdempotencyKey: newID()}
		if action == "close" {
			request.ExpectedReviewID = current.Risk.CurrentReviewID
		}
		return issueRepairRuntimeDecode[ProjectRiskView](t, f.call("POST", "/risks/"+created.Risk.ID+"/commands", request, 200))
	}
	probability, impact := 3, 4
	fields := created.Risk.ProjectRiskFields
	fields.OwnerID, fields.ReviewDate, fields.Strategy, fields.Plan = owner.ID, "2026-11-12", "mitigate", "Commit the narrow arithmetic fix, independently verify its exact candidate and review the delivery evidence."
	fields.Probability, fields.Impact = &probability, &impact
	assessed := save(created, fields)
	if assessed.Risk.Score == nil || *assessed.Risk.Score != 12 || !assessed.Readiness.AssessmentReady {
		t.Fatal("risk assessment did not retain its explicit current owner and ratings", assessed)
	}
	task := issueRepairRuntimeDecode[Record](t, f.call("POST", "/tasks", f.body(map[string]any{"title": "Mitigate the adopted arithmetic project risk", "description": "Produce a committed candidate; use a different Worker for exact regression and obtain formal Review", "acceptance_criteria": []string{"sum(2,3) returns 5"}}), 201))
	fields = assessed.Risk.ProjectRiskFields
	fields.Links = []ProjectRiskReference{{Kind: "task", ID: task.ID}}
	linked := save(assessed, fields)
	started := command(linked, "start", nil)
	if started.Risk.State != "mitigating" || started.Readiness.MitigationReady {
		t.Fatal("risk started without retaining the unverified mitigation gate", started)
	}
	evidence := actualFixedCandidateDelivery(t, f, task, nil, false)
	current := read()
	if !current.Readiness.MitigationReady {
		t.Fatal("current formal acceptance and independent candidate evidence were not recognized", current)
	}
	residualProbability, residualImpact := 1, 2
	reviewed := command(current, "submit_review", &ProjectRiskReviewInput{Disposition: "mitigated", ResidualProbability: &residualProbability, ResidualImpact: &residualImpact, Rationale: "The narrow arithmetic mitigation passed independent current-candidate regression and explicit owner task Review.", Evidence: fmt.Sprintf("Run %s; exact candidate %s; independent report %s; fixed plan %s revision %d (%s).", evidence.RunID, evidence.CandidateCommit, evidence.ReportSHA256, evidence.PlanID, evidence.PlanRevision, evidence.PlanSHA256)})
	if reviewed.Risk.State != "in_review" || reviewed.CurrentReview == nil || reviewed.Risk.CurrentReviewID != reviewed.CurrentReview.ID || !reviewed.Readiness.CloseReady || len(reviewed.CurrentReview.Links) != 1 || reviewed.CurrentReview.Links[0].VerifiedRunID != evidence.RunID || reviewed.CurrentReview.Links[0].ReviewID == "" || reviewed.CurrentReview.OwnerUserID != f.user.ID {
		t.Fatal("risk review did not freeze current owner and formal delivery evidence", reviewed)
	}
	reviewID := reviewed.CurrentReview.ID
	closed := command(reviewed, "close", nil)
	if closed.Risk.State != "closed" || closed.CurrentReview == nil || closed.CurrentReview.ID != reviewID {
		t.Fatal("explicit close lost the exact current review", closed)
	}
	reopened := command(closed, "reopen", nil)
	if reopened.Risk.State != "open" || reopened.Risk.Cycle != 2 || reopened.CurrentReview != nil || reopened.HistoryTotal != 1 || !reopened.HistoryComplete || len(reopened.History) != 1 || reopened.History[0].ID != reviewID || reopened.History[0].Links[0].VerifiedRunID != evidence.RunID || reopened.Risk.Source == nil || reopened.Risk.Source.AlertID != alert.ID {
		t.Fatal("reopen rewrote prior mitigation history or its alert source", reopened)
	}
	summary := issueRepairRuntimeDecode[ProjectRiskSummary](t, f.call("GET", "/risks/summary?project_id="+f.project, nil, 200))
	if summary.Total != 1 || summary.ActiveCount != 1 || summary.OpenCount != 1 || summary.ClosedCount != 0 {
		t.Fatal("risk summary does not reflect the persisted reopened lifecycle", summary)
	}
	receipt := map[string]any{"kind": "actual_os_http_project_risk", "risk_id": created.Risk.ID, "alert_id": alert.ID, "analysis_id": analysis.ID, "planning_run_id": analysis.RunID, "planning_source": "owned_protocol_output", "model_calls": 0, "owner_resource_id": owner.ID, "review_id": reviewID, "cycle": reopened.Risk.Cycle, "state": reopened.Risk.State, "actual_node_processes": 2, "customer_human_signoff": false, "mitigation": evidence, "history_total": reopened.HistoryTotal, "history_complete": reopened.HistoryComplete}
	raw, _ := json.Marshal(receipt)
	t.Logf("project_risk_runtime_receipt=%s", raw)
	projectRiskRuntimeHold(t, f, created.Risk.ID, evidence, receipt)
}

// Reuse the original T119 profile/inventory, publication and decoding helpers;
// keep only the new risk journey's two-process candidate verification here.
func actualFixedCandidateDelivery(t *testing.T, f *fixture, task Record, extraFiles map[string]string, exportCandidate bool) actualFixedCandidateEvidence {
	t.Helper()
	ctx := context.Background()
	f.e.AttemptAccounting = f.s
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("Node is required for actual risk mitigation", err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal("Git is required for actual risk mitigation", err)
	}
	evidence := actualFixedCandidateEvidence{Repository: t.TempDir(), TaskID: task.ID, SourceNodeID: "mitigate"}
	repo := evidence.Repository
	workspaceGitTestCommand(t, repo, "init", "--initial-branch=main")
	workspaceGitTestWrite(t, repo, ".gitignore", ".cache/\n")
	const original = "module.exports=(a,b)=>a-b;\n"
	workspaceGitTestWrite(t, repo, "sum.cjs", original)
	paths := []string{}
	for path := range extraFiles {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if path == "sum.cjs" || path == ".gitignore" || filepath.IsAbs(path) || !under(repo, filepath.Join(repo, filepath.FromSlash(path))) {
			t.Fatal("extra fixture file is outside its owned source scope")
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, filepath.FromSlash(path))), 0700); err != nil {
			t.Fatal(err)
		}
		workspaceGitTestWrite(t, repo, path, extraFiles[path])
	}
	workspaceGitTestCommand(t, repo, "add", "--", ".")
	workspaceGitTestCommand(t, repo, "commit", "-m", "fixture: project risk original source")
	evidence.BaseCommit = workspaceGitTestCommand(t, repo, "rev-parse", "HEAD")
	const fix = `const fs=require('node:fs'),cp=require('node:child_process'),path=require('node:path');const root=path.resolve('.cache','project-risk121-git'),hooks=path.join(root,'empty-hooks'),config=path.join(root,'empty.gitconfig');fs.mkdirSync(hooks,{recursive:true,mode:0o700});fs.writeFileSync(config,'',{flag:'wx',mode:0o600});fs.writeFileSync('sum.cjs','module.exports=(a,b)=>a+b;\n');const git=process.argv[1],flags=['-c','core.hooksPath='+hooks,'-c','user.name=Risk fixture','-c','user.email=risk@example.invalid','-c','commit.gpgsign=false','-c','core.autocrlf=false'];for(const args of [['add','--','sum.cjs'],['commit','-m','fixture: mitigate the exact project risk']])cp.execFileSync(git,[...flags,...args],{env:{...process.env,GIT_CONFIG_NOSYSTEM:'1',GIT_CONFIG_GLOBAL:config,GIT_TERMINAL_PROMPT:'0'},stdio:'pipe'});console.log('Owned mitigation candidate committed.');`
	const verify = `const fs=require('node:fs');const ok=require('./sum.cjs')(2,3)===5;fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="risk121" name="sum">'+(ok?'':'<failure message="sum(2,3) must equal 5"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	developer := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: evidence.BaseCommit, Commands: map[string]runner.CommandSpec{"mitigate121": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", fix, git}}}}
	verifier := runner.ExecConfig{AllowExec: true, Workspace: repo, Repository: repo, BaseCommit: evidence.BaseCommit, Commands: map[string]runner.CommandSpec{"verify121": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", verify, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}}
	producerProfile, producerToken := issueRepairRuntimeTool(t, f, "risk-developer-121", &developer)
	testProfile, testToken := issueRepairRuntimeTool(t, f, "risk-independent-121", &verifier)
	plan := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "risk-mitigation", Title: "Mitigation returns the correct sum", Description: "Verify the exact committed mitigation candidate", Preconditions: "Use the scheduler-pinned candidate", Steps: []string{"Evaluate sum(2,3)", "Capture the exact independent JUnit selector"}, ExpectedResult: "The result equals 5", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "risk121", Name: "sum"}})
	evidence.PlanID, evidence.PlanRevision, evidence.PlanSHA256 = plan.ID, plan.Published.Revision, plan.Published.SHA256
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Project risk fixed mitigation and independent regression", "nodes": []any{map[string]any{"id": "mitigate", "type": "command", "command": []string{"mitigate121"}, "config": map[string]any{"tool_profile_id": producerProfile.ID, "tool_profile_sha256": producerProfile.Profile.ProfileSHA256}}, map[string]any{"id": "verify", "type": "test", "command": []string{"verify121"}, "depends_on": []string{"mitigate"}, "config": map[string]any{"candidate_from": "mitigate", "tool_profile_id": testProfile.ID, "tool_profile_sha256": testProfile.Profile.ProfileSHA256}}}}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "workflow_version": workflow["version"], "task_id": task.ID, "expected_version": task.Version}), 201)
	evidence.RunID = run["id"].(string)
	control := httptest.NewServer(f.handler)
	defer control.Close()
	producer := &runner.Client{BaseURL: control.URL, Token: producerToken, ProjectID: f.project}
	tester := &runner.Client{BaseURL: control.URL, Token: testToken, ProjectID: f.project}
	if exportCandidate {
		developer.Artifacts = producer
		verifier.Artifacts = tester
	}
	a, err := producer.Claim(ctx, producerProfile.WorkerID, []string{"command:mitigate121"})
	if err != nil || a == nil || a.RunID != evidence.RunID {
		t.Fatal("actual mitigation claim missing", err)
	}
	produced := runner.Execute(ctx, *a, developer)
	if produced.OperationID == "" {
		produced.OperationID = newID()
	}
	if produced.ExitCode != 0 || produced.Error != "" || produced.OutcomeUnknown || produced.ToolEvidence == nil || produced.ToolEvidence.Profile.ProfileSHA256 != producerProfile.Profile.ProfileSHA256 || produced.WorkspaceEvidence["clean"] != true {
		t.Fatalf("actual mitigation failed: exit=%d error=%s output=%q", produced.ExitCode, produced.Error, produced.Output[:min(len(produced.Output), 4000)])
	}
	evidence.CandidateCommit, _ = produced.WorkspaceEvidence["head_commit"].(string)
	if exportCandidate && produced.WorkspaceEvidence["candidate_bundle"] == nil {
		t.Fatal("the actual candidate bundle was not sealed through the control plane")
	}
	if evidence.CandidateCommit == "" || evidence.CandidateCommit == evidence.BaseCommit {
		t.Fatal("mitigation did not create a new committed candidate")
	}
	if err = producer.Complete(ctx, produced); err != nil {
		t.Fatal("actual mitigation callback rejected", err)
	}
	b, err := tester.Claim(ctx, testProfile.WorkerID, []string{"command:verify121"})
	if err != nil || b == nil || b.RunID != evidence.RunID || b.CandidateCommit != evidence.CandidateCommit || b.CandidateFrom != "mitigate" || b.WorkerID == a.WorkerID {
		t.Fatal("independent test did not claim the exact candidate", err)
	}
	tested := runner.Execute(ctx, *b, verifier)
	tested.OperationID = newID()
	if tested.ExitCode != 0 || tested.Error != "" || tested.OutcomeUnknown || tested.TestReport == nil || tested.WorkspaceEvidence["base_commit"] != evidence.CandidateCommit || tested.WorkspaceEvidence["head_commit"] != evidence.CandidateCommit || tested.WorkspaceEvidence["clean"] != true {
		t.Fatalf("actual independent mitigation test failed: exit=%d error=%s", tested.ExitCode, tested.Error)
	}
	if err = tester.Complete(ctx, tested); err != nil {
		t.Fatal("actual independent JUnit callback rejected", err)
	}
	quality := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+plan.ID+"/executions/"+evidence.RunID+"?project_id="+f.project, nil, 200))
	if !quality.BasisCurrent || quality.State != "passed" || quality.Passed != 1 || len(quality.SourceCommits) != 1 || quality.SourceCommits[0] != evidence.CandidateCommit || len(quality.Cases) != 1 || len(quality.Cases[0].Evidence) != 1 || quality.Cases[0].Evidence[0].WorkerID != b.WorkerID {
		t.Fatal("mitigation quality differs from the actual independent candidate", quality)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": current.Version, "run_id": evidence.RunID, "decision": "accepted", "reason": "Owned protocol fixture owner explicitly accepts the real independently tested mitigation; no customer human sign-off is claimed"}), 200)
	if head := workspaceGitTestCommand(t, repo, "rev-parse", "HEAD"); head != evidence.BaseCommit {
		t.Fatal("mitigation changed the administrator checkout")
	}
	if raw, err := os.ReadFile(filepath.Join(repo, "sum.cjs")); err != nil || string(raw) != original {
		t.Fatal("mitigation changed administrator source", err)
	}
	evidence.ProducerWorker, evidence.TestWorker = a.WorkerID, b.WorkerID
	evidence.ProducerEvidenceSHA256, _ = produced.WorkspaceEvidence["evidence_sha256"].(string)
	evidence.TestEvidenceSHA256, _ = tested.WorkspaceEvidence["evidence_sha256"].(string)
	evidence.ReportSHA256 = tested.TestReport.SHA256
	// Receipt is safe for test logs and the public UI evidence manifest.
	raw, _ := json.Marshal(evidence)
	t.Logf("project_risk_actual_mitigation=%s", strings.TrimSpace(string(raw)))
	return evidence
}
