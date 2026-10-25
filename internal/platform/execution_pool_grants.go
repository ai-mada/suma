package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
)

const poolGrantColumns = `g.id,g.pool_id,p.name,g.owner_tenant_id,g.tenant_id,g.project_id,g.shared_project_name,g.version,g.status,g.project_weight,g.request_sha256,g.requested_by,g.decided_by,g.created_at,g.updated_at`

func scanPoolGrant(row interface{ Scan(...any) error }) (ExecutionPoolGrant, error) {
	var out ExecutionPoolGrant
	err := row.Scan(&out.ID, &out.PoolID, &out.PoolName, &out.OwnerTenantID, &out.TenantID, &out.ProjectID, &out.SharedProjectName, &out.Version, &out.Status, &out.ProjectWeight, &out.RequestSHA256, &out.RequestedBy, &out.DecidedBy, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	out.AllowedActions = []string{}
	return out, err
}

func (s *Service) poolGrantTx(ctx context.Context, tx *sql.Tx, id string) (ExecutionPoolGrant, error) {
	return scanPoolGrant(tx.QueryRowContext(ctx, s.q(`SELECT `+poolGrantColumns+` FROM suma_execution_pool_grants g JOIN suma_execution_pools p ON p.id=g.pool_id WHERE g.id=?`), id))
}

func (s *Service) currentPoolGrantTx(ctx context.Context, tx *sql.Tx, id string) (ExecutionPoolGrant, error) {
	return scanPoolGrant(tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT `+poolGrantColumns+` FROM suma_execution_pool_grants g JOIN suma_execution_pools p ON p.id=g.pool_id WHERE g.id=?`)), id))
}

// A grant delegates execution on behalf of a project. A reviewer can approve
// task evidence, but cannot grant a new machine access to the whole project.
func (s *Service) poolProjectAccessTx(ctx context.Context, tx *sql.Tx, tenant, project, user string, admin bool) (bool, error) {
	var owner, role string
	err := tx.QueryRowContext(ctx, s.q(`SELECT tenant_id FROM suma_projects WHERE id=?`), project).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner != tenant {
		return false, nil
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT m.role FROM suma_org_members m JOIN suma_users u ON u.id=m.user_id WHERE m.tenant_id=? AND m.user_id=?`), tenant, user).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !validOrgRole(role) {
		return false, nil
	}
	if role == "owner" || role == "admin" {
		return true, nil
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`), project, user).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return validProjectRole(role) && (!admin || role == "admin"), nil
}

func (s *Service) currentPoolProjectAccessTx(ctx context.Context, tx *sql.Tx, tenant, project, user string, admin bool) (bool, error) {
	var owner, role string
	err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT tenant_id FROM suma_projects WHERE id=?`)), project).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if owner != tenant {
		return false, nil
	}
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT m.role FROM suma_org_members m JOIN suma_users u ON u.id=m.user_id WHERE m.tenant_id=? AND m.user_id=?`)), tenant, user).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !validOrgRole(role) {
		return false, nil
	}
	if role == "owner" || role == "admin" {
		return true, nil
	}
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`)), project, user).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return validProjectRole(role) && (!admin || role == "admin"), nil
}

func (s *Service) poolProjectAdminTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project string) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(tx, auth.target, auth.purpose); err != nil {
		return err
	}
	if !auth.actor.IsBrowser() || auth.target.projectID != project {
		return forbidden()
	}
	p := auth.currentPrincipal
	allowed, err := s.currentPoolProjectAccessTx(ctx, tx, p.TenantID, project, p.ID, true)
	if err != nil {
		return err
	}
	if !allowed {
		return forbidden()
	}
	return nil
}

func (s *Service) poolGrantViewTx(ctx context.Context, tx *sql.Tx, p Principal, g ExecutionPoolGrant, owner bool) (ExecutionPoolGrant, error) {
	g.AllowedActions = []string{}
	canRead, canAdmin := false, false
	var err error
	if g.TenantID == p.TenantID {
		canRead, err = s.currentPoolProjectAccessTx(ctx, tx, p.TenantID, g.ProjectID, p.ID, false)
		if err != nil {
			return g, err
		}
		canAdmin, err = s.currentPoolProjectAccessTx(ctx, tx, p.TenantID, g.ProjectID, p.ID, true)
		if err != nil {
			return g, err
		}
	}
	g.CanOpenProject = canRead
	if owner && g.OwnerTenantID == p.TenantID {
		if g.Status == "pending" {
			g.AllowedActions = []string{"approve", "reject"}
		} else if g.Status == "approved" {
			g.AllowedActions = []string{"revoke"}
		}
	} else if canAdmin && (g.Status == "pending" || g.Status == "approved") {
		g.AllowedActions = []string{"withdraw"}
	}
	return g, nil
}

// RequestExecutionPoolGrant freezes the only metadata shared with the owning
// organization: this project's ID and its current title. It creates no access
// until the other organization approves that exact request digest/version.
func (s *Service) RequestExecutionPoolGrant(ctx context.Context, actor Actor, request ExecutionPoolGrantRequest) (ExecutionPoolGrant, error) {
	p := actor.Projection()

	var out ExecutionPoolGrant
	if !executionPoolID.MatchString(request.PoolID) || !executionPoolID.MatchString(request.ProjectID) || request.ProjectWeight < 1 || request.ProjectWeight > 100 {
		return out, invalid("项目、资源池或项目权重无效")
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
	pool, err := s.executionPoolMetadataTx(ctx, tx, request.PoolID)
	if err != nil {
		return out, err
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: request.ProjectID}, purposeBrowserProjectApprove, []string{pool.OwnerTenantID})
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	lockedPool, e := s.currentExecutionPoolMetadataTx(ctx, tx, request.PoolID)
	if e != nil {
		return out, e
	}
	if lockedPool.OwnerTenantID != pool.OwnerTenantID {
		return out, conflict("资源池归属已变化，请重试")
	}
	pool = lockedPool
	if err = s.poolProjectAdminTx(ctx, tx, auth, request.ProjectID); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"request_pool_grant": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	var duplicate, total int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pool_grants WHERE pool_id=? AND tenant_id=? AND project_id=? AND status IN ('pending','approved')`), pool.ID, p.TenantID, request.ProjectID).Scan(&duplicate); err != nil {
		return out, err
	}
	if duplicate != 0 {
		return out, conflict("该项目已有待处理或有效的池授权，请先查看现有申请")
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pool_grants WHERE pool_id=? AND status IN ('pending','approved')`), pool.ID).Scan(&total); err != nil {
		return out, err
	}
	if total >= 256 {
		return out, conflict("资源池已达到256个待处理或有效项目授权上限")
	}
	var title, status string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT title,status FROM suma_projects WHERE id=? AND tenant_id=?`), request.ProjectID, p.TenantID).Scan(&title, &status); err != nil {
		return out, err
	}
	if status != "active" || len(title) > 500 {
		return out, conflict("仅有效且名称不超过500字节的项目可申请执行授权")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	at := automationStamp(clock)
	out = ExecutionPoolGrant{ID: newID(), PoolID: pool.ID, PoolName: pool.Name, OwnerTenantID: pool.OwnerTenantID, TenantID: p.TenantID, ProjectID: request.ProjectID, SharedProjectName: title, Version: 1, Status: "pending", ProjectWeight: request.ProjectWeight, RequestedBy: p.ID, CreatedAt: at, UpdatedAt: at, AllowedActions: []string{"withdraw"}, CanOpenProject: true}
	out.RequestSHA256 = digest(stringify(map[string]any{"pool_id": pool.ID, "owner_tenant_id": pool.OwnerTenantID, "tenant_id": p.TenantID, "project_id": request.ProjectID, "shared_project_name": title, "project_weight": request.ProjectWeight}))
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_execution_pool_grants(id,pool_id,owner_tenant_id,tenant_id,project_id,shared_project_name,version,status,project_weight,request_sha256,requested_by,decided_by,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`), out.ID, out.PoolID, out.OwnerTenantID, out.TenantID, out.ProjectID, out.SharedProjectName, 1, "pending", out.ProjectWeight, out.RequestSHA256, p.ID, "", at, at); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "execution_pool.grant_requested", out.ID, out.SharedProjectName); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) CommandOwnedPoolGrant(ctx context.Context, engine *dag.Engine, actor Actor, pool, grant string, request ExecutionPoolGrantCommand) (ExecutionPoolGrant, error) {
	return s.commandPoolGrant(ctx, engine, actor, pool, grant, request, true)
}

func (s *Service) WithdrawProjectPoolGrant(ctx context.Context, engine *dag.Engine, actor Actor, grant string, request ExecutionPoolGrantCommand) (ExecutionPoolGrant, error) {
	return s.commandPoolGrant(ctx, engine, actor, "", grant, request, false)
}

func (s *Service) commandPoolGrant(ctx context.Context, engine *dag.Engine, actor Actor, pool, grant string, request ExecutionPoolGrantCommand, owner bool) (ExecutionPoolGrant, error) {
	p := actor.Projection()

	var out ExecutionPoolGrant
	if request.ExpectedVersion < 1 {
		return out, invalid("请提供授权当前版本")
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
	out, err = s.poolGrantTx(ctx, tx, grant)
	if err != nil {
		return out, err
	}
	if owner && (out.PoolID != pool || out.OwnerTenantID != p.TenantID) || !owner && (out.TenantID != p.TenantID || out.ProjectID != request.ProjectID) {
		return ExecutionPoolGrant{}, missing()
	}
	receiptScope, purpose := out.ProjectID, purposeBrowserProjectApprove
	if owner {
		receiptScope = organizationScope
		purpose = purposeOrganizationWrite
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: receiptScope}, purpose, []string{out.OwnerTenantID, out.TenantID})
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	discovered := out
	out, err = s.currentPoolGrantTx(ctx, tx, grant)
	if err != nil {
		return out, err
	}
	if out.OwnerTenantID != discovered.OwnerTenantID || out.TenantID != discovered.TenantID || out.ProjectID != discovered.ProjectID || out.PoolID != discovered.PoolID {
		return out, conflict("授权归属已变化，请重试")
	}
	if owner {
		if _, err = s.poolAdminTx(ctx, tx, auth, out.PoolID); err != nil {
			return out, err
		}
	} else if err = s.poolProjectAdminTx(ctx, tx, auth, out.ProjectID); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"pool_grant_command": grant, "owner_context": owner, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	out, err = s.currentPoolGrantTx(ctx, tx, grant)
	if err != nil {
		return out, err
	}
	if out.Version != request.ExpectedVersion {
		return out, conflict("授权版本已变化，请刷新")
	}
	target := ""
	if owner && out.Status == "pending" && request.Action == "approve" {
		target = "approved"
	}
	if owner && out.Status == "pending" && request.Action == "reject" {
		target = "rejected"
	}
	if owner && out.Status == "approved" && request.Action == "revoke" {
		target = "revoked"
	}
	if !owner && (out.Status == "pending" || out.Status == "approved") && request.Action == "withdraw" {
		target = "withdrawn"
	}
	if target == "" {
		return out, conflict("该授权状态不支持所请求操作")
	}
	if target == "approved" {
		allowed, e := s.currentPoolProjectAccessTx(ctx, tx, out.TenantID, out.ProjectID, out.RequestedBy, true)
		if e != nil {
			return out, e
		}
		if !allowed {
			return out, conflict("申请人的当前项目管理权限已失效，请重新申请")
		}
	}
	oldStatus := out.Status
	out.Status, out.Version, out.DecidedBy = target, out.Version+1, p.ID
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.UpdatedAt = automationStamp(clock)
	// The authoritative bilateral status and the scheduler's scope mirror
	// advance in one transaction. Terminal grants cannot be reopened by a new
	// operation; changing scope requires a new reviewed grant and run plan.
	if target == "approved" || oldStatus == "approved" {
		mirror := "inactive"
		if target == "approved" {
			mirror = "active"
		}
		if err = engine.WritePoolScopeTx(ctx, tx, dag.PoolScope{PoolID: out.PoolID, TenantID: out.TenantID, ProjectID: out.ProjectID, GrantID: out.ID, GrantVersion: out.Version, ProjectWeight: out.ProjectWeight, Status: mirror}); err != nil {
			return out, err
		}
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_grants SET version=?,status=?,decided_by=?,updated_at=? WHERE id=? AND version=? AND status=?`), out.Version, target, p.ID, out.UpdatedAt, grant, request.ExpectedVersion, oldStatus)
	if err != nil {
		return out, err
	}
	if changed, e := result.RowsAffected(); e != nil || changed != 1 {
		return out, conflict("授权已被其他操作修改")
	}
	out, err = s.poolGrantViewTx(ctx, tx, p, out, owner)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "execution_pool.grant_"+request.Action, grant, out.SharedProjectName); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
