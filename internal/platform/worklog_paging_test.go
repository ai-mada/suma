package platform

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorklogCurrentAndRevisionPagingBindScopeAndFilters(t *testing.T) {
	f, member, task := worklogFixture(t)
	ids := map[string]bool{}
	for i := 0; i < 3; i++ {
		entry := f.call("POST", "/worklogs", worklogBody(f, member, task, 10), 201)
		ids[entry["id"].(string)] = true
	}
	base := "/worklogs?project_id=" + f.project + "&from_date=2026-10-29&to_date=2026-10-29&limit=1"
	page := f.call("GET", base, nil, 200)
	if page["total"] != float64(3) || page["summary"].(map[string]any)["total_minutes"] != float64(30) || len(page["items"].([]any)) != 1 || page["next_cursor"] == "" {
		t.Fatal(page)
	}
	firstID := page["items"].([]any)[0].(map[string]any)["id"].(string)
	delete(ids, firstID)
	cursor := page["next_cursor"].(string)
	second := f.call("GET", base+"&cursor="+cursor, nil, 200)
	secondID := second["items"].([]any)[0].(map[string]any)["id"].(string)
	if !ids[secondID] {
		t.Fatal("page duplicated a record", second)
	}
	f.call("GET", base+"&status=active&cursor="+cursor, nil, 400)
	f.call("POST", "/worklogs", worklogBody(f, member, task, 10), 201)
	f.call("GET", base+"&cursor="+cursor, nil, 409)
	path := "/worklogs/" + firstID
	for version := 1; version <= 2; version++ {
		f.call("POST", path+"/corrections", f.body(map[string]any{"expected_version": version, "task_id": task["id"], "expected_task_version": task["version"], "work_date": "2026-10-29", "minutes": 10 + version, "note": "", "reason": "correct minutes", "on_behalf_ack": false}), 200)
	}
	history := f.call("GET", path+"/revisions?project_id="+f.project+"&limit=1", nil, 200)
	entry := history["items"].([]any)[0].(map[string]any)
	if history["total"] != float64(3) || entry["version"] != float64(3) || entry["can_correct"] != false || history["next_cursor"] == "" {
		t.Fatal(history)
	}
	next := f.call("GET", path+"/revisions?project_id="+f.project+"&limit=1&cursor="+history["next_cursor"].(string), nil, 200)
	if next["items"].([]any)[0].(map[string]any)["version"] != float64(2) {
		t.Fatal(next)
	}
	f.call("GET", "/worklogs/"+secondID+"/revisions?project_id="+f.project+"&limit=1&cursor="+history["next_cursor"].(string), nil, 400)
}

func TestWorklogClosedWireRequiresCompleteFieldsAndOriginalView(t *testing.T) {
	f, member, task := worklogFixture(t)
	for _, omitted := range []string{"note", "on_behalf_ack"} {
		body := worklogBody(f, member, task, 10)
		delete(body, omitted)
		f.call("POST", "/worklogs", body, 400)
	}
	body := worklogBody(f, member, task, 10)
	raw := stringify(body)
	for _, malformed := range []string{
		strings.Replace(raw, `"minutes":10`, `"minutes":10,"minutes":20`, 1),
		strings.Replace(raw, `"minutes":10`, `"minutes":null`, 1),
		strings.Replace(raw, `"minutes":10`, `"minutes":1e1`, 1),
		strings.Replace(raw, `"note":`, `"Note":`, 1),
		strings.Replace(raw, `"on_behalf_ack":false`, `"on_behalf_ack":null`, 1),
	} {
		req := httptest.NewRequest("POST", "http://suma.test/api/v1/worklogs", bytes.NewBufferString(malformed))
		req.AddCookie(f.cookie)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Suma-Expected-User", f.user.ID)
		req.Header.Set("X-Suma-Expected-Organization", f.user.TenantID)
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, req)
		if response.Code != 400 {
			t.Fatalf("closed wire accepted %s: %d %s", malformed, response.Code, response.Body.String())
		}
	}
	f.call("GET", "/worklogs?project_id="+f.project+"&from_date=2026-10-29&to_date=2026-10-29&status=active&status=voided", nil, 400)
	f.call("GET", "/worklogs?project_id="+f.project+"&from_date=2026-10-29&to_date=2026-10-29&limit=01", nil, 400)
	request := httptest.NewRequest("POST", "http://suma.test/api/v1/worklogs", bytes.NewBufferString(raw))
	request.AddCookie(f.cookie)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	f.handler.ServeHTTP(response, request)
	if response.Code != 428 {
		t.Fatalf("missing original view accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestWorklogTaskSnapshotsAndOriginalReceiptSurviveCurrentChanges(t *testing.T) {
	f, member, task := worklogFixture(t)
	body := worklogBody(f, member, task, 20)
	body["note"] = strings.Repeat("中", 200)
	entry := f.call("POST", "/worklogs", body, 201)
	f.call("PATCH", "/tasks/"+task["id"].(string), f.body(map[string]any{"expected_version": task["version"], "title": "Changed current task title"}), 200)
	current := f.call("GET", "/worklogs/"+entry["id"].(string)+"?project_id="+f.project, nil, 200)
	if current["task_title"] != task["title"] || current["task_version"] != task["version"] {
		t.Fatal("task snapshot followed mutable current task", current)
	}
	replay := f.call("POST", "/worklogs", body, 201)
	if replay["version"] != float64(1) || replay["task_title"] != entry["task_title"] {
		t.Fatal(replay)
	}
	body["note"] = "different payload"
	f.call("POST", "/worklogs", body, 409)
	body["idempotency_key"] = newID()
	f.call("POST", "/worklogs", body, 409)
	if err := f.s.Initialize(context.Background()); err != nil {
		t.Fatal("current domain reopen failed", err)
	}
	f.call("GET", "/worklogs/"+entry["id"].(string)+"?project_id="+f.project, nil, 200)
}
