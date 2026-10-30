package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestExpertTeamExecutionProjectionCurrentStates(t *testing.T) {
	h, p := orchestrationFixture(t)
	f := h.f
	run := p.Items[0].RunID
	for _, tc := range []struct {
		name, state, reason, runState, want string
		attempt                             int
		executing, waiting, blocked         int
	}{
		{"queued", "ready", "", "running", "queued", 0, 0, 2, 0},
		{"historical_attempt", "pending", "", "running", "waiting_execution", 2, 0, 2, 0},
		{"budget_wait", "ready", "Model token or cost allowance is unavailable", "running", "waiting_execution", 0, 0, 2, 0},
		{"approval_wait", "waiting", "approval_required", "running", "waiting_execution", 0, 0, 2, 0},
		{"actual_running", "running", "", "running", "executing", 1, 1, 1, 0},
		{"cancelling", "running", "", "cancelling", "blocked", 1, 0, 1, 1},
		{"unknown_node", "future_state", "", "succeeded", "execution_unknown", 1, 0, 1, 1},
		{"unknown_run", "ready", "", "future_state", "execution_unknown", 0, 0, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state=?,reason=?,attempt=? WHERE run_id=? AND id='develop'`, tc.state, tc.reason, tc.attempt, run); err != nil {
				t.Fatal(err)
			}
			if _, err := f.s.DB.Exec(`UPDATE dag_runs SET state=? WHERE id=?`, tc.runState, run); err != nil {
				t.Fatal(err)
			}
			view := orchestrationCurrent(t, h, p.ID)
			item := view.Items[0]
			if item.State != tc.want || view.Progress.Executing != tc.executing || view.Progress.Waiting != tc.waiting || view.Progress.Blocked != tc.blocked {
				t.Fatal(item.State, view.Progress)
			}
			if item.Execution == nil || item.Execution.RunID != run || item.Execution.ObservedAt == "" || item.Execution.NodesTotal != 2 || item.Execution.Truncated || view.Items[1].Execution != nil {
				t.Fatal("incorrect exact execution projection", item.Execution)
			}
			found := false
			for _, n := range item.Execution.Nodes {
				if n.NodeID == "develop" {
					found = true
					if n.State != tc.state || n.Attempt != tc.attempt || n.Reason != tc.reason {
						t.Fatal(n)
					}
				}
			}
			if !found {
				t.Fatal("exact node missing")
			}
			var stored string
			if err := f.s.DB.QueryRow(`SELECT state FROM dag_nodes WHERE run_id=? AND id='develop'`, run).Scan(&stored); err != nil || stored != tc.state {
				t.Fatal("read changed DAG", err)
			}
		})
	}
}

func TestExpertTeamExecutionProjectionBoundedFullFactsAndFields(t *testing.T) {
	h, p := orchestrationFixture(t)
	f := h.f
	run := p.Items[0].RunID
	for i := 0; i < 30; i++ {
		state := "running"
		if i == 29 {
			state = "future_state"
		}
		spec := fmt.Sprintf(`{"id":"node-%02d","name":"Step %d","type":"command","command":["private-command"],"config":{"secret":"private-secret"}}`, i, i)
		_, err := f.s.DB.Exec(`INSERT INTO dag_nodes(run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, run, fmt.Sprintf("node-%02d", i), state, "", 1, 1, "worker-one", "private-output", "private-error", "", "", 0, 0, "", spec)
		if err != nil {
			t.Fatal(err)
		}
	}
	view := orchestrationCurrent(t, h, p.ID)
	fact := view.Items[0].Execution
	if fact == nil || fact.NodesTotal != 32 || len(fact.Nodes) != 24 || !fact.Truncated || view.Items[0].State != "execution_unknown" || view.Progress.Executing != 0 {
		t.Fatal("bounded sample affected full fact", view.Items[0])
	}
	for _, n := range fact.Nodes {
		if n.State != "running" && n.State != "future_state" {
			t.Fatal("priority or truncation wrong", n)
		}
	}
	raw, _ := json.Marshal(fact)
	for _, secret := range []string{"private-command", "private-secret", "private-output", "private-error", "\"config\":", "\"command\":", "\"output\":"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("execution field leak", secret)
		}
	}
	if fact.Nodes[0].NodeID != "node-29" {
		t.Fatal("unknown state hidden", fact.Nodes[0])
	}
	// Running work outside the displayed waiting-node page still determines state.
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state='waiting' WHERE run_id=?`, run); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state='running' WHERE run_id=? AND id='node-29'`, run); err != nil {
		t.Fatal(err)
	}
	view = orchestrationCurrent(t, h, p.ID)
	if view.Items[0].State != "executing" {
		t.Fatal("classification used only displayed waiting page")
	}
	for _, n := range view.Items[0].Execution.Nodes {
		if n.State != "waiting" {
			t.Fatal("fixture did not truncate active step", n)
		}
	}
	// Prioritize a reconciliation even when its stable ID sorts after all work.
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state='reconciling' WHERE run_id=? AND id='node-29'`, run); err != nil {
		t.Fatal(err)
	}
	view = orchestrationCurrent(t, h, p.ID)
	if view.Items[0].State != "blocked" || view.Items[0].Execution.Nodes[0].NodeID != "node-29" {
		t.Fatal("reconciliation hidden", view.Items[0])
	}
}

func TestExpertTeamExecutionProjectionReadOnlyHistoryAndAuthorization(t *testing.T) {
	h, p := orchestrationFixture(t)
	f := h.f
	ctx := context.Background()
	// Normalize controller-derived state once, then reads must never consume CAS.
	baseline, err := f.s.projectExpertTeamPlanFacts(ctx, f.user.TenantID, f.project, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeRuns, beforeNodes := expertCount(t, f, "dag_runs"), expertCount(t, f, "dag_nodes")
	first := orchestrationCurrent(t, h, p.ID)
	second := orchestrationCurrent(t, h, p.ID)
	if first.Items[0].Execution == nil || second.Version != baseline.Version {
		t.Fatal("read snapshot missing or changed CAS")
	}
	after, err := f.s.projectExpertTeamPlanFacts(ctx, f.user.TenantID, f.project, p.ID)
	if err != nil || after.Version != baseline.Version || after.Items[0].Execution != nil || expertCount(t, f, "dag_runs") != beforeRuns || expertCount(t, f, "dag_nodes") != beforeNodes {
		t.Fatal("projection persisted evidence or changed execution", err)
	}
	var raw string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_expert_team_plans WHERE id=?`, p.ID).Scan(&raw); err != nil || strings.Contains(raw, `"execution":`) {
		t.Fatal("dynamic facts persisted", err)
	}
	// History remains a source snapshot even if a stored payload contains a stale read cache.
	if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_revisions SET payload=? WHERE plan_id=? AND revision=1`, stringify(first), p.ID); err != nil {
		t.Fatal(err)
	}
	revision, err := f.s.GetExpertTeamPlanRevision(ctx, f.e, f.actor, f.project, p.ID, 1)
	if err != nil || revision.Items[0].Execution != nil {
		t.Fatal("history leaked cached execution", err)
	}
	history, err := f.s.ExpertTeamPlanRevisions(ctx, f.e, f.actor, f.project, p.ID, 20, "")
	if err != nil || len(history.Items) == 0 || history.Items[0].Items[0].Execution != nil {
		t.Fatal("history list leaked cached execution", err)
	}
	if _, err = f.s.GetExpertTeamPlan(ctx, f.e, f.actor, newID(), p.ID); err == nil {
		t.Fatal("cross-project read authorized")
	}
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	if _, err = f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, p.ID); err == nil {
		t.Fatal("revoked actor read execution facts")
	}
}

func TestExpertTeamExecutionProjectionPreservesFormalAcceptanceWithTerminalBranches(t *testing.T) {
	h, p := orchestrationFixture(t)
	f := h.f
	run := p.Items[0].RunID
	orchestrationCompleteRoot(t, h, p)
	orchestrationAcceptRoot(t, h, p)
	// The projection must respect Core's aggregate and formal acceptance. Extra
	// terminal branch rows model an already-settled quorum/conditional branch;
	// this is a projection regression, not an execution-engine acceptance test.
	for _, state := range []string{"failed", "cancelled", "skipped"} {
		_, err := f.s.DB.Exec(`INSERT INTO dag_nodes(run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, run, "optional-"+state, state, "optional branch settled", 1, 1, "", "", "", "", "", 0, 0, "", `{"name":"Optional branch","type":"command"}`)
		if err != nil {
			t.Fatal(err)
		}
	}
	view := orchestrationCurrent(t, h, p.ID)
	if view.Items[0].State != "accepted" || view.Progress.Accepted != 1 || view.Items[0].Execution.NodesTotal != 5 {
		t.Fatal("terminal branches overwrote formal acceptance", view.Items[0], view.Progress)
	}
	// Unknown evidence is different: do not advertise completed delivery while
	// the same exact run contains an unrecognized state.
	if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET state='future_state' WHERE run_id=? AND id='optional-failed'`, run); err != nil {
		t.Fatal(err)
	}
	view = orchestrationCurrent(t, h, p.ID)
	if view.Items[0].State != "execution_unknown" || view.Progress.Accepted != 0 {
		t.Fatal("unknown evidence hidden by accepted task", view.Items[0])
	}
	task, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", p.Items[0].TaskID)
	if err != nil || task.Status != "accepted" {
		t.Fatal("read changed formal acceptance", err)
	}
}
