package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
)

func resourceClass(name string) string {
	switch name {
	case "budget":
		return "cumulative"
	case "execution_slots", "cpu_millis", "memory_bytes", "gpu_slots":
		return "process"
	default:
		return "effect"
	}
}
func resourceUnit(name string) string {
	switch name {
	case "execution_slots", "gpu_slots":
		return "slots"
	case "cpu_millis":
		return "millicores"
	case "memory_bytes":
		return "bytes"
	case "model_concurrency":
		return "requests"
	case "budget":
		return "admission_units"
	default:
		return "units"
	}
}
func safeSchedulingInteger(v int64) bool { return v >= 0 && v <= maxSchedulingInteger }

func (e *Engine) ReadSchedulingPolicyTx(ctx context.Context, tx *sql.Tx, tenant, project string) (SchedulingPolicy, error) {
	return e.readSchedulingPolicyTx(ctx, tx, tenant, project, false, true)
}
func (e *Engine) currentReadSuffix() string {
	if e.dialect == "oceanbase" || e.dialect == "postgres" {
		return " FOR UPDATE"
	}
	return ""
}
func (e *Engine) readSchedulingPolicyTx(ctx context.Context, tx *sql.Tx, tenant, project string, current, includeResources bool) (SchedulingPolicy, error) {
	out := SchedulingPolicy{ProjectID: project, Default: true, ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{}, Unit: "admission_units", BudgetHistory: "complete", Resources: []ResourceSnapshot{}}
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	clock, err := e.clockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	out.AsOf = stamp(clock)
	var limits string
	var budget sql.NullInt64
	query := `SELECT version,concurrency_limit,resource_limits,budget_limit,budget_spent,budget_history FROM dag_scheduling_policies WHERE tenant_id=? AND project_id=?`
	if current {
		query += e.currentReadSuffix()
	}
	err = tx.QueryRowContext(ctx, e.q(query), tenant, project).Scan(&out.Version, &out.ConcurrencyLimit, &limits, &budget, &out.BudgetSpent, &out.BudgetHistory)
	if err != nil && err != sql.ErrNoRows {
		return out, err
	}
	if err == nil {
		out.Default = false
		if err = json.Unmarshal([]byte(limits), &out.ResourceLimits); err != nil {
			return out, err
		}
		if out.ResourceLimits == nil {
			out.ResourceLimits = map[string]*int64{}
		}
		if budget.Valid {
			v := budget.Int64
			out.BudgetLimit = &v
		}
	}
	if out.Default {
		var exists int
		if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*) FROM dag_attempts a JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=?`), tenant, project).Scan(&exists); err != nil {
			return out, err
		}
		if exists > 0 {
			out.BudgetHistory = "partial"
		}
	}
	if !includeResources {
		return out, nil
	}
	accountsQuery := `SELECT name,capacity,used,consumed,release_class FROM dag_resource_accounts WHERE tenant_id=? AND project_id=? ORDER BY name`
	if current {
		accountsQuery += e.currentReadSuffix()
	}
	accounts, err := tx.QueryContext(ctx, e.q(accountsQuery), tenant, project)
	if err != nil {
		return out, err
	}
	seen := map[string]bool{}
	for accounts.Next() {
		var r ResourceSnapshot
		var cap sql.NullInt64
		if err = accounts.Scan(&r.Name, &cap, &r.Used, &r.Consumed, &r.ReleaseClass); err != nil {
			accounts.Close()
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
		seen[r.Name] = true
	}
	err = accounts.Err()
	accounts.Close()
	if err != nil {
		return out, err
	}
	if !seen["execution_slots"] {
		v := out.ConcurrencyLimit
		out.Resources = append(out.Resources, ResourceSnapshot{Name: "execution_slots", Unit: "slots", Capacity: &v, Mode: "limited", ReleaseClass: "process"})
	}
	if !seen["budget"] {
		mode := "unlimited"
		if out.BudgetLimit != nil {
			mode = "limited"
		}
		out.Resources = append(out.Resources, ResourceSnapshot{Name: "budget", Unit: "admission_units", Capacity: out.BudgetLimit, Consumed: out.BudgetSpent, Mode: mode, ReleaseClass: "cumulative"})
	}
	sort.Slice(out.Resources, func(i, j int) bool { return out.Resources[i].Name < out.Resources[j].Name })
	return out, nil
}

func (e *Engine) WriteSchedulingPolicyTx(ctx context.Context, tx *sql.Tx, tenant, project string, expectedVersion int, input SchedulingPolicyInput) (SchedulingPolicy, error) {
	var out SchedulingPolicy
	if expectedVersion < 0 || !safeSchedulingInteger(input.ConcurrencyLimit) || len(input.ResourceLimits) > 64 || input.BudgetLimit != nil && !safeSchedulingInteger(*input.BudgetLimit) {
		return out, dagDiagnostic(fmt.Errorf("%w: scheduling capacities require safe nonnegative integers", ErrInvalid), "dag.policy.capacities", nil)
	}
	for name, cap := range input.ResourceLimits {
		if !identifier.MatchString(name) || name == "execution_slots" || name == "budget" || cap != nil && !safeSchedulingInteger(*cap) {
			return out, dagDiagnostic(fmt.Errorf("%w: resource name/capacity", ErrInvalid), "dag.policy.resource", nil)
		}
	}
	if err := e.projectGuardTx(ctx, tx, tenant, project); err != nil {
		return out, err
	}
	current, err := e.readSchedulingPolicyTx(ctx, tx, tenant, project, true, false)
	if err != nil {
		return out, err
	}
	if current.Version != expectedVersion {
		return out, dagDiagnostic(fmt.Errorf("%w: scheduling policy changed", ErrConflict), "dag.policy.changed", nil)
	}
	limits := input.ResourceLimits
	if limits == nil {
		limits = map[string]*int64{}
	}
	if current.Default {
		_, err = tx.ExecContext(ctx, e.q(`INSERT INTO dag_scheduling_policies(tenant_id,project_id,version,concurrency_limit,resource_limits,budget_limit,budget_spent,budget_history,updated_at)VALUES(?,?,1,?,?,?,?,?,?)`), tenant, project, input.ConcurrencyLimit, encoded(limits), input.BudgetLimit, current.BudgetSpent, current.BudgetHistory, current.AsOf)
	} else {
		var result sql.Result
		result, err = tx.ExecContext(ctx, e.q(`UPDATE dag_scheduling_policies SET version=version+1,concurrency_limit=?,resource_limits=?,budget_limit=?,updated_at=? WHERE tenant_id=? AND project_id=? AND version=?`), input.ConcurrencyLimit, encoded(limits), input.BudgetLimit, current.AsOf, tenant, project, expectedVersion)
		if err == nil {
			var n int64
			n, err = result.RowsAffected()
			if err == nil && n != 1 {
				err = dagDiagnostic(fmt.Errorf("%w: scheduling policy changed", ErrConflict), "dag.policy.changed", nil)
			}
		}
	}
	if err != nil {
		return out, err
	}
	names := map[string]bool{"execution_slots": true, "budget": true}
	for name := range current.ResourceLimits {
		names[name] = true
	}
	for name := range limits {
		names[name] = true
	}
	for _, r := range current.Resources {
		names[r.Name] = true
	}
	ordered := []string{}
	for name := range names {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	for _, name := range ordered {
		var capacity *int64
		switch name {
		case "execution_slots":
			v := input.ConcurrencyLimit
			capacity = &v
		case "budget":
			capacity = input.BudgetLimit
		default:
			capacity = limits[name]
		}
		if err = e.ensureResourceAccountTx(ctx, tx, tenant, project, name, capacity); err != nil {
			return out, err
		}
		if _, err = tx.ExecContext(ctx, e.q(`UPDATE dag_resource_accounts SET capacity=?,version=version+1 WHERE tenant_id=? AND project_id=? AND name=?`), capacity, tenant, project, name); err != nil {
			return out, err
		}
	}
	return e.readSchedulingPolicyTx(ctx, tx, tenant, project, true, true)
}

func (e *Engine) ensureResourceAccountTx(ctx context.Context, tx *sql.Tx, tenant, project, name string, capacity *int64) error {
	q := `INSERT INTO dag_resource_accounts(tenant_id,project_id,name,capacity,used,consumed,version,release_class)VALUES(?,?,?,?,0,0,1,?) ON CONFLICT(tenant_id,project_id,name) DO NOTHING`
	if e.dialect == "oceanbase" {
		q = `INSERT IGNORE INTO dag_resource_accounts(tenant_id,project_id,name,capacity,used,consumed,version,release_class)VALUES(?,?,?,?,0,0,1,?)`
	}
	_, err := tx.ExecContext(ctx, e.q(q), tenant, project, name, capacity, resourceClass(name))
	return err
}

func (t *transaction) ensureDefaultPolicy(tenant, project string) (SchedulingPolicy, error) {
	current, err := t.e.readSchedulingPolicyTx(t.ctx, t.tx, tenant, project, true, false)
	if err != nil || !current.Default {
		return current, err
	}
	return t.e.WriteSchedulingPolicyTx(t.ctx, t.tx, tenant, project, 0, SchedulingPolicyInput{ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{}})
}
