package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func executionFromDeployment(d ReleaseDeployment, p ReleasePlan) rc.ReleaseExecution {
	return rc.ReleaseExecution{Protocol: rc.ReleaseProtocol, OperationID: d.OperationID, PlanID: p.ID, PlanRevision: p.Revision, PlanSHA256: p.SHA256, EnvironmentID: d.EnvironmentID, ProviderID: d.ProviderID, ExpectedGeneration: p.Content.ExpectedGeneration, Generation: d.Generation, ArtifactID: d.ArtifactID, ArtifactSHA256: d.ArtifactSHA256}
}
func (s *Service) ReleaseExecutionForAssignment(ctx context.Context, p Principal, project string, a dag.Assignment) (*rc.ReleaseExecution, error) {
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return nil, err
	}
	if err = s.releaseSchedulerLock(ctx, tx, p.TenantID, project); err != nil {
		return nil, err
	}
	var operation string
	err = tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), p.TenantID, project, a.RunID, a.NodeID).Scan(&operation)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, project, operation)
	if err != nil {
		return nil, err
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, d.PlanID)
	if err != nil {
		return nil, err
	}
	out := executionFromDeployment(d, plan)
	return &out, nil
}
func (s *Service) releaseDeploymentBasisTx(ctx context.Context, tx *sql.Tx, tenant, project string, d ReleaseDeployment) (ReleasePlan, error) {
	p := Principal{ID: d.ExecutedBy, TenantID: tenant}
	plan, err := s.currentReleasePlanTx(ctx, tx, tenant, project, d.PlanID)
	if err != nil {
		return plan, err
	}
	if plan.ExecutionOperationID != d.OperationID || plan.SHA256 != d.PlanSHA256 || plan.Revision != d.PlanRevision || plan.Approval == nil || plan.Approval.Decision != "approved" || plan.Approval.PlanSHA256 != plan.SHA256 || plan.Approval.ActorID != d.ApprovedBy || plan.State != "executing" {
		return plan, releaseConflict("release_approval_required", "此操作的固定审批已失效")
	}
	for _, actor := range []string{d.ApprovedBy, d.ExecutedBy} {
		if err = s.releaseActorAdminTx(ctx, tx, tenant, project, actor); err != nil {
			return plan, err
		}
	}
	if err = s.currentReleasePlanBasisTx(ctx, tx, p, project, plan, d.OperationID); err != nil {
		return plan, err
	}
	return plan, nil
}
func (s *Service) IssueReleaseAuthorization(ctx context.Context, p Principal, token string, r rc.ReleaseAuthorizationRequest) (rc.ReleaseAuthorization, error) {
	var out rc.ReleaseAuthorization
	if !rc.ValidOperationID(r.OperationID) || !r.Assignment.Valid() {
		return out, invalid("部署操作或执行身份无效")
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.releaseWorkerTx(ctx, tx, p, r.ProjectID, token, r.Assignment.WorkerID, "", "", "deployment"); err != nil {
		return out, err
	}
	if _, err = s.currentArtifactAssignmentTx(ctx, tx, p, r.ProjectID, r.Assignment); err != nil {
		return out, err
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, p.TenantID, r.ProjectID, r.OperationID)
	if err != nil {
		return out, err
	}
	if d.RunID != r.Assignment.RunID || d.NodeID != r.Assignment.NodeID {
		return out, forbidden()
	}
	plan, err := s.releaseDeploymentBasisTx(ctx, tx, p.TenantID, r.ProjectID, d)
	if err != nil {
		return out, err
	}
	if d.Assignment != nil && !releaseSame(*d.Assignment, r.Assignment) {
		return out, releaseConflict("stale_assignment", "此操作已冻结在原执行尝试，必须核对原操作")
	}
	requestHash, err := rc.DeploymentRequestDigest(rc.DeploymentRequest{ReleaseExecution: executionFromDeployment(d, plan), Assignment: r.Assignment})
	if err != nil {
		return out, invalid(err.Error())
	}
	var consumed int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='consumed'`), p.TenantID, r.ProjectID, r.OperationID).Scan(&consumed); err != nil {
		return out, err
	}
	if consumed > 0 {
		return out, releaseConflict("authorization_consumed", "提供方已受理原操作，请按 operation_id 查询，不重领副作用")
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_release_authorizations SET state='revoked' WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='issued'`), p.TenantID, r.ProjectID, r.OperationID); err != nil {
		return out, err
	}
	secret := newID() + newID()
	out = rc.ReleaseAuthorization{AuthorizationID: newID(), AuthorizationTicket: secret, ExpiresAt: time.Now().UTC().Add(120 * time.Second).Format(time.RFC3339Nano), OperationID: r.OperationID, RequestSHA256: requestHash}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_authorizations(id,tenant_id,project_id,operation_id,ticket_hash,expires_at,state,assignment,request_sha256,effect_grant_id,grant_data,actor_id,worker_token_hash)VALUES(?,?,?,?,?,?,'issued',?,?,'','',?,?)`), out.AuthorizationID, p.TenantID, r.ProjectID, r.OperationID, digest(secret), out.ExpiresAt, stringify(r.Assignment), requestHash, p.ID, digest(token)); err != nil {
		return rc.ReleaseAuthorization{}, err
	}
	old := d.Version
	d.Version++
	d.Assignment = &r.Assignment
	d.RequestSHA256 = requestHash
	d.UpdatedAt = now()
	if err = s.saveReleaseJSON(ctx, tx, "deployments", p.TenantID, r.ProjectID, d.OperationID, old, d); err != nil {
		return rc.ReleaseAuthorization{}, err
	}
	if err = tx.Commit(); err != nil {
		return rc.ReleaseAuthorization{}, err
	}
	return out, nil
}

type releaseAuthorizationRow struct {
	ID, OperationID, ExpiresAt, State, RequestSHA256, EffectGrantID, GrantData, ActorID, WorkerTokenHash string
	Assignment                                                                                           rc.AssignmentRef
}

func (s *Service) releaseAuthorizationTx(ctx context.Context, tx *sql.Tx, cfg ReleaseProviderConfig, where string, arg any) (releaseAuthorizationRow, error) {
	var a releaseAuthorizationRow
	var raw string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id,operation_id,expires_at,state,assignment,request_sha256,effect_grant_id,grant_data,actor_id,worker_token_hash FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND `+where), cfg.TenantID, cfg.ProjectID, arg).Scan(&a.ID, &a.OperationID, &a.ExpiresAt, &a.State, &raw, &a.RequestSHA256, &a.EffectGrantID, &a.GrantData, &a.ActorID, &a.WorkerTokenHash)
	if err != nil {
		return a, forbidden()
	}
	if err = json.Unmarshal([]byte(raw), &a.Assignment); err != nil {
		return a, err
	}
	return a, nil
}
func (s *Service) releaseAuthorizationCurrentTx(ctx context.Context, tx *sql.Tx, cfg ReleaseProviderConfig, a releaseAuthorizationRow) (ReleaseDeployment, ReleasePlan, error) {
	var d ReleaseDeployment
	var plan ReleasePlan
	p := Principal{ID: a.ActorID, TenantID: cfg.TenantID}
	if err := s.authorizeProjectTx(ctx, tx, p, cfg.ProjectID, true, false); err != nil {
		return d, plan, err
	}
	var worker, status, expiry, user string
	var deploy bool
	err := tx.QueryRowContext(ctx, s.q(`SELECT w.worker_id,w.status,w.deployment,x.user_id,x.expires_at FROM suma_release_workers w JOIN suma_sessions x ON x.token_hash=w.token_hash JOIN suma_token_scopes t ON t.token_hash=w.token_hash WHERE w.token_hash=? AND w.tenant_id=? AND w.project_id=? AND t.kind='runner' AND t.project_id=w.project_id`), a.WorkerTokenHash, cfg.TenantID, cfg.ProjectID).Scan(&worker, &status, &deploy, &user, &expiry)
	expires, parseErr := time.Parse(time.RFC3339Nano, expiry)
	if err != nil || parseErr != nil || !time.Now().Before(expires) || worker != a.Assignment.WorkerID || user != a.ActorID || status != "active" || !deploy {
		return d, plan, forbidden()
	}
	if _, err = s.currentArtifactAssignmentTx(ctx, tx, p, cfg.ProjectID, a.Assignment); err != nil {
		return d, plan, err
	}
	d, err = s.currentReleaseDeploymentTx(ctx, tx, cfg.TenantID, cfg.ProjectID, a.OperationID)
	if err != nil {
		return d, plan, err
	}
	if d.ProviderID != cfg.ID || d.Assignment == nil || !releaseSame(*d.Assignment, a.Assignment) || d.RequestSHA256 != a.RequestSHA256 {
		return d, plan, forbidden()
	}
	plan, err = s.releaseDeploymentBasisTx(ctx, tx, cfg.TenantID, cfg.ProjectID, d)
	return d, plan, err
}
func (s *Service) ConsumeReleaseAuthorization(ctx context.Context, token string, r rc.ConsumeDeploymentAuthorization) (rc.DeploymentAuthorizationGrant, error) {
	var out rc.DeploymentAuthorizationGrant
	cfg, err := s.authenticateReleaseProvider(token, r.ProviderID)
	if err != nil {
		return out, err
	}
	if !releaseID(r.ProviderID) || !rc.ValidOperationID(r.OperationID) || !rc.ValidDigest(r.RequestSHA256) || r.AuthorizationTicket == "" || r.CallerWorkerID != r.Assignment.WorkerID || !r.Assignment.Valid() {
		return out, forbidden()
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, cfg.TenantID); err != nil {
		return out, err
	}
	a, err := s.releaseAuthorizationTx(ctx, tx, cfg, "ticket_hash=?", digest(r.AuthorizationTicket))
	if err != nil {
		return out, err
	}
	if a.OperationID != r.OperationID || a.RequestSHA256 != r.RequestSHA256 || !releaseSame(a.Assignment, r.Assignment) || a.State != "issued" && a.State != "consumed" {
		return out, forbidden()
	}
	expiry, err := time.Parse(time.RFC3339Nano, a.ExpiresAt)
	if err != nil || !time.Now().Before(expiry) {
		return out, releaseError(403, "authorization_expired", "发布票据已过期")
	}
	_, plan, err := s.releaseAuthorizationCurrentTx(ctx, tx, cfg, a)
	if err != nil {
		return out, err
	}
	if a.State == "consumed" {
		if err = json.Unmarshal([]byte(a.GrantData), &out); err != nil {
			return out, err
		}
		return out, nil
	}
	out = rc.DeploymentAuthorizationGrant{AuthorizationID: a.ID, OperationID: a.OperationID, ProviderID: cfg.ID, Assignment: a.Assignment, Plan: plan.Content, RequestSHA256: a.RequestSHA256, State: "authorized"}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_release_authorizations SET state='consumed',grant_data=? WHERE id=? AND state='issued'`), stringify(out), a.ID); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) BeginReleaseEffect(ctx context.Context, token string, r rc.BeginDeploymentEffect) (rc.DeploymentEffectGrant, error) {
	var out rc.DeploymentEffectGrant
	cfg, err := s.authenticateReleaseProvider(token, "")
	if err != nil {
		return out, err
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, cfg.TenantID); err != nil {
		return out, err
	}
	a, err := s.releaseAuthorizationTx(ctx, tx, cfg, "id=?", r.AuthorizationID)
	if err != nil {
		return out, err
	}
	if a.State != "consumed" || a.OperationID != r.OperationID || a.RequestSHA256 != r.RequestSHA256 || !releaseSame(a.Assignment, r.Assignment) {
		return out, forbidden()
	}
	d, _, err := s.releaseAuthorizationCurrentTx(ctx, tx, cfg, a)
	if err != nil {
		return out, err
	}
	if d.Generation != r.Generation {
		return out, releaseConflict("environment_generation_conflict", "副作用代数不匹配")
	}
	id := a.EffectGrantID
	if id == "" {
		id = newID()
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_release_authorizations SET effect_grant_id=? WHERE id=? AND effect_grant_id=''`), id, a.ID); err != nil {
			return out, err
		}
	}
	out = rc.DeploymentEffectGrant{EffectGrantID: id, OperationID: r.OperationID, Generation: r.Generation, State: "granted"}
	return out, tx.Commit()
}

// A lost consume ACK is recovered without persisting or reissuing a ticket.
// This reads a historical grant only; begin-effect still enforces current basis.
func (s *Service) InspectReleaseAuthorization(ctx context.Context, token string, r rc.InspectDeploymentAuthorization) (rc.DeploymentAuthorizationGrant, error) {
	var out rc.DeploymentAuthorizationGrant
	cfg, err := s.authenticateReleaseProvider(token, "")
	if err != nil {
		return out, err
	}
	if !r.Assignment.Valid() || !rc.ValidOperationID(r.OperationID) || !rc.ValidDigest(r.RequestSHA256) {
		return out, invalid("授权查询绑定无效")
	}
	var raw string
	err = s.DB.QueryRowContext(ctx, s.q(`SELECT grant_data FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND assignment=? AND request_sha256=? AND state='consumed'`), cfg.TenantID, cfg.ProjectID, r.OperationID, stringify(r.Assignment), r.RequestSHA256).Scan(&raw)
	if err == sql.ErrNoRows {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if err = json.Unmarshal([]byte(raw), &out); err != nil {
		return out, err
	}
	if out.ProviderID != cfg.ID {
		return rc.DeploymentAuthorizationGrant{}, forbidden()
	}
	return out, nil
}

// ProviderArtifactAccess authenticates and pins a provider to one consumed
// operation's exact sealed artifact. The returned principal is the live executor.
func (s *Service) ProviderArtifactAccess(ctx context.Context, token, operationID, artifactID string) (Principal, string, error) {
	cfg, err := s.authenticateReleaseProvider(token, "")
	if err != nil {
		return Principal{}, "", err
	}
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return Principal{}, "", err
	}
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, cfg.TenantID); err != nil {
		return Principal{}, "", err
	}
	if err = s.releaseSchedulerLock(ctx, tx, cfg.TenantID, cfg.ProjectID); err != nil {
		return Principal{}, "", err
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, cfg.TenantID, cfg.ProjectID, operationID)
	if err != nil {
		return Principal{}, "", err
	}
	if d.ProviderID != cfg.ID || d.ArtifactID != artifactID {
		return Principal{}, "", forbidden()
	}
	if _, err = s.releaseDeploymentBasisTx(ctx, tx, cfg.TenantID, cfg.ProjectID, d); err != nil {
		return Principal{}, "", err
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_release_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='consumed'`), cfg.TenantID, cfg.ProjectID, operationID).Scan(&count); err != nil {
		return Principal{}, "", err
	}
	if count == 0 {
		return Principal{}, "", forbidden()
	}
	return Principal{ID: d.ExecutedBy, TenantID: cfg.TenantID}, cfg.ProjectID, nil
}
