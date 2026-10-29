package platform

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func calendarTestMember(f *fixture) map[string]any {
	return f.call("POST", "/members", f.body(map[string]any{"title": "日历资源", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 201)
}

func calendarTestSchedule(hours any) map[string]any {
	return map[string]any{"weekdays": []any{hours, hours, hours, hours, hours, float64(0), float64(0)}, "exceptions": []any{}}
}

func calendarTestPath(member map[string]any) string {
	return "/resource-calendars/" + member["id"].(string)
}

func TestResourceCalendarUnknownReplacementCASAndOriginalReceipt(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	path := calendarTestPath(member)
	before := [3]int{domainACount(t, f.s, "suma_resource_calendars"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
	unknown := f.call("GET", path+"?project_id="+f.project, nil, 200)
	if unknown["version"] != float64(0) || len(unknown["schedule"].(map[string]any)["weekdays"].([]any)) != 7 {
		t.Fatalf("unconfigured calendar is not explicit unknown: %v", unknown)
	}
	for _, hours := range unknown["schedule"].(map[string]any)["weekdays"].([]any) {
		if hours != nil {
			t.Fatal("unconfigured weekday fabricated hours")
		}
	}
	if after := [3]int{domainACount(t, f.s, "suma_resource_calendars"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}; after != before {
		t.Fatal("calendar GET wrote business state")
	}
	request := f.body(map[string]any{"expected_version": 0, "schedule": calendarTestSchedule(float64(8))})
	first := f.call("PUT", path, request, 200)
	f.call("PUT", path, f.body(map[string]any{"expected_version": 0, "schedule": calendarTestSchedule(float64(6))}), 409)
	second := f.call("PUT", path, f.body(map[string]any{"expected_version": 1, "schedule": calendarTestSchedule(float64(6))}), 200)
	replayed := f.call("PUT", path, request, 200)
	if first["version"] != float64(1) || second["version"] != float64(2) || stringify(first) != stringify(replayed) {
		t.Fatal("CAS or original receipt changed")
	}
	request["schedule"] = calendarTestSchedule(float64(4))
	f.call("PUT", path, request, 409)
	current := f.call("GET", path+"?project_id="+f.project, nil, 200)
	if current["version"] != float64(2) || current["schedule"].(map[string]any)["weekdays"].([]any)[0] != float64(6) {
		t.Fatal("receipt replay overwrote the current calendar")
	}
}

func TestResourceCalendarValidatesCompleteQuarterHourSchedule(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	path := calendarTestPath(member)
	f.call("PUT", path, f.body(map[string]any{"expected_version": 0, "schedule": calendarTestSchedule(float64(8))}), 200)
	for _, schedule := range []any{
		nil,
		map[string]any{"weekdays": []any{8}, "exceptions": []any{}},
		map[string]any{"weekdays": []any{8, 8, 8, 8, 8, 0, 0}},
		calendarTestSchedule("8"),
		calendarTestSchedule(float64(8.1)),
		calendarTestSchedule(float64(25)),
		map[string]any{"weekdays": []any{8, 8, 8, 8, 8, 0, 0}, "exceptions": []any{map[string]any{"date": "2026-10-24", "note": "missing hours"}}},
		map[string]any{"weekdays": []any{8, 8, 8, 8, 8, 0, 0}, "exceptions": []any{map[string]any{"date": "2026-02-30", "available_hours": 8, "note": "bad date"}}},
	} {
		f.call("PUT", path, f.body(map[string]any{"expected_version": 1, "schedule": schedule}), 400)
	}
	f.call("PUT", path, f.body(map[string]any{"expected_version": 1.5, "schedule": calendarTestSchedule(float64(8))}), 400)
	current := f.call("GET", path+"?project_id="+f.project, nil, 200)
	if current["version"] != float64(1) {
		t.Fatal("invalid schedule changed calendar")
	}
}

func TestResourceCalendarWeeklyOverrideAndWeightedEffort(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "按实际可用时间分摊", "owner_id": member["id"]}), 201)
	change := f.planChange(task, 0)
	change["start_date"], change["due_date"], change["estimated_hours"] = "2026-10-19", "2026-11-01", 76
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 200)
	schedule := calendarTestSchedule(float64(8))
	schedule["exceptions"] = []any{
		map[string]any{"date": "2026-10-20", "available_hours": 0, "note": "请假"},
		map[string]any{"date": "2026-10-24", "available_hours": 4, "note": "调休工作日"},
	}
	f.call("PUT", calendarTestPath(member), f.body(map[string]any{"expected_version": 0, "schedule": schedule}), 200)
	for _, check := range []struct {
		week  string
		hours float64
	}{{"2026-10-19", 36}, {"2026-10-26", 40}} {
		out := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start="+check.week, nil, 200)["items"].([]any)[0].(map[string]any)
		if out["available_hours"] != check.hours || math.Abs(out["allocated_hours"].(float64)-check.hours) > 1e-8 || out["calendar_version"] != float64(1) || out["distribution_unknown_tasks"] != float64(0) {
			t.Fatalf("calendar weight ignored: %v", out)
		}
		if _, old := out["version"]; old {
			t.Fatal("obsolete week-row version remains on wire")
		}
		days := out["days"].([]any)
		if len(days) != 7 || days[0].(map[string]any)["date"] != check.week {
			t.Fatal("daily calendar scope missing")
		}
	}
	// An unknown day anywhere in the full task interval makes its entire
	// distribution uncertain, including a queried week whose own days are known.
	schedule["exceptions"] = []any{map[string]any{"date": "2026-10-27", "available_hours": nil, "note": "待确认"}}
	f.call("PUT", calendarTestPath(member), f.body(map[string]any{"expected_version": 1, "schedule": schedule}), 200)
	first := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start=2026-10-19", nil, 200)["items"].([]any)[0].(map[string]any)
	second := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start=2026-10-26", nil, 200)["items"].([]any)[0].(map[string]any)
	if first["available_hours"] != float64(40) || second["available_hours"] != nil || first["allocated_hours"] != float64(0) || first["distribution_unknown_tasks"] != float64(1) || first["distribution_unknown_hours"] != float64(76) {
		t.Fatal("unknown full-interval distribution fabricated allocation", first, second)
	}
	if day := second["days"].([]any)[1].(map[string]any); day["source"] != "exception" || day["available_hours"] != nil {
		t.Fatal("unknown exception provenance lost")
	}
	zero := calendarTestSchedule(float64(0))
	f.call("PUT", calendarTestPath(member), f.body(map[string]any{"expected_version": 2, "schedule": zero}), 200)
	allZero := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start=2026-10-19", nil, 200)["items"].([]any)[0].(map[string]any)
	if allZero["available_hours"] != float64(0) || allZero["distribution_unknown_tasks"] != float64(1) || allZero["distribution_unknown_hours"] != float64(76) {
		t.Fatal("zero-weight interval treated as allocated", allZero)
	}
}

func TestResourceCalendarRechecksCurrentCredentialBeforeReadAndReplay(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	request := f.body(map[string]any{"expected_version": 0, "schedule": calendarTestSchedule(float64(8))})
	f.call("PUT", calendarTestPath(member), request, 200)
	var req ResourceCalendarSaveRequest
	raw, _ := json.Marshal(request)
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatal(err)
	}
	if err := logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.GetResourceCalendar(context.Background(), f.actor, f.project, member["id"].(string))
	requireAuthorityStatus(t, err, 401)
	_, err = f.s.SaveResourceCalendar(context.Background(), f.actor, f.project, member["id"].(string), req)
	requireAuthorityStatus(t, err, 401)
}

func TestResourceCalendarDoesNotInferSingleDayFromPartialDatesAndKeepsZeroEstimate(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	changes := []any{}
	for i, dates := range [][2]string{{"2026-10-19", ""}, {"", "2026-10-21"}, {"2026-10-19", "2026-10-23"}} {
		task := f.call("POST", "/tasks", f.body(map[string]any{"title": "完整时间范围", "owner_id": member["id"]}), 201)
		change := f.planChange(task, 0)
		change["start_date"], change["due_date"] = dates[0], dates[1]
		if i == 2 {
			change["estimated_hours"] = 0
		}
		changes = append(changes, change)
	}
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": changes}), 200)
	entry := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start=2026-10-19", nil, 200)["items"].([]any)[0].(map[string]any)
	if entry["unscheduled_tasks"] != float64(2) || entry["unestimated_tasks"] != float64(0) || entry["distribution_unknown_tasks"] != float64(0) || entry["allocated_hours"] != float64(0) {
		t.Fatal("partial dates or explicit zero estimate inferred effort", entry)
	}
}

func TestResourceCalendarProjectsCurrentEditPermissionBeforeReceiptRecovery(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	path := calendarTestPath(member)
	request := f.body(map[string]any{"expected_version": 0, "schedule": calendarTestSchedule(float64(8))})
	f.call("PUT", path, request, 200)
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'viewer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	read := f.call("GET", path+"?project_id="+f.project, nil, 200)
	if read["can_edit"] != false {
		t.Fatal("stale owner role advertised edit permission")
	}
	f.call("PUT", path, request, 403)
	f.call("PUT", path, f.body(map[string]any{"expected_version": 1, "schedule": calendarTestSchedule(float64(4))}), 403)
	current := f.call("GET", path+"?project_id="+f.project, nil, 200)
	if current["version"] != float64(1) {
		t.Fatal("viewer changed resource calendar")
	}
}

func TestResourceCalendarFullScheduleChangesFormalPlanningDigest(t *testing.T) {
	f := newFixture(t)
	member := calendarTestMember(f)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "完整日历依据", "owner_id": member["id"]}), 201)
	read := func() string {
		t.Helper()
		var result string
		err := f.s.withCurrentRead(context.Background(), f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectRead, func(auth *authorizedCommand) error {
			var err error
			result, err = f.s.planningFormalDigestTx(context.Background(), auth.tx, f.user.TenantID, f.project, []string{task["id"].(string)}, "2026-10-19")
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	schedule := calendarTestSchedule(float64(8))
	f.call("PUT", calendarTestPath(member), f.body(map[string]any{"expected_version": 0, "schedule": schedule}), 200)
	before := read()
	// A later-week override changes the denominator for spanning tasks. A
	// source freeze must cover the full calendar, not only the selected week.
	schedule["exceptions"] = []any{map[string]any{"date": "2026-10-27", "available_hours": 0, "note": "next-week leave"}}
	f.call("PUT", calendarTestPath(member), f.body(map[string]any{"expected_version": 1, "schedule": schedule}), 200)
	if read() == before {
		t.Fatal("planning source digest ignored calendar revision")
	}
}

func TestResourceCalendarRejectsUnsupportedWeekAndTaskDates(t *testing.T) {
	f := newFixture(t)
	calendarTestMember(f)
	for _, week := range []string{"0000-01-03", "9999-12-27"} {
		f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start="+week, nil, 400)
	}
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "有效日期范围"}), 201)
	change := f.planChange(task, 0)
	change["start_date"], change["due_date"] = "0000-01-01", "0000-01-02"
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 400)
}
