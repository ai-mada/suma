package dag

import (
	"context"
	"database/sql"
	"fmt"
)

func (e *Engine) TickBatch(ctx context.Context, limit int) (TickReport, error) {
	report := TickReport{Failures: []TickFailure{}}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return report, dagDiagnostic(fmt.Errorf("%w: wake batch limit exceeds 1000", ErrInvalid), "dag.wake.batch_limit", nil)
	}
	read, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return report, err
	}
	clock, err := e.clockTx(ctx, read)
	if err != nil {
		read.Rollback()
		return report, err
	}
	wakes, err := e.readDueWakes(ctx, read, clock, limit)
	if err != nil {
		read.Rollback()
		return report, err
	}
	groups := map[string][]dueWake{}
	order := []string{}
	for _, v := range wakes {
		if _, ok := groups[v.run]; !ok {
			order = append(order, v.run)
		}
		groups[v.run] = append(groups[v.run], v)
		report.Examined++
	}
	if err = read.Commit(); err != nil {
		return report, err
	}
	for _, id := range order {
		list := groups[id]
		key := list[0]
		changed := false
		err = e.writeBefore(ctx, nil, func(t *transaction) error {
			r, err := t.loadHot(key.tenant, key.project, key.run)
			if err != nil {
				return err
			}
			processed := false
			for _, w := range list {
				matches, err := t.matchesDueWake(w)
				if err != nil {
					return err
				}
				if !matches {
					continue
				}
				processed = true
				if w.state == "deferred" {
					if err = t.exec(`DELETE FROM dag_wake_retries WHERE run_id=? AND node_id=? AND kind=?`, w.run, w.node, w.kind); err != nil {
						return err
					}
				}
				if r.State == "paused" && w.kind != "lease" {
					if err = t.exec(`UPDATE dag_wakeups SET state='paused' WHERE run_id=? AND node_id=? AND kind=?`, w.run, w.node, w.kind); err != nil {
						return err
					}
					continue
				}
				if err = t.exec(`DELETE FROM dag_wakeups WHERE run_id=? AND node_id=? AND kind=?`, w.run, w.node, w.kind); err != nil {
					return err
				}
				if terminal(r.State) {
					continue
				}
				if w.kind == "propagation" {
					changed = true
					continue
				}
				n := r.find(w.node)
				if r.loadErr != nil {
					return r.loadErr
				}
				if n == nil {
					return dagDiagnostic(fmt.Errorf("%w: wake target missing", ErrInvalid), "dag.wake.target", nil)
				}
				var currentGeneration int
				if err = t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&currentGeneration); err != nil {
					return err
				}
				if currentGeneration != w.generation || n.Attempt != w.attempt || n.Fence != w.fence {
					continue
				}
				if w.kind == "lease" {
					if n.State != "running" {
						continue
					}
					if n.lease > t.now {
						if err = t.putWake(r, n, "lease", n.lease); err != nil {
							return err
						}
						continue
					}
					if WorkBuddyCloudNode(n.spec) {
						if err = t.parkCloudLease(r, n); err != nil {
							return err
						}
						changed = true
						continue
					}
					if err = t.transition(r, n, "reconciling", "lease expired; external outcome unknown"); err != nil {
						return err
					}
					if err = t.exec(`UPDATE dag_attempts SET state='reconciling' WHERE run_id=? AND node_id=? AND attempt=? AND fence=?`, r.ID, n.ID, n.Attempt, n.Fence); err != nil {
						return err
					}
					changed = true
					continue
				}
				if w.kind == "expansion" && n.Type == "map" && n.State == "waiting" {
					if err = t.materializeMapChunk(r, n); err != nil {
						return err
					}
				}
				if err = t.enqueueNode(r, n.ID); err != nil {
					return err
				}
				changed = true
			}
			if !processed {
				return nil
			}
			if err = t.advance(r); err != nil {
				return err
			}
			if r.loadErr != nil {
				return r.loadErr
			}
			return t.save(r)
		})
		if err != nil {
			if retryErr := e.deferFailedWakes(ctx, list); retryErr != nil {
				err = fmt.Errorf("%w (could not schedule wake retry: %v)", err, retryErr)
			}
			report.Failed++
			report.Failures = append(report.Failures, TickFailure{TenantID: key.tenant, ProjectID: key.project, RunID: key.run, Message: err.Error()})
			continue
		}
		if changed {
			report.Advanced++
		}
	}
	return report, nil
}

func (t *transaction) resumeRunWakes(r *runState) error {
	if err := t.exec(`UPDATE dag_wakeups SET state='pending' WHERE run_id=? AND state='paused'`, r.ID); err != nil {
		return err
	}
	rows, err := t.rows(`SELECT node_id FROM dag_wakeups WHERE run_id=? AND state='pending' AND due_at<=? AND node_id<>'' ORDER BY due_at,node_id LIMIT 2048`, r.ID, t.now)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = t.enqueueNode(r, id); err != nil {
			return err
		}
	}
	return nil
}
