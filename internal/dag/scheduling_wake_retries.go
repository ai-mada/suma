package dag

import (
	"context"
	"database/sql"
	"math"
)

type dueWake struct {
	tenant, project, run, node, kind string
	state                            string
	generation, attempt, failures    int
	fence, due, eligible             int64
}

// Read at most one batch from each indexed queue before merging their heads.
// Deferred wakes rejoin by retry eligibility, with the original due time intact.
func (e *Engine) readDueWakes(ctx context.Context, tx *sql.Tx, clock int64, limit int) ([]dueWake, error) {
	rows, err := tx.QueryContext(ctx, e.q(`
		SELECT tenant_id,project_id,run_id,node_id,kind,state,generation,attempt,fence,due_at,eligible_at,failure_count
		FROM (
			SELECT pending.*,due_at AS eligible_at,0 AS failure_count
			FROM (
				SELECT tenant_id,project_id,run_id,node_id,kind,state,generation,attempt,fence,due_at
				FROM dag_wakeups WHERE state='pending' AND due_at<=?
				ORDER BY due_at,run_id,node_id,kind LIMIT ?
			) AS pending
			UNION ALL
			SELECT w.tenant_id,w.project_id,w.run_id,w.node_id,w.kind,w.state,w.generation,w.attempt,w.fence,w.due_at,retry.next_attempt_at,retry.failure_count
			FROM (
				SELECT run_id,node_id,kind,next_attempt_at,failure_count
				FROM dag_wake_retries WHERE next_attempt_at<=?
				ORDER BY next_attempt_at,run_id,node_id,kind LIMIT ?
			) AS retry
			JOIN dag_wakeups w ON w.run_id=retry.run_id AND w.node_id=retry.node_id AND w.kind=retry.kind
			WHERE w.state='deferred'
		) AS wakes
		ORDER BY eligible_at,run_id,node_id,kind LIMIT ?`), clock, limit, clock, limit, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	wakes := make([]dueWake, 0, limit)
	for rows.Next() {
		var w dueWake
		if err = rows.Scan(&w.tenant, &w.project, &w.run, &w.node, &w.kind, &w.state, &w.generation, &w.attempt, &w.fence, &w.due, &w.eligible, &w.failures); err != nil {
			return nil, err
		}
		wakes = append(wakes, w)
	}
	return wakes, rows.Err()
}

// Call under the project write guard before consuming or deferring the wake.
func (t *transaction) matchesDueWake(w dueWake) (bool, error) {
	current := w
	err := t.row(`SELECT w.state,w.generation,w.attempt,w.fence,w.due_at,COALESCE(r.next_attempt_at,w.due_at),COALESCE(r.failure_count,0)
		FROM dag_wakeups w LEFT JOIN dag_wake_retries r ON r.run_id=w.run_id AND r.node_id=w.node_id AND r.kind=w.kind
		WHERE w.run_id=? AND w.node_id=? AND w.kind=? AND w.tenant_id=? AND w.project_id=?`, w.run, w.node, w.kind, w.tenant, w.project).
		Scan(&current.state, &current.generation, &current.attempt, &current.fence, &current.due, &current.eligible, &current.failures)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil && current == w && current.due <= t.now && current.eligible <= t.now, err
}

func (e *Engine) deferFailedWakes(ctx context.Context, wakes []dueWake) error {
	return e.write(ctx, func(t *transaction) error {
		// Wake metadata is serialized by the project guard. It can be parked
		// even when the failed run cannot acquire or load its family state.
		if err := t.guardProject(wakes[0].tenant, wakes[0].project); err != nil {
			return err
		}
		for _, w := range wakes {
			matches, err := t.matchesDueWake(w)
			if err != nil {
				return err
			}
			if !matches {
				continue
			}
			failures := min(w.failures, math.MaxInt32-1) + 1
			delay := int64(5) << min(failures-1, 4)
			next := t.now + min(delay, 60)
			if w.state == "deferred" {
				if err = t.exec(`UPDATE dag_wake_retries SET failure_count=?,next_attempt_at=? WHERE run_id=? AND node_id=? AND kind=?`, failures, next, w.run, w.node, w.kind); err != nil {
					return err
				}
				continue
			}
			if err = t.exec(`INSERT INTO dag_wake_retries(run_id,node_id,kind,failure_count,next_attempt_at) VALUES(?,?,?,?,?)`, w.run, w.node, w.kind, failures, next); err != nil {
				return err
			}
			if err = t.exec(`UPDATE dag_wakeups SET state='deferred' WHERE run_id=? AND node_id=? AND kind=?`, w.run, w.node, w.kind); err != nil {
				return err
			}
		}
		return nil
	})
}

func (t *transaction) deleteWakes(run, node, kind string) error {
	where := ` WHERE run_id=? AND node_id=?`
	args := []any{run, node}
	if kind != "" {
		where += ` AND kind=?`
		args = append(args, kind)
	}
	// Also clear the child explicitly for SQLite handles without FK enforcement.
	if err := t.exec(`DELETE FROM dag_wake_retries`+where, args...); err != nil {
		return err
	}
	return t.exec(`DELETE FROM dag_wakeups`+where, args...)
}
