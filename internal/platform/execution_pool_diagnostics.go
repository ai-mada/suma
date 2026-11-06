package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

// Core reports resource/scheduler facts. This projection adds the current
// owning-domain authority in the *same read transaction*, never another
// tenant's project names, attempts or tool inventories. Active/uncertain work
// remains a drain/reconciliation fact, not a new-admission suggestion.
func (s *Service) decoratePoolAdmissionTx(ctx context.Context, tx *sql.Tx, tenant, project string, out *dag.NodeAdmissionSnapshot, byRun map[string][]dag.SchedulingBlocker) error {
	if out.ExecutionPoolID == "" || out.NodeState != "ready" && out.NodeState != "pending" {
		return nil
	}
	blockers, cached := byRun[out.RunID]
	if !cached {
		var err error
		blockers, err = s.poolAdmissionBlockersTx(ctx, tx, tenant, project, out.ExecutionPoolID, out.RunID)
		if err != nil {
			return err
		}
		// An immutable run route is shared by its nodes. Cache only within the
		// current authorized page's transaction, then attach each node identity.
		if byRun != nil {
			byRun[out.RunID] = blockers
		}
	}
	for _, blocker := range blockers {
		blocker.RunID, blocker.NodeID = out.RunID, out.NodeID
		out.Blockers = append(out.Blockers, blocker)
		out.Status = "blocked"
	}
	return nil
}

func (s *Service) poolAdmissionBlockersTx(ctx context.Context, tx *sql.Tx, tenant, project, poolID, runID string) ([]dag.SchedulingBlocker, error) {
	var blockers []dag.SchedulingBlocker
	add := func(code, message string) {
		blockers = append(blockers, dag.SchedulingBlocker{Code: code, Message: message, Source: "pool"})
	}
	var status string
	err := tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_execution_pools WHERE id=?`), poolID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		add("pool_metadata_missing", "资源池正式配置缺失，停止新增派发")
		return blockers, nil
	}
	if err != nil {
		return nil, err
	}
	if status != "active" {
		add("pool_paused", "共享执行池已暂停，已派发工作继续排空")
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, runID)
	if err != nil {
		return nil, err
	}
	var currentVersion int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,status FROM suma_execution_pool_grants WHERE id=? AND pool_id=? AND tenant_id=? AND project_id=?`), route.GrantID, route.PoolID, tenant, project).Scan(&currentVersion, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (status != "approved" || currentVersion != route.GrantVersion) {
		add("pool_grant_not_current", "项目资源池授权已变化，当前运行不能新增派发")
		return blockers, nil
	}
	return blockers, err
}
