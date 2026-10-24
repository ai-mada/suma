package platform

import (
	"context"
	"fmt"
	"testing"
)

func (f *fixture) planChange(task map[string]any, version int) map[string]any {
	return map[string]any{"task_id": task["id"], "expected_task_version": task["version"], "expected_version": version, "iteration_id": "", "milestone_id": "", "start_date": "2026-10-19", "due_date": "2026-10-25", "estimated_hours": 14}
}

func TestPlanningScheduleBatchVersionReplayAndScope(t *testing.T) {
	f := newFixture(t)
	iteration := f.call("POST", "/iterations", f.body(map[string]any{"title": "第一轮", "start_date": "2026-10-19", "end_date": "2026-10-25"}), 201)
	f.call("POST", "/iterations", f.body(map[string]any{"title": "无效日期", "start_date": "2026-10-26", "end_date": "2026-10-25"}), 400)
	a := f.call("POST", "/tasks", f.body(map[string]any{"title": "A"}), 201)
	b := f.call("POST", "/tasks", f.body(map[string]any{"title": "B"}), 201)
	ca, cb := f.planChange(a, 0), f.planChange(b, 0)
	ca["iteration_id"] = iteration["id"]
	cb["expected_task_version"] = 99
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{ca, cb}}), 409)
	read := f.call("GET", "/planning?project_id="+f.project+"&task_id="+a["id"].(string), nil, 200)
	if read["tasks"].([]any)[0].(map[string]any)["plan"].(map[string]any)["version"] != float64(0) {
		t.Fatal("partial schedule committed")
	}
	cb["expected_task_version"] = b["version"]
	request := f.body(map[string]any{"changes": []any{ca, cb}})
	f.call("POST", "/planning/schedule", request, 200)
	f.call("PATCH", "/tasks/"+a["id"].(string), f.body(map[string]any{"expected_version": a["version"], "title": "A修订"}), 200)
	replay := f.call("POST", "/planning/schedule", request, 200)
	if replay["total"] != float64(2) {
		t.Fatal("schedule receipt not replayed")
	}
	f.call("PATCH", "/iterations/"+iteration["id"].(string), f.body(map[string]any{"expected_version": iteration["version"], "status": "completed"}), 409)
	other := f.call("POST", "/projects", map[string]any{"title": "其他", "idempotency_key": newID()}, 201)
	foreign := map[string]any{"project_id": other["id"], "title": "外部里程碑", "target_date": "2026-10-25", "idempotency_key": newID()}
	milestone := f.call("POST", "/milestones", foreign, 201)
	cb = f.planChange(b, 1)
	cb["milestone_id"] = milestone["id"]
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{cb}}), 400)
}

func TestBaselineCapturesBeyondUIPageAndSeparatesScopeAndPlanChanges(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	tx, e := f.s.DB.BeginTx(ctx, nil)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 405; i++ {
		r := recordFrom(f.user, f.project, map[string]any{"title": fmt.Sprintf("任务%04d", i), "status": "todo"})
		if e = f.s.insertRecord(ctx, tx, r, "tasks"); e != nil {
			t.Fatal(e)
		}
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	first := f.call("GET", "/planning?project_id="+f.project+"&limit=2", nil, 200)
	if len(first["tasks"].([]any)) != 2 || first["total"] != float64(405) || first["metrics"].(map[string]any)["total_tasks"] != float64(405) {
		t.Fatal("planning totals limited to UI page")
	}
	request := f.body(map[string]any{"title": "范围基线"})
	baseline := f.call("POST", "/planning/baselines", request, 201)
	again := f.call("POST", "/planning/baselines", request, 201)
	if baseline["id"] != again["id"] || baseline["task_count"] != float64(405) {
		t.Fatal("baseline scope truncated or duplicated")
	}
	id := baseline["id"].(string)
	cursor := ""
	seen := map[string]bool{}
	for {
		path := "/planning/baselines/" + id + "?project_id=" + f.project + "&limit=200&cursor=" + cursor
		out := f.call("GET", path, nil, 200)["snapshot"].(map[string]any)
		for _, raw := range out["tasks"].([]any) {
			task := raw.(map[string]any)
			key := task["id"].(string)
			if seen[key] {
				t.Fatal("baseline page duplicated")
			}
			seen[key] = true
		}
		cursor = out["next_cursor"].(string)
		if cursor == "" {
			break
		}
	}
	if len(seen) != 405 {
		t.Fatal("baseline omitted tasks")
	}
	a := first["tasks"].([]any)[0].(map[string]any)
	a = f.call("PATCH", "/tasks/"+a["id"].(string), f.body(map[string]any{"expected_version": a["version"], "title": "改变范围"}), 200)
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{f.planChange(a, 0)}}), 200)
	f.call("POST", "/tasks", f.body(map[string]any{"title": "基线新增范围"}), 201)
	detail := f.call("GET", "/planning/baselines/"+id+"?project_id="+f.project, nil, 200)
	comparison := detail["comparison"].(map[string]any)
	if comparison["added_tasks"] != float64(1) || comparison["definition_changed_tasks"] != float64(1) || comparison["plan_changed_tasks"] != float64(1) {
		t.Fatalf("scope comparison %v", comparison)
	}
	if detail["task_count"] != float64(405) {
		t.Fatal("immutable snapshot changed")
	}
}

func TestCapacityUnknownZeroAndProRatedCalendarWeeks(t *testing.T) {
	f := newFixture(t)
	member := f.call("POST", "/members", f.body(map[string]any{"title": "负责人", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "跨周工作", "owner_id": member["id"]}), 201)
	change := f.planChange(task, 0)
	change["due_date"] = "2026-11-01"
	change["estimated_hours"] = 28
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 200)
	overview := f.call("GET", "/planning?project_id="+f.project+"&week_start=2026-10-19", nil, 200)
	entry := overview["capacity"].([]any)[0].(map[string]any)
	if entry["available_hours"] != nil || entry["allocated_hours"] != float64(14) {
		t.Fatal("unknown capacity fabricated or weekly share double counted")
	}
	body := f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": 0, "expected_version": 0})
	zero := f.call("POST", "/planning/capacity", body, 200)
	if zero["available_hours"] != float64(0) {
		t.Fatal("zero capacity lost")
	}
	f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": 40, "expected_version": 0}), 409)
	unknown := f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": nil, "expected_version": 1}), 200)
	if unknown["available_hours"] != nil {
		t.Fatal("explicit unknown not preserved")
	}
	f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-20", "available_hours": 40, "expected_version": 0}), 400)
}

func TestPlanningChecksLivePermissionInsteadOfStalePrincipalRole(t *testing.T) {
	f := newFixture(t)
	_, e := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,?)`, f.project, f.user.ID, "viewer")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.SavePlanningPeriod(context.Background(), f.user, f.project, "milestones", "", map[string]any{"title": "越权", "target_date": "2026-10-25", "idempotency_key": newID()}); e == nil {
		t.Fatal("stale owner authorized a write")
	}
	f.call("GET", "/planning?project_id="+f.project, nil, 200)
	f.call("POST", "/milestones", f.body(map[string]any{"title": "只读不能创建", "target_date": "2026-10-25"}), 403)
}

func TestCompletedPlanningReopensWithInvalidatedTaskEvidence(t *testing.T) {
	f := newFixture(t)
	wf := f.call("POST", "/workflows", f.body(map[string]any{"name": "验收流程", "nodes": []any{map[string]any{"id": "test", "type": "test", "command": []string{"test"}}}, "edges": []any{}}), 201)
	task, _, _ := f.testedTask(wf, "已验收交付", nil)
	iteration := f.call("POST", "/iterations", f.body(map[string]any{"title": "可追溯完成", "start_date": "2026-10-19", "end_date": "2026-10-25"}), 201)
	change := f.planChange(task, 0)
	change["iteration_id"] = iteration["id"]
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 200)
	complete := f.call("PATCH", "/iterations/"+iteration["id"].(string), f.body(map[string]any{"expected_version": iteration["version"], "status": "completed"}), 200)
	change["expected_version"] = 1
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 409)
	f.call("PATCH", "/tasks/"+task["id"].(string), f.body(map[string]any{"expected_version": task["version"], "description": "交付范围变化"}), 200)
	periods := f.call("GET", "/iterations?project_id="+f.project, nil, 200)["items"].([]any)
	current := periods[0].(map[string]any)
	if current["status"] != "active" || current["version"] != complete["version"].(float64)+1 {
		t.Fatalf("stale completed planning %v", current)
	}
}

func TestPlanningRejectsFractionalCASAndIncompleteReplacement(t *testing.T) {
	f := newFixture(t)
	period := f.call("POST", "/milestones", f.body(map[string]any{"title": "严格版本", "target_date": "2026-10-25"}), 201)
	f.call("PATCH", "/milestones/"+period["id"].(string), f.body(map[string]any{"expected_version": 1.25, "title": "不应被截断"}), 400)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "完整排期字段"}), 201)
	change := f.planChange(task, 0)
	delete(change, "iteration_id")
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 400)
	change = f.planChange(task, 0)
	change["expected_task_version"] = 1.25
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 400)
	f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": "missing", "available_hours": 40, "expected_version": 0}), 400)
}

func TestProjectHumanIdentityCannotBeInventedDuplicatedOrReplaced(t *testing.T) {
	f := newFixture(t)
	f.call("POST", "/members", f.body(map[string]any{"title": "虚构人员", "kind": "human"}), 400)
	person := f.call("POST", "/members", f.body(map[string]any{"title": "真实人员", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 201)
	f.call("POST", "/members", f.body(map[string]any{"title": "重复身份", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 409)
	f.call("PATCH", "/members/"+person["id"].(string), f.body(map[string]any{"expected_version": person["version"], "kind": "agent"}), 400)
	f.call("PATCH", "/members/"+person["id"].(string), f.body(map[string]any{"expected_version": person["version"], "data": map[string]any{"user_id": "other-person"}}), 400)
}

func TestCapacityCountsOnlyWorkInSelectedCalendarWeek(t *testing.T) {
	f := newFixture(t)
	member := f.call("POST", "/members", f.body(map[string]any{"title": "按周规划", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 201)
	future := f.call("POST", "/tasks", f.body(map[string]any{"title": "另一个周的未知工作量", "owner_id": member["id"]}), 201)
	change := f.planChange(future, 0)
	change["start_date"], change["due_date"], change["estimated_hours"] = "2026-11-02", "2026-11-08", nil
	f.call("POST", "/planning/schedule", f.body(map[string]any{"changes": []any{change}}), 200)
	f.call("POST", "/tasks", f.body(map[string]any{"title": "尚未排期的未知范围", "owner_id": member["id"]}), 201)
	for _, check := range []struct {
		week    string
		unknown float64
	}{{"2026-10-19", 1}, {"2026-11-02", 2}} {
		items := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start="+check.week, nil, 200)["items"].([]any)
		entry := items[0].(map[string]any)
		if entry["unestimated_tasks"] != check.unknown || entry["allocated_hours"] != float64(0) {
			t.Fatalf("capacity includes work outside %s: %v", check.week, entry)
		}
	}
}

func TestCapacityRequiresExplicitHoursAndPreservesExistingValueOnInvalidWrite(t *testing.T) {
	f := newFixture(t)
	member := f.call("POST", "/members", f.body(map[string]any{"title": "显式容量", "kind": "human", "data": map[string]any{"user_id": f.user.ID}}), 201)
	first := f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": 40, "expected_version": 0}), 200)
	f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "expected_version": first["version"]}), 400)
	f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": "unknown", "expected_version": first["version"]}), 400)
	current := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start=2026-10-19", nil, 200)["items"].([]any)[0].(map[string]any)
	if current["available_hours"] != float64(40) || current["version"] != first["version"] {
		t.Fatalf("invalid request changed known capacity: %v", current)
	}
	unknown := f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": member["id"], "week_start": "2026-10-19", "available_hours": nil, "expected_version": first["version"]}), 200)
	if unknown["available_hours"] != nil {
		t.Fatal("explicit unknown was rejected or converted to zero")
	}
}

func TestPlanningPeriodRejectsWrongStringTypesWithoutErasingMetadata(t *testing.T) {
	f := newFixture(t)
	period := f.call("POST", "/milestones", f.body(map[string]any{"title": "字段类型验证", "description": "保留已有说明", "goal": "保留已有目标", "target_date": "2026-10-25"}), 201)
	for _, field := range []string{"title", "description", "status", "start_date", "end_date", "target_date", "goal"} {
		f.call("PATCH", "/milestones/"+period["id"].(string), f.body(map[string]any{"expected_version": period["version"], field: map[string]any{"invalid": true}}), 400)
	}
	current := f.call("GET", "/milestones?project_id="+f.project, nil, 200)["items"].([]any)[0].(map[string]any)
	if current["description"] != period["description"] || current["goal"] != period["goal"] || current["version"] != period["version"] {
		t.Fatalf("wrong field type erased planning metadata: %v", current)
	}
	f.call("POST", "/milestones", f.body(map[string]any{"title": "不能用错误类型创建", "description": false, "target_date": "2026-10-25"}), 400)
}

func TestCapacityExcludesRevokedHumansAndPreservesHistoricalRows(t *testing.T) {
	for _, removal := range []string{"project", "organization"} {
		t.Run(removal, func(t *testing.T) {
			f := newFixture(t)
			ownerCookie := f.cookie
			inv := governanceInvite(f, "capacity-revoke@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
			session := governanceAccept(f, inv, "capacity-revoke@example.test")
			userID := session["user"].(map[string]any)["id"].(string)
			f.cookie = ownerCookie
			resource := f.call("POST", "/members", f.body(map[string]any{"title": "已撤权资源", "kind": "human", "data": map[string]any{"user_id": userID}}), 201)
			known := f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": resource["id"], "week_start": "2026-10-19", "available_hours": 40, "expected_version": 0}), 200)
			if removal == "project" {
				f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": userID, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
			} else {
				f.call("PATCH", "/organization/members/"+userID, map[string]any{"action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
			}
			current := f.call("GET", "/planning/capacity?project_id="+f.project+"&week_start=2026-10-19", nil, 200)
			if current["total"] != float64(0) {
				t.Fatalf("revoked human contributes current capacity: %v", current)
			}
			f.call("POST", "/planning/capacity", f.body(map[string]any{"member_id": resource["id"], "week_start": "2026-10-19", "available_hours": 35, "expected_version": known["version"]}), 400)
			var hours float64
			var version int
			if err := f.s.DB.QueryRow(`SELECT available_hours,version FROM suma_capacity WHERE tenant_id=? AND project_id=? AND member_id=? AND week_start=?`, f.user.TenantID, f.project, resource["id"], "2026-10-19").Scan(&hours, &version); err != nil {
				t.Fatal(err)
			}
			if hours != 40 || version != 1 {
				t.Fatal("revocation or rejected write changed historical capacity")
			}
		})
	}
}
