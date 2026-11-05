package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func priorityChangeRequest(project, priority string, tasks ...Record) TaskPriorityChangeRequest {
	selections := make([]TaskPriorityChangeSelection, 0, len(tasks))
	for _, task := range tasks {
		selections = append(selections, TaskPriorityChangeSelection{TaskID: task.ID, ExpectedVersion: task.Version})
	}
	return TaskPriorityChangeRequest{ProjectID: project, Priority: priority, Tasks: selections, IdempotencyKey: newID()}
}

func priorityChangeTask(t *testing.T, f *fixture, title, priority string, deps ...string) Record {
	t.Helper()
	dependencies := make([]any, 0, len(deps))
	for _, dep := range deps {
		dependencies = append(dependencies, dep)
	}
	task, err := f.s.Create(context.Background(), f.actor, f.project, "tasks", f.body(map[string]any{
		"title": title, "priority": priority, "depends_on": dependencies,
	}))
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func priorityChangeEffects(t *testing.T, f *fixture) [5]int {
	t.Helper()
	var counts [5]int
	for i, table := range []string{"suma_operations", "suma_activity", "suma_notification_inbox", "dag_runs", "suma_reviews"} {
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&counts[i]); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}

func TestTaskPriorityChangesHTTPPreservesFormalAcceptanceAndOriginalReceipt(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	ctx := context.Background()
	root := priorityChangeTask(t, f, "Custom old priority", "triage-now")
	child := priorityChangeTask(t, f, "Accepted dependency consumer", "high", root.ID)
	accept := func(task Record) Record {
		t.Helper()
		task = h.bindRun(t, task, true)
		out, err := f.s.Review(ctx, f.actor, f.project, task.ID, f.body(map[string]any{
			"expected_version": task.Version, "run_id": task.RunID, "decision": "accepted", "reason": "Verified by an independent test",
		}), f.e)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	root, child = accept(root), accept(child)
	if root.Status != "accepted" || child.Status != "accepted" || root.Data["verified_run_id"] != root.RunID || !reflect.DeepEqual(child.DependsOn, []string{root.ID}) {
		t.Fatal("fixture did not record formal acceptance")
	}
	before := priorityChangeEffects(t, f)
	req := priorityChangeRequest(f.project, "high", child, root)
	ack := f.call("POST", "/task-priority-changes", req, 200)
	if ack["project_id"] != f.project || ack["operation_id"] != req.IdempotencyKey {
		t.Fatal(ack)
	}
	changes := ack["changes"].([]any)
	want := []TaskPriorityChangeReceipt{
		{TaskID: child.ID, PreviousVersion: child.Version, Version: child.Version, PreviousPriority: "high", Priority: "high", Changed: false},
		{TaskID: root.ID, PreviousVersion: root.Version, Version: root.Version + 1, PreviousPriority: "triage-now", Priority: "high", Changed: true},
	}
	var actual []TaskPriorityChangeReceipt
	if err := json.Unmarshal([]byte(stringify(changes)), &actual); err != nil || !reflect.DeepEqual(actual, want) {
		t.Fatal("original selection order or previous-priority evidence changed", changes)
	}
	current, err := f.s.Get(ctx, f.actor, f.project, "tasks", root.ID)
	if err != nil {
		t.Fatal(err)
	}
	expected := root
	expected.Priority, expected.Version, expected.UpdatedAt = "high", root.Version+1, current.UpdatedAt
	if !reflect.DeepEqual(current, expected) {
		t.Fatal("reprioritization changed task acceptance, ownership, or evidence", current, expected)
	}
	currentChild, err := f.s.Get(ctx, f.actor, f.project, "tasks", child.ID)
	if err != nil || !reflect.DeepEqual(currentChild, child) {
		t.Fatal("accepted dependent changed", currentChild, err)
	}
	if after := priorityChangeEffects(t, f); after != [5]int{before[0] + 1, before[1] + 1, before[2], before[3], before[4]} {
		t.Fatal("expected one receipt and one changed-task activity", before, after)
	}

	next := priorityChangeRequest(f.project, "urgent", current)
	f.call("POST", "/task-priority-changes", next, 200)
	if replay := f.call("POST", "/task-priority-changes", req, 200); !reflect.DeepEqual(replay, ack) {
		t.Fatal("recovery returned a later task version or priority", replay, ack)
	}
	req.Priority = "medium"
	errBody := f.call("POST", "/task-priority-changes", req, 409)
	if errBody["error"].(map[string]any)["code"] != "idempotency_conflict" {
		t.Fatal(errBody)
	}

	if _, err = f.s.DB.Exec(`UPDATE suma_tasks SET version=? WHERE id=?`, math.MaxInt32, child.ID); err != nil {
		t.Fatal(err)
	}
	child.Version = math.MaxInt32
	before = priorityChangeEffects(t, f)
	noop := priorityChangeRequest(f.project, "high", child)
	ack = f.call("POST", "/task-priority-changes", noop, 200)
	entry := ack["changes"].([]any)[0].(map[string]any)
	if entry["changed"] != false || entry["version"] != float64(math.MaxInt32) || entry["previous_version"] != float64(math.MaxInt32) {
		t.Fatal(ack)
	}
	if after := priorityChangeEffects(t, f); after != [5]int{before[0] + 1, before[1], before[2], before[3], before[4]} {
		t.Fatal("no-op performed business writes", before, after)
	}
}

func TestTaskPriorityChangesHTTPRefusesAnyStaleOrIneligibleTaskAtomically(t *testing.T) {
	f := newFixture(t)
	first := priorityChangeTask(t, f, "First selected task", "high")
	second := priorityChangeTask(t, f, "Second selected task", "medium")
	other := f.call("POST", "/projects", map[string]any{"title": "Other selection scope", "idempotency_key": newID()}, 201)["id"].(string)
	foreign := f.call("POST", "/tasks", map[string]any{"project_id": other, "title": "Foreign task", "idempotency_key": newID()}, 201)["id"].(string)
	check := func(t *testing.T, req TaskPriorityChangeRequest, status int, code string) {
		t.Helper()
		before := priorityChangeEffects(t, f)
		sub := *f
		sub.t = t
		reply := sub.call("POST", "/task-priority-changes", req, status)
		if code != "" && reply["error"].(map[string]any)["code"] != code {
			t.Fatal(reply)
		}
		current, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", first.ID)
		if err != nil || !reflect.DeepEqual(current, first) {
			t.Fatal("first task was partly changed", current, err)
		}
		if after := priorityChangeEffects(t, f); after != before {
			t.Fatal("refused batch retained side effects", before, after)
		}
	}

	t.Run("stale no-op member", func(t *testing.T) {
		req := priorityChangeRequest(f.project, "medium", first, second)
		req.Tasks[1].ExpectedVersion++
		check(t, req, 409, "version_conflict")
	})
	t.Run("foreign member", func(t *testing.T) {
		req := priorityChangeRequest(f.project, "urgent", first, Record{ID: foreign, Version: 1})
		check(t, req, 409, "version_conflict")
	})
	for _, status := range []string{"archived", "deleted"} {
		t.Run(status, func(t *testing.T) {
			if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status=? WHERE id=?`, status, second.ID); err != nil {
				t.Fatal(err)
			}
			check(t, priorityChangeRequest(f.project, "urgent", first, second), 409, "version_conflict")
		})
	}
	t.Run("version exhausted", func(t *testing.T) {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='draft',version=? WHERE id=?`, math.MaxInt32, second.ID); err != nil {
			t.Fatal(err)
		}
		capped := second
		capped.Version = math.MaxInt32
		check(t, priorityChangeRequest(f.project, "urgent", first, capped), 409, "version_conflict")
	})
	t.Run("later activity write fails", func(t *testing.T) {
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET version=? WHERE id=?`, second.Version, second.ID); err != nil {
			t.Fatal(err)
		}
		trigger := fmt.Sprintf(`CREATE TRIGGER reject_priority_activity BEFORE INSERT ON suma_activity WHEN NEW.object_id='%s' BEGIN SELECT RAISE(ABORT,'fixture activity failure'); END`, second.ID)
		if _, err := f.s.DB.Exec(trigger); err != nil {
			t.Fatal(err)
		}
		check(t, priorityChangeRequest(f.project, "urgent", first, second), 500, "")
		current, err := f.s.Get(context.Background(), f.actor, f.project, "tasks", second.ID)
		if err != nil || current.Version != second.Version || current.Priority != second.Priority {
			t.Fatal("failure after the second task CAS did not roll back", current, err)
		}
	})
}

func TestTaskPriorityChangesHTTPClosedDTOAndCurrentCaller(t *testing.T) {
	h := newHandoffFixture(t)
	f := h.f
	req := priorityChangeRequest(f.project, "low", h.task)
	raw := stringify(req)
	for _, body := range []string{
		strings.Replace(raw, `"expected_version":1`, `"expected_version":1.0000000000000001`, 1),
		strings.Replace(raw, `"expected_version":1`, `"expected_version":2147483648`, 1),
		strings.Replace(raw, `"expected_version":1`, `"expected_version":null`, 1),
		strings.Replace(raw, `"expected_version":1`, `"ExpectedVersion":1`, 1),
		strings.Replace(raw, `"priority":"low"`, `"priority":"normal"`, 1),
		strings.Replace(raw, `"priority":"low"`, `"priority":"low","priority":"urgent"`, 1),
		strings.Replace(raw, `"priority":"low"`, `"priority":"\uD800"`, 1),
		strings.Replace(raw, `"tasks":[`, `"other":true,"tasks":[`, 1),
		raw + strings.Repeat(" ", taskPriorityChangeBodyLimit),
	} {
		ownerChangeRaw(t, f, "/task-priority-changes", body, 400)
	}
	f.call("POST", "/task-priority-changes?project_id="+f.project, req, 400)
	f.call("GET", "/task-priority-changes", nil, 405)
	for _, tasks := range [][]TaskPriorityChangeSelection{nil, {}, {req.Tasks[0], req.Tasks[0]}, make([]TaskPriorityChangeSelection, 17)} {
		invalid := req
		invalid.Tasks = tasks
		f.call("POST", "/task-priority-changes", invalid, 400)
	}
	token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "api", "label": "Batch authorization test"}), 201)["token"].(string)
	f.bearer("POST", "/task-priority-changes", token, req, 403)

	// This real browser actor can write initially. A later project role change
	// has to stop access even when the previous body and operation ID match.
	out, err := f.s.ChangeTaskPriorities(context.Background(), h.receiver, req)
	if err != nil || len(out.Changes) != 1 {
		t.Fatal(out, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, h.receiver.credential.subjectID); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ChangeTaskPriorities(context.Background(), h.receiver, req)
	handoffWantStatus(t, err, 403)
}
