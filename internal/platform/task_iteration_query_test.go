package platform

import (
	"context"
	"fmt"
	"net/url"
	"testing"
)

func taskIterationSchedule(t *testing.T, h *taskCollectionFixture, index, version int, iteration string) {
	t.Helper()
	r := h.tasks[index]
	_, err := h.f.s.ScheduleTasks(context.Background(), h.f.actor, h.f.project, map[string]any{
		"idempotency_key": newID(), "changes": []any{map[string]any{
			"task_id": r.ID, "expected_task_version": r.Version, "expected_version": version,
			"iteration_id": iteration, "milestone_id": "", "start_date": "", "due_date": "", "estimated_hours": nil,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTaskIterationQueryUsesPlansAndCurrentPlanVersions(t *testing.T) {
	h := newTaskCollectionFixture(t, 6)
	f := h.f
	ctx := context.Background()
	iterations := make([]string, 3)
	for i := range iterations {
		row := f.call("POST", "/iterations", f.body(map[string]any{"title": fmt.Sprintf("Iteration %d", i), "start_date": "2026-10-19", "end_date": "2026-10-25"}), 201)
		iterations[i] = row["id"].(string)
	}
	taskIterationSchedule(t, h, 0, 0, iterations[0])
	taskIterationSchedule(t, h, 1, 0, iterations[0])
	taskIterationSchedule(t, h, 2, 0, iterations[1])
	taskIterationSchedule(t, h, 3, 0, "")
	taskIterationSchedule(t, h, 5, 0, iterations[2]) // Existing fixture has an accepted task at index 5.
	f.call("PATCH", "/iterations/"+iterations[2], f.body(map[string]any{"expected_version": 1, "status": "completed"}), 200)
	// A task payload does not establish a planning relationship.
	f.call("PATCH", "/tasks/"+h.tasks[4].ID, f.body(map[string]any{"expected_version": h.tasks[4].Version, "data": map[string]any{"iteration_id": iterations[0]}}), 200)
	read := func(q TaskWorkspaceQuery) TaskWorkspacePage {
		t.Helper()
		page, err := f.s.ListTaskWorkspace(ctx, f.actor, f.project, q)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.TaskPlans) != len(page.Items) {
			t.Fatalf("missing exact page plan projection: %+v", page)
		}
		for i, plan := range page.TaskPlans {
			if plan.TaskID != page.Items[i].ID {
				t.Fatal("plan order not bound to page tasks")
			}
		}
		return page
	}
	a := read(TaskWorkspaceQuery{IterationID: iterations[0], Limit: 1})
	if a.Total != 2 || len(a.Items) != 1 || a.NextCursor == "" || a.TaskPlans[0].PlanVersion != 1 || a.TaskPlans[0].IterationID != iterations[0] {
		t.Fatalf("wrong scoped first page: %+v", a)
	}
	b := read(TaskWorkspaceQuery{IterationID: iterations[0], Limit: 1, Cursor: a.NextCursor})
	if b.Total != 2 || len(b.Items) != 1 || b.Items[0].ID == a.Items[0].ID || b.NextCursor != "" {
		t.Fatalf("wrong scoped continuation: %+v", b)
	}
	if _, err := f.s.ListTaskWorkspace(ctx, f.actor, f.project, TaskWorkspaceQuery{IterationID: iterations[1], Limit: 1, Cursor: a.NextCursor}); err == nil {
		t.Fatal("iteration cursor accepted changed scope")
	}
	empty := read(TaskWorkspaceQuery{IterationUnset: true})
	if empty.Total != 2 {
		t.Fatalf("empty iteration lost absent or empty plan: %+v", empty)
	}
	versions := map[string]int{}
	for _, p := range empty.TaskPlans {
		versions[p.TaskID] = p.PlanVersion
		if p.IterationID != "" {
			t.Fatal("payload leaked into formal relation")
		}
	}
	if versions[h.tasks[3].ID] != 1 || versions[h.tasks[4].ID] != 0 {
		t.Fatal("missing-plan version conflated with empty iteration")
	}
	if p := read(TaskWorkspaceQuery{IterationID: iterations[2]}); p.Total != 1 {
		t.Fatal("completed iteration disappeared")
	}
	foreignProject := f.call("POST", "/projects", map[string]any{"title": "Foreign", "idempotency_key": newID()}, 201)
	foreign := f.call("POST", "/iterations", map[string]any{"project_id": foreignProject["id"], "title": "Foreign iteration", "start_date": "2026-10-19", "end_date": "2026-10-25", "idempotency_key": newID()}, 201)
	for _, id := range []string{newID(), foreign["id"].(string)} {
		if p := read(TaskWorkspaceQuery{IterationID: id}); p.Total != 0 || len(p.Items) != 0 || len(p.TaskPlans) != 0 {
			t.Fatal("unknown/foreign iteration broadened scope")
		}
	}
	taskIterationSchedule(t, h, 0, 1, iterations[1])
	changed := read(TaskWorkspaceQuery{IterationID: iterations[1]})
	if changed.Total != 2 {
		t.Fatal("rescheduled task missing from current iteration")
	}
	for i, p := range changed.TaskPlans {
		if p.TaskID == h.tasks[0].ID && (p.PlanVersion != 2 || changed.Items[i].Version != h.tasks[0].Version) {
			t.Fatal("plan revision was substituted with task version")
		}
	}
}

func TestTaskIterationGroupPagesAndHTTPRejectInvalidScope(t *testing.T) {
	h := newTaskCollectionFixture(t, 4)
	f := h.f
	iteration := f.call("POST", "/iterations", f.body(map[string]any{"title": "Group iteration", "start_date": "2026-10-19", "end_date": "2026-10-25"}), 201)["id"].(string)
	taskIterationSchedule(t, h, 0, 0, iteration)
	taskIterationSchedule(t, h, 1, 0, iteration)
	taskIterationSchedule(t, h, 2, 0, "")
	base := "/task-collection/summary?project_id=" + f.project + "&group_by=iteration&group_limit=1"
	first := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", base, nil, 200))
	if first.Total != 4 || first.GroupsTotal != 2 || len(first.Groups) != 1 || first.Groups[0].Key != "" || first.Groups[0].Total != 2 || first.NextGroupCursor == "" {
		t.Fatalf("wrong empty group or complete count: %+v", first)
	}
	second := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", base+"&group_cursor="+url.QueryEscape(first.NextGroupCursor), nil, 200))
	if len(second.Groups) != 1 || second.Groups[0].Key != iteration || second.Groups[0].Total != 2 || second.NextGroupCursor != "" {
		t.Fatalf("wrong next group: %+v", second)
	}
	f.call("GET", base+"&iteration_unset=true&group_cursor="+url.QueryEscape(first.NextGroupCursor), nil, 400)
	filtered := taskCollectionDecode[TaskCollectionSummary](t, f.call("GET", base+"&iteration_id="+iteration, nil, 200))
	if filtered.Total != 2 || filtered.GroupsTotal != 1 || filtered.Groups[0].Key != iteration {
		t.Fatal("summary and item predicates diverged")
	}
	for _, route := range []string{"/task-workspace", "/task-collection/summary"} {
		for _, suffix := range []string{"&iteration_id=" + iteration + "&iteration_unset=true", "&iteration_id=" + iteration + "&iteration_id=" + iteration, "&iteration_unset=true&iteration_unset=true", "&iteration_unset=1", "&iteration_id=bad%2Fid"} {
			f.call("GET", route+"?project_id="+f.project+suffix, nil, 400)
		}
	}
}

func TestTaskIterationProjectionRechecksCurrentReaderAuthorization(t *testing.T) {
	h := newTaskCollectionFixture(t, 2)
	f := h.f
	ctx := context.Background()
	adminActor := f.actor
	taskIterationSchedule(t, h, 0, 0, "")
	invite := governanceInvite(f, "task-iteration-reader@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, invite, "task-iteration-reader@example.test")
	viewer := domainAActor(t, f.s, f.cookie.Value)
	page, err := f.s.ListTaskWorkspace(ctx, viewer, f.project, TaskWorkspaceQuery{IterationUnset: true, Limit: 1})
	if err != nil || len(page.TaskPlans) != 1 || page.NextCursor == "" {
		t.Fatalf("authorized reader lacks bounded plans: %+v %v", page, err)
	}
	if _, err = f.s.SaveProjectAccess(ctx, adminActor, map[string]any{
		"project_id": f.project, "user_id": viewer.Projection().ID, "action": "remove",
		"expected_version": float64(1), "idempotency_key": newID(),
	}); err != nil {
		t.Fatal(err)
	}
	denied, err := f.s.ListTaskWorkspace(ctx, viewer, f.project, TaskWorkspaceQuery{IterationUnset: true, Limit: 1, Cursor: page.NextCursor})
	requireAuthorityStatus(t, err, 403)
	if len(denied.Items) != 0 || len(denied.TaskPlans) != 0 {
		t.Fatal("revoked reader received task or planning facts")
	}
	_, err = f.s.TaskCollectionSummary(ctx, viewer, f.project, TaskWorkspaceQuery{IterationUnset: true}, TaskCollectionGroupOptions{GroupBy: "iteration"})
	requireAuthorityStatus(t, err, 403)
}
