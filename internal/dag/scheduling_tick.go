package dag

import (
	"context"
	"database/sql"
	"fmt"
)

type dueWake struct {
	tenant, project, run, node, kind string
	generation, attempt              int
	fence, due                       int64
}

func (e *Engine) TickBatch(ctx context.Context, limit int) (TickReport, error) {
	report := TickReport{Failures: []TickFailure{}}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		return report, fmt.Errorf("%w: wake batch limit exceeds 1000", ErrInvalid)
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
	rows, err := read.QueryContext(ctx, e.q(`SELECT tenant_id,project_id,run_id,node_id,kind,generation,attempt,fence,due_at FROM dag_wakeups WHERE state='pending' AND due_at<=? ORDER BY due_at,run_id,node_id,kind LIMIT ?`), clock, limit)
	if err != nil {
		read.Rollback()
		return report, err
	}
	groups := map[string][]dueWake{}
	order := []string{}
	for rows.Next() {
		var v dueWake
		if err = rows.Scan(&v.tenant, &v.project, &v.run, &v.node, &v.kind, &v.generation, &v.attempt, &v.fence, &v.due); err != nil {
			rows.Close()
			read.Rollback()
			return report, err
		}
		if _, ok := groups[v.run]; !ok {
			order = append(order, v.run)
		}
		groups[v.run] = append(groups[v.run], v)
		report.Examined++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		read.Rollback()
		return report, err
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
			for _, w := range list {
				var generation, attempt int
				var fence, due int64
				var state string
				err = t.row(`SELECT generation,attempt,fence,due_at,state FROM dag_wakeups WHERE run_id=? AND node_id=? AND kind=?`, w.run, w.node, w.kind).Scan(&generation, &attempt, &fence, &due, &state)
				if err == sql.ErrNoRows {
					continue
				}
				if err != nil {
					return err
				}
				if state != "pending" || due > t.now || generation != w.generation || attempt != w.attempt || fence != w.fence || due != w.due {
					continue
				}
				if r.State == "paused" && w.kind != "lease" {
					if err = t.exec(`UPDATE dag_wakeups SET state='paused' WHERE run_id=? AND node_id=? AND kind=? AND generation=? AND attempt=? AND fence=? AND due_at=?`, w.run, w.node, w.kind, generation, attempt, fence, due); err != nil {
						return err
					}
					continue
				}
				if err = t.exec(`DELETE FROM dag_wakeups WHERE run_id=? AND node_id=? AND kind=? AND generation=? AND attempt=? AND fence=? AND due_at=?`, w.run, w.node, w.kind, generation, attempt, fence, due); err != nil {
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
					return fmt.Errorf("%w: wake target missing", ErrInvalid)
				}
				var currentGeneration int
				if err = t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&currentGeneration); err != nil {
					return err
				}
				if currentGeneration != generation || n.Attempt != attempt || n.Fence != fence {
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
			if err = t.advance(r); err != nil {
				return err
			}
			if r.loadErr != nil {
				return r.loadErr
			}
			return t.save(r)
		})
		if err != nil {
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
