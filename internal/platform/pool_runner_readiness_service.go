package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"

	pc "github.com/ai-mada/suma/internal/poolconnectioncontract"
)

type PoolRunnerReadiness = pc.PoolRunnerReadiness
type PoolRunnerReadinessRunner = pc.PoolRunnerReadinessRunner
type PoolRunnerReadinessScope = pc.PoolRunnerReadinessScope

// GetExecutionPoolRunnerReadiness returns scoped, as-of facts. It does not turn
// a registration, historical contact or current report into execution approval.
func (s *Service) GetExecutionPoolRunnerReadiness(ctx context.Context, actor Actor, poolID, runnerID, tenantID, projectID string) (PoolRunnerReadiness, error) {
	return s.readPoolRunnerReadiness(ctx, actor, poolID, runnerID, func() (string, string, error) { return tenantID, projectID, nil })
}
func (s *Service) readPoolRunnerReadiness(parent context.Context, actor Actor, poolID, runnerID string, read func() (string, string, error)) (out PoolRunnerReadiness, err error) {
	for attempt := 0; attempt < 3; attempt++ {
		out, err = s.readPoolRunnerReadinessTx(parent, actor, poolID, runnerID, read)
		if !authoritySetRetry(err) {
			break
		}
	}
	return out, err
}
func (s *Service) readPoolRunnerReadinessTx(parent context.Context, actor Actor, poolID, runnerID string, read func() (string, string, error)) (out PoolRunnerReadiness, err error) {
	if !executionPoolID.MatchString(poolID) || !executionPoolID.MatchString(runnerID) {
		return out, invalid("输入无效")
	}
	ctx, tx, done, e := s.beginAuthorityTx(parent)
	if e != nil {
		return out, e
	}
	defer done()
	defer tx.Rollback()
	target := commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}
	auth, e := s.authorizeTx(ctx, tx, actor, target, purposeOrganizationRead)
	if e != nil {
		return out, e
	}
	if e = s.poolHumanWriteTx(ctx, tx, auth); e != nil {
		return out, e
	}
	// Discover only an owned Pool before parsing its exact public scope. Hold
	// current human/org authority first, then project proof before domain locks.
	provisional, e := s.executionPoolMetadataTx(ctx, tx, poolID)
	if e != nil {
		return out, e
	}
	if provisional.OwnerTenantID != auth.currentPrincipal.TenantID {
		return out, missing()
	}
	tenantID, projectID, e := read()
	if e != nil {
		return out, e
	}
	if !executionPoolID.MatchString(tenantID) || !executionPoolID.MatchString(projectID) {
		return out, invalid("输入无效")
	}
	grant, e := s.poolReadinessGrantTx(ctx, tx, poolID, provisional.OwnerTenantID, tenantID, projectID)
	if e != nil {
		return out, e
	}
	var projectAuth *authorizedCommand
	if tenantID == auth.currentPrincipal.TenantID {
		allowed, e := s.currentPoolProjectAccessTx(ctx, tx, tenantID, projectID, auth.currentPrincipal.ID, false)
		if e != nil {
			return out, e
		}
		if allowed {
			projectAuth, e = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: tenantID, projectID: projectID}, purposeBrowserProjectRead)
			if e != nil {
				return out, e
			}
		}
	}
	pool, e := s.poolAdminTx(ctx, tx, auth, poolID)
	if e != nil {
		return out, e
	}
	currentGrant, e := s.currentPoolGrantTx(ctx, tx, grant.ID)
	if e != nil {
		return out, e
	}
	if currentGrant.PoolID != poolID || currentGrant.OwnerTenantID != pool.OwnerTenantID || currentGrant.TenantID != tenantID || currentGrant.ProjectID != projectID {
		return out, conflict("数据已变化，请刷新后重试")
	}
	clock, e := s.toolInventoryClock(ctx, tx)
	if e != nil {
		return out, e
	}
	registered, credential, e := s.poolReadinessRunnerTx(ctx, tx, poolID, runnerID, clock)
	if e != nil {
		return out, e
	}
	out = PoolRunnerReadiness{PoolID: poolID, RunnerID: runnerID, WorkerID: registered.WorkerID, PoolStatus: pool.Status, CheckedAt: clock.UTC().Format(time.RFC3339Nano), Runner: credential, Scope: PoolRunnerReadinessScope{TenantID: tenantID, ProjectID: projectID, GrantID: currentGrant.ID, GrantVersion: currentGrant.Version, GrantStatus: currentGrant.Status, SharedProjectName: currentGrant.SharedProjectName, RouteState: "restricted", SourceReason: "project_access_required"}}
	// Foreign-tenant ownership is deliberately insufficient even if the same
	// human belongs to that other organization. Never read its project data.
	if projectAuth != nil {
		var state string
		if e = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status FROM suma_projects WHERE tenant_id=? AND id=?`)), tenantID, projectID).Scan(&state); e != nil {
			return PoolRunnerReadiness{}, e
		}
		if state == "active" {
			out.Scope.CanOpenProject = true
			if e = s.poolReadinessProjectTx(ctx, tx, &out, pool, currentGrant, credential, clock); e != nil {
				return PoolRunnerReadiness{}, e
			}
		}
		if e = projectAuth.assertTx(tx, commandTarget{tenantID: tenantID, projectID: projectID}, purposeBrowserProjectRead); e != nil {
			return PoolRunnerReadiness{}, e
		}
	}
	if e = auth.assertTx(tx, target, purposeOrganizationRead); e != nil {
		return PoolRunnerReadiness{}, e
	}
	return out, tx.Commit()
}

// Select only already shared Grant metadata. An approved relationship is
// preferred to history; project route and records are not consulted here.
func (s *Service) poolReadinessGrantTx(ctx context.Context, tx *sql.Tx, pool, owner, tenant, project string) (ExecutionPoolGrant, error) {
	return scanPoolGrant(tx.QueryRowContext(ctx, s.q(`SELECT `+poolGrantColumns+` FROM suma_execution_pool_grants g JOIN suma_execution_pools p ON p.id=g.pool_id WHERE g.pool_id=? AND g.owner_tenant_id=? AND g.tenant_id=? AND g.project_id=? ORDER BY CASE WHEN g.status='approved' THEN 0 ELSE 1 END,g.created_at DESC,g.id DESC LIMIT 1`), pool, owner, tenant, project))
}
func (s *Service) poolReadinessRunnerTx(ctx context.Context, tx *sql.Tx, pool, runner string, clock time.Time) (registered ExecutionPoolRunner, out PoolRunnerReadinessRunner, err error) {
	registered, err = s.poolRunnerTx(ctx, tx, pool, runner)
	if err != nil {
		return registered, out, err
	}
	var status, tokenHash, expiry string
	var version int
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT status,version,token_hash,expires_at FROM suma_execution_pool_credentials WHERE runner_id=?`)), runner).Scan(&status, &version, &tokenHash, &expiry)
	if err != nil {
		return registered, out, err
	}
	until, e := time.Parse(time.RFC3339Nano, expiry)
	if e != nil || !ingressVersion(registered.Version) || !ingressVersion(registered.CredentialVersion) || !ingressVersion(version) || registered.WorkerID != "pool-"+runner || registered.MaxSlots < 1 || registered.MaxSlots > 128 || !contentDigest.MatchString(tokenHash) {
		return registered, out, conflict("数据已变化，请刷新后重试")
	}
	if registered.LastSeenAt != "" {
		if _, e = time.Parse(time.RFC3339Nano, registered.LastSeenAt); e != nil {
			return registered, out, conflict("数据已变化，请刷新后重试")
		}
	}
	out = PoolRunnerReadinessRunner{Status: registered.Status, Version: registered.Version, CredentialVersion: registered.CredentialVersion, CredentialExpiresAt: expiry, HistoricalLastSeenAt: registered.LastSeenAt}
	switch {
	case registered.Status == "revoked" || status == "revoked":
		out.CredentialState = "revoked"
	case registered.Status != "active" || status != "active":
		return registered, out, conflict("数据已变化，请刷新后重试")
	case version != registered.CredentialVersion:
		out.CredentialState = "version_mismatch"
	case until.Unix() <= clock.Unix():
		out.CredentialState = "expired"
	default:
		// Reuse original current machine identity validation without synthesizing
		// a machine Actor or granting this human any delegated project authority.
		if _, e = s.poolMachineIdentityByHashTx(ctx, tx, tokenHash, runner, false); e != nil {
			return registered, out, e
		}
		out.CredentialState = "valid"
	}
	return registered, out, nil
}
func (s *Service) poolReadinessProjectTx(ctx context.Context, tx *sql.Tx, out *PoolRunnerReadiness, pool ExecutionPool, grant ExecutionPoolGrant, credential PoolRunnerReadinessRunner, clock time.Time) error {
	scope := &out.Scope
	scope.RouteState = "unconfigured"
	scope.SourceReason = "route_not_current"
	var currentPool, currentGrant string
	var version int
	e := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT pool_id,grant_id,grant_version FROM dag_project_execution_routes WHERE tenant_id=? AND project_id=?`)), scope.TenantID, scope.ProjectID).Scan(&currentPool, &currentGrant, &version)
	if errors.Is(e, sql.ErrNoRows) {
		return nil
	}
	if e != nil {
		return e
	}
	scope.RouteState = "different"
	if currentPool != pool.ID || currentGrant != grant.ID || version != grant.Version || grant.Status != "approved" {
		return nil
	}
	scope.RouteState = "current"
	scope.SourceReason = "maintenance_unknown"
	if pool.Status != "active" || out.Runner.Status != "active" || credential.CredentialState != "valid" {
		return nil
	}
	discovery, e := s.readToolDiscoveryInventoryTx(ctx, tx, scope.TenantID, scope.ProjectID, out.WorkerID, clock)
	if e != nil {
		return e
	}
	scope.InventoryObservedAt = discovery.ObservedAt
	scope.InventoryExpiresAt = discovery.ExpiresAt
	switch discovery.State {
	case "unreported":
		scope.SourceReason = "inventory_unreported"
		return nil
	case "stale":
		scope.SourceReason = "inventory_stale"
		return nil
	case "fresh":
	default:
		return conflict("数据已变化，请刷新后重试")
	}
	maintenance, identity, e := s.toolMaintenanceStatusTx(ctx, tx, scope.TenantID, scope.ProjectID, out.WorkerID, clock)
	if e != nil {
		return e
	}
	if maintenance.State == "expired" {
		scope.SourceReason = "maintenance_stale"
		return nil
	}
	if maintenance.State != "supported" || identity == "" {
		return nil
	}
	observed, e := time.Parse(time.RFC3339Nano, discovery.ObservedAt)
	if e != nil {
		return e
	}
	maintained, e := time.Parse(time.RFC3339Nano, maintenance.ObservedAt)
	if e != nil {
		return e
	}
	if !observed.Equal(maintained) {
		scope.SourceReason = "observation_mismatch"
		return nil
	}
	count := len(discovery.Items)
	scope.SourceCurrent = true
	scope.SourceReason = "current"
	scope.InventoryItemCount = &count
	return nil
}
