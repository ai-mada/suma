package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

type qualityPlanFixture struct {
	f              *fixture
	task           Record
	plan           TestPlanView
	profile        ToolProfile
	token          string
	publishRequest TestPlanCommandRequest
}

func qualityCase() QualityTestCase {
	return QualityTestCase{Key: "contract", Title: "Enforce current project access", Description: "Check the selected project", Preconditions: "An authenticated member", Steps: []string{"Request the current project"}, ExpectedResult: "Access is checked", Priority: "high", Covers: []int{1}, Selector: tr.TestCaseSelector{ClassName: "delivery", Name: "contract"}}
}
func newQualityPlanFixture(t *testing.T, publish bool) *qualityPlanFixture {
	t.Helper()
	f := newFixture(t)
	ctx := context.Background()
	if err := f.s.InitializeTestPlans(ctx); err != nil {
		t.Fatal(err)
	}
	task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Plan-bound delivery", "acceptance_criteria": []any{"Current project authority is enforced"}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(task.AcceptanceCriteria) != 1 {
		t.Fatal("fixture must create its actual task acceptance criterion", task)
	}
	request := TestPlanCreateRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, Title: "Reviewed delivery tests", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()}
	plan, err := f.s.CreateTestPlan(ctx, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	publishRequest := TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: plan.Version, Action: "publish", Reason: "Explicit local fixture review", IdempotencyKey: newID()}
	if publish {
		plan, err = f.s.CommandTestPlan(ctx, f.actor, f.project, plan.ID, publishRequest)
		if err != nil {
			t.Fatal(err)
		}
	}
	token := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "quality-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	definition := toolTestProfile()
	definition.Adapter, definition.Protocol = "command", "text-v1"
	definition.TestReportProtocol = tr.Protocol
	profile, err := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "quality-worker", Name: "Administrator report command", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: definition, Available: true}}})
	if err != nil {
		t.Fatal(err)
	}
	profile, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "enable", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	return &qualityPlanFixture{f: f, task: task, plan: plan, profile: profile, token: token, publishRequest: publishRequest}
}
func (h *qualityPlanFixture) run(t *testing.T) dag.Assignment {
	t.Helper()
	f := h.f
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "Plan-bound exact test", "nodes": []any{map[string]any{"id": "test", "type": "test", "command": []string{h.profile.Profile.CommandID}, "config": map[string]any{"tool_profile_id": h.profile.ID, "tool_profile_sha256": h.profile.Profile.ProfileSHA256}}}}), 201)
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": h.task.ID}), 201)
	claimed := f.bearer("POST", "/runner/claim", h.token, f.body(map[string]any{"worker_id": h.profile.WorkerID, "capabilities": []string{"command:" + h.profile.Profile.CommandID}}), 200)
	var a dag.Assignment
	if err := json.Unmarshal([]byte(stringify(claimed["assignment"])), &a); err != nil || a.RunID == "" {
		t.Fatal("missing actual plan assignment", claimed, err)
	}
	return a
}
func (h *qualityPlanFixture) result(a dag.Assignment, xml string) dag.Result {
	r := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: map[string]any{"head_commit": strings.Repeat("a", 40), "base_commit": strings.Repeat("a", 40), "clean": true, "state": "completed", "outcome_unknown": false, "exit_code": 0, "test_command": true, "evidence_sha256": strings.Repeat("b", 64)}, ToolEvidence: &tc.Evidence{Profile: h.profile.Profile, Terminal: "completed", EventCount: 1, UsageState: tc.UsageUnreported}}
	if xml != "" {
		r.TestReport = &tr.TestReport{Protocol: tr.Protocol, XML: xml, SHA256: digest(xml)}
	}
	return r
}

func TestTestPlanImmutableDraftPublicationAndCurrentAuthority(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	ctx := context.Background()
	f := h.f
	saveRequest := TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, ExpectedTaskVersion: h.task.Version, Title: "Unpublished revision", Cases: []QualityTestCase{}, IdempotencyKey: newID()}
	saved, err := f.s.SaveTestPlan(ctx, f.actor, f.project, h.plan.ID, saveRequest)
	if err != nil || !saved.HasDraft || saved.Published.SHA256 != h.plan.Published.SHA256 || len(saved.Latest.Cases) != 0 {
		t.Fatal("draft altered publication", saved, err)
	}
	_, err = f.s.CommandTestPlan(ctx, f.actor, f.project, saved.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: saved.Version, Action: "publish", Reason: "Incomplete drafts remain editable", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 400)
	token, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	apiActor, err := f.s.AuthenticateActor(ctx, token.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetTestPlan(ctx, apiActor, f.project, saved.ID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandTestPlan(ctx, apiActor, f.project, saved.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: saved.Version, Action: "retire", Reason: "API may not self-approve", IdempotencyKey: newID()})
	requireAuthorityStatus(t, err, 403)
	wrong := TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: saved.Version, ExpectedTaskVersion: h.task.Version, Title: "Duplicate selector", Cases: []QualityTestCase{qualityCase(), qualityCase()}, IdempotencyKey: newID()}
	wrong.Cases[1].Key = "other"
	_, err = f.s.SaveTestPlan(ctx, f.actor, f.project, saved.ID, wrong)
	requireAuthorityStatus(t, err, 400)
	f.call("PATCH", "/tasks/"+h.task.ID, f.body(map[string]any{"expected_version": h.task.Version, "title": "Changed business source without changing the plan"}), 200)
	replayedSave, err := f.s.SaveTestPlan(ctx, f.actor, f.project, h.plan.ID, saveRequest)
	if err != nil || replayedSave.Version != saved.Version || replayedSave.BasisCurrent || replayedSave.Eligibility != "source_changed" || replayedSave.Latest.SHA256 != saved.Latest.SHA256 || replayedSave.Latest.Title != saved.Latest.Title || replayedSave.Published.SHA256 != saved.Published.SHA256 {
		t.Fatal("original save receipt retained stale source eligibility or replaced original content", replayedSave, err)
	}
	for _, action := range replayedSave.AllowedActions {
		if action == "publish" {
			t.Fatal("stale source retained publish action")
		}
	}
	replayedPublish, err := f.s.CommandTestPlan(ctx, f.actor, f.project, h.plan.ID, h.publishRequest)
	if err != nil || replayedPublish.Version != h.plan.Version || replayedPublish.BasisCurrent || replayedPublish.Eligibility != "source_changed" || replayedPublish.Latest.SHA256 != h.plan.Latest.SHA256 || replayedPublish.Published.SHA256 != h.plan.Published.SHA256 || len(replayedPublish.AllowedActions) != 0 {
		t.Fatal("original publication receipt rebased source or allowed stale CAS", replayedPublish, err)
	}
}

func TestTestPlanExactReportCoverageAndGate(t *testing.T) {
	for _, scenario := range []string{"passed", "skipped", "missing", "extra-failure", "unknown", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			h := newQualityPlanFixture(t, true)
			f := h.f
			a := h.run(t)
			xml := `<testsuite tests="1"><testcase classname="delivery" name="contract" time="0.01"/></testsuite>`
			switch scenario {
			case "skipped":
				xml = `<testsuite tests="1" skipped="1"><testcase classname="delivery" name="contract"><skipped message="Not exercised"/></testcase></testsuite>`
			case "missing":
				xml = ""
			case "extra-failure":
				xml = `<testsuite tests="2" failures="1"><testcase classname="delivery" name="contract"/><testcase classname="unmapped" name="extra"><failure message="Unexpected failure"/></testcase></testsuite>`
			case "malformed":
				xml = "<invalid/>"
			}
			result := h.result(a, xml)
			if scenario == "unknown" {
				result.OutcomeUnknown = true
				result.ToolEvidence.Terminal = "unknown"
			}
			request := f.body(map[string]any{"result": result})
			f.bearer("POST", "/runner/results", h.token, request, 200)
			f.bearer("POST", "/runner/results", h.token, request, 200)
			execution, err := f.s.GetTestPlanExecution(context.Background(), f.actor, f.project, h.plan.ID, a.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "passed" {
				if execution.State != "passed" || execution.Passed != 1 || execution.Cases[0].Status != "passed" {
					t.Fatal(execution)
				}
			} else if execution.State == "passed" {
				t.Fatal("unverified test became passed", scenario, execution)
			}
			tx, err := f.s.DB.Begin()
			if err != nil {
				t.Fatal(err)
			}
			err = f.s.checkCurrentTaskTestPlanRunTx(context.Background(), tx, f.user.TenantID, f.project, h.task.ID, a.RunID)
			tx.Rollback()
			if (err == nil) != (scenario == "passed") {
				t.Fatal("formal test gate disagrees with exact report", scenario, err)
			}
			if scenario == "passed" || scenario == "skipped" {
				// The process/DAG is successful in both scenarios. This exercises
				// the public Review gate rather than only its private helper.
				if execution.RunState != "succeeded" {
					t.Fatal("fixture did not reach an ordinary successful run", execution)
				}
				current := f.call("GET", "/tasks/"+h.task.ID+"?project_id="+f.project, nil, 200)
				review := f.body(map[string]any{"expected_version": current["version"], "run_id": a.RunID, "decision": "accepted", "reason": "Explicit isolated fixture review of exact case evidence"})
				if scenario == "passed" {
					accepted := f.call("POST", "/tasks/"+h.task.ID+"/review", review, 200)
					replayed := f.call("POST", "/tasks/"+h.task.ID+"/review", review, 200)
					if accepted["status"] != "accepted" || replayed["id"] != accepted["id"] || replayed["version"] != accepted["version"] {
						t.Fatal("formal acceptance or original receipt failed", accepted, replayed)
					}
				} else {
					if err == nil || err.Error() != testPlanGate {
						t.Fatal("skip was not blocked by the quality-plan gate", err)
					}
					f.call("POST", "/tasks/"+h.task.ID+"/review", review, 409)
				}
			}
			if scenario == "passed" {
				if len(execution.Reports) != 1 || execution.Reports[0].Unmapped != 0 {
					t.Fatal(execution)
				}
				report, err := f.s.GetTestPlanReport(context.Background(), f.actor, f.project, h.plan.ID, execution.Reports[0].ID)
				if err != nil || report.XML != xml || report.SHA256 != digest(xml) {
					t.Fatal("original report changed", report, err)
				}
			}
		})
	}
}

func TestTestPlanNewPublicationAndRetirementInvalidateOldRun(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f := h.f
	ctx := context.Background()
	a := h.run(t)
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": h.result(a, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	var current Record
	if err := json.Unmarshal([]byte(stringify(f.call("GET", "/tasks/"+h.task.ID+"?project_id="+f.project, nil, 200))), &current); err != nil {
		t.Fatal(err)
	}
	saved, err := f.s.SaveTestPlan(ctx, f.actor, f.project, h.plan.ID, TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, ExpectedTaskVersion: current.Version, Title: "Next reviewed plan", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	old, err := f.s.GetTestPlanExecution(ctx, f.actor, f.project, h.plan.ID, a.RunID)
	if err != nil || old.State != "passed" {
		t.Fatal("draft invalidated approved baseline", old, err)
	}
	published, err := f.s.CommandTestPlan(ctx, f.actor, f.project, saved.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: saved.Version, Action: "publish", Reason: "New explicit baseline", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	old, err = f.s.GetTestPlanExecution(ctx, f.actor, f.project, published.ID, a.RunID)
	if err != nil || old.State != "run_changed" {
		t.Fatal("old report survived a new publication", old, err)
	}
	_, err = f.s.CommandTestPlan(ctx, f.actor, f.project, published.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: published.Version, Action: "retire", Reason: "Retirement is not an old-run gate bypass", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := f.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = f.s.checkCurrentTaskTestPlanRunTx(ctx, tx, f.user.TenantID, f.project, h.task.ID, a.RunID)
	tx.Rollback()
	if err == nil {
		t.Fatal("retired plan bypassed old-run evidence")
	}
	actual := f.call("GET", "/tasks/"+h.task.ID+"?project_id="+f.project, nil, 200)
	f.call("POST", "/tasks/"+h.task.ID+"/review", f.body(map[string]any{"expected_version": actual["version"], "run_id": a.RunID, "decision": "accepted", "reason": "Retiring a plan cannot approve an old run"}), 409)

	// A no-plan run must not regain eligibility after publish then retire.
	// Draft editing leaves the governance epoch alone; a future retired-policy
	// run freezes the new epoch and retains the existing optional-plan semantics.
	optional := newQualityPlanFixture(t, false)
	g := optional.f
	oldRun := optional.run(t)
	g.bearer("POST", "/runner/results", optional.token, g.body(map[string]any{"result": optional.result(oldRun, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	task := g.call("GET", "/tasks/"+optional.task.ID+"?project_id="+g.project, nil, 200)
	draft, err := g.s.SaveTestPlan(ctx, g.actor, g.project, optional.plan.ID, TestPlanSaveRequest{ProjectID: g.project, ExpectedVersion: optional.plan.Version, ExpectedTaskVersion: int(task["version"].(float64)), Title: "Still an unpublished draft", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	noPlan, err := g.s.GetTestPlanExecution(ctx, g.actor, g.project, draft.ID, oldRun.RunID)
	if err != nil || !noPlan.BasisCurrent || noPlan.Revision != 0 {
		t.Fatal("draft save changed no-plan governance", noPlan, err)
	}
	tx, err = g.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = g.s.checkCurrentTaskTestPlanRunTx(ctx, tx, g.user.TenantID, g.project, optional.task.ID, oldRun.RunID)
	tx.Rollback()
	if err != nil {
		t.Fatal("draft save invalidated an optional-plan run", err)
	}
	active, err := g.s.CommandTestPlan(ctx, g.actor, g.project, draft.ID, TestPlanCommandRequest{ProjectID: g.project, ExpectedVersion: draft.Version, Action: "publish", Reason: "Introduce an explicit test policy", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	g.call("POST", "/tasks/"+optional.task.ID+"/review", g.body(map[string]any{"expected_version": task["version"], "run_id": oldRun.RunID, "decision": "accepted", "reason": "An old no-plan run cannot satisfy a new publication"}), 409)
	_, err = g.s.CommandTestPlan(ctx, g.actor, g.project, active.ID, TestPlanCommandRequest{ProjectID: g.project, ExpectedVersion: active.Version, Action: "retire", Reason: "Retire prospectively, never restore old-run evidence", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	g.call("POST", "/tasks/"+optional.task.ID+"/review", g.body(map[string]any{"expected_version": task["version"], "run_id": oldRun.RunID, "decision": "accepted", "reason": "Retirement must not restore the older no-plan run"}), 409)
	noPlan, err = g.s.GetTestPlanExecution(ctx, g.actor, g.project, draft.ID, oldRun.RunID)
	if err != nil || noPlan.BasisCurrent || noPlan.State != "run_changed" {
		t.Fatal("retirement restored the old no-plan snapshot", noPlan, err)
	}
	future := optional.run(t)
	g.bearer("POST", "/runner/results", optional.token, g.body(map[string]any{"result": optional.result(future, `<testsuite tests="1"><testcase classname="delivery" name="contract"/></testsuite>`)}), 200)
	tx, err = g.s.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	err = g.s.checkCurrentTaskTestPlanRunTx(ctx, tx, g.user.TenantID, g.project, optional.task.ID, future.RunID)
	tx.Rollback()
	if err != nil {
		t.Fatal("new run after explicit retirement lost optional-plan semantics", err)
	}
}

func TestTestPlanDefectUsesProtectedCaseLineageAndOriginalReceipt(t *testing.T) {
	h := newQualityPlanFixture(t, true)
	f := h.f
	ctx := context.Background()
	a := h.run(t)
	result := h.result(a, `<testsuite tests="1" failures="1"><testcase classname="delivery" name="contract"><failure message="Scope check failed"/></testcase></testsuite>`)
	result.ExitCode = 1
	result.WorkspaceEvidence["exit_code"] = 1
	result.ToolEvidence.Terminal = "failed"
	f.bearer("POST", "/runner/results", h.token, f.body(map[string]any{"result": result}), 200)
	execution, err := f.s.GetTestPlanExecution(ctx, f.actor, f.project, h.plan.ID, a.RunID)
	if err != nil || execution.Cases[0].Status != "failed" {
		t.Fatal(execution, err)
	}
	request := TestPlanDefectRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, CaseKey: "contract", Reason: "Independent reviewer confirms the case failure", IdempotencyKey: newID()}
	issue, err := f.s.CreateTestPlanDefect(ctx, f.actor, f.project, h.plan.ID, a.RunID, request)
	if err != nil || issue.Status != "open" {
		t.Fatal(issue, err)
	}
	replay, err := f.s.CreateTestPlanDefect(ctx, f.actor, f.project, h.plan.ID, a.RunID, request)
	if err != nil || replay.ID != issue.ID {
		t.Fatal("original issue duplicated", replay, err)
	}
	request.IdempotencyKey = newID()
	another, err := f.s.CreateTestPlanDefect(ctx, f.actor, f.project, h.plan.ID, a.RunID, request)
	if err != nil || another.ID != issue.ID {
		t.Fatal("new key duplicated a protected run/case defect", another, err)
	}
	execution, err = f.s.GetTestPlanExecution(ctx, f.actor, f.project, h.plan.ID, a.RunID)
	if err != nil || execution.Cases[0].IssueID != issue.ID {
		t.Fatal("editable issue data replaced the protected case relation", execution, err)
	}
	delivery, err := f.s.GetTaskDelivery(ctx, f.actor, f.project, h.task.ID)
	if err != nil || delivery.TestPlanID != h.plan.ID {
		t.Fatal("task cannot reach its quality plan", delivery, err)
	}
	linked := false
	for _, ref := range delivery.Issues.Items {
		if ref.Source == "test_case" && ref.IssueID == issue.ID {
			linked = true
		}
	}
	if !linked {
		t.Fatal("protected case defect is missing from task delivery", delivery.Issues)
	}
	request.CaseKey = "missing"
	request.IdempotencyKey = newID()
	_, err = f.s.CreateTestPlanDefect(ctx, f.actor, f.project, h.plan.ID, a.RunID, request)
	if err == nil {
		t.Fatal("unreported case created a defect")
	}

	wantOrigin := IssueRepairCaseOrigin{PlanID: h.plan.ID, RunID: a.RunID, CaseKey: "contract", PlanRevision: h.plan.Published.Revision, PlanSHA256: h.plan.Published.SHA256}
	readOrigin := func(id string) *IssueRepairCaseOrigin {
		t.Helper()
		var view IssueRepairView
		body := f.call("GET", "/issues/"+id+"/repair?project_id="+f.project, nil, 200)
		if err := json.Unmarshal([]byte(stringify(body)), &view); err != nil {
			t.Fatal(err)
		}
		return view.OriginalCase
	}
	if got := readOrigin(issue.ID); got == nil || *got != wantOrigin {
		t.Fatal("issue could not locate its exact originating failure", got)
	}
	forged, err := f.s.Create(ctx, f.actor, f.project, "issues", f.body(map[string]any{"title": "Manually reported failure", "data": issue.Data}))
	if err != nil {
		t.Fatal(err)
	}
	if got := readOrigin(forged.ID); got != nil {
		t.Fatal("editable metadata forged a protected origin", got)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.task.ID)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := f.s.SaveTestPlan(ctx, f.actor, f.project, h.plan.ID, TestPlanSaveRequest{ProjectID: f.project, ExpectedVersion: h.plan.Version, ExpectedTaskVersion: current.Version, Title: "Replacement reviewed plan", Cases: []QualityTestCase{qualityCase()}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	published, err := f.s.CommandTestPlan(ctx, f.actor, f.project, draft.ID, TestPlanCommandRequest{ProjectID: f.project, ExpectedVersion: draft.Version, Action: "publish", Reason: "Publish a new basis for future runs", IdempotencyKey: newID()})
	if err != nil || published.Published == nil || published.Published.SHA256 == wantOrigin.PlanSHA256 {
		t.Fatal("could not establish a distinct current plan", published, err)
	}
	if got := readOrigin(issue.ID); got == nil || *got != wantOrigin {
		t.Fatal("republishing the plan replaced the historical failure origin", got)
	}
}
