package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func TestIssueRepairPackageScopeKeepsOriginalRepositoryAuthority(t *testing.T) {
	scope := ApplicationCodeScope{ID: "library", TargetID: "repository", ProviderID: "provider", TargetRef: "refs/heads/main", ConfigSHA256: digest("config"), RelativeRoot: "packages/library"}
	origin := issueRepairPackageOrigin{IssueRepairPackageOrigin: IssueRepairPackageOrigin{ApplicationID: "app", SourceCommit: strings.Repeat("a", 40)}, Scope: &scope}
	build := ArtifactProof{ApplicationBuild: &rc.ApplicationBuildBasis{ApplicationID: "app"}, SourceCommit: strings.Repeat("b", 40)}
	if !issueRepairPackageScopeMatches(&origin, build, &scope) {
		t.Fatal("a new commit within the same registered scope must remain eligible")
	}
	for _, change := range []struct {
		name  string
		apply func(*ApplicationCodeScope)
	}{
		{"scope", func(s *ApplicationCodeScope) { s.ID = "other-scope" }},
		{"root", func(s *ApplicationCodeScope) { s.RelativeRoot = "packages/other" }},
		{"target", func(s *ApplicationCodeScope) { s.TargetID = "other-repository" }},
		{"provider", func(s *ApplicationCodeScope) { s.ProviderID = "other-provider" }},
		{"ref", func(s *ApplicationCodeScope) { s.TargetRef = "refs/heads/other" }},
		{"config", func(s *ApplicationCodeScope) { s.ConfigSHA256 = digest("other-config") }},
	} {
		t.Run(change.name, func(t *testing.T) {
			changed := scope
			change.apply(&changed)
			if issueRepairPackageScopeMatches(&origin, build, &changed) {
				t.Fatal("a different scope cannot close the original issue")
			}
		})
	}
	build.ApplicationBuild = &rc.ApplicationBuildBasis{ApplicationID: "other-app"}
	if issueRepairPackageScopeMatches(&origin, build, &scope) {
		t.Fatal("a different application cannot close the original issue")
	}
}

func issueRepairPrepareFixture(t *testing.T, f *fixture, issue Record) (IssueRepairView, IssueRepairPrepareRequest) {
	t.Helper()
	r := IssueRepairPrepareRequest{ProjectID: f.project, ExpectedIssueVersion: issue.Version, Title: "Repair current project authority", Description: "Preserve the failure and independently check the repair", AcceptanceCriteria: []string{"Current project authority is enforced"}, Reason: "Explicit repair preparation", IdempotencyKey: newID()}
	v, err := f.s.PrepareIssueRepair(context.Background(), f.actor, f.project, issue.ID, r)
	if err != nil || v.Current == nil || v.Task == nil || v.Issue.Status != "in_progress" {
		t.Fatal("prepare did not establish one ordinary task and protected cycle", v, err)
	}
	return v, r
}

func issueRepairPlanFixture(t *testing.T, h *qualityPlanFixture, v IssueRepairView, cases []QualityTestCase) TestPlanView {
	t.Helper()
	f := h.f
	h.task = *v.Task
	requirement, err := f.s.Get(context.Background(), f.actor, f.project, "requirements", h.task.RequirementID)
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.s.CreateTestPlan(context.Background(), f.actor, f.project, TestPlanCreateRequest{ProjectID: f.project, TaskID: h.task.ID, ExpectedTaskVersion: h.task.Version, ExpectedRequirementVersion: requirement.Version, Title: "Independent repair regression", Cases: cases, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandTestPlan(context.Background(), f.actor, f.project, p.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "publish", Reason: "Review exact regression identity", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.plan = p
	return p
}

// The existing report-capable Runner protocol fixture supplies actual immutable
// report rows; this is not evidence of vendor execution or physical multi-hosts.
func issueRepairFailedCaseFixture(t *testing.T) (*qualityPlanFixture, Record) {
	t.Helper()
	h := newQualityPlanFixture(t, true)
	f := h.f
	a := h.run(t)
	r := h.result(a, `<testsuite tests="1" failures="1"><testcase classname="delivery" name="contract"><failure message="Missing project authority"/></testcase></testsuite>`)
	r.ExitCode = 1
	r.WorkspaceEvidence["exit_code"] = 1
	r.ToolEvidence.Terminal = "failed"
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": r}), 200)
	issue, err := f.s.CreateTestPlanDefect(context.Background(), f.actor, f.project, h.plan.ID, a.RunID, TestPlanDefectRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, CaseKey: "contract", Reason: "Review the original failed case", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return h, issue
}

// Protocol-level proof uses two actual authorized Runner identities and the
// scheduler's frozen candidate pin. OS workspace execution has its own test.
func issueRepairCandidateRunFixture(t *testing.T, h *qualityPlanFixture) dag.Assignment {
	t.Helper()
	f := h.f
	producerID := "issue-repair-producer"
	producerToken := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": producerID, "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := tc.Profile{CommandID: "issue-repair-produce", Adapter: "command", Protocol: "text-v1", ProfileSHA256: strings.Repeat("d", 64), ExecutableSHA256: strings.Repeat("f", 64), ToolVersion: "fixed repair command fixture"}
	profile, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: producerID, Name: "Administrator repair command", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, producerToken), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: producerID, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(context.Background(), f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Repair candidate and independent regression", "nodes": []any{
		map[string]any{"id": "repair", "type": "command", "command": []string{definition.CommandID}, "config": map[string]any{"tool_profile_id": profile.ID, "tool_profile_sha256": profile.Profile.ProfileSHA256}},
		map[string]any{"id": "test", "type": "test", "command": []string{h.profile.Profile.CommandID}, "depends_on": []string{"repair"}, "config": map[string]any{"candidate_from": "repair", "tool_profile_id": h.profile.ID, "tool_profile_sha256": h.profile.Profile.ProfileSHA256}},
	}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"], "task_id": h.task.ID}), 201)
	claim := f.bearer("POST", "/runner/claim", producerToken, f.body(map[string]any{"worker_id": producerID, "capabilities": []string{"command:issue-repair-produce"}}), 200)
	var source dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claim["assignment"])), &source); err != nil || source.RunID == "" || source.NodeID != "repair" {
		t.Fatal("missing actual repair producer", claim, err)
	}
	proof := map[string]any{"head_commit": strings.Repeat("a", 40), "base_commit": strings.Repeat("c", 40), "evidence_sha256": strings.Repeat("e", 64), "state": "completed", "clean": true, "outcome_unknown": false, "exit_code": 0}
	f.bearer("POST", "/runner/results", producerToken, f.body(map[string]any{"result": dag.Result{RunID: source.RunID, NodeID: source.NodeID, Attempt: source.Attempt, Fence: source.Fence, WorkerID: source.WorkerID, OperationID: newID(), WorkspaceEvidence: proof, ToolEvidence: &tc.Evidence{Profile: definition, Terminal: "completed", UsageState: tc.UsageUnreported}}}), 200)
	claim = f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var test dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claim["assignment"])), &test); err != nil || test.RunID != source.RunID || test.WorkerID == source.WorkerID || test.CandidateFrom != source.NodeID || test.CandidateAttempt != source.Attempt || test.CandidateFence != source.Fence || test.CandidateEvidenceSHA256 != proof["evidence_sha256"] {
		t.Fatal("independent test lost the original candidate envelope", claim, err)
	}
	return test
}

func issueRepairCandidateResultFixture(h *qualityPlanFixture, a dag.Assignment) dag.Result {
	r := h.result(a, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)
	r.WorkspaceEvidence["base_commit"], r.WorkspaceEvidence["head_commit"] = a.CandidateCommit, a.CandidateCommit
	r.WorkspaceEvidence["candidate_from"], r.WorkspaceEvidence["candidate_attempt"], r.WorkspaceEvidence["candidate_fence"], r.WorkspaceEvidence["candidate_evidence_sha256"] = a.CandidateFrom, a.CandidateAttempt, a.CandidateFence, a.CandidateEvidenceSHA256
	return r
}

func TestIssueRepairGenericBypassAndCanonicalPreparation(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	_, err := f.s.Create(ctx, f.actor, f.project, "issues", f.body(map[string]any{"title": "Forged closure", "status": "closed"}))
	requireAuthorityStatus(t, err, 400)
	issue, err := f.s.Create(ctx, f.actor, f.project, "issues", f.body(map[string]any{"title": "Scoped failure"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Update(ctx, f.actor, f.project, "issues", issue.ID, f.body(map[string]any{"expected_version": issue.Version, "status": "closed"}))
	requireAuthorityStatus(t, err, 400)
	_, err = f.s.Update(ctx, f.actor, f.project, "issues", issue.ID, f.body(map[string]any{"expected_version": issue.Version, "data": map[string]any{"issue_repair": map[string]any{"task_id": "forged"}}}))
	requireAuthorityStatus(t, err, 400)
	canonical, err := f.s.IssueToRequirement(ctx, f.actor, f.project, issue.ID, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	v, request := issueRepairPrepareFixture(t, f, issue)
	if v.Current.RequirementID != canonical.ID || v.Task.RequirementID != canonical.ID {
		t.Fatal("forked canonical intake", v, canonical)
	}
	replay, err := f.s.PrepareIssueRepair(ctx, f.actor, f.project, issue.ID, request)
	if err != nil || replay.Current.ID != v.Current.ID || replay.Task.ID != v.Task.ID {
		t.Fatal("stale original receipt did not recover", replay, err)
	}
	request.IdempotencyKey = newID()
	_, err = f.s.PrepareIssueRepair(ctx, f.actor, f.project, issue.ID, request)
	requireAuthorityStatus(t, err, 409)
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_issue_repairs WHERE tenant_id=? AND project_id=? AND issue_id=?`, f.user.TenantID, f.project, issue.ID).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate repair cycle", count, err)
	}
}

func TestIssueRepairOriginalCaseGateCloseReopenAndReceipts(t *testing.T) {
	h, issue := issueRepairFailedCaseFixture(t)
	f := h.f
	ctx := context.Background()
	v, _ := issueRepairPrepareFixture(t, f, issue)
	changed := qualityCase()
	changed.Selector.Name = "unrelated-green"
	p := issueRepairPlanFixture(t, h, v, []QualityTestCase{changed})
	bind := IssueRepairBindRequest{ProjectID: f.project, ExpectedIssueVersion: v.Issue.Version, ExpectedRepairVersion: v.Current.Version, ExpectedPlanVersion: p.Version, PlanID: p.ID, CaseKeys: []string{"contract"}, Reason: "Select exact regression", IdempotencyKey: newID()}
	_, err := f.s.BindIssueRegression(ctx, f.actor, f.project, issue.ID, bind)
	requireAuthorityStatus(t, err, 409)
	// Publishing the correct selector changes the plan revision; binding must use
	// that exact current publication, not an unrelated successful test name.
	requirement, err := f.s.Get(ctx, f.actor, f.project, "requirements", h.task.RequirementID)
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.s.SaveTestPlan(ctx, f.actor, f.project, p.ID, TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: p.Version, ExpectedTaskVersion: h.task.Version, ExpectedRequirementVersion: requirement.Version, Title: "Original failure regression", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	p, err = f.s.CommandTestPlan(ctx, f.actor, f.project, p.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "publish", Reason: "Review original selector", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	h.plan = p
	bind.ExpectedPlanVersion, bind.IdempotencyKey = p.Version, newID()
	v, err = f.s.BindIssueRegression(ctx, f.actor, f.project, issue.ID, bind)
	if err != nil {
		t.Fatal(err)
	}
	// Generic task Review permits non-code test-only work. Even an accepted
	// green test-only task must not close a defect without a repair producer.
	self := h.run(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(self, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	selfTask, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, selfTask.ID, f.body(map[string]any{"expected_version": selfTask.Version, "run_id": self.RunID, "decision": "accepted", "reason": "Review the non-code test-only fixture"}), f.e); err != nil {
		t.Fatal(err)
	}
	selfView, err := f.s.GetIssueRepair(ctx, f.actor, f.project, issue.ID)
	if err != nil || selfView.CanClose {
		t.Fatal("test-only reported SHA became repair proof", selfView, err)
	}
	_, err = f.s.CloseIssueRepair(ctx, f.actor, f.project, issue.ID, IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: v.Issue.Version, ExpectedRepairVersion: v.Current.Version, RunID: self.RunID, Reason: "A green test alone cannot prove repaired code", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
	a := issueRepairCandidateRunFixture(t, h)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": issueRepairCandidateResultFixture(h, a)}), 200)
	closeRequest := IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: v.Issue.Version, ExpectedRepairVersion: v.Current.Version, RunID: a.RunID, Reason: "Confirm regression and formal acceptance", IdempotencyKey: newID()}
	_, err = f.s.CloseIssueRepair(ctx, f.actor, f.project, issue.ID, closeRequest)
	requireAuthorityStatus(t, err, 409)
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": a.RunID, "decision": "accepted", "reason": "Explicit independent regression review"}), f.e)
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.s.GetIssueRepair(ctx, f.actor, f.project, issue.ID)
	if err != nil || !v.CanClose {
		t.Fatal("current accepted exact evidence not ready", v, err)
	}
	closed, err := f.s.CloseIssueRepair(ctx, f.actor, f.project, issue.ID, closeRequest)
	if err != nil || closed.Issue.Status != "closed" || closed.Current.Closure.RunID != a.RunID {
		t.Fatal("close lost exact evidence", closed, err)
	}
	replay, err := f.s.CloseIssueRepair(ctx, f.actor, f.project, issue.ID, closeRequest)
	if err != nil || replay.Issue.Version != closed.Issue.Version {
		t.Fatal("close receipt failed before stale checks", replay, err)
	}
	reopen := IssueRepairReopenRequest{ProjectID: f.project, ExpectedIssueVersion: closed.Issue.Version, ExpectedRepairVersion: closed.Current.Version, Reason: "Failure recurred after review", IdempotencyKey: newID()}
	opened, err := f.s.ReopenIssueRepair(ctx, f.actor, f.project, issue.ID, reopen)
	if err != nil || opened.Issue.Status != "open" || opened.Current != nil || opened.Cycle != 2 || len(opened.History) != 1 || opened.History[0].Closure == nil {
		t.Fatal("reopen rewrote accepted history", opened, err)
	}
	next, _ := issueRepairPrepareFixture(t, f, opened.Issue)
	if next.Current.Cycle != 2 || next.Task.ID == task.ID {
		t.Fatal("new cycle reused old task", next)
	}
	_, err = f.s.CloseIssueRepair(ctx, f.actor, f.project, issue.ID, IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: next.Issue.Version, ExpectedRepairVersion: next.Current.Version, RunID: a.RunID, Reason: "Old run cannot close new recurrence", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 409)
}

func TestIssueRepairCurrentAuthorityAndPublicationInvalidation(t *testing.T) {
	h, issue := issueRepairFailedCaseFixture(t)
	f := h.f
	ctx := context.Background()
	v, _ := issueRepairPrepareFixture(t, f, issue)
	p := issueRepairPlanFixture(t, h, v, []QualityTestCase{qualityCase()})
	v, err := f.s.BindIssueRegression(ctx, f.actor, f.project, issue.ID, IssueRepairBindRequest{ProjectID: f.project, ExpectedIssueVersion: v.Issue.Version, ExpectedRepairVersion: v.Current.Version, ExpectedPlanVersion: p.Version, PlanID: p.ID, CaseKeys: []string{"contract"}, Reason: "Bind reviewed regression", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	credential, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api, err := f.s.AuthenticateActor(ctx, credential.Token)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CloseIssueRepair(ctx, api, f.project, issue.ID, IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: v.Issue.Version, ExpectedRepairVersion: v.Current.Version, RunID: strings.Repeat("a", 32), Reason: "An API cannot act as a human reviewer", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.GetIssueRepair(ctx, toolActorForTest(t, f, h.token), f.project, issue.ID)
	requireAuthorityStatus(t, err, 403)
	_, err = f.s.CommandTestPlan(ctx, f.actor, f.project, p.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "retire", Reason: "Withdraw obsolete regression", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	v, err = f.s.GetIssueRepair(ctx, f.actor, f.project, issue.ID)
	if err != nil || v.CanClose {
		t.Fatal("retired regression remained closeable", v, err)
	}
	found := false
	for _, blocker := range v.Blockers {
		found = found || blocker.Code == "regression_plan_changed"
	}
	if !found {
		t.Fatal("missing actionable obsolete-plan blocker", v.Blockers)
	}
	other, err := f.s.CreateProject(ctx, f.actor, f.body(map[string]any{"title": "Other repair scope"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GetIssueRepair(ctx, f.actor, other.ID, issue.ID)
	requireAuthorityStatus(t, err, 404)
	if _, err = f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'developer') ON CONFLICT(project_id,user_id)DO UPDATE SET role='developer'`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CloseIssueRepair(ctx, f.actor, f.project, issue.ID, IssueRepairCloseRequest{ProjectID: f.project, ExpectedIssueVersion: v.Issue.Version, ExpectedRepairVersion: v.Current.Version, RunID: strings.Repeat("a", 32), Reason: "A developer is not the current reviewer", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 403)
}
