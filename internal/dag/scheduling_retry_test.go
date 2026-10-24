package dag

import (
	"errors"
	"testing"
)

func TestAutomaticRetryRequiresTransientFactAndDurableBackoff(t *testing.T) {
	for _, class := range []string{"", "permanent", "protocol", "transient"} {
		t.Run(class, func(t *testing.T) {
			e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Idempotent: true, MaxRetries: 2}}})
			a := claim(t, e)
			if a.TimeoutSeconds != 300 {
				t.Fatalf("first dispatch lacks overall window: %+v", a)
			}
			err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "failed", ExitCode: 1, FailureClass: class})
			if err != nil {
				t.Fatal(err)
			}
			current, _ := e.GetRun(ctx, "t", "p", r.ID)
			if class != "transient" {
				if current.State != "failed" {
					t.Fatalf("unclassified/permanent failure retried: %+v", current)
				}
				return
			}
			if claim(t, e) != nil {
				t.Fatal("backoff was not durable")
			}
			if _, err = e.db.Exec(`UPDATE dag_nodes SET wake_at=1 WHERE run_id=? AND id='a'`, r.ID); err != nil {
				t.Fatal(err)
			}
			if _, err = e.db.Exec(`UPDATE dag_wakeups SET due_at=1 WHERE run_id=? AND node_id='a' AND kind='retry'`, r.ID); err != nil {
				t.Fatal(err)
			}
			if err = e.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			b := claim(t, e)
			if b == nil || b.Attempt != 2 || b.TimeoutSeconds > 300 || b.TimeoutSeconds <= 0 {
				t.Fatal(b)
			}
		})
	}
}

func TestAutomaticRetryDeadlineAndManualRetryStartDistinctWindows(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Idempotent: true, MaxRetries: 2, TimeoutSeconds: 20}}})
	a := claim(t, e)
	if a.TimeoutSeconds != 20 {
		t.Fatal(a)
	}
	if _, err := e.db.Exec(`UPDATE dag_retry_windows SET deadline_at=1 WHERE run_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "fail", ExitCode: 1, FailureClass: "transient"}); err != nil {
		t.Fatal(err)
	}
	current, _ := e.GetRun(ctx, "t", "p", r.ID)
	if current.State != "failed" {
		t.Fatal("expired window auto retried", current)
	}
	current, err := e.Command(ctx, "t", "p", r.ID, "retry", "manual-new-window", current.Version, map[string]any{"node_id": "a", "reason": "operator verified cause"})
	if err != nil {
		t.Fatal(err)
	}
	b := claim(t, e)
	if b == nil || b.Attempt != 2 || b.TimeoutSeconds != 20 {
		t.Fatal("manual retry reused expired window", b)
	}
	var windows int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_retry_windows WHERE run_id=?`, r.ID).Scan(&windows)
	if windows != 2 {
		t.Fatal(windows)
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: b.RunID, NodeID: b.NodeID, Attempt: b.Attempt, Fence: b.Fence, WorkerID: b.WorkerID, OperationID: "invalid-class", FailureClass: "network_guess"}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestExecutionDurationAndRetryCompileArithmeticBounds(t *testing.T) {
	if err := Compile(Workflow{Nodes: []Node{{ID: "a", Type: "task", TimeoutSeconds: maxExecutionTimeoutSeconds, Idempotent: true, MaxRetries: 2}}}); err != nil {
		t.Fatal(err)
	}
	for _, node := range []Node{{ID: "a", Type: "task", TimeoutSeconds: maxExecutionTimeoutSeconds + 1}, {ID: "a", Type: "task", Idempotent: true, MaxRetries: 3}, {ID: "a", Type: "task", Resources: map[string]int64{"budget": 0}}, {ID: "a", Type: "task", Resources: map[string]int64{"execution_slots": 0}}} {
		if err := Compile(Workflow{Nodes: []Node{node}}); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid arithmetic boundary admitted", node, err)
		}
	}
}
