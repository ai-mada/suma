package dag

import (
	"database/sql"
	"fmt"
)

type poolResourceCheck struct {
	request                 ResourceRequest
	capacity                sql.NullInt64
	used, consumed, version int64
}

// All requested resources are checked before either project or pool is charged.
// The pool guard remains held through project CAS and receipt/event commit.
func (t *transaction) checkPoolResources(r *runState, n *nodeState) ([]poolResourceCheck, bool, error) {
	policy, err := t.e.readPoolPolicy(t.ctx, t.tx, r.ExecutionPoolID, true)
	if err != nil {
		return nil, false, err
	}
	if policy.Version == 0 {
		return nil, false, ErrNotFound
	}
	checks := []poolResourceCheck{}
	for _, request := range nodeResourceRequests(n.spec) {
		var cap *int64
		switch request.Name {
		case "execution_slots":
			v := policy.ConcurrencyLimit
			cap = &v
		case "budget":
			cap = policy.BudgetLimit
		default:
			cap = policy.ResourceLimits[request.Name]
		}
		if err = t.e.ensurePoolAccount(t.ctx, t.tx, r.ExecutionPoolID, request.Name, cap); err != nil {
			return nil, false, err
		}
		c := poolResourceCheck{request: request}
		if err = t.row(`SELECT capacity,used,consumed,version FROM dag_execution_pool_accounts WHERE pool_id=? AND name=?`+t.e.currentReadSuffix(), r.ExecutionPoolID, request.Name).Scan(&c.capacity, &c.used, &c.consumed, &c.version); err != nil {
			return nil, false, err
		}
		used := c.used
		if request.ReleaseClass == "cumulative" {
			used = c.consumed
		}
		if !safeSchedulingInteger(used) || !safeSchedulingInteger(request.Amount) || request.Amount > maxSchedulingInteger-used || c.capacity.Valid && request.Amount > c.capacity.Int64-used {
			return nil, false, nil
		}
		checks = append(checks, c)
	}
	return checks, true, nil
}
func (t *transaction) reservePoolResources(r *runState, n *nodeState, checks []poolResourceCheck) error {
	for _, c := range checks {
		req := c.request
		column, state := "used", "active"
		if req.ReleaseClass == "cumulative" {
			column, state = "consumed", "charged"
		}
		res, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_execution_pool_accounts SET `+column+`=`+column+`+?,version=version+1 WHERE pool_id=? AND name=? AND version=? AND `+column+`<=? AND (capacity IS NULL OR ?<=capacity-`+column+`)`), req.Amount, r.ExecutionPoolID, req.Name, c.version, maxSchedulingInteger-req.Amount, req.Amount)
		if err != nil {
			return err
		}
		changed, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return dagDiagnostic(fmt.Errorf("%w: pool resource changed", ErrConflict), "dag.pool.resource_changed", nil)
		}
		if req.ReleaseClass == "cumulative" {
			res, err = t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_execution_pool_policies SET budget_spent=budget_spent+? WHERE pool_id=? AND budget_spent=? AND budget_spent<=?`), req.Amount, r.ExecutionPoolID, c.consumed, maxSchedulingInteger-req.Amount)
			if err != nil {
				return err
			}
			changed, err = res.RowsAffected()
			if err != nil {
				return err
			}
			if changed != 1 {
				return dagDiagnostic(fmt.Errorf("%w: pool budget account differs", ErrConflict), "dag.pool.budget_account", nil)
			}
		}
		if err = t.exec(`INSERT INTO dag_execution_pool_ledger(pool_id,run_id,node_id,attempt,fence,name,tenant_id,project_id,grant_id,grant_version,amount,release_class,state)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, r.ExecutionPoolID, r.ID, n.ID, n.Attempt+1, n.Fence+1, req.Name, r.TenantID, r.ProjectID, r.ExecutionPoolGrantID, r.ExecutionPoolGrantVersion, req.Amount, req.ReleaseClass, state); err != nil {
			return err
		}
	}
	return nil
}

// A stopped process frees its machine slot/process resources. Unknown effects
// retain effect-class holds; lease expiry alone calls neither release path.
func (t *transaction) releasePoolResources(r *runState, n *nodeState, processOnly bool) error {
	if r.ExecutionPoolID == "" {
		return nil
	}
	q := `SELECT name,amount FROM dag_execution_pool_ledger WHERE pool_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND state='active'`
	if processOnly {
		q += ` AND release_class='process'`
	}
	q += ` ORDER BY name`
	rows, err := t.rows(q, r.ExecutionPoolID, r.ID, n.ID, n.Attempt, n.Fence)
	if err != nil {
		return err
	}
	type held struct {
		name   string
		amount int64
	}
	items := []held{}
	for rows.Next() {
		var h held
		if err = rows.Scan(&h.name, &h.amount); err != nil {
			rows.Close()
			return err
		}
		items = append(items, h)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, h := range items {
		res, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_execution_pool_accounts SET used=used-?,version=version+1 WHERE pool_id=? AND name=? AND used>=?`), h.amount, r.ExecutionPoolID, h.name, h.amount)
		if err != nil {
			return err
		}
		count, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return dagDiagnostic(fmt.Errorf("%w: pool release counter mismatch", ErrConflict), "dag.pool.release_counter", nil)
		}
		res, err = t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_execution_pool_ledger SET state='released' WHERE pool_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=? AND name=? AND state='active'`), r.ExecutionPoolID, r.ID, n.ID, n.Attempt, n.Fence, h.name)
		if err != nil {
			return err
		}
		count, err = res.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return ErrConflict
		}
	}
	return t.exec(`DELETE FROM dag_execution_pool_slots WHERE pool_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`, r.ExecutionPoolID, r.ID, n.ID, n.Attempt, n.Fence)
}
