package platform

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

const scheduleOverlapTestWeek = "2026-11-02"

func scheduleOverlapMember(f *fixture, project, user, title string) string {
	f.t.Helper()
	return f.call("POST", "/members", map[string]any{
		"project_id": project, "title": title, "kind": "human",
		"data": map[string]any{"user_id": user}, "idempotency_key": newID(),
	}, 201)["id"].(string)
}

func scheduleOverlapTask(f *fixture, project, member, title, start, end string) string {
	f.t.Helper()
	task := f.call("POST", "/tasks", map[string]any{
		"project_id": project, "owner_id": member, "title": title, "idempotency_key": newID(),
	}, 201)
	plan := f.planChange(task, 0)
	plan["start_date"], plan["due_date"], plan["estimated_hours"] = start, end, nil
	f.call("POST", "/planning/schedule", map[string]any{
		"project_id": project, "changes": []any{plan}, "idempotency_key": newID(),
	}, 200)
	return task["id"].(string)
}

func scheduleOverlapRead(f *fixture, actor Actor) ProjectScheduleOverlapPage {
	f.t.Helper()
	result, err := f.s.ProjectScheduleOverlaps(context.Background(), actor, scheduleOverlapTestWeek)
	if err != nil {
		f.t.Fatal(err)
	}
	return result
}

func TestProjectScheduleOverlapUsesActualOwnersAndIntersectingDays(t *testing.T) {
	f := newFixture(t)
	second := portfolioFixtureProject(t, f, "Second project", "")
	firstMember := scheduleOverlapMember(f, f.project, f.user.ID, "Local name A")
	secondMember := scheduleOverlapMember(f, second.ID, f.user.ID, "Local name B")
	first := scheduleOverlapTask(f, f.project, firstMember, "Crosses into the selected week", "2026-10-31", "2026-11-04")
	scheduleOverlapTask(f, f.project, firstMember, "Same-project overlap only", "2026-11-02", "2026-11-03")
	other := scheduleOverlapTask(f, second.ID, secondMember, "Starts on the first task's due date", "2026-11-04", "2026-11-08")
	scheduleOverlapTask(f, second.ID, secondMember, "Only one boundary", "2026-11-02", "")
	accepted := scheduleOverlapTask(f, second.ID, secondMember, "Already accepted", "2026-11-02", "2026-11-03")
	if _, err := f.s.DB.Exec(`UPDATE suma_tasks SET status='accepted' WHERE id=?`, accepted); err != nil {
		t.Fatal(err)
	}

	page := scheduleOverlapRead(f, f.actor)
	stamp, err := time.Parse(time.RFC3339, page.AsOf)
	if err != nil || stamp.UTC().Format(time.RFC3339) != page.AsOf || page.WeekStart != scheduleOverlapTestWeek || len(page.Items) != 1 {
		t.Fatalf("unexpected schedule overlap page: %+v (%v)", page, err)
	}
	person := page.Items[0]
	if person.UserID != f.user.ID || person.Name != f.user.Name ||
		!slices.Equal(person.Days, []string{"2026-11-04"}) || len(person.Assignments) != 2 {
		t.Fatalf("expected only one cross-project day for the real user: %+v", person)
	}
	sources := make(map[string]ProjectScheduleOverlapAssignment)
	for _, item := range person.Assignments {
		sources[item.TaskID] = item
	}
	if sources[first].ProjectID != f.project || sources[first].StartDate != "2026-10-31" ||
		sources[other].ProjectID != second.ID || sources[other].ProjectTitle != second.Title {
		t.Fatalf("expected exact participating task sources: %+v", person.Assignments)
	}
}

func TestProjectScheduleOverlapUsesCurrentCallerAndAssigneeGrants(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := portfolioFixtureProject(t, f, "Hidden project", "")
	owner := f.cookie
	assigneeEmail := "schedule-owner@example.test"
	inv := governanceInvite(f, assigneeEmail, "member", []any{
		map[string]any{"project_id": f.project, "role": "developer"},
		map[string]any{"project_id": other.ID, "role": "developer"},
	})
	assignee := governanceAccept(f, inv, assigneeEmail)["user"].(map[string]any)["id"].(string)
	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	a := scheduleOverlapMember(f, f.project, assignee, "Human A")
	b := scheduleOverlapMember(f, other.ID, assignee, "Human B")
	scheduleOverlapTask(f, f.project, a, "Visible assignment", "2026-11-02", "2026-11-04")
	scheduleOverlapTask(f, other.ID, b, "Hidden assignment", "2026-11-03", "2026-11-05")

	viewerEmail := "schedule-viewer@example.test"
	inv = governanceInvite(f, viewerEmail, "member", []any{
		map[string]any{"project_id": f.project, "role": "viewer"},
	})
	viewerID := governanceAccept(f, inv, viewerEmail)["user"].(map[string]any)["id"].(string)
	viewer := domainAActor(t, f.s, f.cookie.Value)
	page := scheduleOverlapRead(f, viewer)
	if page.Items == nil || len(page.Items) != 0 {
		t.Fatalf("hidden assignment must not create an apparent overlap: %+v", page)
	}
	writeGrant := func(user, action string, version float64) {
		t.Helper()
		_, err := f.s.SaveProjectAccess(ctx, f.actor, map[string]any{
			"project_id": other.ID, "user_id": user, "role": "viewer", "action": action,
			"expected_version": version, "idempotency_key": newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	writeGrant(viewerID, "", 0)
	if page = scheduleOverlapRead(f, viewer); len(page.Items) != 1 ||
		!slices.Equal(page.Items[0].Days, []string{"2026-11-03", "2026-11-04"}) {
		t.Fatalf("new grant should include exact overlap: %+v", page)
	}
	writeGrant(viewerID, "remove", 1)
	if page = scheduleOverlapRead(f, viewer); len(page.Items) != 0 {
		t.Fatalf("viewer revocation left an overlap visible: %+v", page)
	}
	if page = scheduleOverlapRead(f, f.actor); len(page.Items) != 1 {
		t.Fatalf("owner should still see assignment before assignee revocation: %+v", page)
	}
	writeGrant(assignee, "remove", 1)
	if page = scheduleOverlapRead(f, f.actor); len(page.Items) != 0 {
		t.Fatalf("inactive assignee's historical plan affected current schedule: %+v", page)
	}
}

func TestProjectScheduleOverlapBoundsAuthorizedAssignments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := portfolioFixtureProject(t, f, "Busy project", "")
	owner := scheduleOverlapMember(f, other.ID, f.user.ID, "Human")
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var lastID string
	for i := 0; i <= projectScheduleOverlapRowLimit; i++ {
		item := recordFrom(f.user, other.ID, map[string]any{"title": "Scheduled work", "owner_id": owner})
		if err := f.s.insertRecord(ctx, tx, item, "tasks"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO suma_task_plans
			(tenant_id,project_id,task_id,version,iteration_id,milestone_id,start_date,due_date,estimated_hours,updated_at)
			VALUES (?,?,?,1,'','',?,?,NULL,?)`, f.user.TenantID, other.ID, item.ID, scheduleOverlapTestWeek, scheduleOverlapTestWeek, now()); err != nil {
			t.Fatal(err)
		}
		lastID = item.ID
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	result, err := f.s.ProjectScheduleOverlaps(ctx, f.actor, scheduleOverlapTestWeek)
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.Status != http.StatusConflict ||
		apiError.Code != "schedule_overlap_scope_too_large" || result.Items != nil {
		t.Fatalf("expected a bounded-scope error with no partial result: %+v, %v", result, err)
	}
	email := "limited-schedule-viewer@example.test"
	inv := governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, inv, email)
	if page := scheduleOverlapRead(f, domainAActor(t, f.s, f.cookie.Value)); len(page.Items) != 0 {
		t.Fatalf("unreadable scope must not appear in result: %+v", page)
	}
	if _, err := f.s.DB.Exec(`DELETE FROM suma_task_plans WHERE tenant_id=? AND project_id=? AND task_id=?`, f.user.TenantID, other.ID, lastID); err != nil {
		t.Fatal(err)
	}
	if page := scheduleOverlapRead(f, f.actor); len(page.Items) != 0 {
		t.Fatalf("same-project schedules at the exact limit are not an overlap: %+v", page)
	}
}

func TestProjectScheduleOverlapHTTPValidatesScopeMethodAndWeek(t *testing.T) {
	f := newFixture(t)
	path := "/api/v1/project-portfolio/schedule-overlaps"
	call := func(method, suffix, bearer string, browser bool, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path+suffix, nil)
		if browser {
			req.AddCookie(f.cookie)
			req.Header.Set("X-Suma-Expected-User", f.view.ID)
			req.Header.Set("X-Suma-Expected-Organization", f.view.TenantID)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		reply := httptest.NewRecorder()
		f.handler.ServeHTTP(reply, req)
		if reply.Code != status {
			t.Fatalf("%s %s => %d, want %d: %s", method, suffix, reply.Code, status, reply.Body.String())
		}
		return reply
	}
	week := "?week=" + scheduleOverlapTestWeek
	reply := call(http.MethodGet, week, "", true, 200)
	if reply.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("authenticated overlap data must not be cached")
	}
	for _, suffix := range []string{
		"", "?week=", "?week=2026-11-03", "?week=2026-2-02", "?week=2026-02-30", "?week=0000-01-03",
		"?week=%ZZ", week + "&week=" + scheduleOverlapTestWeek, week + "&project_id=" + f.project, week + "&org=other",
	} {
		call(http.MethodGet, suffix, "", true, 400)
	}
	if reply = call(http.MethodPost, week, "", true, 405); reply.Header().Get("Allow") != http.MethodGet {
		t.Fatal("method restriction must advertise GET")
	}
	call(http.MethodGet, week, "", false, 401)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{
			ProjectID: f.project, Scope: scope, IdempotencyKey: newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
		call(http.MethodGet, week, issued.Token, false, 403)
	}
}
