package platform

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

const testTaskPlanningStatusPath = "/task-planning-status-changes"
const testTaskPlanningStatusPreviewPath = testTaskPlanningStatusPath + "/preview"

func planningStatusRequest(project, status string, tasks ...Record) TaskPlanningStatusChangeRequest {
	selected := make([]TaskPlanningStatusSelection, 0, len(tasks))
	for _, task := range tasks {
		selected = append(selected, TaskPlanningStatusSelection{TaskID: task.ID, ExpectedVersion: task.Version})
	}
	return TaskPlanningStatusChangeRequest{ProjectID: project, Status: status, Tasks: selected, IdempotencyKey: newID()}
}

func planningStatusTask(t *testing.T, f *fixture, title, status string) Record {
	t.Helper()
	task, err := f.s.Create(context.Background(), f.actor, f.project, "tasks", f.body(map[string]any{"title": title, "status": status}))
	if err != nil {
		t.Fatal(err)
	}
	return currentPlanningStatusTask(t, f, task.ID)
}

func currentPlanningStatusTask(t *testing.T, f *fixture, id string) Record {
	t.Helper()
	task, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", id)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

func planningStatusRun(t *testing.T, f *fixture, id, state string) dag.Run {
	t.Helper()
	run, err := f.e.GetRun(context.Background(), f.user.TenantID, f.project, id)
	if err != nil || run.State != state {
		t.Fatalf("run %s has state %q, want %q: %v", id, run.State, state, err)
	}
	return run
}

func finishPlanningStatusRun(t *testing.T, f *fixture, run string, exitCode int, unknown bool) {
	t.Helper()
	ctx := context.Background()
	assignment, err := f.e.Claim(ctx, f.user.TenantID, f.project, "planning-status-fixture-worker", toolFixtureCapabilities(t, f))
	if err != nil || assignment == nil || assignment.RunID != run {
		t.Fatalf("claim must bind the intended run %s: %v, %v", run, assignment, err)
	}
	err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{
		RunID: run, NodeID: assignment.NodeID, Attempt: assignment.Attempt, Fence: assignment.Fence, WorkerID: assignment.WorkerID,
		ExitCode: exitCode, OperationID: newID(), OutcomeUnknown: unknown,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskPlanningStatusesHTTPPreviewProtectsWorkflowAndUnknownRuns(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	create := func(title, status string) Record { return planningStatusTask(t, f, title, status) }

	failed := h.bindRun(t, create("Failed work may be replanned", "todo"), false)
	finishPlanningStatusRun(t, f, failed.RunID, 1, false)
	planningStatusRun(t, f, failed.RunID, "failed")

	cancelled := h.bindRun(t, create("Confirmed cancellation may be replanned", "todo"), false)
	ready := planningStatusRun(t, f, cancelled.RunID, "running")
	stop, err := f.e.Command(ctx, f.user.TenantID, f.project, ready.ID, "cancel", newID(), ready.Version, map[string]any{"reason": "No worker had claimed the run"})
	if err != nil || stop.State != "cancelled" {
		t.Fatalf("expected confirmed cancellation: %v, %v", stop.State, err)
	}

	succeeded := h.bindRun(t, create("Successful execution awaits review", "todo"), true)
	planningStatusRun(t, f, succeeded.RunID, "succeeded")
	accepted := h.bindRun(t, create("Actual human acceptance", "todo"), true)
	accepted, err = f.s.Review(ctx, f.actor, f.project, accepted.ID, f.body(map[string]any{
		"expected_version": accepted.Version, "run_id": accepted.RunID, "decision": "accepted", "reason": "Reviewed independent test",
	}), f.e)
	if err != nil || accepted.Status != "accepted" || accepted.Data["verified_run_id"] != accepted.RunID {
		t.Fatalf("fixture must contain actual formal acceptance: %+v, %v", accepted, err)
	}

	unknown := h.bindRun(t, create("Worker outcome pending reconciliation", "todo"), false)
	finishPlanningStatusRun(t, f, unknown.RunID, 1, true)
	run := planningStatusRun(t, f, unknown.RunID, "running")
	if len(run.Nodes) != 1 || run.Nodes[0].State != "reconciling" {
		t.Fatalf("fixture did not retain the unknown outcome: %+v", run.Nodes)
	}
	active := h.bindRun(t, create("Ready execution", "todo"), false)
	planningStatusRun(t, f, active.RunID, "running")

	missing := create("Dangling run reference", "draft")
	malformed := create("Malformed evidence", "blocked")
	custom := create("Custom workflow status", "external_validation")
	manual := create("Manual plan", "draft")
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET run_id=? WHERE id=?`, newID(), missing.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET data=? WHERE id=?`, `{"verified_run_id":null}`, malformed.ID); err != nil {
		t.Fatal(err)
	}

	before := priorityChangeEffects(t, f)
	tasks := []Record{manual, active, unknown, missing, succeeded, accepted, custom, malformed, failed, cancelled}
	req := planningStatusRequest(f.project, "blocked", tasks...)
	preview := TaskPlanningStatusPreviewRequest{ProjectID: req.ProjectID, Tasks: req.Tasks}
	data := f.call("POST", testTaskPlanningStatusPreviewPath, preview, 200)
	want := []string{"editable", "active_run", "active_run", "unverified_run", "review_required", "workflow_status", "workflow_status", "workflow_status", "editable", "editable"}
	got := data["items"].([]any)
	if data["project_id"] != f.project || len(got) != len(tasks) {
		t.Fatal(data)
	}
	for i, raw := range got {
		item := raw.(map[string]any)
		task := tasks[i]
		if item["task_id"] != task.ID || item["version"] != float64(task.Version) || item["status"] != task.Status || item["eligibility"] != want[i] {
			t.Fatalf("item %d: got %v, want %s", i, item, want[i])
		}
	}
	result := f.call("POST", testTaskPlanningStatusPath, req, 409)
	if result["error"].(map[string]any)["code"] != "task_planning_status_locked" {
		t.Fatal(result)
	}
	if after := priorityChangeEffects(t, f); after != before {
		t.Fatal("preview or rejected command changed persisted facts", before, after)
	}
	if current := currentPlanningStatusTask(t, f, manual.ID); !reflect.DeepEqual(current, manual) {
		t.Fatal("first eligible task was partly updated", current, manual)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET status='blocked' WHERE id=?`, accepted.ID); err != nil {
		t.Fatal(err)
	}
	verification := TaskPlanningStatusPreviewRequest{ProjectID: f.project, Tasks: []TaskPlanningStatusSelection{{TaskID: accepted.ID, ExpectedVersion: accepted.Version}}}
	result = f.call("POST", testTaskPlanningStatusPreviewPath, verification, 200)
	if result["items"].([]any)[0].(map[string]any)["eligibility"] != "workflow_status" {
		t.Fatal("a manual status must not bypass actual verification", result)
	}

	apply := planningStatusRequest(f.project, "cancelled", cancelled, failed)
	f.call("POST", testTaskPlanningStatusPath, apply, 200)
	for _, previous := range []Record{cancelled, failed} {
		current := currentPlanningStatusTask(t, f, previous.ID)
		want := previous
		want.Status, want.Version, want.UpdatedAt = "cancelled", previous.Version+1, current.UpdatedAt
		if !reflect.DeepEqual(current, want) {
			t.Fatal("business edit changed original evidence or run binding", current, want)
		}
	}
	planningStatusRun(t, f, cancelled.RunID, "cancelled")
	planningStatusRun(t, f, failed.RunID, "failed")
}

func TestTaskPlanningStatusesHTTPRechecksRunAndReturnsOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	active := h.bindRun(t, planningStatusTask(t, f, "Original retryable execution", "todo"), false)
	finishPlanningStatusRun(t, f, active.RunID, 1, false)
	failed := planningStatusRun(t, f, active.RunID, "failed")
	todo := planningStatusTask(t, f, "Change planned work", "todo")
	blocked := planningStatusTask(t, f, "Skip unchanged work", "blocked")
	req := planningStatusRequest(f.project, "blocked", todo, active, blocked)
	preview := TaskPlanningStatusPreviewRequest{ProjectID: f.project, Tasks: req.Tasks}
	f.call("POST", testTaskPlanningStatusPreviewPath, preview, 200)
	retry, err := f.e.Command(context.Background(), f.user.TenantID, f.project, failed.ID, "retry", newID(), failed.Version, map[string]any{"node_id": failed.Nodes[0].ID})
	if err != nil || retry.State != "running" {
		t.Fatalf("fixture failed to start original retry: %v, %v", retry.State, err)
	}
	before := priorityChangeEffects(t, f)
	result := f.call("POST", testTaskPlanningStatusPath, req, 409)
	if result["error"].(map[string]any)["code"] != "task_planning_status_locked" || currentPlanningStatusTask(t, f, active.ID).Version != active.Version {
		t.Fatal("expected refusal from current execution, without task-version change", result)
	}
	if current := currentPlanningStatusTask(t, f, todo.ID); !reflect.DeepEqual(current, todo) || priorityChangeEffects(t, f) != before {
		t.Fatal("rejected transaction changed another task", current)
	}

	stale := planningStatusRequest(f.project, "blocked", todo, blocked)
	stale.Tasks[1].ExpectedVersion++
	result = f.call("POST", testTaskPlanningStatusPath, stale, 409)
	if result["error"].(map[string]any)["code"] != "version_conflict" || priorityChangeEffects(t, f) != before {
		t.Fatal("stale no-op selection should reject the entire batch", result)
	}
	req = planningStatusRequest(f.project, "blocked", todo, blocked)
	ack := f.call("POST", testTaskPlanningStatusPath, req, 200)
	want := []TaskPlanningStatusChangeReceipt{
		{TaskID: todo.ID, PreviousVersion: todo.Version, Version: todo.Version + 1, PreviousStatus: "todo", Status: "blocked", Changed: true},
		{TaskID: blocked.ID, PreviousVersion: blocked.Version, Version: blocked.Version, PreviousStatus: "blocked", Status: "blocked", Changed: false},
	}
	var got []TaskPlanningStatusChangeReceipt
	if err = json.Unmarshal([]byte(stringify(ack["changes"])), &got); err != nil || !reflect.DeepEqual(got, want) || ack["operation_id"] != req.IdempotencyKey {
		t.Fatal("receipt did not retain request order and unchanged rows", ack, err)
	}
	if after := priorityChangeEffects(t, f); after != [5]int{before[0] + 1, before[1] + 1, before[2], before[3], before[4]} {
		t.Fatal("expected exactly one task change and one receipt", before, after)
	}
	current := currentPlanningStatusTask(t, f, todo.ID)
	f.call("POST", testTaskPlanningStatusPath, planningStatusRequest(f.project, "draft", current), 200)
	if replay := f.call("POST", testTaskPlanningStatusPath, req, 200); !reflect.DeepEqual(replay, ack) {
		t.Fatal("replay replaced the original response with later task state", replay)
	}
	req.Status = "in_progress"
	result = f.call("POST", testTaskPlanningStatusPath, req, 409)
	if result["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatal(result)
	}
}

func TestTaskPlanningStatusesHTTPCloseDTOAndEnforceCurrentAuthority(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	req := planningStatusRequest(f.project, "draft", h.task)
	preview := TaskPlanningStatusPreviewRequest{ProjectID: req.ProjectID, Tasks: req.Tasks}
	raw := stringify(req)
	for _, body := range []string{
		strings.Replace(raw, `"expected_version":1`, `"expected_version":1.0000000000000001`, 1),
		strings.Replace(raw, `"expected_version":1`, `"expected_version":null`, 1),
		strings.Replace(raw, `"status":"draft"`, `"status":"accepted"`, 1),
		strings.Replace(raw, `"status":"draft"`, `"status":"draft","status":"blocked"`, 1),
		strings.Replace(raw, `"tasks":[`, `"other":true,"tasks":[`, 1),
		raw + strings.Repeat(" ", taskPlanningStatusBodyLimit),
	} {
		ownerChangeRaw(t, f, testTaskPlanningStatusPath, body, 400)
	}
	ownerChangeRaw(t, f, testTaskPlanningStatusPreviewPath, raw, 400)
	f.call("POST", testTaskPlanningStatusPreviewPath+"?project_id="+f.project, preview, 400)
	f.call("POST", testTaskPlanningStatusPath+"?project_id="+f.project, req, 400)
	f.call("GET", testTaskPlanningStatusPath, nil, 405)
	duplicate := preview
	duplicate.Tasks = append(duplicate.Tasks, duplicate.Tasks[0])
	f.call("POST", testTaskPlanningStatusPreviewPath, duplicate, 400)

	other := f.call("POST", "/projects", f.body(map[string]any{"title": "Other project"}), 201)["id"].(string)
	foreign := f.call("POST", "/tasks", map[string]any{"project_id": other, "title": "Foreign task", "idempotency_key": newID()}, 201)["id"].(string)
	cross := planningStatusRequest(f.project, "todo", h.task, Record{ID: foreign, Version: 1})
	f.call("POST", testTaskPlanningStatusPreviewPath, TaskPlanningStatusPreviewRequest{ProjectID: f.project, Tasks: cross.Tasks}, 409)
	f.call("POST", testTaskPlanningStatusPath, cross, 409)

	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api", "label": "Planning status authorization test"}), 201)["token"].(string)
	f.bearer("POST", testTaskPlanningStatusPath, token, req, 403)
	f.bearer("POST", testTaskPlanningStatusPreviewPath, token, preview, 403)

	first, err := f.s.ChangeTaskPlanningStatuses(ctx, h.receiver, req)
	if err != nil || len(first.Changes) != 1 {
		t.Fatal(first, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ChangeTaskPlanningStatuses(ctx, h.receiver, req)
	handoffWantStatus(t, err, 403)
	_, err = f.s.PreviewTaskPlanningStatuses(ctx, h.receiver, preview)
	var api *APIError
	if !errors.As(err, &api) || api.Status != 403 {
		t.Fatal("the old authorized actor cannot preview after revocation", err)
	}
}

func TestTaskPlanningStatusesHTTPProtectsActiveCompensation(t *testing.T) {
	f, ctx := newFixture(t), context.Background()
	task := planningStatusTask(t, f, "Task with a compensable failed run", "draft")
	workflow, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{
		Name: "Undo a verified effect before replanning",
		Nodes: []dag.Node{
			{
				ID: "apply", Type: "command", Command: []string{"fixture-apply"}, Idempotent: true,
				Config: map[string]any{"compensation": map[string]any{"type": "command", "command": []any{"fixture-undo"}, "idempotent": true}},
			},
			{ID: "check", Type: "test", Command: []string{"fixture-check"}, DependsOn: []string{"apply"}, Idempotent: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow.ID, "task_id": task.ID, "expected_version": task.Version}), 201)
	task = currentPlanningStatusTask(t, f, task.ID)
	finishPlanningStatusRun(t, f, task.RunID, 0, false)
	finishPlanningStatusRun(t, f, task.RunID, 1, false)
	failed := planningStatusRun(t, f, task.RunID, "failed")
	req := planningStatusRequest(f.project, "blocked", task)
	preview := TaskPlanningStatusPreviewRequest{ProjectID: req.ProjectID, Tasks: req.Tasks}
	eligibility := func(want string) {
		t.Helper()
		reply := f.call("POST", testTaskPlanningStatusPreviewPath, preview, 200)
		if reply["items"].([]any)[0].(map[string]any)["eligibility"] != want {
			t.Fatalf("expected %s, got %v", want, reply)
		}
	}
	eligibility("editable")
	started, err := f.e.Command(ctx, f.user.TenantID, f.project, task.RunID, "compensate", newID(), failed.Version, map[string]any{"reason": "Undo the apply effect before replanning"})
	if err != nil || started.State != "failed" || started.CompensationRunID == "" || started.CompensationState != "running" {
		t.Fatalf("fixture must have an active compensation on the failed run: %+v, %v", started, err)
	}
	eligibility("active_run")
	reply := f.call("POST", testTaskPlanningStatusPath, req, 409)
	if reply["error"].(map[string]any)["code"] != "task_planning_status_locked" {
		t.Fatal(reply)
	}
	finishPlanningStatusRun(t, f, started.CompensationRunID, 0, false)
	planningStatusRun(t, f, started.CompensationRunID, "succeeded")
	eligibility("editable")
	f.call("POST", testTaskPlanningStatusPath, req, 200)
	if current := currentPlanningStatusTask(t, f, task.ID); current.Status != "blocked" || current.RunID != task.RunID {
		t.Fatal(current)
	}
}

func TestTaskPlanningStatusesHTTPProtectsStillLivePriorRun(t *testing.T) {
	h := newHandoffFixture(t)
	f, ctx := h.f, context.Background()
	parallel := schedulingPreview(f, 2, nil)
	f.call("POST", "/scheduling/policy-previews/"+parallel["id"].(string)+"/commit", f.body(map[string]any{"expected_version": 1}), 200)
	task := h.bindRun(t, h.task, false)
	older, err := f.e.Claim(ctx, f.user.TenantID, f.project, "prior-planning-status-worker", toolFixtureCapabilities(t, f))
	if err != nil || older == nil || older.RunID != task.RunID {
		t.Fatalf("prior run should own a live worker attempt: %+v, %v", older, err)
	}
	current := h.bindRun(t, task, false)
	if current.RunID == older.RunID {
		t.Fatal("fixture should rebind the task to a distinct execution")
	}
	finishPlanningStatusRun(t, f, current.RunID, 1, false)
	planningStatusRun(t, f, current.RunID, "failed")
	planningStatusRun(t, f, older.RunID, "running")

	req := planningStatusRequest(f.project, "blocked", current)
	preview := TaskPlanningStatusPreviewRequest{ProjectID: req.ProjectID, Tasks: req.Tasks}
	eligibility := func(want string) {
		t.Helper()
		reply := f.call("POST", testTaskPlanningStatusPreviewPath, preview, 200)
		if reply["items"].([]any)[0].(map[string]any)["eligibility"] != want {
			t.Fatalf("expected %s, got %v", want, reply)
		}
	}
	eligibility("active_run")
	locked := f.call("POST", testTaskPlanningStatusPath, req, 409)
	if locked["error"].(map[string]any)["code"] != "task_planning_status_locked" {
		t.Fatal(locked)
	}
	if got := currentPlanningStatusTask(t, f, current.ID); !reflect.DeepEqual(got, current) {
		t.Fatal("previous execution must prevent writing a new planning status", got, current)
	}

	err = f.e.Complete(ctx, f.user.TenantID, f.project, dag.Result{
		RunID: older.RunID, NodeID: older.NodeID, Attempt: older.Attempt, Fence: older.Fence, WorkerID: older.WorkerID,
		ExitCode: 0, OperationID: newID(), Output: "Original execution ended",
	})
	if err != nil {
		t.Fatal(err)
	}
	planningStatusRun(t, f, older.RunID, "succeeded")
	eligibility("editable")
	f.call("POST", testTaskPlanningStatusPath, req, 200)
	got := currentPlanningStatusTask(t, f, current.ID)
	if got.Status != "blocked" || got.Version != current.Version+1 || got.RunID != current.RunID {
		t.Fatal(got)
	}
	planningStatusRun(t, f, current.RunID, "failed")
	planningStatusRun(t, f, older.RunID, "succeeded")
}
