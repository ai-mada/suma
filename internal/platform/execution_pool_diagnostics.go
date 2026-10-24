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
func (s *Service) decoratePoolAdmissionTx(ctx context.Context, tx *sql.Tx, tenant, project string, out *dag.NodeAdmissionSnapshot) error {
	if out.ExecutionPoolID == "" || out.NodeState != "ready" && out.NodeState != "pending" {
		return nil
	}
	add := func(code, message string) {
		out.Status = "blocked"
		out.Blockers = append(out.Blockers, dag.SchedulingBlocker{Code: code, Message: message, Source: "pool", RunID: out.RunID, NodeID: out.NodeID})
	}
	var status string
	err := tx.QueryRowContext(ctx, s.q(`SELECT status FROM suma_execution_pools WHERE id=?`), out.ExecutionPoolID).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		add("pool_metadata_missing", "资源池正式配置缺失，停止新增派发")
		return nil
	}
	if err != nil {
		return err
	}
	if status != "active" {
		add("pool_paused", "共享执行池已暂停，已派发工作继续排空")
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, out.RunID)
	if err != nil {
		return err
	}
	var currentVersion int
	err = tx.QueryRowContext(ctx, s.q(`SELECT version,status FROM suma_execution_pool_grants WHERE id=? AND pool_id=? AND tenant_id=? AND project_id=?`), route.GrantID, route.PoolID, tenant, project).Scan(&currentVersion, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (status != "approved" || currentVersion != route.GrantVersion) {
		add("pool_grant_not_current", "项目资源池授权已变化，当前运行不能新增派发")
		return nil
	}
	return err
}
