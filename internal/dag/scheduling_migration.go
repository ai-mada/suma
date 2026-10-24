package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// A maintenance transaction adds derived indexes without advancing a run,
// changing its fixed semantics, rewriting an attempt, or appending an event.
// The ledger marker is committed atomically with the complete backfill.
func (e *Engine) migrateSchedulingData(ctx context.Context) error {
	return e.write(ctx, func(t *transaction) error {
		if err := t.insertIgnore(`INSERT INTO dag_kernel_migration_lock(id,version)VALUES(1,0)`, "id"); err != nil {
			return err
		}
		if err := t.exec(`UPDATE dag_kernel_migration_lock SET version=version WHERE id=1`); err != nil {
			return err
		}
		var state string
		err := t.row(`SELECT state FROM dag_kernel_migrations WHERE version=2`).Scan(&state)
		if err == nil && state == "complete" {
			return nil
		}
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		type scope struct{ tenant, project string }
		scopes := []scope{}
		rows, err := t.rows(`SELECT tenant_id,project_id FROM dag_runs UNION SELECT tenant_id,project_id FROM dag_resource_limits ORDER BY tenant_id,project_id`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s scope
			if err = rows.Scan(&s.tenant, &s.project); err != nil {
				rows.Close()
				return err
			}
			scopes = append(scopes, s)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, s := range scopes {
			if err = t.migrateSchedulingProject(s.tenant, s.project); err != nil {
				return err
			}
		}
		return t.exec(`INSERT INTO dag_kernel_migrations(version,state,completed_at)VALUES(2,'complete',?)`, stamp(t.now))
	})
}

func (t *transaction) migrateSchedulingProject(tenant, project string) error {
	if err := t.guardProject(tenant, project); err != nil {
		return err
	}
	var previousSequence int64
	if err := t.row(`SELECT COALESCE(MAX(sequence),0) FROM (SELECT q.sequence AS sequence FROM dag_ready q JOIN dag_runs r ON r.id=q.run_id WHERE r.tenant_id=? AND r.project_id=? UNION ALL SELECT f.last_claim_sequence AS sequence FROM dag_run_fairness f JOIN dag_runs r ON r.id=f.run_id WHERE r.tenant_id=? AND r.project_id=?) previous_queue`, tenant, project, tenant, project).Scan(&previousSequence); err != nil {
		return err
	}
	if err := t.exec(`UPDATE dag_scheduler_projects SET queue_sequence=CASE WHEN queue_sequence<? THEN ? ELSE queue_sequence END WHERE tenant_id=? AND project_id=?`, previousSequence, previousSequence, tenant, project); err != nil {
		return err
	}
	policy, err := t.e.readSchedulingPolicyTx(t.ctx, t.tx, tenant, project, true, false)
	if err != nil {
		return err
	}
	if policy.Default {
		input := SchedulingPolicyInput{ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{}}
		var knownBudget int64
		rows, err := t.rows(`SELECT name,capacity,consumed FROM dag_resource_limits WHERE tenant_id=? AND project_id=? ORDER BY name`, tenant, project)
		if err != nil {
			return err
		}
		for rows.Next() {
			var name string
			var cap, consumed int64
			if err = rows.Scan(&name, &cap, &consumed); err != nil {
				rows.Close()
				return err
			}
			if !safeSchedulingInteger(cap) || !safeSchedulingInteger(consumed) {
				rows.Close()
				return fmt.Errorf("%w: legacy resource outside safe bounds", ErrConflict)
			}
			v := cap
			switch name {
			case "execution_slots":
				input.ConcurrencyLimit = v
			case "budget":
				input.BudgetLimit = &v
				knownBudget = consumed
			default:
				input.ResourceLimits[name] = &v
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if _, err = t.e.WriteSchedulingPolicyTx(t.ctx, t.tx, tenant, project, 0, input); err != nil {
			return err
		}
		if err = t.exec(`UPDATE dag_scheduling_policies SET budget_spent=? WHERE tenant_id=? AND project_id=?`, knownBudget, tenant, project); err != nil {
			return err
		}
		if err = t.exec(`UPDATE dag_resource_accounts SET consumed=? WHERE tenant_id=? AND project_id=? AND name='budget'`, knownBudget, tenant, project); err != nil {
			return err
		}
	}
	rows, err := t.rows(`SELECT id FROM dag_runs WHERE tenant_id=? AND project_id=? ORDER BY id`, tenant, project)
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
		var present int
		err = t.row(`SELECT 1 FROM dag_run_counters WHERE run_id=?`, id).Scan(&present)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		if err = t.guardFamily(tenant, project, id); err != nil {
			return err
		}
		r, err := load(t.ctx, t.tx, t.e, tenant, project, id)
		if err != nil {
			return err
		}
		if err = t.initializeGraph(r, true); err != nil {
			return err
		}
		for _, n := range r.nodes {
			if err = t.syncWake(r, n); err != nil {
				return err
			}
			if n.State == "ready" {
				var sequence int64
				if err = t.row(`SELECT sequence FROM dag_ready WHERE run_id=? AND node_id=?`, id, n.ID).Scan(&sequence); err != nil {
					return fmt.Errorf("legacy ready entry missing: %w", err)
				}
				readyAt := ""
				err = t.row(`SELECT at_time FROM dag_events WHERE run_id=? AND node_id=? AND event_type='node_ready' ORDER BY sequence DESC LIMIT 1`, id, n.ID).Scan(&readyAt)
				if err != nil && err != sql.ErrNoRows {
					return err
				}
				command := ""
				if len(n.spec.Command) == 1 {
					command = n.spec.Command[0]
				}
				required, _ := n.spec.Config["capability"].(string)
				if n.Type == "external_workflow" {
					plugin, _ := n.spec.Config["plugin"].(string)
					required = "external_workflow:" + plugin
				}
				if err = t.exec(`INSERT INTO dag_ready_v2(run_id,node_id,tenant_id,project_id,generation,sequence,ready_at,node_type,command_id,required_capability)VALUES(?,?,?,?,1,?,?,?,?,?)`, id, n.ID, tenant, project, sequence, readyAt, n.Type, command, required); err != nil {
					return err
				}
				if err = t.exec(`UPDATE dag_node_dependencies SET ready_at=? WHERE run_id=? AND node_id=?`, readyAt, id, n.ID); err != nil {
					return err
				}
				if err = t.exec(`UPDATE dag_scheduler_projects SET queue_sequence=CASE WHEN queue_sequence<? THEN ? ELSE queue_sequence END WHERE tenant_id=? AND project_id=?`, sequence, sequence, tenant, project); err != nil {
					return err
				}
			}
			if n.Attempt > 0 && (n.State == "running" || n.State == "reconciling") {
				if err = t.migrateLegacyReservations(r, n); err != nil {
					return err
				}
			}
		}
		if err = t.refreshRunTurn(r); err != nil {
			return err
		}
	}
	return nil
}

func (t *transaction) migrateLegacyReservations(r *runState, n *nodeState) error {
	if n.Type == "subworkflow" {
		return nil
	}
	requests := map[string]int64{}
	rows, err := t.rows(`SELECT name,amount FROM dag_reservations WHERE run_id=? AND node_id=? ORDER BY name`, r.ID, n.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		var amount int64
		if err = rows.Scan(&name, &amount); err != nil {
			rows.Close()
			return err
		}
		if !safeSchedulingInteger(amount) {
			rows.Close()
			return fmt.Errorf("%w: legacy reservation outside safe bounds", ErrConflict)
		}
		requests[name] = amount
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	// A missing old reservation is not proof that the process has stopped. Hold
	// its new default slot conservatively, without inventing past budget spend.
	var raw string
	var result Result
	if err = t.row(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, r.ID, n.ID, n.Attempt).Scan(&raw); err != nil {
		return err
	}
	confirmedStopped := json.Unmarshal([]byte(raw), &result) == nil && result.RunID == r.ID && result.NodeID == n.ID && result.Attempt == n.Attempt && result.Fence == n.Fence && result.Stopped
	if !confirmedStopped && requests["execution_slots"] < 1 {
		requests["execution_slots"] = 1
	}
	policy, err := t.e.readSchedulingPolicyTx(t.ctx, t.tx, r.TenantID, r.ProjectID, true, false)
	if err != nil {
		return err
	}
	for name, amount := range requests {
		if name == "budget" || amount == 0 {
			continue
		}
		if confirmedStopped && resourceClass(name) == "process" {
			continue
		}
		var cap *int64
		if name == "execution_slots" {
			v := policy.ConcurrencyLimit
			cap = &v
		} else {
			cap = policy.ResourceLimits[name]
		}
		if err = t.e.ensureResourceAccountTx(t.ctx, t.tx, r.TenantID, r.ProjectID, name, cap); err != nil {
			return err
		}
		if err = t.exec(`INSERT INTO dag_resource_ledger(run_id,node_id,attempt,fence,name,tenant_id,project_id,amount,release_class,state)VALUES(?,?,?,?,?,?,?,?,?,'active')`, r.ID, n.ID, n.Attempt, n.Fence, name, r.TenantID, r.ProjectID, amount, resourceClass(name)); err != nil {
			return err
		}
		res, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_resource_accounts SET used=used+?,version=version+1 WHERE tenant_id=? AND project_id=? AND name=? AND used<=?`), amount, r.TenantID, r.ProjectID, name, maxSchedulingInteger-amount)
		if err != nil {
			return err
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("%w: legacy resource aggregate overflow", ErrConflict)
		}
		if err = t.insertIgnore(`INSERT INTO dag_reservations(run_id,node_id,name,amount)VALUES(?,?,?,?)`, "run_id,node_id,name", r.ID, n.ID, name, amount); err != nil {
			return err
		}
	}
	return nil
}
