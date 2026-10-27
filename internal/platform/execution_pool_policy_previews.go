package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"sort"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func normalizedPoolInput(in ExecutionPoolPolicyInput) ExecutionPoolPolicyInput {
	out := ExecutionPoolPolicyInput{ConcurrencyLimit: in.ConcurrencyLimit, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}
	if in.BudgetLimit != nil {
		v := *in.BudgetLimit
		out.BudgetLimit = &v
	}
	for key, value := range in.ResourceLimits {
		if value == nil {
			out.ResourceLimits[key] = nil
		} else {
			v := *value
			out.ResourceLimits[key] = &v
		}
	}
	for key, value := range in.TenantWeights {
		out.TenantWeights[key] = value
	}
	return out
}

func poolWeightChanges(before, after map[string]int) []ExecutionPoolWeightChange {
	keys := map[string]bool{}
	for key := range before {
		keys[key] = true
	}
	for key := range after {
		keys[key] = true
	}
	ordered := []string{}
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	out := []ExecutionPoolWeightChange{}
	for _, key := range ordered {
		a, b := before[key], after[key]
		if a == 0 {
			a = 1
		}
		if b == 0 {
			b = 1
		}
		if a != b {
			out = append(out, ExecutionPoolWeightChange{TenantID: key, Before: a, After: b})
		}
	}
	return out
}

func poolPolicyPreviewHash(p ExecutionPoolPolicyPreview) string {
	return digest(stringify(map[string]any{"id": p.ID, "pool_id": p.PoolID, "policy_version": p.PolicyVersion, "pool_version": p.PoolVersion, "before": p.Before, "policy": p.Policy, "changes": p.Changes, "weight_changes": p.WeightChanges, "impact": p.Impact, "created_by": p.CreatedBy, "created_at": p.CreatedAt, "expires_at": p.ExpiresAt}))
}

func (s *Service) decodePoolPolicyPreview(row executionPreviewRecord) (ExecutionPoolPolicyPreview, error) {
	var out ExecutionPoolPolicyPreview
	if err := json.Unmarshal([]byte(row.payload), &out); err != nil {
		return out, err
	}
	if out.ID != row.id || out.PoolID != row.pool || out.CreatedBy != row.createdBy || out.CreatedAt != row.createdAt || out.ExpiresAt != row.expiresAt || out.PayloadSHA256 != poolPolicyPreviewHash(out) {
		return out, conflict("预览内容校验失败，请重新生成")
	}
	out.Version, out.State, out.AppliedPolicyVersion = row.version, row.state, row.applied
	out.Valid, out.Reason, out.AllowedActions = false, "", []string{}
	return out, nil
}

func (s *Service) poolPolicyPreviewReasonTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, pool ExecutionPool, preview ExecutionPoolPolicyPreview) (string, error) {
	if preview.State != "ready" {
		return "该预览已提交，请查看已采用版本", nil
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return "", err
	}
	if reason := previewExpiryReason(clock, preview.ExpiresAt); reason != "" {
		return reason, nil
	}
	if pool.Version != preview.PoolVersion {
		return "资源池状态依据已变化，请重新预览", nil
	}
	current, err := engine.ReadPoolPolicyTx(ctx, tx, pool.ID)
	if err != nil {
		return "", err
	}
	if current.Version != preview.PolicyVersion {
		return "资源池策略依据已变化，请重新预览", nil
	}
	return "", nil
}

// CreateExecutionPoolPolicyPreview observes impact without reserving slots or
// charging budget. It fixes both policy and owning-pool metadata generations.
func (s *Service) CreateExecutionPoolPolicyPreview(ctx context.Context, engine *dag.Engine, actor Actor, poolID string, request ExecutionPoolPolicyPreviewRequest) (ExecutionPoolPolicyPreview, error) {
	p := actor.Projection()

	var out ExecutionPoolPolicyPreview
	if request.ExpectedVersion < 1 {
		return out, invalid("请提供当前资源池策略版本")
	}
	if err := validatePoolOperation(request.IdempotencyKey); err != nil {
		return out, err
	}
	if err := validateExecutionPoolPolicy(request.Policy); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: organizationScope}, purposeOrganizationWrite)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	pool, err := s.poolAdminTx(ctx, tx, auth, poolID)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"pool_policy_preview": poolID, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	before, err := engine.ReadPoolPolicyTx(ctx, tx, poolID)
	if err != nil {
		return out, err
	}
	if before.Version != request.ExpectedVersion {
		return out, conflict("资源池策略已变化，请读取当前版本")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	input := normalizedPoolInput(request.Policy)
	out = ExecutionPoolPolicyPreview{ID: newID(), PoolID: poolID, Version: 1, State: "ready", PolicyVersion: before.Version, PoolVersion: pool.Version, Before: poolPolicyView(before), Policy: input, Changes: schedulingChanges(dag.SchedulingPolicyInput{ConcurrencyLimit: before.ConcurrencyLimit, ResourceLimits: before.ResourceLimits, BudgetLimit: before.BudgetLimit}, dag.SchedulingPolicyInput{ConcurrencyLimit: input.ConcurrencyLimit, ResourceLimits: input.ResourceLimits, BudgetLimit: input.BudgetLimit}), WeightChanges: poolWeightChanges(before.TenantWeights, input.TenantWeights), CreatedBy: p.ID, CreatedAt: automationStamp(clock), ExpiresAt: automationStamp(clock.Add(30 * time.Minute)), AllowedActions: []string{"commit"}, Valid: true}
	out.Impact.AsOf = automationStamp(clock)
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_ready q JOIN dag_run_execution_routes r ON r.run_id=q.run_id WHERE r.pool_id=?`), poolID).Scan(&out.Impact.ReadyNodes); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes n JOIN dag_run_execution_routes r ON r.run_id=n.run_id WHERE r.pool_id=? AND n.state IN ('running','reconciling')`), poolID).Scan(&out.Impact.ActiveAttempts); err != nil {
		return out, err
	}
	out.PayloadSHA256 = poolPolicyPreviewHash(out)
	if err = s.saveExecutionPreviewTx(ctx, tx, executionPreviewRecord{id: out.ID, tenant: p.TenantID, project: organizationScope, pool: poolID, kind: "policy", createdBy: p.ID, createdAt: out.CreatedAt, expiresAt: out.ExpiresAt, payload: stringify(out)}); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) GetExecutionPoolPolicyPreview(ctx context.Context, engine *dag.Engine, actor Actor, pool, id string) (ExecutionPoolPolicyPreview, error) {
	var result0 ExecutionPoolPolicyPreview
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readGetExecutionPoolPolicyPreviewTx(bounded, auth, engine, pool, id)
		return e
	})
	return result0, err
}
func (s *Service) readGetExecutionPoolPolicyPreviewTx(ctx context.Context, auth *authorizedCommand, engine *dag.Engine, pool, id string) (ExecutionPoolPolicyPreview, error) {
	if auth == nil {
		return ExecutionPoolPolicyPreview{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionPoolPolicyPreview{}, e
	}
	if auth.target.projectID != organizationScope {
		return ExecutionPoolPolicyPreview{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error
	if !orgAdmin(p) {
		return ExecutionPoolPolicyPreview{}, forbidden()
	}

	var out ExecutionPoolPolicyPreview
	meta, err := s.executionPoolMetadataTx(ctx, tx, pool)
	if err != nil {
		return out, err
	}
	if meta.OwnerTenantID != p.TenantID {
		return out, missing()
	}
	row, err := s.executionPreviewRecordTx(ctx, tx, p.TenantID, organizationScope, pool, "policy", id)
	if err != nil {
		return out, err
	}
	out, err = s.decodePoolPolicyPreview(row)
	if err != nil {
		return out, err
	}
	out.Reason, err = s.poolPolicyPreviewReasonTx(ctx, tx, engine, meta, out)
	if err != nil {
		return out, err
	}
	out.Valid = out.Reason == ""
	if out.Valid {
		out.AllowedActions = []string{"commit"}
	}
	return out, nil

}

func (s *Service) CommitExecutionPoolPolicyPreview(ctx context.Context, engine *dag.Engine, actor Actor, pool, id string, request ExecutionPreviewCommitRequest) (ExecutionPool, error) {
	p := actor.Projection()

	var out ExecutionPool
	if request.ProjectID != "" || request.ExpectedVersion < 1 {
		return out, invalid("请提供拥有方预览版本")
	}
	if err := validatePoolOperation(request.IdempotencyKey); err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: organizationScope}, purposeOrganizationWrite)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	out, err = s.poolAdminTx(ctx, tx, auth, pool)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"commit_pool_policy": id, "pool": pool, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	row, err := s.executionPreviewRecordTx(ctx, tx, p.TenantID, organizationScope, pool, "policy", id)
	if err != nil {
		return out, err
	}
	preview, err := s.decodePoolPolicyPreview(row)
	if err != nil {
		return out, err
	}
	if preview.Version != request.ExpectedVersion {
		return out, conflict("预览版本已变化，请刷新")
	}
	reason, err := s.poolPolicyPreviewReasonTx(ctx, tx, engine, out, preview)
	if err != nil {
		return out, err
	}
	if reason != "" {
		return out, conflict(reason)
	}
	policy, err := engine.WritePoolPolicyTx(ctx, tx, pool, preview.PolicyVersion, poolCoreInput(preview.Policy))
	if err != nil {
		return out, err
	}
	if err = s.commitExecutionPreviewTx(ctx, tx, row, policy.Version); err != nil {
		return out, err
	}
	out, err = s.poolViewTx(ctx, tx, engine, out)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "execution_pool.policy_applied", pool, out.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
