package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

func validateExecutionRouteInput(input ExecutionRouteInput) error {
	if input.Mode == "local" {
		if input.PoolID != "" || input.GrantID != "" || input.GrantVersion != 0 {
			return invalid("项目执行路由不接受资源池授权字段")
		}
		return nil
	}
	if input.Mode != "pool" || !executionPoolID.MatchString(input.PoolID) || !executionPoolID.MatchString(input.GrantID) || input.GrantVersion < 1 {
		return invalid("请选择明确执行路由与有效资源池授权版本")
	}
	return nil
}

func (s *Service) executionRouteTargetTx(ctx context.Context, tx *sql.Tx, tenant, project string, input ExecutionRouteInput) (ExecutionPool, error) {
	if input.Mode == "local" {
		return ExecutionPool{}, nil
	}
	grant, err := s.poolGrantTx(ctx, tx, input.GrantID)
	if err != nil {
		return ExecutionPool{}, err
	}
	if grant.TenantID != tenant || grant.ProjectID != project || grant.PoolID != input.PoolID {
		return ExecutionPool{}, missing()
	}
	if grant.Status != "approved" || grant.Version != input.GrantVersion {
		return ExecutionPool{}, conflict("项目资源池授权已变化，请重新选择有效授权")
	}
	return s.executionPoolMetadataTx(ctx, tx, input.PoolID)
}

func (s *Service) currentExecutionRouteTargetTx(ctx context.Context, tx *sql.Tx, tenant, project string, input ExecutionRouteInput) (ExecutionPool, error) {
	if input.Mode == "local" {
		return ExecutionPool{}, nil
	}
	grant, err := s.currentPoolGrantTx(ctx, tx, input.GrantID)
	if err != nil {
		return ExecutionPool{}, err
	}
	if grant.TenantID != tenant || grant.ProjectID != project || grant.PoolID != input.PoolID {
		return ExecutionPool{}, missing()
	}
	if grant.Status != "approved" || grant.Version != input.GrantVersion {
		return ExecutionPool{}, conflict("项目资源池授权已变化，请重新选择有效授权")
	}
	return s.currentExecutionPoolMetadataTx(ctx, tx, input.PoolID)
}

func (s *Service) executionRouteViewTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, p Principal, project string) (ExecutionRouteView, error) {
	var out ExecutionRouteView
	route, err := engine.ReadProjectExecutionRouteTx(ctx, tx, p.TenantID, project)
	if err != nil {
		return out, err
	}
	out = ExecutionRouteView{ProjectExecutionRoute: route, Mode: "local", AllowedActions: []string{}}
	if route.PoolID != "" {
		out.Mode = "pool"
		pool, err := s.executionPoolMetadataTx(ctx, tx, route.PoolID)
		if err != nil {
			return out, err
		}
		out.PoolName = pool.Name
		grant, err := s.poolGrantTx(ctx, tx, route.GrantID)
		if err != nil {
			return out, err
		}
		out.GrantState = grant.Status
	}
	admin, err := s.currentPoolProjectAccessTx(ctx, tx, p.TenantID, project, p.ID, true)
	if err != nil {
		return out, err
	}
	if admin {
		out.AllowedActions = []string{"preview"}
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out.AsOf = automationStamp(clock)
	return out, nil
}

func (s *Service) GetProjectExecutionRoute(ctx context.Context, engine *dag.Engine, actor Actor, project string) (ExecutionRouteView, error) {
	var result0 ExecutionRouteView
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeBrowserProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readGetProjectExecutionRouteTx(bounded, auth, engine, project)
		return e
	})
	return result0, err
}
func (s *Service) readGetProjectExecutionRouteTx(ctx context.Context, auth *authorizedCommand, engine *dag.Engine, project string) (ExecutionRouteView, error) {
	if auth == nil {
		return ExecutionRouteView{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionRouteView{}, e
	}
	if auth.target.projectID != project {
		return ExecutionRouteView{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal

	return s.executionRouteViewTx(ctx, tx, engine, p, project)

}

func executionRoutePreviewHash(p ExecutionRoutePreview) string {
	return digest(stringify(map[string]any{"id": p.ID, "project_id": p.ProjectID, "route_version": p.RouteVersion, "before": p.Before, "route": p.Route, "pool_name": p.PoolName, "pool_status": p.PoolStatus, "pool_version": p.PoolVersion, "pool_policy_version": p.PoolPolicyVersion, "impact": p.Impact, "created_by": p.CreatedBy, "created_at": p.CreatedAt, "expires_at": p.ExpiresAt}))
}

func (s *Service) decodeExecutionRoutePreview(row executionPreviewRecord) (ExecutionRoutePreview, error) {
	var out ExecutionRoutePreview
	if err := json.Unmarshal([]byte(row.payload), &out); err != nil {
		return out, err
	}
	if out.ID != row.id || out.ProjectID != row.project || out.Route.PoolID != row.pool || out.CreatedBy != row.createdBy || out.CreatedAt != row.createdAt || out.ExpiresAt != row.expiresAt || out.PayloadSHA256 != executionRoutePreviewHash(out) {
		return out, conflict("路由预览内容校验失败，请重新生成")
	}
	out.Version, out.State, out.AppliedRouteVersion = row.version, row.state, row.applied
	out.Valid, out.Reason, out.AllowedActions = false, "", []string{}
	return out, nil
}

func (s *Service) executionRoutePreviewReasonTx(ctx context.Context, tx *sql.Tx, engine *dag.Engine, p Principal, preview ExecutionRoutePreview) (string, error) {
	if preview.State != "ready" {
		return "该路由预览已提交，请查看已采用版本", nil
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return "", err
	}
	if reason := previewExpiryReason(clock, preview.ExpiresAt); reason != "" {
		return reason, nil
	}
	current, err := engine.ReadProjectExecutionRouteTx(ctx, tx, p.TenantID, preview.ProjectID)
	if err != nil {
		return "", err
	}
	if current.Version != preview.RouteVersion {
		return "项目执行路由依据已变化，请重新预览", nil
	}
	if preview.Route.Mode == "pool" {
		pool, err := s.executionRouteTargetTx(ctx, tx, p.TenantID, preview.ProjectID, preview.Route)
		if err != nil {
			var apiError *APIError
			if automationAuthorityFailure(err) || errors.As(err, &apiError) && apiError.Status == 409 {
				return "资源池授权依据已失效，请重新预览", nil
			}
			return "", err
		}
		if pool.Version != preview.PoolVersion {
			return "目标资源池状态依据已变化，请重新预览", nil
		}
		policy, err := engine.ReadPoolPolicyTx(ctx, tx, pool.ID)
		if err != nil {
			return "", err
		}
		if policy.Version != preview.PoolPolicyVersion {
			return "目标资源池策略依据已变化，请重新预览", nil
		}
	}
	return "", nil
}

// Route previews change no runs. The selected grant and target pool policy are
// fixed review inputs; actual resource admission is still checked at dispatch.
func (s *Service) CreateExecutionRoutePreview(ctx context.Context, engine *dag.Engine, actor Actor, request ExecutionRoutePreviewRequest) (ExecutionRoutePreview, error) {
	p := actor.Projection()

	var out ExecutionRoutePreview
	if !executionPoolID.MatchString(request.ProjectID) || request.ExpectedVersion < 0 {
		return out, invalid("项目与路由版本无效")
	}
	if err := validateExecutionRouteInput(request.Route); err != nil {
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
	pool, err := s.executionRouteTargetTx(ctx, tx, p.TenantID, request.ProjectID, request.Route)
	if err != nil {
		return out, err
	}
	tenants := []string{p.TenantID}
	if pool.ID != "" {
		tenants = append(tenants, pool.OwnerTenantID)
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: request.ProjectID}, purposeBrowserProjectApprove, tenants)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	if err = s.poolProjectAdminTx(ctx, tx, auth, request.ProjectID); err != nil {
		return out, err
	}
	lockedPool, e := s.currentExecutionRouteTargetTx(ctx, tx, p.TenantID, request.ProjectID, request.Route)
	if e != nil {
		return out, e
	}
	if lockedPool.OwnerTenantID != pool.OwnerTenantID {
		return out, conflict("资源池归属已变化，请重试")
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"execution_route_preview": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	pool, err = s.currentExecutionRouteTargetTx(ctx, tx, p.TenantID, request.ProjectID, request.Route)
	if err != nil {
		return out, err
	}
	before, err := s.executionRouteViewTx(ctx, tx, engine, p, request.ProjectID)
	if err != nil {
		return out, err
	}
	if before.Version != request.ExpectedVersion {
		return out, conflict("项目执行路由已变化，请读取当前版本")
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	out = ExecutionRoutePreview{ID: newID(), ProjectID: request.ProjectID, Version: 1, State: "ready", RouteVersion: before.Version, Before: before, Route: request.Route, PoolName: pool.Name, PoolStatus: pool.Status, PoolVersion: pool.Version, CreatedBy: p.ID, CreatedAt: automationStamp(clock), ExpiresAt: automationStamp(clock.Add(30 * time.Minute)), AllowedActions: []string{"commit"}, Valid: true}
	if pool.ID != "" {
		policy, e := engine.ReadPoolPolicyTx(ctx, tx, pool.ID)
		if e != nil {
			return out, e
		}
		if policy.Version < 1 {
			return out, conflict("目标资源池策略不可用")
		}
		out.PoolPolicyVersion = policy.Version
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_runs WHERE tenant_id=? AND project_id=?`), p.TenantID, request.ProjectID).Scan(&out.Impact.ExistingRuns); err != nil {
		return out, err
	}
	out.Impact.AsOf = automationStamp(clock)
	out.PayloadSHA256 = executionRoutePreviewHash(out)
	if err = s.saveExecutionPreviewTx(ctx, tx, executionPreviewRecord{id: out.ID, tenant: p.TenantID, project: request.ProjectID, pool: out.Route.PoolID, kind: "route", createdBy: p.ID, createdAt: out.CreatedAt, expiresAt: out.ExpiresAt, payload: stringify(out)}); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) GetExecutionRoutePreview(ctx context.Context, engine *dag.Engine, actor Actor, project, id string) (ExecutionRoutePreview, error) {
	var result0 ExecutionRoutePreview
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeBrowserProjectRead, func(ctx context.Context, auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		var e error
		result0, e = s.readGetExecutionRoutePreviewTx(bounded, auth, engine, project, id)
		return e
	})
	return result0, err
}
func (s *Service) readGetExecutionRoutePreviewTx(ctx context.Context, auth *authorizedCommand, engine *dag.Engine, project, id string) (ExecutionRoutePreview, error) {
	if auth == nil {
		return ExecutionRoutePreview{}, unauthenticated()
	}
	if e := auth.assertTx(auth.tx, auth.target, auth.purpose); e != nil {
		return ExecutionRoutePreview{}, e
	}
	if auth.target.projectID != project {
		return ExecutionRoutePreview{}, forbidden()
	}

	tx, p := auth.tx, auth.currentPrincipal
	var err error

	var out ExecutionRoutePreview
	row, err := s.executionPreviewRecordTx(ctx, tx, p.TenantID, project, "", "route", id)
	if err != nil {
		return out, err
	}
	out, err = s.decodeExecutionRoutePreview(row)
	if err != nil {
		return out, err
	}
	admin, err := s.currentPoolProjectAccessTx(ctx, tx, p.TenantID, project, p.ID, true)
	if err != nil {
		return out, err
	}
	if !admin {
		out.Reason = "当前身份不能修改项目执行路由"
		return out, nil
	}
	out.Reason, err = s.executionRoutePreviewReasonTx(ctx, tx, engine, p, out)
	if err != nil {
		return out, err
	}
	out.Valid = out.Reason == ""
	if out.Valid {
		out.AllowedActions = []string{"commit"}
	}
	return out, nil

}

func (s *Service) CommitExecutionRoutePreview(ctx context.Context, engine *dag.Engine, actor Actor, id string, request ExecutionPreviewCommitRequest) (ExecutionRouteView, error) {
	p := actor.Projection()

	var out ExecutionRouteView
	if !executionPoolID.MatchString(request.ProjectID) || request.ExpectedVersion < 1 {
		return out, invalid("请提供项目与预览当前版本")
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
	row, err := s.executionPreviewRecordTx(ctx, tx, p.TenantID, request.ProjectID, "", "route", id)
	if err != nil {
		return out, err
	}
	preview, err := s.decodeExecutionRoutePreview(row)
	if err != nil {
		return out, err
	}
	tenants := []string{p.TenantID}
	if preview.Route.PoolID != "" {
		pool, e := s.executionPoolMetadataTx(ctx, tx, preview.Route.PoolID)
		if e != nil {
			return out, e
		}
		tenants = append(tenants, pool.OwnerTenantID)
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: request.ProjectID}, purposeBrowserProjectApprove, tenants)
	if err != nil {
		return out, err
	}
	p = auth.currentPrincipal
	if err = s.poolProjectAdminTx(ctx, tx, auth, request.ProjectID); err != nil {
		return out, err
	}
	hash, replay, err := s.commandReceipt(ctx, auth, request.IdempotencyKey, map[string]any{"commit_execution_route": id, "request": request})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	row, err = s.currentExecutionPreviewRecordTx(ctx, tx, p.TenantID, request.ProjectID, "", "route", id)
	if err != nil {
		return out, err
	}
	preview, err = s.decodeExecutionRoutePreview(row)
	if err != nil {
		return out, err
	}
	lockedPool, e := s.currentExecutionRouteTargetTx(ctx, tx, p.TenantID, request.ProjectID, preview.Route)
	if e != nil {
		return out, e
	}
	lockedOwner := lockedPool.OwnerTenantID
	known := lockedOwner == ""
	for _, org := range tenants {
		if org == lockedOwner {
			known = true
		}
	}
	if !known {
		return out, conflict("资源池归属已变化，请重试")
	}
	if preview.Version != request.ExpectedVersion {
		return out, conflict("路由预览版本已变化，请刷新")
	}
	reason, err := s.executionRoutePreviewReasonTx(ctx, tx, engine, p, preview)
	if err != nil {
		return out, err
	}
	if reason != "" {
		return out, conflict(reason)
	}
	selected := dag.ProjectExecutionRouteInput{PoolID: preview.Route.PoolID, GrantID: preview.Route.GrantID, GrantVersion: preview.Route.GrantVersion}
	route, err := engine.WriteProjectExecutionRouteTx(ctx, tx, p.TenantID, request.ProjectID, preview.RouteVersion, selected)
	if err != nil {
		return out, err
	}
	if err = s.commitExecutionPreviewTx(ctx, tx, row, route.Version); err != nil {
		return out, err
	}
	out, err = s.executionRouteViewTx(ctx, tx, engine, p, request.ProjectID)
	if err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "execution_pool.route_applied", id, preview.Route.Mode); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, request.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
