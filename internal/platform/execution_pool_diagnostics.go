package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

type poolAdmissionReadCache struct {
	byRun        map[string][]dag.SchedulingBlocker
	runnerByPool map[string]dag.SchedulingBlocker
}

// Core reports resource/scheduler facts. This projection adds the current
// owning-domain authority in the *same read transaction*, never another
// tenant's project names, attempts or tool inventories. Active/uncertain work
// remains a drain/reconciliation fact, not a new-admission suggestion.
func (s *Service) decoratePoolAdmissionTx(ctx context.Context, tx *sql.Tx, tenant, project string, out *dag.NodeAdmissionSnapshot, cache poolAdmissionReadCache) error {
	if out.ExecutionPoolID == "" || out.NodeState != "ready" && out.NodeState != "pending" {
		return nil
	}
	blockers, cached := cache.byRun[out.RunID]
	if !cached {
		var err error
		blockers, err = s.poolAdmissionBlockersTx(ctx, tx, tenant, project, out.ExecutionPoolID, out.RunID)
		if err != nil {
			return err
		}
		// An immutable run route is shared by its nodes. Cache only within the
		// current authorized page's transaction, then attach each node identity.
		if cache.byRun != nil {
			cache.byRun[out.RunID] = blockers
		}
	}
	// Internal nodes do not claim a worker even if the run has a frozen pool.
	if len(blockers) == 0 && len(out.Requests) > 0 && hasPoolCredentialScope(out) {
		gate, checked := cache.runnerByPool[out.ExecutionPoolID]
		if !checked {
			var err error
			gate, err = s.poolRunnerCredentialGateTx(ctx, tx, out.ExecutionPoolID)
			if err != nil {
				return err
			}
			if cache.runnerByPool != nil {
				cache.runnerByPool[out.ExecutionPoolID] = gate
			}
		}
		if gate.Code != "" {
			blockers = []dag.SchedulingBlocker{gate}
		}
	}
	for _, blocker := range blockers {
		blocker.RunID, blocker.NodeID = out.RunID, out.NodeID
		out.Blockers = append(out.Blockers, blocker)
		out.Status = "blocked"
	}
	return nil
}

func hasPoolCredentialScope(snapshot *dag.NodeAdmissionSnapshot) bool {
	if snapshot.PoolPolicyVersion == 0 {
		return false
	}
	for _, blocker := range snapshot.Blockers {
		if blocker.Code == "pool_scope_inactive" || blocker.Code == "pool_scope_version_mismatch" {
			return false
		}
	}
	return true
}

func (s *Service) poolAdmissionBlockersTx(ctx context.Context, tx *sql.Tx, tenant, project, poolID, runID string) ([]dag.SchedulingBlocker, error) {
	var blockers []dag.SchedulingBlocker
	add := func(code, message string) {
		blockers = append(blockers, dag.SchedulingBlocker{Code: code, Message: message, Source: "pool"})
	}
	var storedPool, owner, status string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id,owner_tenant_id,status FROM suma_execution_pools WHERE id=?`), poolID).Scan(&storedPool, &owner, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && storedPool != poolID {
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
	var grantID, grantPool, grantOwner, grantTenant, grantProject string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,pool_id,owner_tenant_id,tenant_id,project_id,version,status FROM suma_execution_pool_grants WHERE id=? AND pool_id=? AND tenant_id=? AND project_id=?`), route.GrantID, poolID, tenant, project).
		Scan(&grantID, &grantPool, &grantOwner, &grantTenant, &grantProject, &currentVersion, &status)
	if errors.Is(err, sql.ErrNoRows) || err == nil && (route.PoolID != poolID || grantID != route.GrantID || grantPool != poolID ||
		grantOwner != owner || grantTenant != tenant || grantProject != project || status != "approved" || currentVersion != route.GrantVersion) {
		add("pool_grant_not_current", "项目资源池授权已变化，当前运行不能新增派发")
		return blockers, nil
	}
	return blockers, err
}

// Read only pool-wide registration and credential metadata. A usable
// registration does not prove that a process is online or can match this node.
func (s *Service) poolRunnerCredentialGateTx(ctx context.Context, tx *sql.Tx, poolID string) (dag.SchedulingBlocker, error) {
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return dag.SchedulingBlocker{}, err
	}
	const maxRegistrations = 1000
	rows, err := tx.QueryContext(ctx, s.q(`SELECT r.id,r.pool_id,r.worker_id,r.status,r.max_slots,r.credential_version,c.runner_id,c.status,c.version,c.expires_at
		FROM suma_execution_pool_runners r LEFT JOIN suma_execution_pool_credentials c ON c.runner_id=r.id
		WHERE r.pool_id=? AND r.status='active' ORDER BY r.id LIMIT ?`), poolID, maxRegistrations+1)
	if err != nil {
		return dag.SchedulingBlocker{}, err
	}
	defer rows.Close()
	registered := false
	for scanned := 0; rows.Next(); scanned++ {
		if scanned == maxRegistrations {
			// An incomplete scan cannot prove that no registration can claim.
			return dag.SchedulingBlocker{}, nil
		}
		var runnerID, storedPool, workerID, status string
		var maxSlots, generation int
		var credentialRunner, credentialStatus, expiry sql.NullString
		var credentialVersion sql.NullInt64
		if err = rows.Scan(&runnerID, &storedPool, &workerID, &status, &maxSlots, &generation, &credentialRunner, &credentialStatus, &credentialVersion, &expiry); err != nil {
			return dag.SchedulingBlocker{}, err
		}
		if storedPool != poolID || status != "active" {
			continue
		}
		registered = true
		if !executionPoolID.MatchString(runnerID) || workerID != "pool-"+runnerID || maxSlots < 1 || maxSlots > 128 || generation < 1 ||
			!credentialRunner.Valid || credentialRunner.String != runnerID || !credentialStatus.Valid || credentialStatus.String != "active" ||
			!credentialVersion.Valid || credentialVersion.Int64 != int64(generation) || !expiry.Valid {
			continue
		}
		expires, parseErr := time.Parse(time.RFC3339Nano, expiry.String)
		if parseErr == nil && expires.Unix() > clock.Unix() {
			return dag.SchedulingBlocker{}, nil
		}
	}
	if err = rows.Err(); err != nil {
		return dag.SchedulingBlocker{}, err
	}
	if !registered {
		return dag.SchedulingBlocker{Code: "pool_runner_registration_missing", Message: "共享执行池没有处于启用状态的登记节点，请联系资源池管理员", Source: "pool"}, nil
	}
	return dag.SchedulingBlocker{Code: "pool_runner_credential_unavailable", Message: "共享执行池已登记的节点没有当前可用的机器凭证，请联系资源池管理员", Source: "pool"}, nil
}
