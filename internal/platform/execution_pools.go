package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ai-mada/suma/internal/dag"
)

func poolCoreInput(input ExecutionPoolPolicyInput) dag.PoolPolicyInput {
	return dag.PoolPolicyInput{ConcurrencyLimit: input.ConcurrencyLimit, ResourceLimits: input.ResourceLimits, BudgetLimit: input.BudgetLimit, TenantWeights: input.TenantWeights}
}

func poolPolicyView(policy dag.PoolPolicy) ExecutionPoolPolicy {
	return ExecutionPoolPolicy{ExecutionPoolPolicyInput: ExecutionPoolPolicyInput{ConcurrencyLimit: policy.ConcurrencyLimit, ResourceLimits: policy.ResourceLimits, BudgetLimit: policy.BudgetLimit, TenantWeights: policy.TenantWeights}, Version: policy.Version, BudgetSpent: policy.BudgetSpent, Resources: policy.Resources, Fairness: SchedulingFairness{Mode: "hierarchical_weighted_round_robin", Scope: "pool"}, AsOf: policy.AsOf}
}

func (s *Service) poolViewTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, pool ExecutionPool) (ExecutionPool, error) {
	policy, err := engine.ReadPoolPolicyTx(ctx, tx, pool.ID)
	if err != nil {
		return pool, err
	}
	if policy.Version < 1 {
		return pool, conflict("资源池策略缺失，请由管理员核对持久化配置")
	}
	pool.Policy = poolPolicyView(policy)
	pool.AllowedActions = []string{"preview_policy", "register_runner"}
	if pool.Status == "active" {
		pool.AllowedActions = append(pool.AllowedActions, "pause")
	} else if pool.Status == "paused" {
		pool.AllowedActions = append(pool.AllowedActions, "resume")
	}
	return pool, nil
}

// CreateExecutionPool commits ownership, the complete initial admission policy,
// human audit and idempotent response as one operation. No default or partial
// policy is substituted for an invalid administrator input.
func (s *Service) CreateExecutionPool(ctx context.Context, engine *dag.Engine, actor Actor, request ExecutionPoolCreateRequest) (ExecutionPool, error) {
	p := actor.Projection()

	var out ExecutionPool
	if err := validatePoolName(request.Name); err != nil {
		return out, err
	}
	if len(request.Description) > 8000 || !utf8.ValidString(request.Description) || strings.IndexFunc(request.Description, func(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' }) >= 0 {
		return out, invalid("资源池说明必须为最多8000字节的有效文本")
	}
	if err := validateExecutionPoolPolicy(request.Policy); err != nil {
		return out, err
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
	if err = s.poolHumanWriteTx(ctx, tx, auth); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"create_execution_pool": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	var total int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pools WHERE owner_tenant_id=?`), p.TenantID).Scan(&total); err != nil {
		return out, err
	}
	if total >= 1000 {
		return out, conflict("组织已达到1000个资源池上限")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	out = ExecutionPool{ID: newID(), OwnerTenantID: p.TenantID, Name: request.Name, Description: request.Description, Version: 1, Status: "active", CreatedAt: at, UpdatedAt: at}
	// The new ID is not visible until commit. Policy, ownership, audit and the
	// operation receipt are created atomically; an empty policy is never exposed.
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_execution_pools(id,owner_tenant_id,name,description,version,status,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?)`), out.ID, out.OwnerTenantID, out.Name, out.Description, 1, "active", at, at); err != nil {
		return out, err
	}
	policy, err := engine.WritePoolPolicyTx(ctx, tx, out.ID, 0, poolCoreInput(request.Policy))
	if err != nil {
		return out, err
	}
	out.Policy = poolPolicyView(policy)
	out.AllowedActions = []string{"preview_policy", "register_runner", "pause"}
	if err = s.commandActivity(ctx, auth, "execution_pool.created", out.ID, out.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// GetExecutionPool is an owner-organization snapshot. Project consumption is
// exposed through its separate grant view, which does not disclose other work.
func (s *Service) GetExecutionPool(ctx context.Context, engine *dag.Engine, actor Actor, poolID string) (ExecutionPool, error) {
	var result0 ExecutionPool
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readGetExecutionPoolTx(bounded, auth, engine, poolID)
		return e
	})
	return result0, err
}
func (s *Service) readGetExecutionPoolTx(ctx context.Context, auth *authorizedCommand, engine *dag.Engine, poolID string) (ExecutionPool, error) {
	if auth == nil {
		return ExecutionPool{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionPool{}, e
	}
	if auth.target.projectID != organizationScope {
		return ExecutionPool{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error
	if !orgAdmin(p) {
		return ExecutionPool{}, forbidden()
	}

	var out ExecutionPool
	out, err = s.executionPoolMetadataTx(ctx, tx, poolID)
	if err != nil {
		return out, err
	}
	if out.OwnerTenantID != p.TenantID {
		return ExecutionPool{}, missing()
	}
	return s.poolViewTx(ctx, tx, engine, out)

}

func (s *Service) ListExecutionPools(ctx context.Context, engine *dag.Engine, actor Actor, limit int, cursor string) (ExecutionPoolPage, error) {
	var result0 ExecutionPoolPage
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readListExecutionPoolsTx(bounded, auth, engine, limit, cursor)
		return e
	})
	return result0, err
}
func (s *Service) readListExecutionPoolsTx(ctx context.Context, auth *authorizedCommand, engine *dag.Engine, limit int, cursor string) (ExecutionPoolPage, error) {
	if auth == nil {
		return ExecutionPoolPage{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionPoolPage{}, e
	}
	if auth.target.projectID != organizationScope {
		return ExecutionPoolPage{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error
	if !orgAdmin(p) {
		return ExecutionPoolPage{}, forbidden()
	}

	out := ExecutionPoolPage{Items: []ExecutionPool{}, AllowedActions: []string{}}
	if limit < 1 || limit > 100 {
		return out, invalid("limit必须为1–100")
	}
	last, err := automationDecodeCursor(p, organizationScope, "execution_pools", cursor)
	if err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pools WHERE owner_tenant_id=?`), p.TenantID).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT `+executionPoolColumns+` FROM suma_execution_pools WHERE owner_tenant_id=? AND id>? ORDER BY id LIMIT ?`), p.TenantID, last, limit+1)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		item, e := scanExecutionPool(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = automationEncodeCursor(p, organizationScope, "execution_pools", out.Items[len(out.Items)-1].ID)
	}
	for i, pool := range out.Items {
		out.Items[i], err = s.poolViewTx(ctx, tx, engine, pool)
		if err != nil {
			return out, err
		}
	}
	out.AllowedActions = []string{"create"}
	return out, nil

}

func (s *Service) CommandExecutionPool(ctx context.Context, engine *dag.Engine, actor Actor, poolID string, request ExecutionPoolCommandRequest) (ExecutionPool, error) {
	p := actor.Projection()

	var out ExecutionPool
	if request.ExpectedVersion < 1 || request.Action != "pause" && request.Action != "resume" {
		return out, invalid("资源池操作或版本无效")
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
	out, err = s.poolAdminTx(ctx, tx, auth, poolID)
	if err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"command_execution_pool": poolID, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	target, source := "paused", "active"
	if request.Action == "resume" {
		target, source = "active", "paused"
	}
	if out.Version != request.ExpectedVersion || out.Status != source {
		return out, conflict("资源池版本或状态已变化，请刷新")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.UpdatedAt = automationStamp(clock)
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pools SET status=?,version=version+1,updated_at=? WHERE id=? AND owner_tenant_id=? AND version=? AND status=?`), target, out.UpdatedAt, poolID, p.TenantID, out.Version, source)
	if err != nil {
		return out, err
	}
	if changed, e := result.RowsAffected(); e != nil || changed != 1 {
		return out, conflict("资源池已被其他操作修改")
	}
	out.Status, out.Version = target, out.Version+1
	out, err = s.poolViewTx(ctx, tx, engine, out)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "execution_pool."+request.Action, out.ID, out.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
