package dag

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func TestSchedulingReadersUseCallerTransaction(t *testing.T) {
	e, run := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}, {ID: "c", Type: "task"}}})
	assignment := claim(t, e)
	if assignment == nil {
		t.Fatal("actual attempt was not created")
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE dag_ready SET ready_at='uncommitted-ready' WHERE run_id=?`, run.ID); err != nil {
		t.Fatal(err)
	}
	const raw = `{ "outcome" : "failed", "reason" : "uncommitted resolution", "note": 1e0 }`
	if _, err := tx.ExecContext(ctx, `INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result)VALUES(?,?,2,2,'pending-worker','failed','fixture','fixture',?)`, run.ID, assignment.NodeID, raw); err != nil {
		t.Fatal(err)
	}
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	reader := e
	queue, err := reader.QueuePageTx(bounded, tx, "t", "p", ReadyQueueFilter{}, 1, "")
	if err != nil || queue.Total != 2 || len(queue.Items) != 1 || queue.Items[0].ReadyAt != "uncommitted-ready" || queue.Items[0].Admission == nil || queue.NextCursor == "" {
		t.Fatalf("queue did not share caller transaction: %+v %v", queue, err)
	}
	if _, err := reader.QueuePageTx(bounded, tx, "t", "other", ReadyQueueFilter{}, 1, queue.NextCursor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign queue cursor: %v", err)
	}
	page, err := reader.AttemptsPageTx(bounded, tx, "t", "p", run.ID, assignment.NodeID, 1, "")
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].Attempt != 2 || page.Items[0].ResultSource != "not_loaded" || page.Items[0].Result != nil || page.NextCursor == "" {
		t.Fatalf("attempt page did not share caller transaction: %+v %v", page, err)
	}
	if _, err := reader.AttemptsPageTx(bounded, tx, "t", "p", run.ID, "other", 1, page.NextCursor); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign node attempt cursor: %v", err)
	}
	deep, err := reader.GetAttemptTx(bounded, tx, "t", "p", run.ID, assignment.NodeID, 2)
	if err != nil || deep.WorkerID != "pending-worker" || deep.ResultSource != "manual_reconciliation" || deep.Reconciliation == nil || deep.Reconciliation.Reason != "uncommitted resolution" {
		t.Fatalf("attempt detail lost original source: %+v %v", deep, err)
	}
	var stored string
	if err := tx.QueryRowContext(ctx, `SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=2`, run.ID, assignment.NodeID).Scan(&stored); err != nil || stored != raw {
		t.Fatalf("reader changed raw attempt SQL text: %q %v", stored, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("reader closed caller transaction: %v", err)
	}
	if _, err := e.GetAttempt(ctx, "t", "p", run.ID, assignment.NodeID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reader committed pending attempt: %v", err)
	}
}

func TestSchedulingReadersRemainReadOnlyAndRejectNil(t *testing.T) {
	e, run := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}}})
	a := claim(t, e)
	if a == nil {
		t.Fatal("actual attempt was not created")
	}
	if _, err := e.db.ExecContext(ctx, `PRAGMA query_only=ON`); err != nil {
		t.Fatal(err)
	}
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	reader := e
	if page, err := reader.QueuePageTx(ctx, tx, "t", "p", ReadyQueueFilter{Route: ReadyQueueProjectRoute, RunID: run.ID}, 1, ""); err != nil || page.Total != 1 {
		t.Fatal(err)
	}
	if _, err := reader.AttemptsPageTx(ctx, tx, "t", "p", run.ID, a.NodeID, 1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.GetAttemptTx(ctx, tx, "t", "p", run.ID, a.NodeID, a.Attempt); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.GetAttemptTx(ctx, tx, "t", "other", run.ID, a.NodeID, a.Attempt); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := reader.AttemptsPageTx(ctx, tx, "t", "p", run.ID, "missing", 1, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, check := range []func() error{
		func() error { _, e := reader.QueuePageTx(ctx, nil, "t", "p", ReadyQueueFilter{}, 1, ""); return e },
		func() error { _, e := reader.AttemptsPageTx(ctx, nil, "t", "p", run.ID, a.NodeID, 1, ""); return e },
		func() error { _, e := reader.GetAttemptTx(ctx, nil, "t", "p", run.ID, a.NodeID, a.Attempt); return e },
	} {
		if err := check(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("nil transaction accepted: %v", err)
		}
	}
}
