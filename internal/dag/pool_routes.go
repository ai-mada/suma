package dag

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

func (e *Engine) ReadProjectExecutionRouteTx(ctx context.Context, tx *sql.Tx, tenant, project string) (ProjectExecutionRoute, error) {
	return e.projectRoute(ctx, tx, tenant, project, false)
}

// ReadProjectExecutionRouteTx is the transaction-scoped display contract for
// services that do not own an Engine. It neither initializes the engine nor
// locks or writes; an unconfigured project uses the private default.
func ReadProjectExecutionRouteTx(ctx context.Context, tx *sql.Tx, dialect, tenant, project string) (ProjectExecutionRoute, error) {
	e := &Engine{dialect: dialect}
	return e.projectRoute(ctx, tx, tenant, project, false)
}
func (e *Engine) projectRoute(ctx context.Context, q queryer, tenant, project string, current bool) (ProjectExecutionRoute, error) {
	out := ProjectExecutionRoute{TenantID: tenant, ProjectID: project}
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	query := `SELECT version,pool_id,grant_id,grant_version FROM dag_project_execution_routes WHERE tenant_id=? AND project_id=?`
	if current {
		query += e.currentReadSuffix()
	}
	err := q.QueryRowContext(ctx, e.q(query), tenant, project).Scan(&out.Version, &out.PoolID, &out.GrantID, &out.GrantVersion)
	if err == sql.ErrNoRows {
		return out, nil
	}
	return out, err
}
func (e *Engine) runRoute(ctx context.Context, q queryer, tenant, project, id string, current bool) (ProjectExecutionRoute, bool, error) {
	out := ProjectExecutionRoute{TenantID: tenant, ProjectID: project}
	query := `SELECT route_version,pool_id,grant_id,grant_version FROM dag_run_execution_routes WHERE run_id=? AND tenant_id=? AND project_id=?`
	if current {
		query += e.currentReadSuffix()
	}
	err := q.QueryRowContext(ctx, e.q(query), id, tenant, project).Scan(&out.Version, &out.PoolID, &out.GrantID, &out.GrantVersion)
	if err == nil {
		return out, true, nil
	}
	if err != sql.ErrNoRows {
		return out, false, err
	}
	return out, false, nil
}

// ReadRunExecutionRouteTx exposes immutable routing for identity gates without
// requiring callers to know private tables or infer routing from request fields.
// It takes no authority or mutation locks and requires the frozen run route.
func ReadRunExecutionRouteTx(ctx context.Context, tx *sql.Tx, dialect, tenant, project, runID string) (ProjectExecutionRoute, error) {
	e := &Engine{dialect: dialect}
	if scopeValid(tenant, project) != nil || runID == "" {
		return ProjectExecutionRoute{}, ErrInvalid
	}
	route, found, err := e.runRoute(ctx, tx, tenant, project, runID, false)
	if err != nil {
		return route, err
	}
	if !found {
		return route, ErrNotFound
	}
	return route, nil
}

// GuardExecutionScopeTx first locates immutable routing without taking locks,
// then locks pool -> project and rechecks the exact routing with a current read.
// A missing reserved run ID selects the default route for managed creation.
func GuardExecutionScopeTx(ctx context.Context, tx *sql.Tx, dialect, tenant, project, runID string) error {
	e := &Engine{dialect: dialect}
	if dialect != "sqlite" && dialect != "oceanbase" && dialect != "postgres" {
		return ErrUnsupported
	}
	locate := func(current bool) (ProjectExecutionRoute, error) {
		if runID != "" {
			r, found, err := e.runRoute(ctx, tx, tenant, project, runID, current)
			if err != nil || found {
				return r, err
			}
		}
		return e.projectRoute(ctx, tx, tenant, project, current)
	}
	before, err := locate(false)
	if err != nil {
		return err
	}
	if before.PoolID != "" {
		if err = e.guardPoolTx(ctx, tx, before.PoolID); err != nil {
			return err
		}
	}
	if err = e.projectGuardTx(ctx, tx, tenant, project); err != nil {
		return err
	}
	after, err := locate(true)
	if err != nil {
		return err
	}
	if before != after {
		return dagDiagnostic(fmt.Errorf("%w: execution route changed before project lock; retry with the same operation", ErrConflict), "dag.pool.route_changed", nil)
	}
	return nil
}

func (e *Engine) WriteProjectExecutionRouteTx(ctx context.Context, tx *sql.Tx, tenant, project string, expected int, in ProjectExecutionRouteInput) (ProjectExecutionRoute, error) {
	var out ProjectExecutionRoute
	if expected < 0 {
		return out, ErrInvalid
	}
	old, err := e.projectRoute(ctx, tx, tenant, project, false)
	if err != nil {
		return out, err
	}
	if in.PoolID == "" {
		if in.GrantID != "" || in.GrantVersion != 0 {
			return out, ErrInvalid
		}
	} else if !identifier.MatchString(in.GrantID) || in.GrantVersion < 1 {
		return out, ErrInvalid
	}
	pools := []string{}
	if old.PoolID != "" {
		pools = append(pools, old.PoolID)
	}
	if in.PoolID != "" && in.PoolID != old.PoolID {
		pools = append(pools, in.PoolID)
	}
	sort.Strings(pools)
	for _, pool := range pools {
		if err = e.guardPoolTx(ctx, tx, pool); err != nil {
			return out, err
		}
	}
	if err = e.projectGuardTx(ctx, tx, tenant, project); err != nil {
		return out, err
	}
	current, err := e.projectRoute(ctx, tx, tenant, project, true)
	if err != nil {
		return out, err
	}
	if current != old || current.Version != expected {
		return out, ErrConflict
	}
	if in.PoolID != "" {
		var grant string
		var version int
		var status string
		err = tx.QueryRowContext(ctx, e.q(`SELECT grant_id,grant_version,status FROM dag_execution_pool_scopes WHERE pool_id=? AND tenant_id=? AND project_id=?`+e.currentReadSuffix()), in.PoolID, tenant, project).Scan(&grant, &version, &status)
		if err != nil {
			return out, err
		}
		if grant != in.GrantID || version != in.GrantVersion || status != "active" {
			return out, dagDiagnostic(fmt.Errorf("%w: current bilateral pool grant required", ErrConflict), "dag.pool.bilateral_grant", nil)
		}
	}
	if expected == 0 {
		_, err = tx.ExecContext(ctx, e.q(`INSERT INTO dag_project_execution_routes(tenant_id,project_id,version,pool_id,grant_id,grant_version)VALUES(?,?,1,?,?,?)`), tenant, project, in.PoolID, in.GrantID, in.GrantVersion)
	} else {
		var res sql.Result
		res, err = tx.ExecContext(ctx, e.q(`UPDATE dag_project_execution_routes SET version=version+1,pool_id=?,grant_id=?,grant_version=? WHERE tenant_id=? AND project_id=? AND version=?`), in.PoolID, in.GrantID, in.GrantVersion, tenant, project, expected)
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
	return e.projectRoute(ctx, tx, tenant, project, true)
}

func (t *transaction) guardExecution(tenant, project, runID string) error {
	if err := GuardExecutionScopeTx(t.ctx, t.tx, t.e.dialect, tenant, project, runID); err != nil {
		return err
	}
	if t.projects == nil {
		t.projects = map[string]bool{}
	}
	t.projects[tenant+"\x00"+project] = true
	return nil
}
func (t *transaction) newRunRoute(tenant, project string) (ProjectExecutionRoute, error) {
	if t.familyRoot != "" {
		route, found, err := t.e.runRoute(t.ctx, t.tx, tenant, project, t.familyRoot, false)
		if err != nil {
			return route, err
		}
		if !found {
			return route, ErrNotFound
		}
		if err = t.guardExecution(tenant, project, t.familyRoot); err != nil {
			return route, err
		}
		return route, nil
	}
	if err := t.guardExecution(tenant, project, ""); err != nil {
		return ProjectExecutionRoute{}, err
	}
	return t.e.projectRoute(t.ctx, t.tx, tenant, project, true)
}
func applyExecutionRoute(r *runState, route ProjectExecutionRoute) {
	r.ExecutionPoolID = route.PoolID
	r.ExecutionRouteVersion = route.Version
	r.ExecutionPoolGrantID = route.GrantID
	r.ExecutionPoolGrantVersion = route.GrantVersion
}
func (e *Engine) loadExecutionRoute(ctx context.Context, q queryer, r *runState) error {
	route, found, err := e.runRoute(ctx, q, r.TenantID, r.ProjectID, r.ID, false)
	if err != nil {
		return err
	}
	if !found {
		return ErrNotFound
	}
	applyExecutionRoute(r, route)
	return nil
}
