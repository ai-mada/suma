package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

// The plan's route basis is private persistence, not a client-selected grant.
// It is the authority basis for the reviewed route generation. The public plan
// snapshot makes that basis reviewable but is never trusted as authorization.
func (s *Service) initializeTaskDispatchRoutes(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, s.ddl(`CREATE TABLE IF NOT EXISTS suma_task_dispatch_route_bases(plan_id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,route_version INTEGER NOT NULL,pool_id VARCHAR(100) NOT NULL,grant_id VARCHAR(100) NOT NULL,grant_version INTEGER NOT NULL)`))
	return err
}

func (s *Service) saveTaskDispatchRouteBasisTx(ctx context.Context, tx *sql.Tx, planID string, route dag.ProjectExecutionRoute) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_task_dispatch_route_bases(plan_id,tenant_id,project_id,route_version,pool_id,grant_id,grant_version)VALUES(?,?,?,?,?,?,?)`), planID, route.TenantID, route.ProjectID, route.Version, route.PoolID, route.GrantID, route.GrantVersion)
	return err
}

func (s *Service) checkTaskDispatchRouteBasisTx(ctx context.Context, tx *sql.Tx, tenant, project, planID string, current dag.ProjectExecutionRoute) error {
	basis := dag.ProjectExecutionRoute{TenantID: tenant, ProjectID: project}
	err := tx.QueryRowContext(ctx, s.q(`SELECT route_version,pool_id,grant_id,grant_version FROM suma_task_dispatch_route_bases WHERE plan_id=? AND tenant_id=? AND project_id=?`), planID, tenant, project).Scan(&basis.Version, &basis.PoolID, &basis.GrantID, &basis.GrantVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return conflict("派发计划缺少固定执行路由依据，请重新预览")
	}
	if err != nil {
		return err
	}
	if basis != current {
		return conflict("项目执行路由依据已变化，请重新派发预览")
	}
	return nil
}

// Caller holds current project organization authority. This reads platform
// registration/grants only; scheduler routing comes from its public contract.
// A healthy, approved tool in another pool is still unreachable by this route.
func (s *Service) checkDispatchProfileRouteTx(ctx context.Context, tx *sql.Tx, tenant, project string, profile ToolProfile, route dag.ProjectExecutionRoute) error {
	var workerPool string
	err := tx.QueryRowContext(ctx, s.q(`SELECT pool_id FROM suma_execution_pool_runners WHERE worker_id=?`), profile.WorkerID).Scan(&workerPool)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if route.PoolID == "" {
		if workerPool != "" {
			return conflict("当前项目采用本地执行路由，请选择项目执行节点的工具或重新配置路由")
		}
		return nil
	}
	if workerPool == "" || workerPool != route.PoolID {
		return conflict("所选工具的执行节点不属于当前目标资源池，请重新选择工具或执行路由")
	}
	grant, err := s.poolGrantTx(ctx, tx, route.GrantID)
	if err != nil {
		return err
	}
	if grant.PoolID != route.PoolID || grant.TenantID != tenant || grant.ProjectID != project || grant.Version != route.GrantVersion || grant.Status != "approved" {
		return conflict("执行路由的固定资源池授权已失效，请重新审阅路由和派发计划")
	}
	return nil
}

func (s *Service) checkTaskDispatchCurrentRouteTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, p Principal, project string, plan TaskDispatchPlan, profile ToolProfile) error {
	if engine == nil {
		return invalid("任务调度器未配置")
	}
	route, err := engine.ReadProjectExecutionRouteTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return err
	}
	if err = s.checkTaskDispatchRouteBasisTx(ctx, tx, p.TenantID, project, plan.ID, route); err != nil {
		return err
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, p.TenantID, project, profile, route); err != nil {
		return err
	}
	return s.checkTaskDispatchTestBindingsRouteTx(ctx, tx, p, project, plan, profile, route)
}

// This hook runs after Core has inserted the immutable run route and before
// business binding, activation or events. A route race rolls everything back,
// including the managed workflow registry, instead of leaving stranded work.
func (s *Service) checkTaskDispatchBoundRouteTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan TaskDispatchPlan, runID string) error {
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, p.TenantID, project, runID)
	if err != nil {
		return err
	}
	if err = s.checkTaskDispatchRouteBasisTx(ctx, tx, p.TenantID, project, plan.ID, route); err != nil {
		return err
	}
	profile, err := s.toolProfileTx(ctx, tx, p, project, plan.ToolProfileID)
	if err != nil {
		return err
	}
	if err = s.checkDispatchProfileRouteTx(ctx, tx, p.TenantID, project, profile, route); err != nil {
		return err
	}
	return s.checkTaskDispatchTestBindingsRouteTx(ctx, tx, p, project, plan, profile, route)
}

func unsupportedPoolRelease() error {
	return &APIError{Status: 422, Code: "pool_release_unsupported", Message: "共享资源池尚未提供独立生产部署授权协议；请使用项目发布执行节点，现有本地发布仍可核对和排空"}
}

// New production plans cannot inherit a shared route without the independent
// provider/environment protocol. This check is read-only: it must not acquire
// a pool lock after a caller's project guard or fabricate a deployment attempt.
func (s *Service) checkNewReleaseRouteTx(ctx context.Context, tx *sql.Tx, tenant, project string) error {
	route, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	if route.PoolID != "" {
		return unsupportedPoolRelease()
	}
	return nil
}

// before may enter Core only after the same pool -> project order as claims.
// An already-created local run keeps its original route even when the project
// later changes its default; recovering its receipt must not create new work.
func (s *Service) guardReleaseExecutionRouteTx(ctx context.Context, tx *sql.Tx, tenant, project, reservedRun string) error {
	if err := dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, tenant, project, reservedRun); err != nil {
		return err
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, reservedRun)
	if errors.Is(err, dag.ErrNotFound) {
		return s.checkNewReleaseRouteTx(ctx, tx, tenant, project)
	}
	if err != nil {
		return err
	}
	if route.PoolID != "" {
		return unsupportedPoolRelease()
	}
	return nil
}
