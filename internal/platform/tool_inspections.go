package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func toolInspectionInvalid() error {
	return invalidDiagnostic("validation.tool_inspection", "工具重检请求无效", nil)
}
func toolInspectionChanged() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_inspection_source_changed", "工具重检依据已变化，请查看原请求后重新配置", nil}, key: "validation.tool_inspection_changed"}
}
func toolInspectionBusy() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_inspection_in_progress", "执行节点已有未完成的工具重检，请先核对原请求", nil}, key: "validation.tool_inspection_busy"}
}

// One snapshot records the exact profile, current execution identity and exact
// project Pool route/grant. It grants no tool/task/configuration write access.
func (s *Service) toolInspectionSourceTx(ctx context.Context, tx *sql.Tx, tenant, project, profile, requester string) (ToolProfile, string, error) {
	p, err := s.toolProfileTx(ctx, tx, Principal{TenantID: tenant}, project, profile)
	if err != nil {
		return p, "", err
	}
	if p.State == "revoked" {
		return p, "", toolInspectionChanged()
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, tenant, requester, project, true, true); err != nil {
		return p, "", err
	}
	var state string
	if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status FROM suma_projects WHERE tenant_id=? AND id=?`)), tenant, project).Scan(&state); err != nil {
		return p, "", err
	}
	if state != "active" {
		return p, "", forbidden()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return p, "", err
	}
	var identity string
	if strings.HasPrefix(strings.ToLower(p.WorkerID), "pool-") {
		var pool, owner, runner, grant, expires string
		var grantVersion, credentialVersion int64
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT d.pool_id,p.owner_tenant_id,r.id,g.id,g.version,r.credential_version,c.expires_at FROM dag_project_execution_routes d JOIN suma_execution_pool_grants g ON g.id=d.grant_id AND g.version=d.grant_version AND g.pool_id=d.pool_id AND g.tenant_id=d.tenant_id AND g.project_id=d.project_id JOIN suma_execution_pools p ON p.id=d.pool_id JOIN suma_execution_pool_runners r ON r.pool_id=d.pool_id JOIN suma_execution_pool_credentials c ON c.runner_id=r.id AND c.version=r.credential_version WHERE d.tenant_id=? AND d.project_id=? AND r.worker_id=? AND g.status='approved' AND p.status='active' AND r.status='active' AND c.status='active'`)), tenant, project, p.WorkerID).Scan(&pool, &owner, &runner, &grant, &grantVersion, &credentialVersion, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return p, "", forbidden()
		}
		if err != nil {
			return p, "", err
		}
		until, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil || !until.After(clock) {
			return p, "", forbidden()
		}
		identity = stringify([]any{pool, owner, runner, grant, grantVersion, credentialVersion, expires})
	} else {
		reason, e := s.toolWorkerReasonTx(ctx, tx, Principal{TenantID: tenant}, project, p.WorkerID, clock)
		if e != nil {
			return p, "", e
		}
		if reason.source != "" {
			return p, "", forbidden()
		}
		var hash, creator string
		if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT token_hash,created_by FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=? AND status='active'`)), tenant, project, p.WorkerID).Scan(&hash, &creator); err != nil {
			return p, "", err
		}
		identity = stringify([]any{hash, creator})
	}
	return p, digest(stringify([]any{p.ID, p.Version, p.WorkerID, p.Profile, identity})), nil
}

// Determine the real Pool participant before taking the original ordered
// organization locks; the lookup itself conveys no authorization.
func (s *Service) toolInspectionBrowserAuthTx(ctx context.Context, tx *sql.Tx, actor Actor, project, profile string, purpose commandPurpose) (*authorizedCommand, error) {
	if err := s.authorityWriter(ctx, tx); err != nil {
		return nil, err
	}
	var owner string
	err := tx.QueryRowContext(ctx, s.q(`SELECT p.owner_tenant_id FROM suma_tool_profiles t JOIN suma_execution_pool_runners r ON r.worker_id=t.worker_id JOIN suma_execution_pools p ON p.id=r.pool_id WHERE t.tenant_id=? AND t.project_id=? AND t.id=?`), actor.credential.tenantID, project, profile).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: profile}, purpose, []string{owner})
}

func toolInspectionEffective(v tic.Inspection, clock time.Time) tic.Inspection {
	if v.State == "pending" {
		until, e := time.Parse(time.RFC3339Nano, v.ExpiresAt)
		if e == nil && !clock.Before(until) {
			v.State = "expired"
			v.ResultCode = "request_expired"
		}
	}
	if v.State == "claimed" {
		until, e := time.Parse(time.RFC3339Nano, v.LeaseUntil)
		if e == nil && !clock.Before(until) {
			v.State = "unknown"
			v.ResultCode = "probe_timeout"
		}
	}
	return v
}
func toolInspectionUnfinished(state string) bool {
	return state == "pending" || state == "claimed" || state == "unknown"
}
func (s *Service) toolInspectionCurrentTx(ctx context.Context, tx *sql.Tx, row toolInspectionRow) error {
	p, sha, err := s.toolInspectionSourceTx(ctx, tx, row.Tenant, row.Value.ProjectID, row.Value.ToolProfileID, row.Value.RequestedBy)
	if err != nil {
		return err
	}
	if sha != row.SourceSHA256 || p.WorkerID != row.Value.WorkerID || p.Version != row.Value.ToolProfileVersion || p.Profile.ProfileSHA256 != row.Value.ProfileSHA256 {
		return toolInspectionChanged()
	}
	return nil
}
func (s *Service) CreateToolInspection(ctx context.Context, actor Actor, project, id string, req tic.CreateRequest) (out tic.Inspection, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolInspectionBrowserAuthTx(ctx, tx, actor, project, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	if req.ProjectID != project || !tic.ID(id) || !ingressVersion(req.ExpectedVersion) || !tic.Operation(req.IdempotencyKey) {
		return out, toolInspectionInvalid()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"tool_inspection": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	p, sha, err := s.toolInspectionSourceTx(ctx, tx, auth.target.tenantID, project, id, auth.currentPrincipal.ID)
	if err != nil {
		return out, err
	}
	if p.Version != req.ExpectedVersion {
		return out, toolInspectionChanged()
	}
	if _, _, err = s.requireToolMaintenanceTx(ctx, tx, auth.target.tenantID, project, p.WorkerID, true); err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = s.toolInstallationBusyTx(ctx, tx, auth.target.tenantID, project, p.WorkerID, clock); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','unknown') ORDER BY id`), auth.target.tenantID, project, p.WorkerID)
	if err != nil {
		return out, err
	}
	var ids []string
	for rows.Next() {
		var x string
		if err = rows.Scan(&x); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, x := range ids {
		r, e := s.toolInspectionReadTx(ctx, tx, auth.target.tenantID, project, x)
		if e != nil {
			return out, e
		}
		v := toolInspectionEffective(r.Value, clock)
		if toolInspectionUnfinished(v.State) {
			return out, toolInspectionBusy()
		}
		old := r.Value.Version
		r.Value = v
		r.Value.Version++
		if e = s.toolInspectionSaveTx(ctx, tx, r, old); e != nil {
			return out, e
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_inspections WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&count); err != nil {
		return out, err
	}
	if count >= tic.MaxProjectRequests {
		return out, &diagnosticAPIError{cause: &APIError{409, "tool_inspection_limit", "项目工具重检记录已达上限", nil}, key: "validation.tool_inspection_limit"}
	}
	out = tic.Inspection{ID: newID(), ProjectID: project, WorkerID: p.WorkerID, ToolProfileID: id, ToolProfileVersion: p.Version, ProfileSHA256: p.Profile.ProfileSHA256, CommandID: p.Profile.CommandID, Version: 1, State: "pending", RequestedBy: auth.currentPrincipal.ID, CreatedAt: clock.UTC().Format(time.RFC3339Nano), ExpiresAt: clock.Add(tic.PendingTTLSeconds * time.Second).UTC().Format(time.RFC3339Nano)}
	row := toolInspectionRow{Value: out, Tenant: auth.target.tenantID, SourceSHA256: sha, Profile: p.Profile}
	if err = s.toolInspectionSaveTx(ctx, tx, row, 0); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "tool_inspection.requested", out.ID, p.Name); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) toolInspectionPublicTx(ctx context.Context, tx *sql.Tx, row toolInspectionRow) (tic.Inspection, error) {
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return tic.Inspection{}, err
	}
	v := toolInspectionEffective(row.Value, clock)
	if toolInspectionUnfinished(v.State) {
		if err = s.toolInspectionCurrentTx(ctx, tx, row); err != nil {
			var api *APIError
			if automationAuthorityFailure(err) || errors.As(err, &api) && api.Code == "tool_inspection_source_changed" {
				v.SourceChanged = true
				if v.State == "pending" {
					v.State = "superseded"
					v.ResultCode = "source_changed"
				}
			} else {
				return tic.Inspection{}, err
			}
		}
	}
	return v, nil
}
func (s *Service) GetToolInspection(ctx context.Context, actor Actor, project, id string) (out tic.Inspection, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return out, err
	}
	// Discover only the real coordination participant, then authorize before
	// revealing request existence. The exact route is reread under these locks.
	var owner string
	err = tx.QueryRowContext(ctx, s.q(`SELECT p.owner_tenant_id FROM suma_tool_inspections i JOIN suma_tool_profiles t ON t.id=i.profile_id AND t.tenant_id=i.tenant_id AND t.project_id=i.project_id JOIN suma_execution_pool_runners r ON r.worker_id=t.worker_id JOIN suma_execution_pools p ON p.id=r.pool_id WHERE i.tenant_id=? AND i.project_id=? AND i.id=?`), actor.credential.tenantID, project, id).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	auth, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, []string{owner})
	if err != nil {
		return out, err
	}
	r, err := s.toolInspectionReadTx(ctx, tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	out, err = s.toolInspectionPublicTx(ctx, tx, r)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ListToolInspections(ctx context.Context, actor Actor, project, profile string) (out tic.ToolInspectionList, err error) {
	out.Items = []tic.Inspection{}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolInspectionBrowserAuthTx(ctx, tx, actor, project, profile, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	if _, err = s.toolProfileTx(ctx, tx, auth.currentPrincipal, project, profile); err != nil {
		return out, err
	}
	// The index covers this profile only; no organization task/run scan.
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND profile_id=? ORDER BY created_at DESC,id DESC LIMIT 10`), auth.target.tenantID, project, profile)
	if err != nil {
		return out, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		r, e := s.toolInspectionReadTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		v, e := s.toolInspectionPublicTx(ctx, tx, r)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, v)
	}
	return out, tx.Commit()
}
