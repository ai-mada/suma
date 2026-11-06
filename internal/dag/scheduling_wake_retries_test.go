package dag

import (
	"path/filepath"
	"testing"
)

func TestTickBatchDefersFailedWakeWithoutStarvingAnotherRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dag.db")
	e := openEngine(t, path)
	workflow, err := e.Publish(ctx, "t", "p", Workflow{
		ID: "timers", Name: "due timers",
		Nodes: []Node{{ID: "timer", Type: "timer", Config: map[string]any{"duration_seconds": 3600}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := e.db.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	createDueRun := func(operationID string, due int64) Run {
		t.Helper()
		r, err := e.CreateRun(ctx, "t", "p", workflow.ID, operationID)
		if err != nil {
			t.Fatal(err)
		}
		exec(`UPDATE dag_nodes SET wake_at=? WHERE run_id=? AND id='timer'`, due, r.ID)
		exec(`UPDATE dag_wakeups SET due_at=? WHERE run_id=? AND node_id='timer' AND kind='timer'`, due, r.ID)
		return r
	}
	bad := createDueRun("bad", 1)
	good := createDueRun("good", 2)

	type wakeIdentity struct {
		generation, attempt int
		fence, due          int64
	}
	readBadWake := func() (wakeIdentity, string) {
		t.Helper()
		var identity wakeIdentity
		var state string
		if err := e.db.QueryRow(`SELECT generation,attempt,fence,due_at,state FROM dag_wakeups WHERE run_id=? AND node_id='timer' AND kind='timer'`, bad.ID).
			Scan(&identity.generation, &identity.attempt, &identity.fence, &identity.due, &state); err != nil {
			t.Fatal(err)
		}
		return identity, state
	}
	original, _ := readBadWake()
	var spec string
	if err = e.db.QueryRow(`SELECT spec FROM dag_nodes WHERE run_id=? AND id='timer'`, bad.ID).Scan(&spec); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE dag_nodes SET spec='{' WHERE run_id=? AND id='timer'`, bad.ID)
	var beforeFailure int64
	if err = e.db.QueryRow(`SELECT unixepoch('now')`).Scan(&beforeFailure); err != nil {
		t.Fatal(err)
	}

	first, err := e.TickBatch(ctx, 1)
	if err != nil || first.Examined != 1 || first.Advanced != 0 || first.Failed != 1 ||
		len(first.Failures) != 1 || first.Failures[0].RunID != bad.ID {
		t.Fatalf("expected the first tick to fail on the earliest wake, got %+v, err=%v", first, err)
	}
	if err = e.db.Close(); err != nil {
		t.Fatal(err)
	}
	e = openEngine(t, path)
	second, err := e.TickBatch(ctx, 1)
	if err != nil || second.Examined != 1 || second.Advanced != 1 || second.Failed != 0 {
		t.Fatalf("failed wake starved the healthy run after reopening SQLite, got %+v, err=%v", second, err)
	}
	assertSucceeded := func(id string) {
		t.Helper()
		r, err := e.GetRun(ctx, "t", "p", id)
		if err != nil || r.State != "succeeded" {
			t.Fatalf("run %s state=%q, err=%v, want succeeded", id, r.State, err)
		}
	}
	assertSucceeded(good.ID)
	if identity, state := readBadWake(); identity != original || state != "deferred" {
		t.Fatalf("failed wake identity=%+v, state=%q, want original identity %+v and deferred", identity, state, original)
	}
	var failures int
	var nextAttempt int64
	if err = e.db.QueryRow(`SELECT failure_count,next_attempt_at FROM dag_wake_retries WHERE run_id=? AND node_id='timer' AND kind='timer'`, bad.ID).
		Scan(&failures, &nextAttempt); err != nil {
		t.Fatal(err)
	}
	if failures != 1 || nextAttempt <= beforeFailure {
		t.Fatalf("failures=%d, next attempt=%d, want one failure and backoff after %d", failures, nextAttempt, beforeFailure)
	}

	exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id='timer'`, spec, bad.ID)
	exec(`UPDATE dag_wake_retries SET next_attempt_at=0 WHERE run_id=? AND node_id='timer' AND kind='timer'`, bad.ID)
	last, err := e.TickBatch(ctx, 1)
	if err != nil || last.Examined != 1 || last.Advanced != 1 || last.Failed != 0 {
		t.Fatalf("repaired wake was not retried, got %+v, err=%v", last, err)
	}
	assertSucceeded(bad.ID)
	var remaining int
	if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_wake_retries WHERE run_id=?`, bad.ID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("successful wake left %d retry records", remaining)
	}
}
