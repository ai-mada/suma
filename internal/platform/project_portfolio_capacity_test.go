package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const portfolioCapacityTestPath = "/project-portfolio/capacity?week=" + scheduleOverlapTestWeek

func portfolioCapacityTestTask(f *fixture, project, member, title, start, due string, estimate any) string {
	f.t.Helper()
	task := f.call("POST", "/tasks", map[string]any{
		"project_id": project, "owner_id": member, "title": title, "idempotency_key": newID(),
	}, 201)
	plan := f.planChange(task, 0)
	plan["start_date"], plan["due_date"], plan["estimated_hours"] = start, due, estimate
	f.call("POST", "/planning/schedule", map[string]any{
		"project_id": project, "changes": []any{plan}, "idempotency_key": newID(),
	}, 200)
	return task["id"].(string)
}

func portfolioCapacityTestItems(f *fixture) []any {
	f.t.Helper()
	page := f.call("GET", portfolioCapacityTestPath, nil, 200)
	if page["week_start"] != scheduleOverlapTestWeek || page["as_of"] == "" || page["items"] == nil {
		f.t.Fatalf("unexpected capacity page: %v", page)
	}
	return page["items"].([]any)
}

func TestProjectPortfolioCapacityWeightsExactVisibleTasks(t *testing.T) {
	f := newFixture(t)
	other := portfolioFixtureProject(t, f, "Design", "")
	firstMember := scheduleOverlapMember(f, f.project, f.user.ID, "Engineering owner")
	secondMember := scheduleOverlapMember(f, other.ID, f.user.ID, "Design owner")
	calendar := calendarTestSchedule(8)
	calendar["exceptions"] = []any{
		map[string]any{"date": "2026-10-30", "available_hours": 4, "note": "outside-week private note"},
		map[string]any{"date": "2026-11-03", "available_hours": 0, "note": "inside-week private note"},
	}
	f.call("PUT", organizationCalendarTestPath(f.user.ID), organizationCalendarTestRequest(0, calendar), 200)

	crossWeek := portfolioCapacityTestTask(f, f.project, firstMember, "Cross-week", "2026-10-30", "2026-11-03", 30)
	next := portfolioCapacityTestTask(f, other.ID, secondMember, "Design", "2026-11-04", "2026-11-04", 16)
	missing := portfolioCapacityTestTask(f, other.ID, secondMember, "Missing estimate", "2026-11-04", "2026-11-04", nil)
	weekend := portfolioCapacityTestTask(f, other.ID, secondMember, "No availability in task span", "2026-11-08", "2026-11-08", 4)
	portfolioCapacityTestTask(f, f.project, firstMember, "No full dates", "2026-11-02", "", 99)
	accepted := portfolioCapacityTestTask(f, f.project, firstMember, "Accepted", "2026-11-02", "2026-11-02", 99)
	if _, err := f.s.DB.Exec("UPDATE suma_tasks SET status='accepted' WHERE id=?", accepted); err != nil {
		t.Fatal(err)
	}

	people := portfolioCapacityTestItems(f)
	if len(people) != 1 {
		t.Fatalf("expected one user across both project bindings: %v", people)
	}
	p := people[0].(map[string]any)
	if p["user_id"] != f.user.ID || p["name"] != f.user.Name || p["calendar_version"] != 1.0 ||
		p["available_hours"] != 32.0 || p["allocated_hours"] != 36.0 ||
		p["state"] != "over_capacity" || p["unestimated_tasks"] != 1.0 || p["unallocated_tasks"] != 1.0 {
		t.Fatalf("unexpected capacity basis or overload: %v", p)
	}
	days := p["days"].([]any)
	monday, tuesday, wednesday := days[0].(map[string]any), days[1].(map[string]any), days[2].(map[string]any)
	if len(days) != 7 || monday["date"] != scheduleOverlapTestWeek || monday["available_hours"] != 8.0 ||
		monday["allocated_hours"] != 20.0 || tuesday["available_hours"] != 0.0 ||
		tuesday["allocated_hours"] != 0.0 || wednesday["allocated_hours"] != 16.0 {
		t.Fatalf("weekly allocation ignored the out-of-week or in-week exception: %v", days)
	}
	sources := map[string]map[string]any{}
	for _, raw := range p["tasks"].([]any) {
		source := raw.(map[string]any)
		sources[source["task_id"].(string)] = source
	}
	if len(sources) != 4 || sources[crossWeek]["estimated_hours"] != 30.0 ||
		sources[crossWeek]["allocated_hours"] != 20.0 || sources[crossWeek]["start_date"] != "2026-10-30" ||
		sources[next]["project_id"] != other.ID || sources[next]["project_title"] != other.Title ||
		sources[missing]["estimated_hours"] != nil || sources[missing]["allocated_hours"] != nil ||
		sources[weekend]["estimated_hours"] != 4.0 || sources[weekend]["allocated_hours"] != nil ||
		strings.Contains(stringify(p), "private note") {
		t.Fatalf("invalid task sources or private calendar data returned: %v", p)
	}
	calendar["exceptions"].([]any)[0].(map[string]any)["available_hours"] = nil
	f.call("PUT", organizationCalendarTestPath(f.user.ID), organizationCalendarTestRequest(1, calendar), 200)
	p = portfolioCapacityTestItems(f)[0].(map[string]any)
	if p["calendar_version"] != 2.0 || p["available_hours"] != 32.0 ||
		p["allocated_hours"] != 16.0 || p["unallocated_tasks"] != 2.0 || p["state"] != "basis_incomplete" {
		t.Fatalf("an unknown day outside the selected week must block allocation over the full task span: %v", p)
	}

}

func TestProjectPortfolioCapacityKeepsUnknownAndZeroSeparate(t *testing.T) {
	f := newFixture(t)
	member := scheduleOverlapMember(f, f.project, f.user.ID, "Human")
	zero := portfolioCapacityTestTask(f, f.project, member, "Zero estimate", scheduleOverlapTestWeek, scheduleOverlapTestWeek, 0)
	positive := portfolioCapacityTestTask(f, f.project, member, "Positive estimate", scheduleOverlapTestWeek, scheduleOverlapTestWeek, 4)
	p := portfolioCapacityTestItems(f)[0].(map[string]any)
	if p["state"] != "basis_incomplete" || p["calendar_version"] != 0.0 || p["available_hours"] != nil ||
		p["allocated_hours"] != 0.0 || p["unallocated_tasks"] != 1.0 || p["unestimated_tasks"] != 0.0 {
		t.Fatalf("unset calendar must stay unknown: %v", p)
	}
	for _, raw := range p["tasks"].([]any) {
		source := raw.(map[string]any)
		if (source["task_id"] == zero && source["allocated_hours"] != 0.0) ||
			(source["task_id"] == positive && source["allocated_hours"] != nil) {
			t.Fatalf("zero and unknown allocation were conflated: %v", source)
		}
	}
	f.call("PUT", organizationCalendarTestPath(f.user.ID), organizationCalendarTestRequest(0, calendarTestSchedule(0)), 200)
	p = portfolioCapacityTestItems(f)[0].(map[string]any)
	if p["available_hours"] != 0.0 || p["state"] != "basis_incomplete" || p["unallocated_tasks"] != 1.0 {
		t.Fatalf("zero-weight date range must keep the positive estimate unallocated: %v", p)
	}
}

func TestProjectPortfolioCapacityUsesCurrentGrantsAndOrganization(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := portfolioFixtureProject(t, f, "Hidden project", "")
	owner := f.cookie
	assigneeEmail := "capacity-owner@example.test"
	inv := governanceInvite(f, assigneeEmail, "member", []any{
		map[string]any{"project_id": f.project, "role": "developer"},
		map[string]any{"project_id": other.ID, "role": "developer"},
	})
	assignee := governanceAccept(f, inv, assigneeEmail)["user"].(map[string]any)["id"].(string)
	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	f.call("PUT", organizationCalendarTestPath(assignee), organizationCalendarTestRequest(0, calendarTestSchedule(8)), 200)
	a := scheduleOverlapMember(f, f.project, assignee, "Human A")
	b := scheduleOverlapMember(f, other.ID, assignee, "Human B")
	portfolioCapacityTestTask(f, f.project, a, "Visible assignment", scheduleOverlapTestWeek, scheduleOverlapTestWeek, 16)
	portfolioCapacityTestTask(f, other.ID, b, "Hidden assignment", scheduleOverlapTestWeek, scheduleOverlapTestWeek, 32)

	viewerEmail := "capacity-viewer@example.test"
	inv = governanceInvite(f, viewerEmail, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	viewer := governanceAccept(f, inv, viewerEmail)["user"].(map[string]any)["id"].(string)
	f.call("GET", organizationCalendarTestPath(assignee), nil, 403)
	assertCapacity := func(wantHours float64, wantState string, wantTasks int) {
		t.Helper()
		items := portfolioCapacityTestItems(f)
		if len(items) != 1 {
			t.Fatalf("expected a single authorized person: %v", items)
		}
		p := items[0].(map[string]any)
		if p["allocated_hours"] != wantHours || p["available_hours"] != 40.0 || p["state"] != wantState ||
			len(p["tasks"].([]any)) != wantTasks {
			t.Fatalf("unexpected authorized capacity: %v", p)
		}
	}
	assertCapacity(16, "within_visible_capacity", 1)

	grant := func(user, action string, version float64) {
		t.Helper()
		_, err := f.s.SaveProjectAccess(ctx, f.actor, map[string]any{
			"project_id": other.ID, "user_id": user, "role": "viewer", "action": action,
			"expected_version": version, "idempotency_key": newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	grant(viewer, "", 0)
	assertCapacity(48, "over_capacity", 2)
	grant(viewer, "remove", 1)
	assertCapacity(16, "within_visible_capacity", 1)
	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	grant(assignee, "remove", 1)
	assertCapacity(16, "within_visible_capacity", 1)

	isolated := f.call("POST", "/organizations", map[string]any{"name": "Separate workload organization", "idempotency_key": newID()}, 201)["id"].(string)
	f.call("POST", "/auth/switch", map[string]any{"organization_id": isolated, "idempotency_key": newID()}, 200)
	if got := portfolioCapacityTestItems(f); len(got) != 0 {
		t.Fatalf("capacity leaked across tenants: %v", got)
	}
}

func TestProjectPortfolioCapacityHTTPValidatesScopeMethodAndWeek(t *testing.T) {
	f := newFixture(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1"+portfolioCapacityTestPath, nil)
	req.AddCookie(f.cookie)
	reply := httptest.NewRecorder()
	f.handler.ServeHTTP(reply, req)
	if reply.Code != 200 || reply.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("browser capacity read got %d and %q: %s", reply.Code, reply.Header().Get("Cache-Control"), reply.Body)
	}
	path := "/project-portfolio/capacity"
	for _, query := range []string{
		"", "?week=", "?week=2026-11-03", "?week=2026-11-2", "?week=0000-01-03", "?week=2026-02-30", "?week=%ZZ",
		"?week=9999-12-27", "?week=" + scheduleOverlapTestWeek + "&week=" + scheduleOverlapTestWeek,
		"?week=" + scheduleOverlapTestWeek + "&project_id=" + f.project, "?week=" + scheduleOverlapTestWeek + "&organization_id=other",
	} {
		f.call("GET", path+query, nil, 400)
	}
	f.call("POST", portfolioCapacityTestPath, nil, 405)
	for _, scope := range []string{"api", "runner", "agent"} {
		issued, err := f.s.IssueProjectCredential(context.Background(), f.actor, ProjectCredentialIssueRequest{
			ProjectID: f.project, Scope: scope, IdempotencyKey: newID(),
		})
		if err != nil {
			t.Fatal(err)
		}
		f.bearer("GET", portfolioCapacityTestPath, issued.Token, nil, 403)
	}
}

func TestProjectPortfolioCapacityBoundsOnlyAuthorizedAssignments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	hidden := portfolioFixtureProject(t, f, "Large hidden project", "")
	member := scheduleOverlapMember(f, hidden.ID, f.user.ID, "Human")
	tx, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	var lastTask string
	for i := 0; i <= projectPortfolioCapacityRowLimit; i++ {
		task := recordFrom(f.user, hidden.ID, map[string]any{"title": "Scheduled work", "owner_id": member})
		if err := f.s.insertRecord(ctx, tx, task, "tasks"); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO suma_task_plans
			(tenant_id,project_id,task_id,version,iteration_id,milestone_id,start_date,due_date,estimated_hours,updated_at)
			VALUES (?,?,?,1,'','',?,?,0,?)`, f.user.TenantID, hidden.ID, task.ID, scheduleOverlapTestWeek, scheduleOverlapTestWeek, now()); err != nil {
			t.Fatal(err)
		}
		lastTask = task.ID
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	page := f.call("GET", portfolioCapacityTestPath, nil, 409)
	if page["error"].(map[string]any)["code"] != "project_capacity_scope_too_large" || page["items"] != nil {
		t.Fatalf("expected an explicit limit error and no partial facts: %v", page)
	}
	owner := f.cookie
	email := "limited-capacity-viewer@example.test"
	inv := governanceInvite(f, email, "member", []any{map[string]any{"project_id": f.project, "role": "viewer"}})
	governanceAccept(f, inv, email)
	if got := portfolioCapacityTestItems(f); len(got) != 0 {
		t.Fatalf("unreadable projects must not affect the candidate limit: %v", got)
	}

	f.cookie = owner
	f.call("GET", "/auth/session", nil, 200)
	if _, err := f.s.DB.Exec("DELETE FROM suma_task_plans WHERE tenant_id=? AND project_id=? AND task_id=?", f.user.TenantID, hidden.ID, lastTask); err != nil {
		t.Fatal(err)
	}
	people := portfolioCapacityTestItems(f)
	if len(people) != 1 || len(people[0].(map[string]any)["tasks"].([]any)) != projectPortfolioCapacityRowLimit {
		t.Fatalf("expected the complete projection at the exact limit, got %d people", len(people))
	}
}
