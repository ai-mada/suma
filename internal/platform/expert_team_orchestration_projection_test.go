package platform

import (
	"context"
	"fmt"
	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	"testing"
)

func TestExpertTeamOrchestrationBoundedFairDueAndNoopVersion(t *testing.T) {
	h := newExpertFixture(t, false)
	f, ctx := h.f, context.Background()
	ids := []string{}
	for i := 0; i < expertTeamTickPlans+1; i++ {
		tasks := []Record{}
		for j := 0; j < 2; j++ {
			body := map[string]any{"title": fmt.Sprintf("Fair delivery %d/%d", i, j)}
			if j == 1 {
				body["depends_on"] = []any{tasks[0].ID}
			}
			task, err := f.s.Create(ctx, f.actor, f.project, "tasks", f.body(body))
			if err != nil {
				t.Fatal(err)
			}
			tasks = append(tasks, task)
		}
		h.tasks = tasks
		p := h.preview(t)
		p, err := f.s.StartExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, h.start(p))
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.s.projectExpertTeamPlanFacts(ctx, f.user.TenantID, f.project, p.ID); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, p.ID)
		if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET next_check_at=?,last_checked_at='' WHERE plan_id=?`, fmt.Sprintf("2000-01-01T00:00:%02dZ", i), p.ID); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	var checked int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_expert_team_plan_activations WHERE last_checked_at<>''`).Scan(&checked); err != nil || checked != expertTeamTickPlans {
		t.Fatal("tick did not respect plan scan bound", checked, err)
	}
	var last string
	if err := f.s.DB.QueryRow(`SELECT last_checked_at FROM suma_expert_team_plan_activations WHERE plan_id=?`, ids[len(ids)-1]).Scan(&last); err != nil || last != "" {
		t.Fatal("due ordering skipped an older head", err)
	}
	if err := f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT last_checked_at FROM suma_expert_team_plan_activations WHERE plan_id=?`, ids[len(ids)-1]).Scan(&last); err != nil || last == "" {
		t.Fatal("waiting heads starved untouched plan", err)
	}
	var before, after int
	if err := f.s.DB.QueryRow(`SELECT version FROM suma_expert_team_plans WHERE id=?`, ids[0]).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET next_check_at='2000-01-01T00:00:00Z' WHERE plan_id=?`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	if err := f.s.DB.QueryRow(`SELECT version FROM suma_expert_team_plans WHERE id=?`, ids[0]).Scan(&after); err != nil || after != before {
		t.Fatal("no-op controller check consumed user CAS version", before, after, err)
	}
	if expertCount(t, f, "dag_runs") != len(ids) {
		t.Fatal("waiting heads created a run or paid execution")
	}
}

func TestExpertTeamOrchestrationReconcilingNodeProjection(t *testing.T) {
	h, plan := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	profile, err := f.s.GetToolProfile(ctx, f.actor, f.project, h.role.ToolProfileID)
	if err != nil {
		t.Fatal(err)
	}
	assignment, err := f.e.Claim(ctx, f.user.TenantID, f.project, "tool-worker", []string{"command:" + profile.Profile.CommandID})
	if err != nil || assignment == nil || assignment.RunID != plan.Items[0].RunID {
		t.Fatal("missing exact plan root assignment", assignment, err)
	}
	// A genuine Core unknown receipt leaves the root run running while its code
	// node needs reconciliation and the independent test remains pending.
	unknown := dag.Result{RunID: assignment.RunID, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID, OperationID: newID(), ExitCode: -1, OutcomeUnknown: true, Error: "injected handshake failure before known process outcome", ToolEvidence: &tc.Evidence{Profile: profile.Profile, Terminal: "unknown", EventCount: 0, UsageState: tc.UsageUnreported}}
	if err = f.e.Complete(ctx, f.user.TenantID, f.project, unknown); err != nil {
		t.Fatal(err)
	}
	run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, assignment.RunID)
	if err != nil || run.State != "running" {
		t.Fatal("fixture must retain aggregate running fact", run.State, err)
	}
	states := map[string]string{}
	for _, node := range run.Nodes {
		states[node.ID] = node.State
	}
	if states[assignment.NodeID] != "reconciling" || states["test"] != "pending" {
		t.Fatal("fixture did not produce mixed original node facts", states)
	}
	current := orchestrationCurrent(t, h, plan.ID)
	if current.Items[0].State != "blocked" || current.Items[0].RunState != "running" || current.Items[0].RunID != assignment.RunID || len(current.Items[0].Blockers) != 1 || current.Items[0].Blockers[0] != expertRunBlocked || current.Progress.Blocked != 1 || current.Progress.Executing != 0 {
		t.Fatal("reconciling node hidden by aggregate running state", current.Items[0], current.Progress)
	}
	orchestrationDue(t, h)
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	after, err := f.e.GetRun(ctx, f.user.TenantID, f.project, assignment.RunID)
	if err != nil || after.State != run.State || after.Nodes[0].State != run.Nodes[0].State || expertCount(t, f, "dag_runs") != 1 {
		t.Fatal("progress projection changed/retried actual execution", after, err)
	}
}
