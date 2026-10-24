package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// Pool registry fallback is independent of the registering person's account.
// Project consent, active machine credential and the human profile approver
// remain separate facts; a pool owner is never made a project member.
func (s *Service) poolToolWorkerReasonTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, clock time.Time) (string, error) {
	var expires, poolStatus string
	err := tx.QueryRowContext(ctx, s.q(`SELECT c.expires_at,p.status FROM suma_execution_pool_runners r JOIN suma_execution_pool_credentials c ON c.runner_id=r.id AND c.version=r.credential_version JOIN suma_execution_pools p ON p.id=r.pool_id JOIN suma_execution_pool_grants g ON g.pool_id=p.id WHERE r.worker_id=? AND r.status='active' AND c.status='active' AND g.tenant_id=? AND g.project_id=? AND g.status='approved'`), worker, tenant, project).Scan(&expires, &poolStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return "执行节点未登记、未获项目授权或机器凭证已撤销", nil
	}
	if err != nil {
		return "", err
	}
	if poolStatus == "paused" {
		return "资源池已暂停，停止新派发", nil
	}
	if poolStatus != "active" {
		return "资源池当前状态不允许新派发", nil
	}
	until, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || until.Unix() <= clock.Unix() {
		return "资源池执行节点凭证已过期", nil
	}
	return "", nil
}

func (s *Service) ReportPoolToolInventory(ctx context.Context, token string, req PoolToolInventoryRequest) (ToolInventoryReceipt, error) {
	var out ToolInventoryReceipt
	if len(req.OperationID) < 8 || len(req.OperationID) > 128 || len(req.Items) > 128 {
		return out, invalid("工具清单操作标识或大小无效")
	}
	seen := map[string]bool{}
	for _, o := range req.Items {
		if err := tc.ValidateObservation(o); err != nil {
			return out, invalid("工具清单数据无效")
		}
		if seen[o.Profile.CommandID] {
			return out, invalid("工具清单命令重复")
		}
		seen[o.Profile.CommandID] = true
	}
	tx, err := s.DB.BeginTx(ctx, schedulingWriteOptions(s.Dialect))
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	runner, _, err := s.PoolRunnerAuthorityTx(ctx, tx, token, req.RunnerID, []dag.PoolCapabilityScope{{TenantID: req.TenantID, ProjectID: req.ProjectID, Capabilities: []string{}}})
	if err != nil {
		return out, err
	}
	// Machine operations use an explicitly namespaced SQL receipt. No human
	// Principal is synthesized to pass business authorization helpers.
	op := digest("pool-inventory/" + req.RunnerID + "/" + req.OperationID)
	hash := digest(stringify([]any{runner.CredentialVersion, req}))
	var stored, raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), req.TenantID, req.ProjectID, op).Scan(&stored, &raw)
	if err == nil {
		if stored != hash {
			return out, conflict("工具清单原操作内容已变化")
		}
		if err = json.Unmarshal([]byte(raw), &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out = ToolInventoryReceipt{Acknowledged: true, ObservedAt: automationStamp(clock)}
	var old string
	err = tx.QueryRowContext(ctx, s.q(`SELECT observed_at FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`), req.TenantID, req.ProjectID, runner.WorkerID).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_inventories(tenant_id,project_id,worker_id,observed_at,items)VALUES(?,?,?,?,?)`), req.TenantID, req.ProjectID, runner.WorkerID, out.ObservedAt, stringify(req.Items))
	} else if err == nil {
		_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_tool_inventories SET observed_at=?,items=? WHERE tenant_id=? AND project_id=? AND worker_id=?`), out.ObservedAt, stringify(req.Items), req.TenantID, req.ProjectID, runner.WorkerID)
	}
	if err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_runners SET last_seen_at=? WHERE id=?`), out.ObservedAt, runner.ID); err != nil {
		return out, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response)VALUES(?,?,?,?,?)`), req.TenantID, req.ProjectID, op, hash, stringify(out)); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) poolBoundToolNodeTx(ctx context.Context, tx *sql.Tx, scope dag.PoolScope, run, node string) (dag.Node, ToolProfile, error) {
	var n dag.Node
	var p ToolProfile
	var raw string
	if err := tx.QueryRowContext(ctx, s.q(`SELECT n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND r.id=? AND n.id=?`), scope.TenantID, scope.ProjectID, run, node).Scan(&raw); err != nil {
		return n, p, err
	}
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		return n, p, err
	}
	id, _, bound, err := dag.ToolBinding(n)
	if err != nil || !bound {
		return n, p, err
	}
	p, err = scanToolProfile(tx.QueryRowContext(ctx, s.q(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=?`), scope.TenantID, scope.ProjectID, id))
	return n, p, err
}

func (s *Service) PoolToolClaimEligibleTx(ctx context.Context, tx *sql.Tx, token string, runner ExecutionPoolRunner, scope dag.PoolScope, run, node string) (bool, error) {
	var managed int
	if err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND run_id=?`), scope.TenantID, scope.ProjectID, run).Scan(&managed); err != nil {
		return false, err
	}
	if managed != 0 {
		task, err := s.TaskDispatchRunTx(ctx, tx, scope.TenantID, scope.ProjectID, run)
		if err != nil || !task {
			return false, err
		}
	}
	n, p, err := s.poolBoundToolNodeTx(ctx, tx, scope, run, node)
	if err != nil {
		if automationAuthorityFailure(err) {
			return false, nil
		}
		return false, err
	}
	// Managed production/external effects need a separate pool capability and
	// authorization protocol; their project-runner privileges are not inherited.
	if n.Type == "external_workflow" || n.Config["artifact_from"] != nil {
		return false, nil
	}
	_, hash, bound, err := dag.ToolBinding(n)
	if err != nil {
		return false, nil
	}
	if !bound {
		return true, nil
	}
	if p.WorkerID != runner.WorkerID || p.Profile.CommandID != n.Command[0] || p.Profile.ProfileSHA256 != hash {
		return false, nil
	}
	// This actor is the actual persisted approver, not the machine identity.
	p, err = s.toolProfileObservationTx(ctx, tx, Principal{ID: p.EnabledBy, TenantID: scope.TenantID}, scope.ProjectID, p)
	return p.Available, err
}

func (s *Service) ValidatePoolToolProgressTx(ctx context.Context, tx *sql.Tx, token string, a dag.Assignment, event *tc.Event) error {
	if event == nil {
		return nil
	}
	n, p, err := s.poolBoundToolNodeTx(ctx, tx, dag.PoolScope{TenantID: a.TenantID, ProjectID: a.ProjectID}, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	_, hash, bound, err := dag.ToolBinding(n)
	if err != nil {
		return err
	}
	if !bound || p.WorkerID != a.WorkerID || p.Profile.ProfileSHA256 != hash || p.Profile.CommandID != event.CommandID || p.Profile.Protocol != event.Protocol || event.ProfileSHA256 != hash {
		return invalid("工具进度与本项目固定档案不匹配")
	}
	return nil
}

func (s *Service) ValidatePoolToolResultTx(ctx context.Context, tx *sql.Tx, token string, a dag.Assignment, result dag.Result) error {
	if result.ToolEvidence == nil {
		return nil
	}
	n, p, err := s.poolBoundToolNodeTx(ctx, tx, dag.PoolScope{TenantID: a.TenantID, ProjectID: a.ProjectID}, a.RunID, a.NodeID)
	if err != nil {
		return err
	}
	_, hash, bound, err := dag.ToolBinding(n)
	if err != nil {
		return err
	}
	if !bound || p.WorkerID != a.WorkerID || p.Profile.ProfileSHA256 != hash || result.ToolEvidence.Profile != p.Profile {
		return invalid("工具结果与本项目固定档案不匹配")
	}
	return nil
}
