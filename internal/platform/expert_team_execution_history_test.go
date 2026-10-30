package platform

import (
	"context"
	"strings"
	"testing"
)

func TestExpertTeamExecutionHistoryCountsAfterPendingRevision(t *testing.T) {
	for _, tc := range []struct {
		name, nodeState, reason, itemState   string
		attempt, waiting, executing, blocked int
	}{
		{"queued", "", "", "queued", 0, 2, 0, 0},
		{"waiting", "waiting", "approval_required", "waiting_execution", 0, 2, 0, 0},
		{"executing", "running", "", "executing", 1, 1, 1, 0},
		{"unknown", "future_state", "", "execution_unknown", 1, 1, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, p := orchestrationFixture(t)
			f, ctx := h.f, context.Background()
			run := p.Items[0].RunID
			// Only exercise projection classification with synthetic node states; this
			// fixture is not evidence of worker execution or vendor protocol support.
			if tc.nodeState != "" {
				if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state=?,reason=?,attempt=? WHERE run_id=? AND id='develop'`, tc.nodeState, tc.reason, tc.attempt, run); err != nil {
					t.Fatal(err)
				}
			}
			p, err := f.s.projectExpertTeamPlanFacts(ctx, f.user.TenantID, f.project, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			if p.Items[0].State != tc.itemState {
				t.Fatalf("fixture state %s", p.Items[0].State)
			}
			var original string
			if err := f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plan_revisions WHERE plan_id=? AND revision=1`, p.ID).Scan(&original); err != nil {
				t.Fatal(err)
			}
			p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, expertPlanCommand(h, p, "pause"))
			if err != nil {
				t.Fatal(err)
			}
			changed, err := f.s.Update(ctx, f.actor, f.project, "tasks", h.tasks[1].ID, f.body(map[string]any{"expected_version": h.tasks[1].Version, "description": "Revised pending scope with original execution retained"}))
			if err != nil {
				t.Fatal(err)
			}
			req := expertPlanCommand(h, p, "revise")
			req.ExpectedConfigurationVersion = h.configuration.Version
			req.Items = []ExpertTeamPlanItemRequest{{TaskID: changed.ID, ExpectedTaskVersion: changed.Version, RoleKey: h.role.Key}}
			p, err = f.s.CommandExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID, req)
			if err != nil {
				t.Fatal(err)
			}
			detail, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, p.ID, 2)
			if err != nil {
				t.Fatal(err)
			}
			page, err := f.s.ExpertTeamPlanRevisions(ctx, f.e, f.actor, f.project, p.ID, 20, "")
			if err != nil || len(page.Items) != 2 {
				t.Fatal(page, err)
			}
			for _, history := range []ExpertTeamPlan{detail, page.Items[1]} {
				if history.Revision != 2 || history.Items[0].RunID != run || history.Items[0].State != tc.itemState || history.Items[1].State != "waiting_dependencies" {
					t.Fatalf("revision lost original facts: %+v", history)
				}
				want := ExpertTeamPlanProgress{Total: 2, Waiting: tc.waiting, Executing: tc.executing, Blocked: tc.blocked}
				if history.Progress != want {
					t.Errorf("revision progress disagrees with item states: got %+v want %+v", history.Progress, want)
				}
				for _, item := range history.Items {
					if item.Execution != nil {
						t.Error("history contains dynamic execution observation")
					}
				}
			}
			var first, second, nodeState string
			var attempt int
			if err := f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plan_revisions WHERE plan_id=? AND revision=1`, p.ID).Scan(&first); err != nil {
				t.Fatal(err)
			}
			if err := f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plan_revisions WHERE plan_id=? AND revision=2`, p.ID).Scan(&second); err != nil {
				t.Fatal(err)
			}
			if first != original || strings.Contains(second, `"execution":`) {
				t.Error("revision rewrite or persisted dynamic execution")
			}
			if err := f.s.DB.QueryRow(`SELECT state,attempt FROM dag_nodes WHERE run_id=? AND id='develop'`, run).Scan(&nodeState, &attempt); err != nil {
				t.Fatal(err)
			}
			expectedNode := tc.nodeState
			if expectedNode == "" {
				expectedNode = "ready"
			}
			if nodeState != expectedNode || attempt != tc.attempt || expertCount(t, f, "dag_runs") != 1 {
				t.Fatal("revision or history read altered execution", nodeState, attempt)
			}
		})
	}
}
