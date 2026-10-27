package dag

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSchedulingRestartKeepsDurableAttemptAndAdmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	e := openEngine(t, path)
	w, err := e.Publish(ctx, "t", "p", Workflow{ID: "current", Name: "current", Nodes: []Node{{ID: "first", Type: "task", Priority: 9}, {ID: "next", Type: "task"}}})
	if err != nil {
		t.Fatal(err)
	}
	run, err := e.CreateRun(ctx, "t", "p", w.ID, "create-current")
	if err != nil {
		t.Fatal(err)
	}
	active := claim(t, e)
	if active == nil || active.NodeID != "first" {
		t.Fatal("fresh queue did not honor current priority", active)
	}
	before, err := e.GetRun(ctx, "t", "p", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	events, err := e.Events(ctx, "t", "p", run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = e.db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := openEngine(t, path)
	after, err := reopened.GetRun(ctx, "t", "p", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterEvents, err := reopened.Events(ctx, "t", "p", run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(events, afterEvents) {
		t.Fatal("restart changed current run or event facts")
	}
	if next := claim(t, reopened); next != nil {
		t.Fatal("restart lost the active attempt's admission hold", next)
	}
	var used, held int
	if err = reopened.db.QueryRow(`SELECT used FROM dag_resource_accounts WHERE tenant_id='t' AND project_id='p' AND name='execution_slots'`).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if err = reopened.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=? AND state='active'`, run.ID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	if used != 1 || held != 1 {
		t.Fatalf("restart changed active resource accounting: used=%d held=%d", used, held)
	}
	done(t, reopened, active, 0)
	next := claim(t, reopened)
	if next == nil || next.NodeID != "next" {
		t.Fatal("current ready work did not resume after confirmed completion", next)
	}
	done(t, reopened, next, 0)
	completed, err := reopened.GetRun(ctx, "t", "p", run.ID)
	if err != nil || completed.State != "succeeded" {
		t.Fatal(completed, err)
	}
}
