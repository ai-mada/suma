package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestProjectRiskNestedExecutionCannotHideCodeOrUnsupportedSources(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	child, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Nested code source", Nodes: []dag.Node{{ID: "produce", Type: "command", Command: []string{"negative-code-fixture"}}}})
	if err != nil {
		t.Fatal(err)
	}
	parent, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Current protected child", Nodes: []dag.Node{{ID: "child", Type: "subworkflow", Config: map[string]any{"workflow_id": child.ID, "workflow_version": child.Version}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, parent.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := f.e.Claim(ctx, f.user.TenantID, f.project, "negative-child-worker", []string{"command:negative-code-fixture"})
	if err != nil || assignment == nil || assignment.RunID == run.ID {
		t.Fatal("actual child assignment is missing", assignment, err)
	}
	// This is deliberately an insufficient protocol result: a code-bearing
	// child without an independent candidate test must never prove mitigation.
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), WorkspaceEvidence: map[string]any{"head_commit": strings.Repeat("a", 40)}}); err != nil {
		t.Fatal(err)
	}
	check := func(runID string) (bool, string) {
		t.Helper()
		current, auth, close, e := f.s.planningAuthority(ctx, f.actor, f.project, "", purposeProjectRead)
		if e != nil {
			t.Fatal(e)
		}
		defer close()
		ok, code, e := f.s.currentCodeCandidateEvidenceTx(current, auth, f.project, runID)
		if e != nil {
			t.Fatal(e)
		}
		return ok, code
	}
	actual, err := f.e.GetRun(ctx, f.user.TenantID, f.project, run.ID)
	if err != nil || actual.State != "succeeded" {
		t.Fatal("parent did not finish through its actual protected child", actual, err)
	}
	if ok, code := check(run.ID); ok || code != "code_evidence_stale" {
		t.Fatal("nested code was mistaken for non-code action", ok, code)
	}
	leaf, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Non-code leaf", Nodes: []dag.Node{{ID: "noop", Type: "noop"}}})
	if err != nil {
		t.Fatal(err)
	}
	wrapper, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Non-code wrapper", Nodes: []dag.Node{{ID: "child", Type: "subworkflow", Config: map[string]any{"workflow_id": leaf.ID, "workflow_version": leaf.Version}}}})
	if err != nil {
		t.Fatal(err)
	}
	noncode, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, wrapper.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	if ok, code := check(noncode.ID); !ok || code != "" {
		t.Fatal("verified non-code child was categorically rejected", ok, code)
	}
	mapped, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Unsupported expanded proof", Nodes: []dag.Node{{ID: "map", Type: "map", Config: map[string]any{"items": []any{}, "template": map[string]any{"type": "noop"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	emptyMap, err := f.e.CreateRun(ctx, f.user.TenantID, f.project, mapped.ID, newID())
	if err != nil {
		t.Fatal(err)
	}
	if ok, code := check(emptyMap.ID); ok || code != "code_source_unsupported" {
		t.Fatal("unsupported source did not expose its exact boundary", ok, code)
	}
}

func projectRiskOwnerFixture(t *testing.T, f *fixture) Record {
	t.Helper()
	owner, err := f.s.Create(context.Background(), f.actor, f.project, "members", f.body(map[string]any{"title": "Current risk owner", "kind": "human", "status": "active", "data": map[string]any{"user_id": f.user.ID}}))
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func projectRiskCreateFixture(t *testing.T, f *fixture, owner string, links []ProjectRiskReference) (ProjectRiskView, ProjectRiskCreateRequest) {
	t.Helper()
	request := ProjectRiskCreateRequest{ProjectID: f.project, ProjectRiskFields: ProjectRiskFields{Title: "Release dependency exposure", Detail: "Track the actual response and explicit human decision", OwnerID: owner, ReviewDate: "2026-01-01", Strategy: "mitigate", Plan: "Coordinate the linked response and review current evidence", Links: links}, Reason: "Explicit risk registration", IdempotencyKey: newID()}
	out, err := f.s.CreateProjectRisk(context.Background(), f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	return out, request
}

func projectRiskCommandFixture(t *testing.T, f *fixture, v ProjectRiskView, action string, review *ProjectRiskReviewInput) ProjectRiskView {
	t.Helper()
	r := ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: v.Risk.Version, Action: action, Review: review, Reason: "Explicit risk lifecycle decision", IdempotencyKey: newID()}
	if v.CurrentReview != nil {
		r.ExpectedReviewID = v.CurrentReview.ID
	}
	out, err := f.s.CommandProjectRisk(context.Background(), f.actor, f.project, v.Risk.ID, r)
	if err != nil {
		t.Fatal(action, err)
	}
	return out
}

func TestProjectRiskCurrentAssessmentCountsCASAndOriginalReceipt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	v, create := projectRiskCreateFixture(t, f, "", []ProjectRiskReference{})
	if v.Created == nil || !*v.Created || v.Risk.Score != nil || v.Readiness.AssessmentReady {
		t.Fatal("draft manufactured assessment", v)
	}
	owner := projectRiskOwnerFixture(t, f)
	p, i := 3, 4
	fields := v.Risk.ProjectRiskFields
	fields.OwnerID = owner.ID
	fields.Probability = &p
	fields.Impact = &i
	save := ProjectRiskSaveRequest{ProjectID: f.project, ExpectedVersion: v.Risk.Version, ProjectRiskFields: fields, Reason: "Assess ordinal exposure", IdempotencyKey: newID()}
	current, err := f.s.SaveProjectRisk(ctx, f.actor, f.project, v.Risk.ID, save)
	if err != nil || current.Risk.Score == nil || *current.Risk.Score != 12 || current.Risk.AssessmentRevision != 2 || !current.Readiness.AssessmentReady {
		t.Fatal(current, err)
	}
	replay, err := f.s.CreateProjectRisk(ctx, f.actor, f.project, create)
	if err != nil || replay.Risk.Version != 1 || replay.Risk.Score != nil {
		t.Fatal("original command was reevaluated", replay, err)
	}
	save.IdempotencyKey = newID()
	_, err = f.s.SaveProjectRisk(ctx, f.actor, f.project, v.Risk.ID, save)
	requireAuthorityStatus(t, err, 409)
	page, err := f.s.ListProjectRisks(ctx, f.actor, f.project, "open", "", 1, "")
	if err != nil || page.Total != 1 || len(page.Items) != 1 || !page.CanCreate {
		t.Fatal(page, err)
	}
	summary, err := f.s.GetProjectRiskSummary(ctx, f.actor, f.project)
	if err != nil || summary.Total != 1 || summary.ActiveCount != 1 || summary.OpenCount != 1 || summary.UnassessedCount != 0 || summary.OverdueCount != 1 || summary.MatrixScope != "active" || len(summary.Matrix) != 25 {
		t.Fatal(summary, err)
	}
	count := 0
	for _, cell := range summary.Matrix {
		count += cell.Count
		if cell.Probability == 3 && cell.Impact == 4 && cell.Count != 1 {
			t.Fatal(cell)
		}
	}
	if count != 1 {
		t.Fatal("matrix did not retain full-project counts", summary)
	}
	_, err = f.s.Create(ctx, f.actor, f.project, "risks", f.body(map[string]any{"title": "Bypass", "status": "closed"}))
	requireAuthorityStatus(t, err, 404)
}

func TestProjectRiskHumanExposureDecisionReturnCloseAndReopen(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := projectRiskOwnerFixture(t, f)
	v, _ := projectRiskCreateFixture(t, f, owner.ID, []ProjectRiskReference{})
	v = projectRiskCommandFixture(t, f, v, "start", nil)
	input := &ProjectRiskReviewInput{Disposition: "accepted", Rationale: "Reviewer explicitly accepts remaining exposure", Evidence: "Decision record and accountable owner; no code test is claimed"}
	v = projectRiskCommandFixture(t, f, v, "submit_review", input)
	if v.CurrentReview == nil || !v.Readiness.CloseReady || v.Readiness.MitigationReady || v.CurrentReview.ResidualScore != nil {
		t.Fatal("human exposure decision fabricated code mitigation", v)
	}
	returnedID := v.CurrentReview.ID
	v = projectRiskCommandFixture(t, f, v, "return", nil)
	if v.Risk.State != "mitigating" || v.CurrentReview != nil || v.HistoryTotal != 1 || v.History[0].Decision != "returned" {
		t.Fatal(v)
	}
	v = projectRiskCommandFixture(t, f, v, "submit_review", input)
	if v.CurrentReview.ID == returnedID {
		t.Fatal("returned review reused")
	}
	closedID := v.CurrentReview.ID
	closeRequest := ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: v.Risk.Version, ExpectedReviewID: closedID, Action: "close", Reason: "Formal human risk acceptance", IdempotencyKey: newID()}
	closed, err := f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, closeRequest)
	if err != nil || closed.Risk.State != "closed" || closed.CurrentReview.Decision != "closed" {
		t.Fatal(closed, err)
	}
	v = projectRiskCommandFixture(t, f, closed, "reopen", nil)
	if v.Risk.State != "open" || v.Risk.Cycle != 2 || v.CurrentReview != nil || v.HistoryTotal != 2 || !v.HistoryComplete {
		t.Fatal("reopen discarded decisions", v)
	}
	replay, err := f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, closeRequest)
	if err != nil || replay.Risk.State != "closed" || replay.Risk.Cycle != 1 {
		t.Fatal("receipt did not preserve original closure", replay, err)
	}
	latest, err := f.s.GetProjectRisk(ctx, f.actor, f.project, v.Risk.ID)
	if err != nil || latest.Risk.State != "open" || latest.Risk.Cycle != 2 {
		t.Fatal("receipt mutated latest cycle", latest, err)
	}
}

func TestProjectRiskMitigationAndReviewBasisCannotUseEditableStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := projectRiskOwnerFixture(t, f)
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Response action"}))
	if err != nil {
		t.Fatal(err)
	}
	v, _ := projectRiskCreateFixture(t, f, owner.ID, []ProjectRiskReference{{Kind: "task", ID: task.ID}})
	v = projectRiskCommandFixture(t, f, v, "start", nil)
	request := ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: v.Risk.Version, Action: "submit_review", Review: &ProjectRiskReviewInput{Disposition: "mitigated", Rationale: "Not complete yet", Evidence: "The task has no formal accepted run"}, Reason: "Cannot substitute task status", IdempotencyKey: newID()}
	_, err = f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, request)
	requireAuthorityStatus(t, err, 409)
	// A human exposure decision may cite an unfinished response, but its exact
	// cited version is frozen and cannot drift underneath a later close.
	request.Review.Disposition = "accepted"
	request.IdempotencyKey = newID()
	v, err = f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "tasks", task.ID, f.body(map[string]any{"expected_version": task.Version, "title": "Changed response scope"}))
	if err != nil {
		t.Fatal(err)
	}
	current, err := f.s.GetProjectRisk(ctx, f.actor, f.project, v.Risk.ID)
	if err != nil || current.Readiness.CloseReady {
		t.Fatal("changed link remained closeable", current, err)
	}
	_, err = f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: v.Risk.Version, ExpectedReviewID: v.CurrentReview.ID, Action: "close", Reason: "Old linked version is not current", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
	// Return stays available even when the review basis is stale.
	v = projectRiskCommandFixture(t, f, current, "return", nil)
	if v.Risk.State != "mitigating" {
		t.Fatal(v)
	}
}

func TestProjectRiskCurrentHumanAuthorityAndReadIsolation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	owner := projectRiskOwnerFixture(t, f)
	v, create := projectRiskCreateFixture(t, f, owner.ID, []ProjectRiskReference{})
	credential, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	read, err := f.s.GetProjectRisk(ctx, api, f.project, v.Risk.ID)
	if err != nil || len(read.AllowedActions) != 0 {
		t.Fatal(read, err)
	}
	_, err = f.s.CreateProjectRisk(ctx, api, f.project, create)
	requireAuthorityStatus(t, err, 403)
	other, err := f.s.CreateProject(ctx, f.actor, f.body(map[string]any{"title": "Other risk scope"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetProjectRisk(ctx, f.actor, other.ID, v.Risk.ID)
	requireAuthorityStatus(t, err, 404)
	v = projectRiskCommandFixture(t, f, v, "start", nil)
	v = projectRiskCommandFixture(t, f, v, "submit_review", &ProjectRiskReviewInput{Disposition: "accepted", Rationale: "Human decision", Evidence: "Accountable explicit review"})
	closeRequest := ProjectRiskCommandRequest{ProjectID: f.project, ExpectedVersion: v.Risk.Version, ExpectedReviewID: v.CurrentReview.ID, Action: "close", Reason: "Original reviewer closes", IdempotencyKey: newID()}
	_, err = f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, closeRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer') ON CONFLICT(project_id,user_id)DO UPDATE SET role='developer'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandProjectRisk(ctx, f.actor, f.project, v.Risk.ID, closeRequest)
	requireAuthorityStatus(t, err, 403)
}
