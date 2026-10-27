package dag

import (
	"database/sql"
	"fmt"
)

func (t *transaction) reservePolicyResources(r *runState, n *nodeState) (bool, error) {
	policy, err := t.ensureDefaultPolicy(r.TenantID, r.ProjectID)
	if err != nil {
		return false, err
	}
	type checked struct {
		request                 ResourceRequest
		used, consumed, version int64
		capacity                sql.NullInt64
	}
	checks := []checked{}
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
		if err = t.e.ensureResourceAccountTx(t.ctx, t.tx, r.TenantID, r.ProjectID, request.Name, cap); err != nil {
			return false, err
		}
		c := checked{request: request}
		if err = t.row(`SELECT capacity,used,consumed,version FROM dag_resource_accounts WHERE tenant_id=? AND project_id=? AND name=?`+t.e.currentReadSuffix(), r.TenantID, r.ProjectID, request.Name).Scan(&c.capacity, &c.used, &c.consumed, &c.version); err != nil {
			return false, err
		}
		used := c.used
		if request.ReleaseClass == "cumulative" {
			used = c.consumed
		}
		if !safeSchedulingInteger(request.Amount) || request.Amount > maxSchedulingInteger-used || c.capacity.Valid && request.Amount > c.capacity.Int64-used {
			return false, nil
		}
		checks = append(checks, c)
	}
	for _, c := range checks {
		request := c.request
		column := "used"
		if request.ReleaseClass == "cumulative" {
			column = "consumed"
		}
		result, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_resource_accounts SET `+column+`=`+column+`+?,version=version+1 WHERE tenant_id=? AND project_id=? AND name=? AND version=? AND `+column+`<=? AND (capacity IS NULL OR ?<=capacity-`+column+`)`), request.Amount, r.TenantID, r.ProjectID, request.Name, c.version, maxSchedulingInteger-request.Amount, request.Amount)
		if err != nil {
			return false, err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if changed != 1 {
			return false, dagDiagnostic(fmt.Errorf("%w: concurrent resource account changed", ErrConflict), "dag.resources.concurrent_changed", nil)
		}
		state := "active"
		if request.ReleaseClass == "cumulative" {
			state = "charged"
			budgetResult, budgetErr := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_scheduling_policies SET budget_spent=budget_spent+? WHERE tenant_id=? AND project_id=? AND budget_spent=? AND budget_spent<=?`), request.Amount, r.TenantID, r.ProjectID, c.consumed, maxSchedulingInteger-request.Amount)
			if budgetErr != nil {
				return false, budgetErr
			}
			budgetChanged, budgetErr := budgetResult.RowsAffected()
			if budgetErr != nil {
				return false, budgetErr
			}
			if budgetChanged != 1 {
				return false, dagDiagnostic(fmt.Errorf("%w: policy and budget account differ", ErrConflict), "dag.resources.budget_account", nil)
			}
		}
		if err = t.exec(`INSERT INTO dag_resource_ledger(run_id,node_id,attempt,fence,name,tenant_id,project_id,amount,release_class,state)VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, n.ID, n.Attempt+1, n.Fence+1, request.Name, r.TenantID, r.ProjectID, request.Amount, request.ReleaseClass, state); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (t *transaction) releasePolicyResources(r *runState, n *nodeState, processOnly bool) error {
	query := `SELECT name,amount FROM dag_resource_ledger WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND state='active'`
	if processOnly {
		query += ` AND release_class='process'`
	}
	query += ` ORDER BY name`
	rows, err := t.rows(query, r.ID, n.ID, n.Attempt, n.Fence)
	if err != nil {
		return err
	}
	type held struct {
		name   string
		amount int64
	}
	list := []held{}
	for rows.Next() {
		var v held
		if err = rows.Scan(&v.name, &v.amount); err != nil {
			rows.Close()
			return err
		}
		list = append(list, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, v := range list {
		result, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_resource_accounts SET used=used-?,version=version+1 WHERE tenant_id=? AND project_id=? AND name=? AND used>=?`), v.amount, r.TenantID, r.ProjectID, v.name, v.amount)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return dagDiagnostic(fmt.Errorf("%w: resource release counter mismatch", ErrConflict), "dag.resources.release_counter", nil)
		}
		ledgerResult, err := t.tx.ExecContext(t.ctx, t.e.q(`UPDATE dag_resource_ledger SET state='released' WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND name=? AND state='active'`), r.ID, n.ID, n.Attempt, n.Fence, v.name)
		if err != nil {
			return err
		}
		ledgerChanged, err := ledgerResult.RowsAffected()
		if err != nil {
			return err
		}
		if ledgerChanged != 1 {
			return dagDiagnostic(fmt.Errorf("%w: reservation already released", ErrConflict), "dag.resources.released", nil)
		}
	}
	return t.releasePoolResources(r, n, processOnly)
}

func (t *transaction) mergeResourceLimits(tenant, project string, limits map[string]int64) error {
	if err := t.guardProject(tenant, project); err != nil {
		return err
	}
	policy, err := t.e.readSchedulingPolicyTx(t.ctx, t.tx, tenant, project, true, false)
	if err != nil {
		return err
	}
	input := SchedulingPolicyInput{ConcurrencyLimit: policy.ConcurrencyLimit, ResourceLimits: map[string]*int64{}, BudgetLimit: policy.BudgetLimit}
	for name, cap := range policy.ResourceLimits {
		input.ResourceLimits[name] = cap
	}
	for name, capacity := range limits {
		if !identifier.MatchString(name) || !safeSchedulingInteger(capacity) {
			return dagDiagnostic(fmt.Errorf("%w: resource limit", ErrInvalid), "dag.resources.limit", nil)
		}
		v := capacity
		switch name {
		case "execution_slots":
			input.ConcurrencyLimit = v
		case "budget":
			input.BudgetLimit = &v
		default:
			input.ResourceLimits[name] = &v
		}
	}
	_, err = t.e.WriteSchedulingPolicyTx(t.ctx, t.tx, tenant, project, policy.Version, input)
	return err
}
