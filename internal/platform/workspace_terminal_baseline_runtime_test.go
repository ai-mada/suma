package platform

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/runner"
	term "github.com/ai-mada/suma/internal/terminalcontract"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

// Continue the already accepted first candidate through real imported code,
// another stopped terminal, a new sealed candidate and independent quality.
func terminalBaselineActualContinuation(t *testing.T, ctx context.Context, f *fixture, task Record, developer, verifier runner.ExecConfig, profiles map[string]ToolProfile, client *runner.Client, origin, adopted, admin, evidenceDir string) map[string]any {
	t.Helper()
	if task.CodeBaseline == nil || task.CodeBaseline.Source.HeadCommit != adopted || task.CodeBaseline.Source.BundleBase() != admin {
		t.Fatal("follow-up fixture did not receive the exact accepted baseline")
	}
	node := developer.Commands["candidate-check130"].Executable
	const junit = `const fs=require('node:fs'),ok=require('./sum.cjs')(2,3)===5&&fs.readFileSync('followup.txt','utf8')==='baseline-followup-131\n';fs.writeFileSync(process.argv[1],'<testsuite tests="1" failures="'+(ok?'0':'1')+'"><testcase classname="baseline131" name="preserves-adopted-source">'+(ok?'':'<failure message="adopted source or follow-up marker differs"/>')+'</testcase></testsuite>');process.exitCode=ok?0:1;`
	verifier.Commands = map[string]runner.CommandSpec{"baseline-test131": {Adapter: "command", Protocol: "text-v1", Executable: node, Args: []string{"-e", junit, "{{test_report_path}}"}, TestEvidence: true, TestReport: true}}
	profile, token := issueRepairRuntimeTool(t, f, "baseline-independent-worker131", &verifier)
	independent := &runner.Client{BaseURL: origin, Token: token, ProjectID: f.project}
	verifier.Artifacts = independent
	quality := issueRepairRuntimePlan(t, f, task, QualityTestCase{Key: "baseline-source-and-edit", Title: "Preserve the adopted source and add the requested edit", Description: "The original administrator base is intentionally broken", Preconditions: "Import the accepted source as the terminal's baseline", Steps: []string{"Verify arithmetic from the accepted candidate", "Verify the follow-up marker"}, ExpectedResult: "Adopted arithmetic and requested marker both pass", Priority: "high", Covers: []int{1, 2}, Selector: tr.TestCaseSelector{ClassName: "baseline131", Name: "preserves-adopted-source"}})
	shell := profiles["terminal130"]
	terminalCandidateRefreshRuntimeTools(t, ctx, f, shell.WorkerID, client.Token, &developer)
	before := workspaceTerminalTaskSQL(t, f, task.ID)
	session, err := f.s.CreateWorkspaceTerminal(ctx, f.e, f.actor, f.project, WorkspaceTerminalCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, ToolProfileID: shell.ID, ExpectedToolVersion: shell.Version, Columns: 100, Rows: 30, TimeoutSeconds: 120, AcknowledgeShell: true, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if session.CodeBaseline == nil || *session.CodeBaseline != *task.CodeBaseline || session.BaseCommit != adopted || session.AdminBaseCommit != admin {
		t.Fatal("follow-up terminal lost selected source or bundle prerequisite", session)
	}
	a, err := client.Claim(ctx, shell.WorkerID, []string{"command:terminal130"})
	if err != nil || a == nil || a.CodeBaseline == nil || *a.CodeBaseline != *task.CodeBaseline {
		t.Fatal("follow-up terminal claim lost protected baseline", a, err)
	}
	done := make(chan runner.Result, 1)
	terminalCtx, cancelTerminal := context.WithCancel(ctx)
	go func() { defer close(done); done <- runner.Execute(terminalCtx, *a, developer) }()
	defer func() {
		cancelTerminal()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("follow-up terminal did not release its original process tree")
		}
	}()
	for deadline := time.Now().Add(15 * time.Second); ; {
		session, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if session.State == "running" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("follow-up terminal did not start", session)
		}
		time.Sleep(30 * time.Millisecond)
	}
	opened, err := os.ReadFile(filepath.Join(session.Directory, "sum.cjs"))
	if err != nil || string(opened) != "module.exports=(a,b)=>a+b;\n" {
		t.Fatalf("terminal silently opened administrator source instead of accepted candidate: %s %v", opened, err)
	}
	input := fmt.Sprintf("\"%s\" -e \"require('node:fs').writeFileSync('followup.txt','baseline-followup-131\\n')\"\r\nexit\r\n", node)
	inputID, key := newID(), newID()
	for retry := 0; ; retry++ {
		_, err = f.s.CommandWorkspaceTerminal(ctx, f.actor, f.project, session.ID, WorkspaceTerminalCommandRequest{ProjectID: f.project, ExpectedVersion: session.Version, ControlEpoch: session.ControlEpoch, Action: "input", InputID: inputID, InputSequence: session.NextInputSequence, DataBase64: base64.StdEncoding.EncodeToString([]byte(input)), IdempotencyKey: key})
		if err == nil {
			break
		}
		if retry >= 4 {
			t.Fatal(err)
		}
		session, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	var stopped runner.Result
	select {
	case stopped = <-done:
	case <-ctx.Done():
		t.Fatal("follow-up shell did not finish")
	}
	if stopped.ExitCode != 0 || stopped.Error != "" || stopped.OutcomeUnknown || stopped.WorkspaceEvidence != nil {
		t.Fatalf("follow-up shell became formal evidence or failed: %+v", stopped)
	}
	stopped.OperationID = newID()
	if err = client.Complete(ctx, stopped); err != nil {
		t.Fatal(err)
	}
	session, err = f.s.GetWorkspaceTerminal(ctx, f.actor, f.project, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session.SourceSnapshotSHA256 == "" || session.BaseCommit != adopted || session.AdminBaseCommit != admin || workspaceTerminalTaskSQL(t, f, task.ID).SHA256 != before.SHA256 {
		t.Fatal("terminal did not preserve task/run separation and selected source", session)
	}
	producer := profiles["candidate-check130"]
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Continue the exact adopted candidate", "nodes": []any{
		map[string]any{"id": "capture", "type": "command", "command": []string{"candidate-check130"}, "config": map[string]any{"tool_profile_id": producer.ID, "tool_profile_sha256": producer.Profile.ProfileSHA256}},
		map[string]any{"id": "independent", "type": "test", "command": []string{"baseline-test131"}, "depends_on": []string{"capture"}, "config": map[string]any{"candidate_from": "capture", "tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}},
	}}), 201)
	terminalCandidateRefreshRuntimeTools(t, ctx, f, producer.WorkerID, client.Token, &developer)
	terminalCandidateRefreshRuntimeTools(t, ctx, f, profile.WorkerID, token, &verifier)
	plan := issueRepairRuntimeDecode[WorkspaceTerminalCandidatePlan](t, f.call("POST", "/workspace/terminal-sessions/"+session.ID+"/candidate-plans", WorkspaceTerminalCandidatePreviewRequest{ProjectID: f.project, ExpectedSessionVersion: session.Version, ExpectedTaskVersion: task.Version, ToolProfileID: producer.ID, ExpectedToolVersion: producer.Version, WorkflowID: workflow["id"].(string), WorkflowVersion: int(workflow["version"].(float64)), ProducerNodeID: "capture", AcknowledgeStoppedChanges: true, IdempotencyKey: newID()}, 201))
	if plan.Source.BaseCommit != adopted || plan.Source.AdminBaseCommit != admin || plan.Source.CodeBaseline == nil || *plan.Source.CodeBaseline != *task.CodeBaseline {
		t.Fatal("candidate preview changed the adopted baseline", plan)
	}
	run := issueRepairRuntimeDecode[dag.Run](t, f.call("POST", "/workspace/terminal-candidate-plans/"+plan.ID+"/start", TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()}, 201))
	assignment, err := client.Claim(ctx, producer.WorkerID, []string{"command:candidate-check130"})
	if err != nil || assignment == nil || assignment.CodeBaseline == nil || *assignment.CodeBaseline != *task.CodeBaseline {
		t.Fatal("follow-up producer claim lost exact baseline", assignment, err)
	}
	produced := runner.Execute(ctx, *assignment, developer)
	if produced.ExitCode != 0 || produced.Error != "" {
		t.Fatalf("follow-up candidate failed: %+v", produced)
	}
	var proof term.TerminalCandidateEvidence
	if json.Unmarshal([]byte(stringify(produced.WorkspaceEvidence["terminal_candidate"])), &proof) != nil || proof.Source.BaseCommit != adopted || proof.Source.AdminBaseCommit != admin || proof.Seal.PreviousCommit != adopted || proof.Seal.Commit == adopted || produced.WorkspaceEvidence["base_commit"] != adopted || produced.WorkspaceEvidence["admin_base_commit"] != admin {
		t.Fatal("follow-up seal lost selected source or administrator anchor", proof, produced.WorkspaceEvidence)
	}
	produced.OperationID = newID()
	if err = client.Complete(ctx, produced); err != nil {
		t.Fatal(err)
	}
	test, err := independent.Claim(ctx, profile.WorkerID, []string{"command:baseline-test131"})
	if err != nil || test == nil || test.CandidateCommit != proof.Seal.Commit || test.WorkerID == assignment.WorkerID {
		t.Fatal("follow-up independent candidate missing", err)
	}
	checked := runner.Execute(ctx, *test, verifier)
	if checked.ExitCode != 0 || checked.Error != "" || checked.TestReport == nil || checked.WorkspaceEvidence["candidate_import"] == nil {
		t.Fatalf("follow-up candidate was not imported and verified independently: %+v", checked)
	}
	checked.OperationID = newID()
	if err = independent.Complete(ctx, checked); err != nil {
		t.Fatal(err)
	}
	execution := issueRepairRuntimeDecode[TestPlanExecution](t, f.call("GET", "/test-plans/"+quality.ID+"/executions/"+run.ID+"?project_id="+f.project, nil, 200))
	if execution.Passed != 1 {
		t.Fatal(execution)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/tasks/"+task.ID+"/review", f.body(map[string]any{"expected_version": current.Version, "run_id": run.ID, "decision": "accepted", "reason": "Fixture owner explicitly accepts the real continuation from the adopted candidate after independent verification"}), 200)
	var review string
	if err = f.s.DB.QueryRow(`SELECT id FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=? AND run_id=? AND decision='accepted'`, f.user.TenantID, f.project, task.ID, run.ID).Scan(&review); err != nil {
		t.Fatal(err)
	}
	if original, err := os.ReadFile(filepath.Join(developer.Repository, "sum.cjs")); err != nil || !strings.Contains(string(original), "a-b") {
		t.Fatal("parent administrator checkout was changed", err)
	}
	result := map[string]any{"task_id": task.ID, "adopted_source_commit": adopted, "admin_bundle_prerequisite": admin, "terminal_session_id": session.ID, "terminal_run_id": session.RunID, "opened_source_sha256": digest(string(opened)), "terminal_snapshot_sha256": session.SourceSnapshotSHA256, "candidate_plan_id": plan.ID, "candidate_run_id": run.ID, "candidate_commit": proof.Seal.Commit, "producer_worker": assignment.WorkerID, "independent_worker": test.WorkerID, "junit_sha256": checked.TestReport.SHA256, "formal_cases_passed": execution.Passed, "accepted_review_id": review, "source_imported_before_shell": true, "task_unchanged_by_terminal": true, "model_calls": 0, "customer_human_signoff": false}
	if evidenceDir != "" {
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "followup-development.public.json"), result)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "followup-terminal.public.json"), session)
		modelSelectionGUIJSON(t, filepath.Join(evidenceDir, "followup-quality.public.json"), execution)
	}
	return result
}
