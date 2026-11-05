package platform

import (
	"context"
	"fmt"
	"strings"
	"testing"

	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
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

func TestTaskDeliveryHTTPWorkBuddyTraceUsesExactCurrentAttemptBeyondNodeLimit(t *testing.T) {
	h := newWorkBuddyTaskDispatchFixture(t)
	f, ctx := h.f, context.Background()
	cloud := &cloudFixture{f: f, w: h.w}
	cloud.claimToken = f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "cloud-worker", "profiles": []any{}, "deployment": false}), 201)["token"].(string)
	var err error
	if cloud.actor, err = f.s.AuthenticateActor(ctx, cloud.claimToken); err != nil {
		t.Fatal(err)
	}
	plan := workBuddyTaskPreview(t, h)
	run := workBuddyTaskStart(t, h, plan, newID())
	view, err := h.w.GetCloudNode(ctx, f.actor, f.project, run.ID, plan.NodeID)
	if err != nil || view.Delegation == nil {
		t.Fatal("missing original delegation", err)
	}
	cloud.delegation = *view.Delegation
	claim := cloudHTTPClaim(t, cloud, false, newID(), 200)
	if claim == nil || claim.RunID != run.ID || claim.NodeID != plan.NodeID {
		t.Fatal("missing original claim")
	}
	cloud.request = wc.CloudCreateRequest{ProjectID: f.project, DelegationID: plan.DelegationID, ExpectedVersion: 1, Assignment: repositoryTestRef(*claim), IdempotencyKey: newID()}
	sub := createCloudFixture(t, cloud)
	a, result := cloudResultFixture(t, cloud)
	result.WorkBuddyCloudWait.SubmissionID, result.WorkBuddyCloudWait.SubmissionVersion = sub.ID, sub.Version
	if err = completeCloudFixture(t, cloud, a, result); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := f.s.DB.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 21 {
		exec(`INSERT INTO dag_nodes(run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec) VALUES(?,?,'waiting','cloud.delivery_required',1,1,'','','','','',0,0,'',?)`, run.ID, fmt.Sprintf("0-unrelated-%02d", i), `{"name":"Unrelated node","type":"command"}`)
	}
	path := "/tasks/" + h.task.ID + "/delivery?project_id=" + f.project
	beforeCreates, beforeQueries := h.creates.Load(), h.queries.Load()
	beforeEvents := expertCount(t, f, "dag_events")
	read := func() map[string]any {
		t.Helper()
		out := f.call("GET", path, nil, 200)
		if out["formal_acceptance"] != false {
			t.Fatal("cloud delivery waiting appeared as formal acceptance")
		}
		return out["run"].(map[string]any)
	}
	projected := read()
	nodes := projected["nodes"].(map[string]any)
	if nodes["total"] != float64(22) || nodes["complete"] != false || len(nodes["items"].([]any)) != 20 {
		t.Fatalf("node limit fixture: %#v", nodes)
	}
	for _, item := range nodes["items"].([]any) {
		if item.(map[string]any)["id"] == plan.NodeID {
			t.Fatal("the cloud node must be beyond the bounded list")
		}
	}
	target, ok := projected["workbuddy_delivery"].(map[string]any)
	if !ok || len(target) != 3 || target["run_id"] != run.ID || target["node_id"] != plan.NodeID || target["attempt"] != float64(1) {
		t.Fatalf("expected a compact exact target beyond the node limit, got %#v", projected["workbuddy_delivery"])
	}

	var originalSpec string
	if err := f.s.DB.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=? AND id=?`, run.ID, plan.NodeID).Scan(&originalSpec); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, column   string
		value, restore any
	}{
		{"other wait marker", "reason", "cloud.waiting_input", "cloud.delivery_required"},
		{"other node state", "state", "succeeded", "waiting"},
		{"other node type", "spec", strings.Replace(originalSpec, `"type":"external_workflow"`, `"type":"command"`, 1), originalSpec},
		{"other node protocol", "spec", strings.Replace(originalSpec, `"workflow":"workbuddy-cloud/v1"`, `"workflow":"workbuddy-cloud/v2"`, 1), originalSpec},
		{"attempt not started", "attempt", 0, a.Attempt},
		{"assignment for older attempt", "attempt", a.Attempt + 1, a.Attempt},
		{"assignment fence differs", "fence", a.Fence + 1, a.Fence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			query := `UPDATE dag_nodes SET ` + tc.column + `=? WHERE run_id=? AND id=?`
			exec(query, tc.value, run.ID, plan.NodeID)
			defer exec(query, tc.restore, run.ID, plan.NodeID)
			if value, exists := read()["workbuddy_delivery"]; exists {
				t.Fatalf("unverified target was projected: %#v", value)
			}
		})
	}
	exec(`INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at) VALUES(?,?,?,'fixture',1,'fixture',1,'running',?,?)`, "current-replacement", f.user.TenantID, f.project, now(), now())
	exec(`UPDATE suma_tasks SET run_id='current-replacement' WHERE id=?`, h.task.ID)
	if current := read(); current["run_id"] != "current-replacement" || current["workbuddy_delivery"] != nil {
		t.Fatalf("old cloud execution escaped current task binding: %#v", current)
	}
	if h.creates.Load() != beforeCreates || h.queries.Load() != beforeQueries || expertCount(t, f, "dag_events") != beforeEvents {
		t.Fatal("delivery reads contacted the provider or created execution events")
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
