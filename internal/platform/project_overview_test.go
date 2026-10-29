package platform

import (
	"context"
	"fmt"
	"net/url"
	"testing"
)

// Seed more than the record page size: dashboard facts must come from the
// authorized database scope, never the browser's first page of tasks.
func seedOverviewTasks(t *testing.T, f *fixture, project string, count int) {
	t.Helper()
	tx, err := f.s.DB.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	for i := 0; i < count; i++ {
		r := recordFrom(f.user, project, map[string]any{"title": fmt.Sprintf("Task %03d", i), "status": "in_review"})
		r.UpdatedAt = "2026-01-01T00:00:00Z"
		if err = f.s.insertRecord(context.Background(), tx, r, "tasks"); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestProjectOverviewWholeScopeAndAttentionPagination(t *testing.T) {
	f := newFixture(t)
	seedOverviewTasks(t, f, f.project, 107)
	out := f.call("GET", "/overview?project_id="+f.project, nil, 200)
	delivery, ok := out["delivery"].(map[string]any)
	if !ok {
		t.Fatal("overview must include authorized delivery facts")
	}
	if delivery["total_tasks"] != float64(107) || delivery["review_tasks"] != float64(107) || delivery["accepted_tasks"] != float64(0) {
		t.Fatalf("wrong full scope: %#v", delivery)
	}
	page := out["attention"].(map[string]any)["review"].(map[string]any)
	if page["total"] != float64(107) || len(page["items"].([]any)) != 6 {
		t.Fatalf("wrong bounded preview %#v", page)
	}
	seen := map[string]bool{}
	for {
		for _, raw := range page["items"].([]any) {
			id := raw.(map[string]any)["id"].(string)
			if seen[id] {
				t.Fatalf("duplicate %s", id)
			}
			seen[id] = true
		}
		cursor := page["next_cursor"].(string)
		if cursor == "" {
			break
		}
		page = f.call("GET", "/overview/attention?project_id="+f.project+"&kind=review&limit=25&cursor="+url.QueryEscape(cursor), nil, 200)
	}
	if len(seen) != 107 {
		t.Fatalf("lost tasks: %d", len(seen))
	}
}

func TestProjectOverviewScopeDatesAndCursorAuthority(t *testing.T) {
	f := newFixture(t)
	seedOverviewTasks(t, f, f.project, 8)
	other := f.call("POST", "/projects", map[string]any{"name": "Other", "idempotency_key": "overview-other"}, 201)["id"].(string)
	seedOverviewTasks(t, f, other, 2)
	page := f.call("GET", "/overview/attention?project_id="+f.project+"&kind=review&limit=1", nil, 200)
	cursor := url.QueryEscape(page["next_cursor"].(string))
	f.call("GET", "/overview/attention?project_id="+other+"&kind=review&cursor="+cursor, nil, 400)
	f.call("GET", "/overview/attention?project_id="+f.project+"&kind=blocked&cursor="+cursor, nil, 400)
	f.call("GET", "/overview/attention?project_id="+f.project+"&kind=review&kind=blocked", nil, 400)
	f.call("GET", "/overview/attention?project_id="+f.project+"&kind=review&limit=51", nil, 400)
	f.call("GET", "/overview/attention?project_id="+f.project+"&kind=not-a-queue", nil, 400)
	f.call("POST", "/auth/logout", map[string]any{}, 200)
	f.call("GET", "/overview/attention?project_id="+f.project+"&kind=review&cursor="+cursor, nil, 401)
}

func TestProjectOverviewOverdueExcludesTerminalTasks(t *testing.T) {
	f := newFixture(t)
	for _, status := range []string{"todo", "accepted", "cancelled"} {
		r := f.call("POST", "/tasks", f.body(map[string]any{"title": status}), 201)
		id := r["id"].(string)
		if _, err := f.s.DB.Exec("UPDATE suma_tasks SET status=? WHERE id=?", status, id); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.DB.Exec("INSERT INTO suma_task_plans(tenant_id,project_id,task_id,version,iteration_id,milestone_id,start_date,due_date,estimated_hours,updated_at) VALUES(?,?,?,1,'','','2001-01-01','2001-01-02',NULL,?)", f.user.TenantID, f.project, id, now()); err != nil {
			t.Fatal(err)
		}
	}
	out := f.call("GET", "/overview?project_id="+f.project, nil, 200)
	delivery, ok := out["delivery"].(map[string]any)
	if !ok {
		t.Fatal("missing delivery")
	}
	if delivery["scope_tasks"] != float64(2) || delivery["cancelled_tasks"] != float64(1) {
		t.Fatalf("wrong scope %#v", delivery)
	}
	page := out["attention"].(map[string]any)["overdue"].(map[string]any)
	if page["total"] != float64(1) || page["date_basis"] != "UTC" {
		t.Fatalf("wrong dates %#v", page)
	}
}

func TestProjectOverviewExactRunFactsAndMilestoneScope(t *testing.T) {
	f := newFixture(t)
	for i, state := range []string{"running", "waiting", "reconciling", "failed", "succeeded"} {
		task := f.call("POST", "/tasks", f.body(map[string]any{"title": state}), 201)
		runID := fmt.Sprintf("overview-run-%d", i)
		if _, err := f.s.DB.Exec(`INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at) VALUES(?,?,?,'fixture',1,'fixture',1,?,?,?)`, runID, f.user.TenantID, f.project, state, now(), now()); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET run_id=? WHERE id=?`, runID, task["id"]); err != nil {
			t.Fatal(err)
		}
	}
	// Unlinked historical runs must not increase current execution counts.
	if _, err := f.s.DB.Exec(`INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at) VALUES('unlinked',?,?,'fixture',1,'fixture',1,'running',?,?)`, f.user.TenantID, f.project, now(), now()); err != nil {
		t.Fatal(err)
	}
	for i, status := range []string{"planned", "completed", "cancelled"} {
		if _, err := f.s.DB.Exec(`INSERT INTO suma_planning_periods(id,tenant_id,project_id,kind,version,title,description,status,start_date,end_date,target_date,goal,created_at,updated_at) VALUES(?,?,?,'milestones',1,'Milestone','',?,'','','2026-12-01','',?,?)`, fmt.Sprintf("mile-%d", i), f.user.TenantID, f.project, status, now(), now()); err != nil {
			t.Fatal(err)
		}
	}
	out := f.call("GET", "/overview?project_id="+f.project, nil, 200)
	d := out["delivery"].(map[string]any)
	for _, key := range []string{"running_tasks", "waiting_tasks", "reconciling_tasks", "blocked_tasks"} {
		if d[key] != float64(1) {
			t.Fatalf("%s: %#v", key, d)
		}
	}
	if d["accepted_tasks"] != float64(0) || d["review_tasks"] != float64(0) {
		t.Fatalf("run success fabricated acceptance %#v", d)
	}
	if out["milestones"].(map[string]any)["total"] != float64(1) {
		t.Fatalf("wrong milestones %#v", out["milestones"])
	}
}
