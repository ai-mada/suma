package dag

import (
	"database/sql"
	"fmt"
)

func (t *transaction) attemptWindow(r *runState, n *nodeState, start bool) (int, error) {
	if r.kernelGeneration < 2 {
		return n.spec.TimeoutSeconds, nil
	}
	var generation int
	if err := t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&generation); err != nil {
		return 0, err
	}
	var deadline int64
	err := t.row(`SELECT deadline_at FROM dag_retry_windows WHERE run_id=? AND node_id=? AND generation=?`, r.ID, n.ID, generation).Scan(&deadline)
	if err == nil {
		if deadline <= t.now {
			return 0, nil
		}
		return int(deadline - t.now), nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	seconds := n.spec.TimeoutSeconds
	if seconds == 0 {
		seconds = 300
	}
	if start {
		if err = t.exec(`INSERT INTO dag_retry_windows(run_id,node_id,generation,base_attempt,started_at,deadline_at)VALUES(?,?,?,?,?,?)`, r.ID, n.ID, generation, n.Attempt+1, t.now, t.now+int64(seconds)); err != nil {
			return 0, err
		}
	}
	return seconds, nil
}

func (t *transaction) scheduleTransientRetry(r *runState, n *nodeState, result Result) (bool, error) {
	if !n.spec.Idempotent || n.spec.MaxRetries == 0 || result.OutcomeUnknown || result.Stopped {
		return false, nil
	}
	if r.kernelGeneration < 2 {
		if n.Attempt > n.spec.MaxRetries {
			return false, nil
		}
		n.State = "pending"
		n.CompletedAt = ""
		n.WorkerID = ""
		if err := t.enqueueNode(r, n.ID); err != nil {
			return false, err
		}
		return true, t.event(r, n.ID, "retry_scheduled", map[string]any{"attempt": n.Attempt + 1, "semantics_generation": 1})
	}
	if result.FailureClass != "transient" {
		return false, nil
	}
	var base int
	var deadline int64
	if err := t.row(`SELECT w.base_attempt,w.deadline_at FROM dag_retry_windows w JOIN dag_node_dependencies d ON d.run_id=w.run_id AND d.node_id=w.node_id AND d.generation=w.generation WHERE w.run_id=? AND w.node_id=?`, r.ID, n.ID).Scan(&base, &deadline); err != nil {
		return false, err
	}
	retries := n.Attempt - base + 1
	if retries > n.spec.MaxRetries || retries > 2 {
		return false, nil
	}
	backoff := int64(1 << uint(retries-1))
	if t.now+backoff >= deadline {
		n.Reason = "automatic retry deadline exhausted"
		return false, t.event(r, n.ID, "retry_blocked", map[string]any{"reason": n.Reason, "deadline_at": stamp(deadline), "attempt": n.Attempt})
	}
	n.State = "pending"
	n.CompletedAt = ""
	n.WorkerID = ""
	n.wake = t.now + backoff
	if err := t.enqueueNode(r, n.ID); err != nil {
		return false, err
	}
	if err := t.syncWake(r, n); err != nil {
		return false, err
	}
	return true, t.event(r, n.ID, "retry_scheduled", map[string]any{"attempt": n.Attempt + 1, "failure_class": result.FailureClass, "backoff_seconds": backoff, "ready_at": stamp(n.wake), "deadline_at": stamp(deadline)})
}

func validateFailureClass(value string) error {
	switch value {
	case "", "transient", "permanent", "protocol":
		return nil
	}
	return dagDiagnostic(fmt.Errorf("%w: unknown execution failure class", ErrInvalid), "dag.result.failure_class", nil)
}
