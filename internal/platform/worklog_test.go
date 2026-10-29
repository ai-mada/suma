package platform

import (
	"context"
	"strings"
	"testing"
)

func worklogFixture(t *testing.T) (*fixture, map[string]any, map[string]any) {
	f := newFixture(t)
	member := calendarTestMember(f)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "Manual work evidence"}), 201)
	return f, member, task
}

func worklogBody(f *fixture, member, task map[string]any, minutes int) map[string]any {
	return f.body(map[string]any{"member_id": member["id"], "task_id": task["id"], "expected_task_version": task["version"], "work_date": "2026-10-29", "minutes": minutes, "note": "实际人工工作", "on_behalf_ack": false})
}

func TestWorklogImmutableLifecycleReceiptAndCurrentTotals(t *testing.T) {
	f, member, task := worklogFixture(t)
	request := worklogBody(f, member, task, 90)
	first := f.call("POST", "/worklogs", request, 201)
	path := "/worklogs/" + first["id"].(string)
	correct := f.body(map[string]any{"expected_version": 1, "task_id": task["id"], "expected_task_version": task["version"], "work_date": "2026-10-29", "minutes": 60, "note": "修正", "reason": "排除休息", "on_behalf_ack": false})
	second := f.call("POST", path+"/corrections", correct, 200)
	if second["version"] != float64(2) || second["minutes"] != float64(60) {
		t.Fatal(second)
	}
	if replay := f.call("POST", "/worklogs", request, 201); replay["version"] != float64(1) || replay["minutes"] != float64(90) {
		t.Fatal("receipt lost original revision", replay)
	}
	f.call("POST", path+"/corrections", f.body(map[string]any{"expected_version": 1, "task_id": task["id"], "expected_task_version": task["version"], "work_date": "2026-10-29", "minutes": 20, "note": "", "reason": "stale", "on_behalf_ack": false}), 409)
	list := f.call("GET", "/worklogs?project_id="+f.project+"&from_date=2026-10-29&to_date=2026-10-29", nil, 200)
	if list["total"] != float64(1) || list["summary"].(map[string]any)["total_minutes"] != float64(60) {
		t.Fatal("revisions counted twice", list)
	}
	f.call("POST", path+"/void", f.body(map[string]any{"expected_version": 2, "reason": "重复登记", "on_behalf_ack": false}), 200)
	list = f.call("GET", "/worklogs?project_id="+f.project+"&from_date=2026-10-29&to_date=2026-10-29", nil, 200)
	if list["summary"].(map[string]any)["total_minutes"] != float64(0) {
		t.Fatal(list)
	}
	history := f.call("GET", path+"/revisions?project_id="+f.project, nil, 200)
	if history["total"] != float64(3) || len(history["items"].([]any)) != 3 {
		t.Fatal(history)
	}
	f.call("POST", path+"/corrections", f.body(map[string]any{"expected_version": 3, "task_id": task["id"], "expected_task_version": task["version"], "work_date": "2026-10-29", "minutes": 10, "note": "", "reason": "cannot revive", "on_behalf_ack": false}), 409)
}

func TestWorklogValidationAndTaskVersion(t *testing.T) {
	f, member, task := worklogFixture(t)
	for _, change := range []map[string]any{{"minutes": 0}, {"minutes": 1441}, {"minutes": 1.5}, {"work_date": "2026-02-30"}, {"work_date": "2026-10-29T00:00:00Z"}, {"note": strings.Repeat("中", 201)}, {"expected_task_version": 0}, {"user_id": f.user.ID}, {"source": "runner"}} {
		body := worklogBody(f, member, task, 30)
		for k, v := range change {
			body[k] = v
		}
		f.call("POST", "/worklogs", body, 400)
	}
	body := worklogBody(f, member, task, 30)
	body["expected_task_version"] = 2
	f.call("POST", "/worklogs", body, 409)
	f.call("GET", "/worklogs?project_id="+f.project, nil, 400)
	f.call("GET", "/worklogs?project_id="+f.project+"&from_date=2026-01-01&to_date=2026-02-02", nil, 400)
}

func TestWorklogCurrentReadAndReplayRejectRevokedCredential(t *testing.T) {
	f, member, task := worklogFixture(t)
	body := worklogBody(f, member, task, 30)
	first := f.call("POST", "/worklogs", body, 201)
	if err := logoutCredentialForTest(context.Background(), f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	f.call("POST", "/worklogs", body, 401)
	f.call("GET", "/worklogs/"+first["id"].(string)+"?project_id="+f.project, nil, 401)
}
