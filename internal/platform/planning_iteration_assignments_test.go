package platform

import (
	"encoding/json"
	"reflect"
	"testing"
)

const iterationAssignmentPath = "/planning/iteration-assignments"

func assignmentPeriod(f *fixture, title string) map[string]any {
	return f.call("POST", "/iterations", f.body(map[string]any{"title": title, "start_date": "2026-11-02", "end_date": "2026-11-08"}), 201)
}

func assignmentSelection(task map[string]any, planVersion int) map[string]any {
	return map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "expected_plan_version": planVersion}
}

func assignmentBody(f *fixture, target map[string]any, tasks ...map[string]any) map[string]any {
	return f.body(map[string]any{"iteration_id": target["id"], "expected_iteration_version": target["version"], "tasks": tasks})
}

func assignmentPlan(f *fixture, task map[string]any) map[string]any {
	f.t.Helper()
	result := f.call("GET", "/planning?project_id="+f.project+"&task_id="+task["id"].(string), nil, 200)
	return result["tasks"].([]any)[0].(map[string]any)["plan"].(map[string]any)
}

func TestTaskIterationAssignmentPreservesPlanAndReplaysExactResult(t *testing.T) {
	f := newFixture(t)
	origin := assignmentPeriod(f, "Origin iteration")
	target := assignmentPeriod(f, "Target iteration")
	milestone := f.call("POST", "/milestones", f.body(map[string]any{"title": "Release milestone", "target_date": "2026-11-08"}), 201)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Acceptance workflow", "nodes": []any{map[string]any{"id": "test", "type": "test", "command": []string{"test"}}}, "edges": []any{}}), 201)
	accepted, _, _ := f.testedTask(wf, "Accepted work", nil)
	unchanged := f.call("POST", "/tasks", f.body(map[string]any{"title": "Already assigned"}), 201)
	fresh := f.call("POST", "/tasks", f.body(map[string]any{"title": "First plan"}), 201)
	previous := f.planChange(accepted, 0)
	previous["iteration_id"], previous["milestone_id"], previous["estimated_hours"] = origin["id"], milestone["id"], 0.0
	same := f.planChange(unchanged, 0)
	same["iteration_id"] = target["id"]
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{previous, same}}), 200)

	acceptedPath := "/tasks/" + accepted["id"].(string) + "?project_id=" + f.project
	acceptedBefore := f.call("GET", acceptedPath, nil, 200)
	activityBefore := domainACount(t, f.s, "suma_activity")
	body := assignmentBody(f, target, assignmentSelection(fresh, 0), assignmentSelection(unchanged, 1), assignmentSelection(accepted, 1))
	out := f.call("POST", iterationAssignmentPath, body, 200)
	want := map[string]any{
		"project_id": f.project, "operation_id": body["idempotency_key"], "iteration_id": target["id"],
		"changes": []any{
			map[string]any{"task_id": fresh["id"], "plan_version": float64(1), "changed": true},
			map[string]any{"task_id": unchanged["id"], "plan_version": float64(1), "changed": false},
			map[string]any{"task_id": accepted["id"], "plan_version": float64(2), "changed": true},
		},
	}
	if !reflect.DeepEqual(out, want) {
		t.Fatalf("unexpected assignment receipt: %v", out)
	}
	plan := assignmentPlan(f, accepted)
	if plan["version"] != float64(2) || plan["iteration_id"] != target["id"] || plan["milestone_id"] != milestone["id"] ||
		plan["start_date"] != previous["start_date"] || plan["due_date"] != previous["due_date"] || plan["estimated_hours"] != float64(0) {
		t.Fatalf("unexpected plan mutation: %v", plan)
	}
	plan = assignmentPlan(f, fresh)
	if plan["estimated_hours"] != nil || plan["start_date"] != "" || plan["due_date"] != "" || plan["milestone_id"] != "" {
		t.Fatalf("fabricated planning metadata: %v", plan)
	}
	if after := f.call("GET", acceptedPath, nil, 200); !reflect.DeepEqual(after, acceptedBefore) || after["status"] != "accepted" {
		t.Fatalf("acceptance or task payload changed: %v", after)
	}
	if count := domainACount(t, f.s, "suma_activity"); count != activityBefore+2 {
		t.Fatalf("recorded %d new activities, want two changed tasks", count-activityBefore)
	}
	clear := map[string]any{"id": "", "version": 0}
	f.call("POST", iterationAssignmentPath, assignmentBody(f, clear, assignmentSelection(fresh, 1)), 200)
	activityAfter := domainACount(t, f.s, "suma_activity")
	if replay := f.call("POST", iterationAssignmentPath, body, 200); !reflect.DeepEqual(replay, out) {
		t.Fatalf("receipt changed on replay: %v", replay)
	}
	if plan = assignmentPlan(f, fresh); plan["iteration_id"] != "" || plan["version"] != float64(2) || domainACount(t, f.s, "suma_activity") != activityAfter {
		t.Fatalf("replay mutated an already changed plan: %v", plan)
	}
	body["iteration_id"] = origin["id"]
	f.call("POST", iterationAssignmentPath, body, 409)
}

func TestTaskIterationAssignmentRejectsStaleAndClosedScopeAtomically(t *testing.T) {
	f := newFixture(t)
	origin := assignmentPeriod(f, "Completed source")
	target := assignmentPeriod(f, "Open target")
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "No plan"}), 201)
	h := f.call("POST", "/tasks", f.body(map[string]any{"title": "Already assigned"}), 201)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "Complete iteration workflow", "nodes": []any{map[string]any{"id": "test", "type": "test", "command": []string{"test"}}}, "edges": []any{}}), 201)
	accepted, _, _ := f.testedTask(wf, "Completed iteration work", nil)
	previous := f.planChange(accepted, 0)
	previous["iteration_id"] = origin["id"]
	same := f.planChange(h, 0)
	same["iteration_id"] = target["id"]
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{previous, same}}), 200)
	completed := f.call("PATCH", "/iterations/"+origin["id"].(string), f.body(map[string]any{"expected_version": origin["version"], "status": "completed"}), 200)
	milestone := f.call("POST", "/milestones", f.body(map[string]any{"title": "Completed milestone", "target_date": "2026-11-08"}), 201)
	milestoneTask, _, _ := f.testedTask(wf, "Completed milestone work", nil)
	withMilestone := f.planChange(milestoneTask, 0)
	withMilestone["milestone_id"] = milestone["id"]
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{withMilestone}}), 200)
	f.call("PATCH", "/milestones/"+milestone["id"].(string), f.body(map[string]any{"expected_version": milestone["version"], "status": "completed"}), 200)
	other := f.call("POST", "/projects", map[string]any{"title": "Other project", "idempotency_key": newID()}, 201)
	foreign := f.call("POST", "/iterations", map[string]any{"project_id": other["id"], "title": "Other iteration", "start_date": "2026-11-02", "end_date": "2026-11-08", "idempotency_key": newID()}, 201)
	foreignTask := f.call("POST", "/tasks", map[string]any{"project_id": other["id"], "title": "Other task", "idempotency_key": newID()}, 201)
	staleTask := assignmentSelection(h, 1)
	staleTask["expected_task_version"] = 99
	activityBefore := domainACount(t, f.s, "suma_activity")
	operationsBefore := domainACount(t, f.s, "suma_operations")
	for name, body := range map[string]map[string]any{
		"stale task even unchanged": assignmentBody(f, target, assignmentSelection(task, 0), staleTask),
		"stale plan even unchanged": assignmentBody(f, target, assignmentSelection(task, 0), assignmentSelection(h, 0)),
		"stale target":              assignmentBody(f, map[string]any{"id": target["id"], "version": 99}, assignmentSelection(task, 0)),
		"completed source":          assignmentBody(f, target, assignmentSelection(task, 0), assignmentSelection(accepted, 1)),
		"completed target":          assignmentBody(f, completed, assignmentSelection(task, 0)),
		"completed milestone":       assignmentBody(f, target, assignmentSelection(task, 0), assignmentSelection(milestoneTask, 1)),
		"foreign target":            assignmentBody(f, foreign, assignmentSelection(task, 0)),
		"foreign task":              assignmentBody(f, target, assignmentSelection(task, 0), assignmentSelection(foreignTask, 0)),
	} {
		t.Run(name, func(t *testing.T) {
			local := *f
			local.t = t
			local.call("POST", iterationAssignmentPath, body, 409)
		})
	}
	if plan := assignmentPlan(f, task); plan["version"] != float64(0) || domainACount(t, f.s, "suma_activity") != activityBefore || domainACount(t, f.s, "suma_operations") != operationsBefore {
		t.Fatalf("rejected batch left partial state, activity or receipt: %v", plan)
	}

	body := assignmentBody(f, target, assignmentSelection(task, 0))
	f.call("POST", iterationAssignmentPath, body, 200)
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,?)`, f.project, f.user.ID, "viewer"); err != nil {
		t.Fatal(err)
	}
	f.call("POST", iterationAssignmentPath, body, 403)
}

func TestTaskIterationAssignmentRejectsMalformedClosedContract(t *testing.T) {
	f := newFixture(t)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Task"}), 201)
	target := assignmentPeriod(f, "Target")
	for name, mutate := range map[string]func(map[string]any, map[string]any){
		"missing removal version":     func(body, _ map[string]any) { body["iteration_id"] = ""; delete(body, "expected_iteration_version") },
		"wrong removal version":       func(body, _ map[string]any) { body["iteration_id"] = "" },
		"unknown field":               func(body, _ map[string]any) { body["milestone_id"] = "not-accepted" },
		"duplicate task":              func(body, item map[string]any) { body["tasks"] = []any{item, item} },
		"null task list":              func(body, _ map[string]any) { body["tasks"] = nil },
		"missing plan version":        func(_, item map[string]any) { delete(item, "expected_plan_version") },
		"fraction rounded to integer": func(_, item map[string]any) { item["expected_task_version"] = json.Number("1.0000000000000001") },
		"unpaired wire surrogate":     func(body, _ map[string]any) { body["idempotency_key"] = json.RawMessage(`"task-iteration-\ud800"`) },
		"negative plan version":       func(_, item map[string]any) { item["expected_plan_version"] = -1 },
		"overrange version":           func(_, item map[string]any) { item["expected_task_version"] = json.Number("2147483648") },
		"case alias":                  func(_, item map[string]any) { item["Task_ID"] = item["task_id"]; delete(item, "task_id") },
	} {
		t.Run(name, func(t *testing.T) {
			item := assignmentSelection(task, 0)
			body := assignmentBody(f, target, item)
			mutate(body, item)
			local := *f
			local.t = t
			local.call("POST", iterationAssignmentPath, body, 400)
		})
	}
	f.call("POST", iterationAssignmentPath+"?project_id="+newID(), assignmentBody(f, target, assignmentSelection(task, 0)), 400)
	f.call("POST", iterationAssignmentPath+"/", assignmentBody(f, target, assignmentSelection(task, 0)), 404)
}
