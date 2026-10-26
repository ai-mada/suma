package platform

import (
	"context"
	"encoding/json"
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

func compositionDispatchProfile(t *testing.T, f *fixture) (ToolProfile, string, dag.Workflow) {
	t.Helper()
	ctx := context.Background()
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "tool-worker", "profiles": []any{}, "deployment": false}), 201)
	p, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: "tool-worker", Name: "Fixed baseline developer", Profile: toolTestProfile(), IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	token := worker["token"].(string)
	_, e = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}})
	if e != nil {
		t.Fatal(e)
	}
	p, e = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "enable", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	w, _, _ := taskDispatchBuildFixture()
	w.ID = ""
	w.Version = 0
	w, e = f.e.Publish(ctx, f.user.TenantID, f.project, w)
	if e != nil {
		t.Fatal(e)
	}
	return p, token, w
}
func compositionBindBaseline(t *testing.T, h *compositionFixture, p CandidateCompositionPlan, task Record, oldVersion int) Record {
	t.Helper()
	r, e := h.f.s.BindTaskCodeBaseline(context.Background(), h.f.e, h.f.actor, h.f.project, task.ID, TaskCodeBaselineBindRequest{ProjectID: h.f.project, ExpectedTaskVersion: task.Version, ExpectedBindingVersion: oldVersion, CompositionID: p.ID, ExpectedCompositionVersion: p.Version, Acknowledge: true, Reason: "Explicitly select this formally accepted composition", IdempotencyKey: newID()})
	if e != nil || r.CodeBaseline == nil {
		t.Fatal(r, e)
	}
	return r
}
func compositionCheckBaselineExecution(t *testing.T, f *fixture, run, node string, current bool) error {
	t.Helper()
	ctx, tx, close, e := f.s.beginAuthorityTx(context.Background())
	if e != nil {
		return e
	}
	defer close()
	defer tx.Rollback()
	return f.s.checkCodeBaselineExecutionTx(ctx, tx, f.user.TenantID, f.project, run, node, current)
}

func TestCandidateCompositionBaselineDispatchFreezesAndRechecksCurrentAuthority(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p, _ := compositionCompleteFixedCandidate(t, h, h.start(t, h.create(t)))
	p = compositionAcceptFixedCandidate(t, h, p)
	profile, token, w := compositionDispatchProfile(t, f)
	task, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Develop from the exact accepted composition"}))
	if e != nil {
		t.Fatal(e)
	}
	request := TaskDispatchPreviewRequest{ProjectID: f.project, TaskID: task.ID, ExpectedTaskVersion: task.Version, WorkflowID: w.ID, WorkflowVersion: w.Version, AgentNodeID: "develop", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, IdempotencyKey: newID()}
	old, e := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if e != nil {
		t.Fatal(e)
	}
	task = compositionBindBaseline(t, h, p, task, 0)
	_, e = f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, old.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: old.Version, IdempotencyKey: newID()})
	expertStatus(t, e, 409)
	request.ExpectedTaskVersion = task.Version
	request.IdempotencyKey = newID()
	plan, e := f.s.CreateTaskDispatchPlan(ctx, f.e, f.actor, f.project, request)
	if e != nil || plan.CodeBaseline == nil || plan.Context.CodeBaseline == nil || *plan.CodeBaseline != *task.CodeBaseline || !codeBaselineConfigMatches(plan.Workflow.Nodes[0].Config["code_baseline"], *task.CodeBaseline) {
		t.Fatal(plan, e)
	}
	run, e := f.s.StartTaskDispatchPlan(ctx, f.e, f.actor, f.project, plan.ID, TaskDispatchStartRequest{ProjectID: f.project, ExpectedVersion: plan.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	if e = compositionCheckBaselineExecution(t, f, run.ID, "develop", true); e != nil {
		t.Fatal(e)
	}
	reply := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": profile.WorkerID, "capabilities": toolFixtureCapabilities(t, f), "idempotency_key": newID()}), 200)
	var a dag.Assignment
	if json.Unmarshal([]byte(stringify(reply["assignment"])), &a) != nil || a.RunID != run.ID || a.CodeBaseline == nil || *a.CodeBaseline != *task.CodeBaseline || a.CandidateCommit != "" || a.CandidateComposition != nil {
		t.Fatal("baseline was lost or confused with a same-run test input", reply)
	}
	req := ccmp.CodeInputRequest{ProjectID: f.project, Assignment: cc.AssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}, InputKind: "code_baseline", BindingID: task.CodeBaseline.ID, TransferID: task.CodeBaseline.Source.TransferID}
	runnerActor := toolActorForTest(t, f, token)
	bounded, tx, close, e := f.s.beginAuthorityTx(ctx)
	if e != nil {
		t.Fatal(e)
	}
	authority, e := f.s.candidateAuthorityTx(bounded, tx, projectCandidateSubject(runnerActor, f.project), req.Assignment)
	if e == nil {
		_, e = f.s.codeInputBindingTx(bounded, tx, authority, req)
	}
	tx.Rollback()
	close()
	if e != nil {
		t.Fatal(e)
	}
	source := h.req.Sources[0]
	_, e = f.s.Update(ctx, f.actor, f.project, "tasks", source.TaskID, f.body(map[string]any{"expected_version": source.ExpectedTaskVersion, "description": "Human correction after the baseline execution began"}))
	if e != nil {
		t.Fatal(e)
	}
	expertStatus(t, compositionCheckBaselineExecution(t, f, run.ID, "develop", true), 409)
	if e = compositionCheckBaselineExecution(t, f, run.ID, "develop", false); e != nil {
		t.Fatal("source drift erased the existing execution authority", e)
	}
	fixed, e := f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
	if e != nil || fixed.Valid || fixed.Candidate == nil || fixed.RunState != "succeeded" {
		t.Fatal(fixed, e)
	}
	acceptedTask, e := f.s.Get(ctx, f.actor, f.project, "tasks", p.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.Review(ctx, f.actor, f.project, p.TaskID, f.body(map[string]any{"expected_version": acceptedTask.Version, "run_id": p.RunID, "decision": "accepted", "reason": "This new approval must fail because a fixed source changed"}), f.e)
	expertStatus(t, e, 409)
	check, auth, end, e := f.s.planningAuthority(ctx, f.actor, f.project, p.TaskID, purposeProjectRead)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.repositoryBasisReadTx(check, auth.tx, auth.currentPrincipal, f.project, p.TaskID, p.ProducerNodeID)
	end()
	expertStatus(t, e, 409)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	expertStatus(t, compositionCheckBaselineExecution(t, f, run.ID, "develop", false), 409)
}

func TestCandidateCompositionExpertPendingBaselineRequiresExplicitRevision(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	p, _ := compositionCompleteFixedCandidate(t, h, h.start(t, h.create(t)))
	p = compositionAcceptFixedCandidate(t, h, p)
	profile, token, w := compositionDispatchProfile(t, f)
	team, e := f.s.SaveTeam(ctx, f.actor, "", map[string]any{"name": "Composition delivery experts", "status": "active", "expected_version": float64(0), "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	resource, e := f.s.Create(ctx, f.actor, f.project, "members", f.body(map[string]any{"title": "Fixed development expert", "kind": "agent", "status": "active"}))
	if e != nil {
		t.Fatal(e)
	}
	teamID := team["id"].(string)
	_, e = f.s.SaveTeamMember(ctx, f.actor, teamID, map[string]any{"resource_kind": "agent", "resource_id": resource.ID, "project_id": f.project, "role": "member", "expected_version": float64(0), "idempotency_key": newID()})
	if e != nil {
		t.Fatal(e)
	}
	role := ExpertTeamRoleRequest{Key: "developer", Title: "Develop fixed baseline", ResourceID: resource.ID, ExpectedResourceVersion: resource.Version, WorkflowID: w.ID, WorkflowVersion: w.Version, AgentNodeID: "develop", ToolProfileID: profile.ID, ExpectedToolVersion: profile.Version, Knowledge: []ProjectKnowledgeReference{}}
	cfg, e := f.s.SaveExpertTeam(ctx, f.e, f.actor, f.project, teamID, ExpertTeamSaveRequest{ProjectID: f.project, Roles: []ExpertTeamRoleRequest{role}, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	cfg, e = f.s.CommandExpertTeam(ctx, f.e, f.actor, f.project, teamID, ExpertTeamCommandRequest{ProjectID: f.project, ExpectedVersion: cfg.Version, Action: "publish", Reason: "Review fixed expert and independent test template", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	root, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Formally accepted prerequisite"}))
	if e != nil {
		t.Fatal(e)
	}
	child, e := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(map[string]any{"title": "Pending inherited development", "depends_on": []any{root.ID}}))
	if e != nil {
		t.Fatal(e)
	}
	child = compositionBindBaseline(t, h, p, child, 0)
	originalBinding := *child.CodeBaseline
	eh := &expertFixture{f: f, runnerToken: token, team: teamID, role: role, resource: resource, configuration: cfg, tasks: []Record{root, child}}
	plan := eh.preview(t)
	plan, e = f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, eh.start(plan))
	if e != nil || plan.Items[0].RunID == "" || plan.Items[1].RunID != "" {
		t.Fatal(plan, e)
	}
	fixedRootRun, fixedRootSource := plan.Items[0].RunID, expertTeamSourceDigest(plan.Items[0])
	plan, e = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, expertPlanCommand(eh, plan, "pause"))
	if e != nil {
		t.Fatal(e)
	}
	child = compositionBindBaseline(t, h, p, child, 1)
	_, e = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, expertPlanCommand(eh, plan, "resume"))
	expertStatus(t, e, 409)
	revise := expertPlanCommand(eh, plan, "revise")
	revise.ExpectedConfigurationVersion = cfg.Version
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: child.ID, ExpectedTaskVersion: child.Version, RoleKey: role.Key}}
	plan, e = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, revise)
	if e != nil || plan.Revision != 2 || plan.Items[0].RunID != fixedRootRun || expertTeamSourceDigest(plan.Items[0]) != fixedRootSource || plan.Items[1].Plan.CodeBaseline == nil || *plan.Items[1].Plan.CodeBaseline != *child.CodeBaseline {
		t.Fatal(plan, e)
	}
	history, e := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, plan.ID, 1)
	if e != nil || history.Items[1].Plan.CodeBaseline == nil || *history.Items[1].Plan.CodeBaseline != originalBinding {
		t.Fatal("revision rewrote historical baseline", history, e)
	}
	plan, e = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID, expertPlanCommand(eh, plan, "resume"))
	if e != nil {
		t.Fatal(e)
	}
	orchestrationCompleteRoot(t, eh, plan)
	orchestrationAcceptRoot(t, eh, plan)
	orchestrationDue(t, eh)
	if e = f.s.TickExpertTeamPlans(ctx, f.e); e != nil {
		t.Fatal(e)
	}
	plan = orchestrationCurrent(t, eh, plan.ID)
	if plan.Items[1].RunID == "" || plan.Items[0].RunID != fixedRootRun {
		t.Fatal(plan)
	}
	if e = compositionCheckBaselineExecution(t, f, plan.Items[1].RunID, "develop", true); e != nil {
		t.Fatal(e)
	}
}

func TestCandidateCompositionDesignTaskUsesOnlyDurableCompositionRun(t *testing.T) {
	h := newCompositionFixture(t)
	f, ctx := h.f, context.Background()
	d := &designIntegrationFixture{f: f, req: f.call("POST", "/requirements", f.body(map[string]any{"title": "Reviewed integration scope", "acceptance_criteria": []any{"Combine and independently verify fixed accepted sources"}}), 201)}
	adopted := d.adopt(d.publish(designIntegrationContent(), designIntegrationTasks()))
	task := d.task(adopted, "access")
	request := h.req
	request.TaskID = task["id"].(string)
	request.ExpectedTaskVersion = int(task["version"].(float64))
	request.IdempotencyKey = newID()
	p, e := f.s.CreateCandidateComposition(ctx, f.e, f.actor, f.project, request)
	if e != nil || p.Context.Design == nil {
		t.Fatal(p, e)
	}
	p = h.start(t, p)
	p, _ = compositionCompleteFixedCandidate(t, h, p)
	p = compositionAcceptFixedCandidate(t, h, p)
	if p.TaskState != "accepted" || p.Context.Design == nil {
		t.Fatal(p)
	}
}
