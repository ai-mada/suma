package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

func (e *Engine) ReadPoolPolicyTx(ctx context.Context, tx *sql.Tx, pool string) (PoolPolicy, error) {
	return e.readPoolPolicy(ctx, tx, pool, false)
}
func (e *Engine) readPoolPolicy(ctx context.Context, tx *sql.Tx, pool string, current bool) (PoolPolicy, error) {
	out := PoolPolicy{PoolID: pool, PoolPolicyInput: PoolPolicyInput{ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}, Resources: []ResourceSnapshot{}}
	if !identifier.MatchString(pool) {
		return out, ErrInvalid
	}
	var limits, weights string
	var budget sql.NullInt64
	q := `SELECT version,concurrency_limit,resource_limits,budget_limit,budget_spent,tenant_weights,updated_at FROM dag_execution_pool_policies WHERE pool_id=?`
	if current {
		q += e.currentReadSuffix()
	}
	err := tx.QueryRowContext(ctx, e.q(q), pool).Scan(&out.Version, &out.ConcurrencyLimit, &limits, &budget, &out.BudgetSpent, &weights, &out.AsOf)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if json.Unmarshal([]byte(limits), &out.ResourceLimits) != nil || json.Unmarshal([]byte(weights), &out.TenantWeights) != nil {
		return out, dagDiagnostic(fmt.Errorf("%w: pool policy encoding", ErrInvalid), "dag.pool.policy_encoding", nil)
	}
	if budget.Valid {
		v := budget.Int64
		out.BudgetLimit = &v
	}
	accountsSQL := `SELECT name,capacity,used,consumed,release_class FROM dag_execution_pool_accounts WHERE pool_id=? ORDER BY name`
	if current {
		accountsSQL += e.currentReadSuffix()
	}
	rows, err := tx.QueryContext(ctx, e.q(accountsSQL), pool)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var r ResourceSnapshot
		var cap sql.NullInt64
		if err = rows.Scan(&r.Name, &cap, &r.Used, &r.Consumed, &r.ReleaseClass); err != nil {
			return out, err
		}
		r.Unit = resourceUnit(r.Name)
		r.Mode = "unlimited"
		if cap.Valid {
			v := cap.Int64
			r.Capacity = &v
			r.Mode = "limited"
		}
		out.Resources = append(out.Resources, r)
	}
	clock, err := e.clockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	out.AsOf = stamp(clock)
	return out, rows.Err()
}

// WritePoolPolicyTx changes capacities without resetting used/consumed or past
// fairness service. Caller owns current pool-administrator authorization.
func (e *Engine) WritePoolPolicyTx(ctx context.Context, tx *sql.Tx, pool string, expected int, in PoolPolicyInput) (PoolPolicy, error) {
	var out PoolPolicy
	if expected < 0 || !safeSchedulingInteger(in.ConcurrencyLimit) || len(in.ResourceLimits) > 64 || len(in.TenantWeights) > 256 || in.BudgetLimit != nil && !safeSchedulingInteger(*in.BudgetLimit) {
		return out, ErrInvalid
	}
	for name, cap := range in.ResourceLimits {
		if !identifier.MatchString(name) || name == "execution_slots" || name == "budget" || cap != nil && !safeSchedulingInteger(*cap) {
			return out, ErrInvalid
		}
	}
	for tenant, weight := range in.TenantWeights {
		if !identifier.MatchString(tenant) || weight < 1 || weight > 100 {
			return out, ErrInvalid
		}
	}
	if err := e.guardPoolTx(ctx, tx, pool); err != nil {
		return out, err
	}
	old, err := e.readPoolPolicy(ctx, tx, pool, true)
	if err != nil {
		return out, err
	}
	if old.Version != expected {
		return out, dagDiagnostic(fmt.Errorf("%w: pool policy changed", ErrConflict), "dag.pool.policy_changed", nil)
	}
	if in.ResourceLimits == nil {
		in.ResourceLimits = map[string]*int64{}
	}
	if in.TenantWeights == nil {
		in.TenantWeights = map[string]int{}
	}
	clock, err := e.clockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	if old.Version == 0 {
		_, err = tx.ExecContext(ctx, e.q(`INSERT INTO dag_execution_pool_policies(pool_id,version,concurrency_limit,resource_limits,budget_limit,budget_spent,tenant_weights,updated_at)VALUES(?,1,?,?,?,0,?,?)`), pool, in.ConcurrencyLimit, encoded(in.ResourceLimits), in.BudgetLimit, encoded(in.TenantWeights), stamp(clock))
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, e.q(`UPDATE dag_execution_pool_policies SET version=version+1,concurrency_limit=?,resource_limits=?,budget_limit=?,tenant_weights=?,updated_at=? WHERE pool_id=? AND version=?`), in.ConcurrencyLimit, encoded(in.ResourceLimits), in.BudgetLimit, encoded(in.TenantWeights), stamp(clock), pool, expected)
		if err == nil {
			n, x := res.RowsAffected()
			if x != nil {
				return out, x
			}
			if n != 1 {
				return out, ErrConflict
			}
		}
	}
	if err != nil {
		return out, err
	}
	names := map[string]bool{"execution_slots": true, "budget": true}
	for _, r := range old.Resources {
		names[r.Name] = true
	}
	for name := range in.ResourceLimits {
		names[name] = true
	}
	ordered := []string{}
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		var cap *int64
		switch name {
		case "execution_slots":
			v := in.ConcurrencyLimit
			cap = &v
		case "budget":
			cap = in.BudgetLimit
		default:
			cap = in.ResourceLimits[name]
		}
		if err = e.ensurePoolAccount(ctx, tx, pool, name, cap); err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, e.q(`UPDATE dag_execution_pool_accounts SET capacity=?,version=version+1 WHERE pool_id=? AND name=?`), cap, pool, name); err != nil {
			return out, err
		}
	}
	return e.readPoolPolicy(ctx, tx, pool, true)
}
func (e *Engine) ensurePoolAccount(ctx context.Context, tx *sql.Tx, pool, name string, cap *int64) error {
	q := `INSERT INTO dag_execution_pool_accounts(pool_id,name,capacity,used,consumed,version,release_class)VALUES(?,?,?,0,0,1,?) ON CONFLICT(pool_id,name) DO NOTHING`
	if e.dialect == "oceanbase" {
		q = `INSERT IGNORE INTO dag_execution_pool_accounts(pool_id,name,capacity,used,consumed,version,release_class)VALUES(?,?,?,0,0,1,?)`
	}
	_, err := tx.ExecContext(ctx, e.q(q), pool, name, cap, resourceClass(name))
	return err
}

// WritePoolScopeTx mirrors an already-authorized bilateral grant. Scope and
// project weight are immutable within an authorization generation.
func (e *Engine) WritePoolScopeTx(ctx context.Context, tx *sql.Tx, in PoolScope) error {
	if scopeValid(in.TenantID, in.ProjectID) != nil || !identifier.MatchString(in.GrantID) || in.GrantVersion < 1 || in.ProjectWeight < 1 || in.ProjectWeight > 100 || (in.Status != "active" && in.Status != "inactive") {
		return ErrInvalid
	}
	if err := e.guardPoolTx(ctx, tx, in.PoolID); err != nil {
		return err
	}
	var old PoolScope
	err := tx.QueryRowContext(ctx, e.q(`SELECT grant_id,grant_version,project_weight,status FROM dag_execution_pool_scopes WHERE pool_id=? AND tenant_id=? AND project_id=?`+e.currentReadSuffix()), in.PoolID, in.TenantID, in.ProjectID).Scan(&old.GrantID, &old.GrantVersion, &old.ProjectWeight, &old.Status)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if old.GrantID == in.GrantID {
			if in.ProjectWeight != old.ProjectWeight || in.GrantVersion < old.GrantVersion || in.GrantVersion == old.GrantVersion && in.Status != old.Status {
				return ErrConflict
			}
		} else if old.Status == "active" {
			return ErrConflict
		}
		_, err = tx.ExecContext(ctx, e.q(`UPDATE dag_execution_pool_scopes SET grant_id=?,grant_version=?,project_weight=?,status=? WHERE pool_id=? AND tenant_id=? AND project_id=?`), in.GrantID, in.GrantVersion, in.ProjectWeight, in.Status, in.PoolID, in.TenantID, in.ProjectID)
		return err
	}
	_, err = tx.ExecContext(ctx, e.q(`INSERT INTO dag_execution_pool_scopes(pool_id,tenant_id,project_id,grant_id,grant_version,project_weight,status)VALUES(?,?,?,?,?,?,?)`), in.PoolID, in.TenantID, in.ProjectID, in.GrantID, in.GrantVersion, in.ProjectWeight, in.Status)
	return err
}
