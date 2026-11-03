package platform

import (
	"context"
	"testing"

	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

// All completion fixtures below use the original controlled read-only
// protocol receipts. They are not supplier/model invocations or independent
// code-test execution; this stage's isolated runtime journey owns that proof.
func collaborativeTestSelection(profile ToolProfile) []TaskDispatchTestSelection {
	return []TaskDispatchTestSelection{{NodeID: "test", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version}}
}

func collaborativeAssertFrozenTest(t *testing.T, plan TaskDispatchPlan, profile ToolProfile) {
	t.Helper()
	want := TaskDispatchTestBinding{NodeID: "test", ToolProfileID: profile.ID, ToolProfileVersion: profile.Version, ToolProfileSHA256: profile.Profile.ProfileSHA256, WorkerID: profile.WorkerID, CommandID: profile.Profile.CommandID}
	if len(plan.TestBindings) != 1 || plan.TestBindings[0] != want {
		t.Fatal("explicit six-field test snapshot was lost", plan.TestBindings, want)
	}
	found := false
	for _, node := range plan.Workflow.Nodes {
		if node.ID != "test" {
			continue
		}
		found = true
		if node.Type != "test" || len(node.Command) != 1 || node.Command[0] != want.CommandID || node.Config["candidate_from"] != plan.AgentNodeID || node.Config["tool_profile_id"] != want.ToolProfileID || node.Config["tool_profile_sha256"] != want.ToolProfileSHA256 {
			t.Fatal("displayed tests disagree with the actual copied workflow", node)
		}
	}
	if !found {
		t.Fatal("original candidate test node disappeared")
	}
}

func collaborativeSameWorkerTest(t *testing.T, f *fixture, developer ToolProfile) ToolProfile {
	t.Helper()
	definition := toolTestProfile()
	definition.Adapter, definition.Protocol, definition.CommandID = "command", "text-v1", "same-worker-independent-claim"
	definition.ProfileSHA256, definition.TestReportProtocol = digest("same-worker-test223"), tr.Protocol
	profile, err := f.s.CreateToolProfile(context.Background(), f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: developer.WorkerID, Name: "Not independent of this developer", Profile: definition, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	// No inventory is replaced. Independence is checked before admitting any
	// profile/observation, so the exact same Worker must fail the original gate.
	return profile
}

func TestCollaborativeDiscussionTestsFreezeRecoverAndKeepHistoryOnDrift(t *testing.T) {
	h, request := taskDiscussionDevelopmentFixture(t)
	f, ctx := h.f, context.Background()
	profile := dispatchTestBindingsProfile(t, f, "discussion-independent-223", "discussion-test223", tr.Protocol)
	request.TestBindings = collaborativeTestSelection(profile)
	var original string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`, f.user.TenantID, f.project, request.WorkflowID, request.WorkflowVersion).Scan(&original); err != nil {
		t.Fatal(err)
	}
	plan, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil {
		t.Fatal(err)
	}
	collaborativeAssertFrozenTest(t, plan.DispatchPlan, profile)
	_, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil || view.DispatchPlan.Valid {
		t.Fatal("test drift did not block current admission", view, err)
	}
	collaborativeAssertFrozenTest(t, view.DispatchPlan, profile)
	recovered, err := f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	if err != nil || recovered.ID != plan.ID {
		t.Fatal("original preview operation was lost", recovered, err)
	}
	collaborativeAssertFrozenTest(t, recovered.DispatchPlan, profile)
	changed := request
	changed.TestBindings = nil
	_, err = f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, changed)
	expertStatus(t, err, 409)
	var after string
	if err = f.s.DB.QueryRow(`SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?`, f.user.TenantID, f.project, request.WorkflowID, request.WorkflowVersion).Scan(&after); err != nil || after != original {
		t.Fatal("preview changed the published quality workflow", err)
	}
}

func TestCollaborativeDiscussionTestsUseActualDeveloperAndOriginalBounds(t *testing.T) {
	h, request := taskDiscussionDevelopmentFixture(t)
	f, ctx := h.f, context.Background()
	developer, err := f.s.GetToolProfile(ctx, f.actor, f.project, request.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}
	same := collaborativeSameWorkerTest(t, f, developer)
	request.TestBindings = collaborativeTestSelection(same)
	_, err = f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	expertStatus(t, err, 400)
	if err.Error() != dispatchTestProfileInvalid {
		t.Fatal("same Worker did not reach the original independence gate", err)
	}
	request.IdempotencyKey = newID()
	request.TestBindings = make([]TaskDispatchTestSelection, 65)
	_, err = f.s.CreateTaskDiscussionDevelopmentPlan(ctx, f.e, f.actor, f.project, request)
	expertStatus(t, err, 400)
	if dispatchOptionsCount(t, f, "suma_task_dispatch_plans") != 0 {
		t.Fatal("rejected discussion created a development plan")
	}
}

func collaborativeRecommendationSource(t *testing.T, h *dispatchRecommendationFixture) {
	t.Helper()
	source, err := h.f.s.GetDispatchRecommendationOptions(context.Background(), h.f.e, h.f.actor, h.f.project, h.request.DispatchRecommendationOptionsRequest)
	if err != nil {
		t.Fatal(err)
	}
	h.request.SourceSHA256 = source.SHA256
}

func TestCollaborativeRecommendationTestsPersistOriginalAdoptionAndProvenance(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f, ctx := h.f, context.Background()
	profile := dispatchTestBindingsProfile(t, f, "recommendation-independent223", "recommendation-test223", tr.Protocol)
	collaborativeRecommendationSource(t, h)
	a, choice := h.ready(t)
	request := DispatchRecommendationAdoptRequest{ProjectID: f.project, ExpectedVersion: a.Version, SourceSHA256: a.Source.SHA256, Choice: choice, TestBindings: collaborativeTestSelection(profile), Reason: "Explicitly adopt this developer with a separate fixed tester", IdempotencyKey: newID()}
	out, err := f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	collaborativeAssertFrozenTest(t, out.Plan, profile)
	if len(out.Recommendation.AdoptedTestBindings) != 1 || out.Recommendation.AdoptedTestBindings[0] != out.Plan.TestBindings[0] || out.Recommendation.Source.SHA256 != a.Source.SHA256 || out.Recommendation.OriginalOutputSHA256 != a.OriginalOutputSHA256 || out.Plan.Recommendation.ChoiceSHA256 != digest(stringify(choice)) {
		t.Fatal("human test input rewrote the model choice or its analysis source", out)
	}
	check := func(plan TaskDispatchPlan) error {
		bounded, tx, close, err := f.s.beginAuthorityTx(ctx)
		if err != nil {
			return err
		}
		defer close()
		defer tx.Rollback()
		return f.s.validateDispatchRecommendationProvenanceTx(bounded, tx, f.user.TenantID, f.project, plan)
	}
	if err = check(out.Plan); err != nil {
		t.Fatal(err)
	}
	dropped := out.Plan
	dropped.TestBindings = nil
	expertStatus(t, check(dropped), 409)
	_, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	if err != nil || recovered.Plan.ID != out.Plan.ID {
		t.Fatal("adoption recovery started a new decision", recovered, err)
	}
	collaborativeAssertFrozenTest(t, recovered.Plan, profile)
	if len(recovered.Recommendation.AdoptedTestBindings) != 1 || recovered.Recommendation.AdoptedTestBindings[0] != out.Plan.TestBindings[0] {
		t.Fatal("history was filled from today's catalogue")
	}
	request.TestBindings = nil
	_, err = f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	expertStatus(t, err, 409)
	_, err = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, out.Plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: out.Plan.Version, IdempotencyKey: newID()})
	expertStatus(t, err, 409)
}

func TestCollaborativeRecommendationTestsRecheckChosenDeveloperWorker(t *testing.T) {
	h := newDispatchRecommendationFixture(t)
	f, ctx := h.f, context.Background()
	same := collaborativeSameWorkerTest(t, f, h.developer)
	collaborativeRecommendationSource(t, h)
	a, choice := h.ready(t)
	request := DispatchRecommendationAdoptRequest{ProjectID: f.project, ExpectedVersion: a.Version, SourceSHA256: a.Source.SHA256, Choice: choice, TestBindings: collaborativeTestSelection(same), Reason: "The proposed developer cannot test its own candidate", IdempotencyKey: newID()}
	_, err := f.s.AdoptDispatchRecommendation(ctx, f.e, f.actor, f.project, a.ID, request)
	expertStatus(t, err, 400)
	if err.Error() != dispatchTestProfileInvalid {
		t.Fatal("recommendation borrowed the form's different Worker assumption", err)
	}
	if dispatchOptionsCount(t, f, "suma_task_dispatch_plans") != 0 || dispatchOptionsCount(t, f, "dag_runs") != 1 {
		t.Fatal("rejected adoption changed execution facts")
	}
}

func collaborativeExpertFixture(t *testing.T) (*expertFixture, ToolProfile, ExpertTeamPlanCreateRequest) {
	t.Helper()
	h := newExpertFixture(t, false)
	profile := dispatchTestBindingsProfile(t, h.f, "expert-independent223", "expert-test223", tr.Protocol)
	create := h.request()
	for i := range create.Items {
		selections := collaborativeTestSelection(profile)
		create.Items[i].TestBindings = &selections
	}
	return h, profile, create
}

func TestCollaborativeExpertTestsKeepFrozenSourceAcrossPendingTaskRevision(t *testing.T) {
	h, profile, create := collaborativeExpertFixture(t)
	f, ctx := h.f, context.Background()
	p, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range p.Items {
		collaborativeAssertFrozenTest(t, item.Plan, profile)
	}
	changed, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[0].ID, f.body(map[string]any{"expected_version": h.tasks[0].Version, "description": "Explicitly reviewed new task scope; keep the same tester"}))
	if err != nil {
		t.Fatal(err)
	}
	revise := expertPlanCommand(h, p, "revise")
	revise.ExpectedConfigurationVersion = h.configuration.Version
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: changed.ID, ExpectedTaskVersion: changed.Version, RoleKey: h.role.Key}}
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, revise)
	if err != nil {
		t.Fatal("omitted same-context tests were lost during source reconstruction", err)
	}
	for _, item := range p.Items {
		collaborativeAssertFrozenTest(t, item.Plan, profile)
	}
	view, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || !view.Valid || !view.CanStart {
		t.Fatal("preserved test bindings failed original read/source checks", view, err)
	}
	for _, item := range view.Items {
		collaborativeAssertFrozenTest(t, item.Plan, profile)
	}
}

func TestCollaborativeExpertTestsDistinguishOmissionResetAndOriginalReceipt(t *testing.T) {
	h, profile, create := collaborativeExpertFixture(t)
	f, ctx := h.f, context.Background()
	create.Items = create.Items[:1]
	p, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	revise := expertPlanCommand(h, p, "revise")
	revise.ExpectedConfigurationVersion = h.configuration.Version
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[0].ID, ExpectedTaskVersion: h.tasks[0].Version, RoleKey: h.role.Key}}
	retained, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, revise)
	if err != nil {
		t.Fatal(err)
	}
	collaborativeAssertFrozenTest(t, retained.Items[0].Plan, profile)
	empty := []TaskDispatchTestSelection{}
	reset := revise
	reset.Items = append([]ExpertTeamPlanItemRequest(nil), revise.Items...)
	reset.Items[0].TestBindings = &empty
	if stringify(reset) == stringify(revise) {
		t.Fatal("nil and explicit [] lost their distinct operation bodies")
	}
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, reset)
	expertStatus(t, err, 409)
	reset.ExpectedVersion, reset.IdempotencyKey = retained.Version, newID()
	cleared, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, reset)
	if err != nil || len(cleared.Items[0].Plan.TestBindings) != 0 {
		t.Fatal("explicit reset did not restore the published definition", cleared, err)
	}
	history, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	collaborativeAssertFrozenTest(t, history.Items[0].Plan, profile)
	original, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil || original.Revision != 1 {
		t.Fatal("original creation receipt was replaced by the reset", original, err)
	}
	collaborativeAssertFrozenTest(t, original.Items[0].Plan, profile)
}

func TestCollaborativeExpertTestsRequireExplicitNewContextAndKeepStartedItems(t *testing.T) {
	h, profile, create := collaborativeExpertFixture(t)
	f, ctx := h.f, context.Background()
	create.Items = create.Items[:1]
	p, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	workflow, _, _ := taskDispatchBuildFixture()
	workflow.ID, workflow.Version = "", 0
	workflow, err = f.e.Publish(ctx, f.user.TenantID, f.project, workflow)
	if err != nil {
		t.Fatal(err)
	}
	role := h.role
	role.WorkflowID, role.WorkflowVersion = workflow.ID, workflow.Version
	configuration, err := f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamSaveRequest{ProjectID: f.project, ExpectedVersion: h.configuration.Version, Roles: []ExpertTeamRoleRequest{role}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, h.team, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: configuration.Version, Action: "publish", Reason: "Explicit new workflow context", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	revise := expertPlanCommand(h, p, "revise")
	revise.ExpectedConfigurationVersion = configuration.Version
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[0].ID, ExpectedTaskVersion: h.tasks[0].Version, RoleKey: h.role.Key}}
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, revise)
	expertStatus(t, err, 409)
	selections := collaborativeTestSelection(profile)
	revise.Items[0].TestBindings, revise.IdempotencyKey = &selections, newID()
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, revise)
	if err != nil {
		t.Fatal("explicit reselection in the new workflow was rejected", err)
	}
	collaborativeAssertFrozenTest(t, p.Items[0].Plan, profile)
	p, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, h.start(p))
	if err != nil || p.Items[0].RunID == "" {
		t.Fatal(p, err)
	}
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, expertPlanCommand(h, p, "pause"))
	if err != nil {
		t.Fatal(err)
	}
	revise = expertPlanCommand(h, p, "revise")
	revise.ExpectedConfigurationVersion = configuration.Version
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[0].ID, ExpectedTaskVersion: p.Items[0].TaskVersion, RoleKey: h.role.Key, TestBindings: &selections}}
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, revise)
	expertStatus(t, err, 409)
}

func TestCollaborativeExpertTestsReadDriftBlocksNewActionsWithoutChangingHistory(t *testing.T) {
	h, profile, create := collaborativeExpertFixture(t)
	f, ctx := h.f, context.Background()
	p, err := f.s.CreateExpertTeamPlan(ctx, f.e, f.actor, f.project, create)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandToolProfile(ctx, f.actor, f.project, profile.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: profile.Version, Action: "pause", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID)
	if err != nil || view.Valid || view.CanStart {
		t.Fatal("current tester drift was treated as an executable preview", view, err)
	}
	for _, item := range view.Items {
		collaborativeAssertFrozenTest(t, item.Plan, profile)
	}
	history, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, p.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range history.Items {
		collaborativeAssertFrozenTest(t, item.Plan, profile)
	}
	_, err = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, h.start(p))
	expertStatus(t, err, 409)
	if expertCount(t, f, "dag_runs") != 0 {
		t.Fatal("invalid tests caused a business run")
	}
}
