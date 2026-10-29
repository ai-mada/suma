package platform

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestWorklogCrossProjectAtomicLastMinuteAndNoProjectLeak(t *testing.T) {
	f, member, task := worklogFixture(t)
	ctx := context.Background()
	f.call("POST", "/worklogs", worklogBody(f, member, task, 1439), 201)
	other := f.call("POST", "/projects", map[string]any{"title": "Secret other project", "idempotency_key": newID()}, 201)["id"].(string)
	resource, err := f.s.Create(ctx, f.actor, other, "members", map[string]any{"title": "Same person", "kind": "human", "data": map[string]any{"user_id": f.user.ID}, "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := f.s.Create(ctx, f.actor, other, "tasks", map[string]any{"title": "Hidden task", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	var seq int
	var name, path string
	if err = f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &path); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	second := &Service{DB: db, Dialect: "sqlite", DataDir: f.s.DataDir}
	firstReq := WorklogCreateRequest{ProjectID: f.project, MemberID: member["id"].(string), TaskID: task["id"].(string), ExpectedTaskVersion: int(task["version"].(float64)), WorkDate: "2026-10-29", Minutes: 1, IdempotencyKey: newID()}
	secondReq := worklogHumanRequest(other, resource, otherTask, 1)
	start, results := make(chan struct{}), make(chan error, 2)
	go func() { <-start; _, e := f.s.CreateWorklog(ctx, f.actor, f.project, firstReq); results <- e }()
	go func() { <-start; _, e := second.CreateWorklog(ctx, f.actor, other, secondReq); results <- e }()
	close(start)
	success, denied := 0, 0
	for i := 0; i < 2; i++ {
		e := <-results
		if e == nil {
			success++
			continue
		}
		var api *APIError
		if !errors.As(e, &api) || api.Status != 409 {
			t.Fatal("concurrent reservation failed unexpectedly", e)
		}
		if strings.Contains(stringify(api), other) || strings.Contains(stringify(api), otherTask.ID) || api.Details != nil {
			t.Fatal("daily cap leaked project data", api)
		}
		denied++
	}
	if success != 1 || denied != 1 {
		t.Fatalf("last minute must have one winner: %d/%d", success, denied)
	}
	var ledger, actual int
	if err = f.s.DB.QueryRow(`SELECT active_minutes FROM suma_worklog_daily_totals WHERE tenant_id=? AND user_id=? AND work_date=?`, f.user.TenantID, f.user.ID, firstReq.WorkDate).Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT SUM(minutes) FROM suma_worklogs WHERE tenant_id=? AND user_id=? AND work_date=? AND status='active'`, f.user.TenantID, f.user.ID, firstReq.WorkDate).Scan(&actual); err != nil {
		t.Fatal(err)
	}
	if ledger != 1440 || actual != ledger {
		t.Fatal("ledger diverged", ledger, actual)
	}
}

func TestWorklogFailedDateMoveRollsBackAndVoidReleasesMinutes(t *testing.T) {
	f, member, task := worklogFixture(t)
	first := f.call("POST", "/worklogs", worklogBody(f, member, task, 120), 201)
	full := worklogBody(f, member, task, 1440)
	full["work_date"] = "2026-10-30"
	f.call("POST", "/worklogs", full, 201)
	path := "/worklogs/" + first["id"].(string)
	before := [3]int{domainACount(t, f.s, "suma_worklog_revisions"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}
	request := f.body(map[string]any{"expected_version": 1, "task_id": task["id"], "expected_task_version": task["version"], "work_date": "2026-10-30", "minutes": 1, "note": "", "reason": "move actual day", "on_behalf_ack": false})
	f.call("POST", path+"/corrections", request, 409)
	if after := [3]int{domainACount(t, f.s, "suma_worklog_revisions"), domainACount(t, f.s, "suma_operations"), domainACount(t, f.s, "suma_activity")}; before != after {
		t.Fatal("failed ledger reservation left writes", before, after)
	}
	var held int
	if err := f.s.DB.QueryRow(`SELECT active_minutes FROM suma_worklog_daily_totals WHERE tenant_id=? AND user_id=? AND work_date=?`, f.user.TenantID, f.user.ID, "2026-10-29").Scan(&held); err != nil || held != 120 {
		t.Fatal("old reservation was not rolled back", held, err)
	}
	request["work_date"], request["minutes"] = "2026-10-29", 60
	f.call("POST", path+"/corrections", request, 200)
	f.call("POST", path+"/void", f.body(map[string]any{"expected_version": 2, "reason": "duplicate", "on_behalf_ack": false}), 200)
	f.call("POST", "/worklogs", worklogBody(f, member, task, 1440), 201)
}
