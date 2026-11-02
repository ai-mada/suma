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

// Identity is independent of the old inspection initiator/profile revision.
// Exact current machine credentials, project and Pool route remain mandatory.
func (s *Service) toolMaintenanceIdentityTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string) (string, error) {
	var projectState string
	if err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status FROM suma_projects WHERE tenant_id=? AND id=?`)), tenant, project).Scan(&projectState); err != nil {
		return "", err
	}
	if projectState != "active" {
		return "", forbidden()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(strings.ToLower(worker), "pool-") {
		var pool, owner, runner, grant, tokenHash, expires string
		var grantVersion, credentialVersion int
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT p.id,p.owner_tenant_id,r.id,g.id,g.version,r.credential_version,c.token_hash,c.expires_at FROM dag_project_execution_routes d JOIN suma_execution_pool_grants g ON g.id=d.grant_id AND g.version=d.grant_version AND g.pool_id=d.pool_id AND g.tenant_id=d.tenant_id AND g.project_id=d.project_id JOIN suma_execution_pools p ON p.id=d.pool_id JOIN suma_execution_pool_runners r ON r.pool_id=p.id JOIN suma_execution_pool_credentials c ON c.runner_id=r.id AND c.version=r.credential_version WHERE d.tenant_id=? AND d.project_id=? AND r.worker_id=? AND g.status='approved' AND p.status='active' AND r.status='active' AND c.status='active'`)), tenant, project, worker).Scan(&pool, &owner, &runner, &grant, &grantVersion, &credentialVersion, &tokenHash, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return "", forbidden()
		}
		if err != nil {
			return "", err
		}
		until, e := time.Parse(time.RFC3339Nano, expires)
		if e != nil || !until.After(clock) {
			return "", forbidden()
		}
		return digest(stringify([]any{tenant, project, worker, pool, owner, runner, grant, grantVersion, credentialVersion, tokenHash})), nil
	}
	reason, err := s.toolWorkerReasonTx(ctx, tx, Principal{TenantID: tenant}, project, worker, clock)
	if err != nil {
		return "", err
	}
	if reason.source != "" {
		return "", forbidden()
	}
	var tokenHash, creator string
	if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT token_hash,created_by FROM suma_release_workers WHERE tenant_id=? AND project_id=? AND worker_id=? AND status='active'`)), tenant, project, worker).Scan(&tokenHash, &creator); err != nil {
		return "", err
	}
	return digest(stringify([]any{tenant, project, worker, tokenHash, creator})), nil
}
func (s *Service) recordToolMaintenanceTx(ctx context.Context, tx *sql.Tx, tenant, project, worker, observed string, value *tic.ToolMaintenanceCapability) error {
	if value == nil {
		return nil
	}
	if tic.ValidateMaintenanceCapability(*value) != nil {
		return toolInspectionInvalid()
	}
	identity, err := s.toolMaintenanceIdentityTx(ctx, tx, tenant, project, worker)
	if err != nil {
		return err
	}
	var old string
	err = tx.QueryRowContext(ctx, s.q(`SELECT observed_at FROM suma_tool_maintenance WHERE tenant_id=? AND project_id=? AND worker_id=?`), tenant, project, worker).Scan(&old)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_tool_maintenance(tenant_id,project_id,worker_id,identity_sha256,observed_at,payload)VALUES(?,?,?,?,?,?)`), tenant, project, worker, identity, observed, stringify(value))
	} else if err == nil {
		_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_tool_maintenance SET identity_sha256=?,observed_at=?,payload=? WHERE tenant_id=? AND project_id=? AND worker_id=?`), identity, observed, stringify(value), tenant, project, worker)
	}
	return err
}
func (s *Service) toolMaintenanceStatusTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, clock time.Time) (out tic.ToolMaintenanceStatus, identity string, err error) {
	out.State = "unknown"
	out.CheckedAt = clock.UTC().Format(time.RFC3339Nano)
	var raw, observed, savedIdentity string
	err = tx.QueryRowContext(ctx, s.q(`SELECT identity_sha256,observed_at,payload FROM suma_tool_maintenance WHERE tenant_id=? AND project_id=? AND worker_id=?`), tenant, project, worker).Scan(&savedIdentity, &observed, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, "", nil
	}
	if err != nil {
		return out, "", err
	}
	var capability tic.ToolMaintenanceCapability
	if json.Unmarshal([]byte(raw), &capability) != nil || tic.ValidateMaintenanceCapability(capability) != nil {
		return out, "", toolRecoveryChanged()
	}
	identity, err = s.toolMaintenanceIdentityTx(ctx, tx, tenant, project, worker)
	if err != nil {
		if automationAuthorityFailure(err) {
			return out, "", nil
		}
		return out, "", err
	}
	if identity != savedIdentity {
		return out, identity, nil
	}
	at, e := time.Parse(time.RFC3339Nano, observed)
	if e != nil || at.After(clock) {
		return out, identity, toolRecoveryChanged()
	}
	out.Protocol = capability.Protocol
	out.InstanceID = capability.InstanceID
	out.InstalledDiscovery = capability.InstalledDiscovery
	out.ObservedAt = observed
	out.ExpiresAt = at.Add(60 * time.Second).UTC().Format(time.RFC3339Nano)
	out.State = "supported"
	if capability.Protocol != tic.MaintenanceProtocol {
		out.State = "unsupported"
	} else if !clock.Before(at.Add(60 * time.Second)) {
		out.State = "expired"
	}
	return out, identity, nil
}
func (s *Service) requireToolMaintenanceTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, fresh bool) (tic.ToolMaintenanceStatus, string, error) {
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return tic.ToolMaintenanceStatus{}, "", err
	}
	status, identity, err := s.toolMaintenanceStatusTx(ctx, tx, tenant, project, worker, clock)
	if err != nil {
		return status, "", err
	}
	if status.Protocol != tic.MaintenanceProtocol || status.InstanceID == "" || (fresh && status.State != "supported") {
		return status, "", &diagnosticAPIError{cause: &APIError{409, "tool_maintenance_unavailable", "执行节点尚未声明当前维护协议，请连接支持该协议的连续执行节点", nil}, key: "validation.tool_maintenance_required"}
	}
	return status, identity, nil
}
