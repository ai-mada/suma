package platform

import (
	"context"
	"strings"
	"testing"
)

func TestTaskDeliveryHTTPUndispatchedReadOnlySnapshot(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Read the current delivery chain"}), 201)
	beforeRuns, beforeIssues := expertCount(t, f, "dag_runs"), expertCount(t, f, "suma_issues")
	out := f.call("GET", "/tasks/"+task["id"].(string)+"/delivery?project_id="+f.project, nil, 200)
	if out["project_id"] != f.project || out["task_id"] != task["id"] || out["task_version"] != float64(1) || out["run_basis_state"] != "not_dispatched" || out["formal_acceptance"] != false || out["run"] != nil {
		t.Fatal(out)
	}
	if expertCount(t, f, "dag_runs") != beforeRuns || expertCount(t, f, "suma_issues") != beforeIssues {
		t.Fatal("a delivery read created execution or issue state")
	}
	for _, suffix := range []string{"", "?project_id=" + f.project + "&project_id=" + f.project, "?project_id=" + f.project + "&limit=20"} {
		f.call("GET", "/tasks/"+task["id"].(string)+"/delivery"+suffix, nil, 400)
	}
}

func TestExpertTeamCompletedSourceChangeDoesNotPretendAwaitingReview(t *testing.T) {
	h, plan := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	expertCompleteProtocol(t, h, plan.Items[0].RunID)
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", plan.Items[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Update(ctx, f.actor, f.project, "tasks", current.ID, f.body(map[string]any{"title": "Changed after the exact run completed", "expected_version": current.Version})); err != nil {
		t.Fatal(err)
	}
	projection, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range projection.Items {
		if item.TaskID == current.ID && (item.State != "blocked" || len(item.Blockers) == 0) {
			t.Fatalf("changed source presented as reviewable: task=%s state=%s source_version=%d blockers=%v", item.TaskID, item.State, item.TaskVersion, item.Blockers)
		}
	}
}

func TestTaskDeliveryRejectedReviewCreatesOneTrustedIssue(t *testing.T) {
	h, plan := orchestrationFixture(t)
	f, ctx := h.f, context.Background()
	expertCompleteProtocol(t, h, plan.Items[0].RunID)
	task, err := f.s.Get(ctx, f.actor, f.project, "tasks", plan.Items[0].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	view := f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	if view["formal_acceptance"] != false || view["run_basis_state"] != "current" {
		t.Fatal("run success bypassed formal acceptance")
	}
	run := view["run"].(map[string]any)
	if run["tests"].(map[string]any)["succeeded"] != float64(1) || run["nodes"].(map[string]any)["total"] != float64(2) {
		t.Fatal("actual test/run facts missing")
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "accepted", "reason": "Formal acceptance of fixed independent test evidence"}), f.e); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_expert_team_plan_activations SET next_check_at='' WHERE plan_id=?`, plan.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.s.TickExpertTeamPlans(ctx, f.e); err != nil {
		t.Fatal(err)
	}
	advanced, err := f.s.GetExpertTeamPlan(ctx, f.e, f.actor, f.project, plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	dependentRun := advanced.Items[1].RunID
	if dependentRun == "" {
		t.Fatal("formal predecessor acceptance did not unlock the dependent task")
	}
	expertCompleteProtocol(t, h, dependentRun)
	dependent, err := f.s.Get(ctx, f.actor, f.project, "tasks", advanced.Items[1].TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, dependent.ID, f.body(map[string]any{"expected_version": dependent.Version, "run_id": dependentRun, "decision": "accepted", "reason": "Independent downstream acceptance"}), f.e); err != nil {
		t.Fatal(err)
	}
	task, err = f.s.Get(ctx, f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	view = f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	if view["formal_acceptance"] != true {
		t.Fatal("current accepted evidence was not projected")
	}
	// A normal editable issue may mention the run, but is not delivery lineage.
	f.call("POST", "/issues", f.body(map[string]any{"title": "Untrusted metadata reference", "data": map[string]any{"run_id": task.RunID}}), 201)
	before := expertCount(t, f, "suma_issues")
	body := f.body(map[string]any{"expected_version": task.Version, "run_id": task.RunID, "decision": "rejected", "reason": "Independent test passed, but the reviewed behavior does not meet the acceptance requirement"})
	result, err := f.s.Review(ctx, f.actor, f.project, task.ID, body, f.e)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "blocked" || expertCount(t, f, "suma_issues") != before+1 {
		t.Fatalf("rejected review did not create its issue: status=%s issues=%d", result.Status, expertCount(t, f, "suma_issues"))
	}
	if _, err = f.s.Review(ctx, f.actor, f.project, task.ID, body, f.e); err != nil {
		t.Fatal(err)
	}
	if expertCount(t, f, "suma_issues") != before+1 {
		t.Fatal("original review replay duplicated the issue")
	}
	dependent, err = f.s.Get(ctx, f.actor, f.project, "tasks", dependent.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dependent.Status != "in_progress" || dependent.Data["verified_run_id"] != nil {
		t.Fatal("rejected predecessor left downstream acceptance intact")
	}
	view = f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	issues := view["issues"].(map[string]any)
	refs := issues["items"].([]any)
	if view["formal_acceptance"] != false || issues["total"] != float64(1) || len(refs) != 1 {
		t.Fatal("quality feedback or formal invalidation projection missing")
	}
	ref := refs[0].(map[string]any)
	if ref["source"] != "review_rejected" || ref["source_review_id"] == "" {
		t.Fatal("quality issue has no canonical review source")
	}
	requirement, err := f.s.IssueToRequirement(ctx, f.actor, f.project, ref["issue_id"].(string), f.body(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	view = f.call("GET", "/tasks/"+task.ID+"/delivery?project_id="+f.project, nil, 200)
	if view["issues"].(map[string]any)["items"].([]any)[0].(map[string]any)["requirement_id"] != requirement.ID {
		t.Fatal("the current intake requirement did not round-trip")
	}
	encoded := stringify(view)
	if strings.Contains(encoded, `"output":`) || strings.Contains(encoded, `"command":`) || strings.Contains(encoded, `"error_text":`) {
		t.Fatal("delivery summary leaked execution payloads")
	}
	var failedEvents int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=? AND event_type='run_failed'`, task.RunID).Scan(&failedEvents); err != nil || failedEvents != 0 {
		t.Fatal("quality rejection fabricated an execution failure", err)
	}
}
