package dag

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestTransactionReadersUseCallerTransaction(t *testing.T) {
	e, run := fixture(t, Workflow{Nodes: []Node{{ID: "gate", Type: "approval"}}})
	reader := e
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	w := Workflow{ID: "wf", Name: "Uncommitted workflow", Version: 2, Nodes: []Node{{ID: "gate", Type: "approval"}}}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dag_workflows(tenant_id,project_id,id,version,name,spec)VALUES('t','p','wf',2,?,?)`, w.Name, encoded(w)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE dag_runs SET workflow_name='Uncommitted run',state='paused',version=version+1 WHERE id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dag_runs(id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at)VALUES('pending-run','t','p','wf',2,'Pending summary',1,'pending','9999-01-01T00:00:00Z','9999-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	var sequence int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(sequence),0)+1 FROM dag_events`).Scan(&sequence); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO dag_events(sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data)VALUES(?,'t','p',?,'','reader_probe','2026-10-25T00:00:00Z','{"pending":true}')`, sequence, run.ID); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	workflow, err := reader.GetWorkflowTx(bounded, tx, "t", "p", "wf", 0)
	if err != nil || workflow.Version != 2 || workflow.Name != w.Name {
		t.Fatalf("reader did not use caller transaction: workflow=%+v err=%v", workflow, err)
	}
	workflows, err := reader.WorkflowsTx(bounded, tx, "t", "p")
	if err != nil || len(workflows) != 2 {
		t.Fatalf("transaction workflow list: %v %v", workflows, err)
	}
	current, err := reader.GetRunTx(bounded, tx, "t", "p", run.ID)
	if err != nil || current.WorkflowName != "Uncommitted run" || current.State != "paused" || current.Version != run.Version+1 || len(current.Nodes) != 1 {
		t.Fatalf("transaction run projection: %+v %v", current, err)
	}
	items, total, cursor, err := reader.ListRunsPageTx(bounded, tx, "t", "p", 1, "")
	if err != nil || total != 2 || len(items) != 1 || items[0].ID != "pending-run" || cursor == "" || len(items[0].Nodes) != 0 {
		t.Fatalf("transaction run page: %v %d %q %v", items, total, cursor, err)
	}
	if _, _, _, err := reader.ListRunsPageTx(bounded, tx, "t", "other", 1, cursor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign transaction cursor accepted: %v", err)
	}
	events, err := reader.EventsTx(bounded, tx, "t", "p", run.ID, sequence-1)
	if err != nil || len(events) != 1 || events[0].Type != "reader_probe" || events[0].Data["pending"] != true {
		t.Fatalf("transaction events: %v %v", events, err)
	}
	// Readers must leave the caller's transaction open and uncommitted.
	if _, err := tx.ExecContext(ctx, `DELETE FROM dag_runs WHERE id='pending-run'`); err != nil {
		t.Fatalf("reader owned caller transaction: %v", err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	published, err := e.GetWorkflow(ctx, "t", "p", "wf", 0)
	if err != nil || published.Version != 1 {
		t.Fatalf("reader committed caller workflow: %+v %v", published, err)
	}
	retained, err := e.GetRun(ctx, "t", "p", run.ID)
	if err != nil || retained.Version != run.Version || retained.WorkflowName == "Uncommitted run" {
		t.Fatalf("reader committed caller run: %+v %v", retained, err)
	}
}

func TestTransactionReadersAreScopedReadOnlyAndRejectNil(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "read-only.db"))
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "wait", Name: "Read-only timer", Nodes: []Node{{ID: "timer", Type: "timer", Config: map[string]any{"duration_seconds": 60}}}}); err != nil {
		t.Fatal(err)
	}
	run, err := e.CreateRun(ctx, "t", "p", "wait", "read-only-run")
	if err != nil {
		t.Fatal(err)
	}
	reader := e
	var before int64
	if err := e.db.QueryRowContext(ctx, `SELECT seq FROM dag_lock WHERE id=1`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := reader.GetWorkflowTx(ctx, tx, "t", "p", "wait", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.WorkflowsTx(ctx, tx, "t", "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.GetRunTx(ctx, tx, "t", "p", run.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := reader.ListRunsPageTx(ctx, tx, "t", "p", 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.EventsTx(ctx, tx, "t", "p", run.ID, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.GetWorkflowTx(ctx, tx, "t", "other", "wait", 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign workflow: %v", err)
	}
	if _, err := reader.GetRunTx(ctx, tx, "t", "other", run.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign run: %v", err)
	}
	if _, err := reader.EventsTx(ctx, tx, "t", "other", run.ID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign events: %v", err)
	}
	if _, err := reader.EventsTx(ctx, tx, "t", "p", run.ID, -1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative cursor: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := e.db.QueryRowContext(ctx, `SELECT seq FROM dag_lock WHERE id=1`).Scan(&after); err != nil || after != before {
		t.Fatalf("read generated events: %d -> %d: %v", before, after, err)
	}
	for _, check := range []func() error{
		func() error { _, e := reader.GetWorkflowTx(ctx, nil, "t", "p", "wait", 0); return e },
		func() error { _, e := reader.WorkflowsTx(ctx, nil, "t", "p"); return e },
		func() error { _, e := reader.GetRunTx(ctx, nil, "t", "p", run.ID); return e },
		func() error { _, _, _, e := reader.ListRunsPageTx(ctx, nil, "t", "p", 1, ""); return e },
		func() error { _, e := reader.EventsTx(ctx, nil, "t", "p", run.ID, 0); return e },
	} {
		if err := check(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("nil transaction accepted: %v", err)
		}
	}
}
