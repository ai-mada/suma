package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestExpertTeamDependentPreviewDoesNotCreateRuns(t *testing.T) {
	h := newExpertFixture(t, false)
	f := h.f
	updated, err := f.s.Update(context.Background(), f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": float64(h.tasks[1].Version), "depends_on": []any{h.tasks[0].ID}}))
	if err != nil {
		t.Fatal(err)
	}
	h.tasks[1] = updated
	before := expertCount(t, f, "dag_runs")
	p, err := f.s.CreateExpertTeamPlan(context.Background(), f.e, f.actor, f.project, h.request())
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 || p.Items[0].State != "ready" || p.Items[1].State != "waiting_dependencies" || p.Revision != 1 || len(p.GraphSHA256) != 64 {
		t.Fatalf("invalid dependent preview: %+v", p)
	}
	if expertCount(t, f, "dag_runs") != before || expertCount(t, f, "suma_task_dispatch_plans") != 0 {
		t.Fatal("preview created execution authority")
	}
	for _, item := range p.Items {
		if item.RunID != "" || item.Plan.RunID != "" || item.DispatchPlanID != "" {
			t.Fatal("reserved ID exposed as running work")
		}
	}
}

func expertDependentFixture(t *testing.T) *expertFixture {
	t.Helper()
	h := newExpertFixture(t, false)
	var err error
	h.tasks[1], err = h.f.s.Update(context.Background(), h.f.actor, h.f.project, "tasks", h.tasks[1].ID, h.f.body(map[string]any{"expected_version": h.tasks[1].Version, "depends_on": []any{h.tasks[0].ID}}))
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func expertCompleteProtocol(t *testing.T, h *expertFixture, runID string) {
	t.Helper()
	f := h.f
	profile, err := f.s.GetToolProfile(context.Background(), f.actor, f.project, h.role.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}
	testWorker, testToken := independentReviewWorker(t, f)
	for i := 0; i < 2; i++ {
		worker, token := profile.WorkerID, h.runnerToken
		if i == 1 {
			worker, token = testWorker, testToken
		}
		claimed := f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": worker, "capabilities": toolFixtureCapabilities(t, f), "idempotency_key": newID()}), 200)
		var a dag.Assignment
		raw, _ := json.Marshal(claimed["assignment"])
		if json.Unmarshal(raw, &a) != nil || a.RunID != runID {
			t.Fatalf("unexpected assignment %+v", claimed)
		}
		proof := reviewWorkspaceEvidence("c", a.NodeID == "test")
		if a.CandidateFrom != "" {
			proof["base_commit"], proof["head_commit"] = a.CandidateCommit, a.CandidateCommit
			proof["candidate_from"], proof["candidate_attempt"], proof["candidate_fence"], proof["candidate_evidence_sha256"] = a.CandidateFrom, a.CandidateAttempt, a.CandidateFence, a.CandidateEvidenceSHA256
		}
		result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), WorkspaceEvidence: proof}
		if a.NodeID == "develop" {
			result.ToolEvidence = &toolcontract.Evidence{Profile: profile.Profile, Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}
		}
		f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), 200)
	}
}

func TestExpertTeamDependentDispatchWaitsForFormalReview(t *testing.T) {
	h := expertDependentFixture(t)
	f, ctx := h.f, context.Background()
	preview := h.preview(t)
	request := h.start(preview)
	started, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, request)
	if err != nil {
		t.Fatal(err)
	}
	if started.State != "active" || started.ActivationGeneration != 1 || started.Items[0].RunID == "" || started.Items[1].RunID != "" || expertCount(t, f, "dag_runs") != 1 {
		t.Fatalf("not one root: %+v", started)
	}
	replay, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID, request)
	if err != nil || replay.Items[0].RunID != started.Items[0].RunID || expertCount(t, f, "dag_runs") != 1 {
		t.Fatal(replay, err)
	}
	expertCompleteProtocol(t, h, started.Items[0].RunID)
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	view, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Items[0].State != "awaiting_review" || view.Items[1].RunID != "" || expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("run success bypassed review", view)
	}
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", h.tasks[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "accepted", "reason": "Test-owner formal review of typed protocol fixture, not vendor execution"}), f.e)
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly make only this test-owned controller row due; no task/run or
	// acceptance state is fabricated and no clock sleep is needed.
	if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET next_check_at='' WHERE plan_id=?`, preview.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	view, err = f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Items[0].State != "accepted" || view.Items[1].RunID == "" || view.Items[1].RunID == view.Items[0].RunID || expertCount(t, f, "dag_runs") != 2 {
		t.Fatal("accepted predecessor did not advance", view)
	}
	original, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, preview.ID, 1)
	if err != nil || original.Items[0].RunID != "" || original.GraphSHA256 != preview.GraphSHA256 || len(original.AllowedActions) != 0 {
		t.Fatal("source revision was rewritten", original, err)
	}
}

func expertPlanCommand(h *expertFixture, p ExpertTeamPlan, action string) ExpertTeamPlanCommandRequest {
	return ExpertTeamPlanCommandRequest{ProjectID: h.f.project, ExpectedVersion: p.Version, Action: action, Reason: "Explicit test-owner delivery plan control", Acknowledge: true, IdempotencyKey: newID()}
}

func TestExpertTeamPlanCommandsReviseOnlyPendingStopAndReplay(t *testing.T) {
	h := expertDependentFixture(t)
	f, ctx := h.f, context.Background()
	p := h.preview(t)
	p, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, h.start(p))
	if err != nil {
		t.Fatal(err)
	}
	rootRun := p.Items[0].RunID
	rootSource := expertTeamSourceDigest(p.Items[0])
	initialGraph := p.GraphSHA256
	pause := expertPlanCommand(h, p, "pause")
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, pause)
	if err != nil || p.State != "paused" {
		t.Fatal(p, err)
	}
	changed, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "description": "Explicit newly reviewed scope for the pending task"}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, expertPlanCommand(h, p, "resume"))
	expertStatus(t, err, 409)
	revise := expertPlanCommand(h, p, "revise")
	revise.ExpectedConfigurationVersion = h.configuration.Version
	revise.Items = []ExpertTeamPlanItemRequest{{TaskID: changed.ID, ExpectedTaskVersion: changed.Version, RoleKey: h.role.Key}}
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, revise)
	if err != nil || p.Revision != 2 || p.GraphSHA256 == initialGraph || p.Items[0].RunID != rootRun || expertTeamSourceDigest(p.Items[0]) != rootSource {
		t.Fatal("revision replaced started facts", p, err)
	}
	invalid := expertPlanCommand(h, p, "revise")
	invalid.ExpectedConfigurationVersion = h.configuration.Version
	invalid.Items = []ExpertTeamPlanItemRequest{{TaskID: h.tasks[0].ID, ExpectedTaskVersion: h.tasks[0].Version + 1, RoleKey: h.role.Key}}
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, invalid)
	expertStatus(t, err, 409)
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, expertPlanCommand(h, p, "resume"))
	if err != nil || p.State != "active" {
		t.Fatal(p, err)
	}
	stop := expertPlanCommand(h, p, "stop")
	p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, stop)
	if err != nil || p.State != "stopped" || p.Items[1].State != "stopped" {
		t.Fatal(p, err)
	}
	replayed, err := f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, stop)
	if err != nil || replayed.Version != p.Version {
		t.Fatal(replayed, err)
	}
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, rootRun)
	if err != nil || run.State == "cancelled" || expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("plan stop pretended to cancel a run", run, err)
	}
	history, err := f.s.ExpertTeamPlanRevisions(ctx, f.e, f.actor, f.project, p.ID, 1, "")
	if err != nil || history.Total != 2 || len(history.Items) != 1 || history.NextCursor == "" {
		t.Fatal(history, err)
	}
	_, err = f.s.ListExpertTeamPlans(ctx, f.e, f.actor, f.project, 1, history.NextCursor)
	expertStatus(t, err, 400)
	_, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, expertPlanCommand(h, p, "resume"))
	expertStatus(t, err, 409)
}

func TestExpertTeamPlanPreparedSourceCannotBeReplaced(t *testing.T) {
	for _, mode := range []string{"task-race", "compiled-workflow"} {
		t.Run(mode, func(t *testing.T) {
			h := newExpertFixture(t, false)
			f, ctx := h.f, context.Background()
			p := h.preview(t)
			prepared, items, err := f.s.prepareExpertTeamReady(ctx, f.e, f.actor, f.project, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "task-race" {
				_, err = f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "description": "Concurrent changed source"}))
				if err != nil {
					t.Fatal(err)
				}
			} else {
				items[0].Plan.Workflow.Nodes[0].Command = []string{"injected-command"}
			}
			_, err = f.s.advanceExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, prepared.Version, 0, items, "source-race-"+newID(), nil)
			expertStatus(t, err, 409)
			if expertCount(t, f, "dag_runs") != 0 || expertCount(t, f, "suma_task_dispatch_plans") != 0 || expertCount(t, f, "suma_expert_team_plan_activations") != 0 {
				t.Fatal("changed source left execution effects")
			}
		})
	}
}

func TestExpertTeamPlanExternalUnacceptedDependencyCannotExpandScope(t *testing.T) {
	h := expertDependentFixture(t)
	r := h.request()
	r.Items = r.Items[1:]
	_, err := h.f.s.CreateExpertTeamPlan(context.Background(), h.f.e, h.f.actor, h.f.project, r)
	expertStatus(t, err, 409)
	if expertCount(t, h.f, "suma_expert_team_plans") != 0 || expertCount(t, h.f, "suma_expert_team_plan_revisions") != 0 {
		t.Fatal("foreign scope was implicitly expanded")
	}
}
