package platform

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

type poolMachineIdentity struct {
	runner                          ExecutionPoolRunner
	owner, credentialID, poolStatus string
}

// poolMachineIdentityTx never authenticates a human account and never returns a
// token/hash. The current read is taken only after ordered organization locks.
func (s *Service) poolMachineIdentityTx(ctx context.Context, tx *sql.Tx, token, runnerID string, current bool) (poolMachineIdentity, error) {
	var out poolMachineIdentity
	query := `SELECT r.id,r.pool_id,r.worker_id,r.name,r.version,r.status,r.max_slots,r.credential_version,r.last_seen_at,r.created_at,c.expires_at,p.owner_tenant_id,c.id,p.status,c.status,c.version FROM suma_execution_pool_runners r JOIN suma_execution_pool_credentials c ON c.runner_id=r.id JOIN suma_execution_pools p ON p.id=r.pool_id WHERE r.id=? AND c.token_hash=?`
	if current && s.Dialect != "sqlite" {
		query += ` FOR UPDATE`
	}
	var credentialStatus string
	var credentialVersion int
	r := &out.runner
	err := tx.QueryRowContext(ctx, s.q(query), runnerID, digest(token)).Scan(&r.ID, &r.PoolID, &r.WorkerID, &r.Name, &r.Version, &r.Status, &r.MaxSlots, &r.CredentialVersion, &r.LastSeenAt, &r.CreatedAt, &r.CredentialExpiresAt, &out.owner, &out.credentialID, &out.poolStatus, &credentialStatus, &credentialVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return out, forbidden()
	}
	if err != nil {
		return out, err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	expires, parseErr := time.Parse(time.RFC3339Nano, r.CredentialExpiresAt)
	if token == "" || r.Status != "active" || credentialStatus != "active" || credentialVersion != r.CredentialVersion || parseErr != nil || expires.Unix() <= clock.Unix() || r.WorkerID != "pool-"+r.ID || r.MaxSlots < 1 || r.MaxSlots > 128 {
		return poolMachineIdentity{}, forbidden()
	}
	r.AllowedActions = []string{}
	return out, nil
}

func (s *Service) poolOrderedOrganizationsTx(ctx context.Context, tx *sql.Tx, tenants []string) error {
	set := map[string]bool{}
	for _, tenant := range tenants {
		set[tenant] = true
	}
	ordered := make([]string, 0, len(set))
	for tenant := range set {
		ordered = append(ordered, tenant)
	}
	sort.Strings(ordered)
	for _, tenant := range ordered {
		if err := s.lockOrganization(ctx, tx, tenant); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) poolActiveScopeTx(ctx context.Context, tx *sql.Tx, pool, tenant, project string) (dag.PoolScope, error) {
	out := dag.PoolScope{PoolID: pool, TenantID: tenant, ProjectID: project, Status: "active"}
	err := tx.QueryRowContext(ctx, s.q(`SELECT g.id,g.version,g.project_weight FROM suma_execution_pool_grants g JOIN suma_projects p ON p.id=g.project_id AND p.tenant_id=g.tenant_id WHERE g.pool_id=? AND g.tenant_id=? AND g.project_id=? AND g.status='approved' AND p.status='active' ORDER BY g.id LIMIT 1`), pool, tenant, project).Scan(&out.GrantID, &out.GrantVersion, &out.ProjectWeight)
	if errors.Is(err, sql.ErrNoRows) {
		return out, forbidden()
	}
	return out, err
}

// PoolRunnerAuthorityTx is the scheduler's trusted before-hook. Requested
// scopes describe local configurations only; every scope must already have a
// live bilateral grant. Initial reads discover real lock targets, then all
// authority is reread under globally ordered organization locks. No submitted
// tenant can create a governance row merely by appearing in a request.
func (s *Service) PoolRunnerAuthorityTx(ctx context.Context, tx *sql.Tx, token, runnerID string, requested []dag.PoolCapabilityScope) (ExecutionPoolRunner, dag.PoolClaimAuthority, error) {
	var authority dag.PoolClaimAuthority
	identity, err := s.poolMachineIdentityTx(ctx, tx, token, runnerID, false)
	if err != nil {
		return ExecutionPoolRunner{}, authority, err
	}
	if len(requested) > 256 {
		return ExecutionPoolRunner{}, authority, invalid("最多声明 256 个项目的本地执行能力")
	}
	tenants := []string{identity.owner}
	seen := map[string]bool{}
	for _, local := range requested {
		key := local.TenantID + "/" + local.ProjectID
		if !executionPoolID.MatchString(local.TenantID) || !executionPoolID.MatchString(local.ProjectID) || seen[key] {
			return ExecutionPoolRunner{}, authority, invalid("本地项目范围无效或重复")
		}
		seen[key] = true
		if _, err = s.poolActiveScopeTx(ctx, tx, identity.runner.PoolID, local.TenantID, local.ProjectID); err != nil {
			return ExecutionPoolRunner{}, authority, err
		}
		tenants = append(tenants, local.TenantID)
	}
	if err = s.poolOrderedOrganizationsTx(ctx, tx, tenants); err != nil {
		return ExecutionPoolRunner{}, authority, err
	}
	identity, err = s.poolMachineIdentityTx(ctx, tx, token, runnerID, true)
	if err != nil {
		return ExecutionPoolRunner{}, authority, err
	}
	authority = dag.PoolClaimAuthority{CredentialID: identity.credentialID, CredentialVersion: identity.runner.CredentialVersion, MaxSlots: identity.runner.MaxSlots, PoolActive: identity.poolStatus == "active", Scopes: []dag.PoolScope{}}
	for _, local := range requested {
		scope, err := s.poolActiveScopeTx(ctx, tx, identity.runner.PoolID, local.TenantID, local.ProjectID)
		if err != nil {
			return ExecutionPoolRunner{}, dag.PoolClaimAuthority{}, err
		}
		authority.Scopes = append(authority.Scopes, scope)
	}
	return identity.runner, authority, nil
}

// PoolRunnerAssignmentAuthorityTx admits only evidence for the exact frozen
// dispatch. A withdrawn/revoked project grant can drain an already dispatched
// attempt; it cannot create or recover an executable new dispatch. Machine
// revocation and credential expiry still reject every privileged response.
func (s *Service) PoolRunnerAssignmentAuthorityTx(ctx context.Context, tx *sql.Tx, token string, a dag.Assignment) error {
	identity, err := s.poolMachineIdentityTx(ctx, tx, token, a.PoolRunnerID, false)
	if err != nil {
		return err
	}
	if identity.runner.PoolID != a.PoolID || identity.runner.WorkerID != a.WorkerID || identity.credentialID != a.PoolCredentialID || identity.runner.CredentialVersion != a.PoolCredentialVersion {
		return forbidden()
	}
	if err = dag.ValidatePoolAssignmentTx(ctx, tx, s.Dialect, a); err != nil {
		return err
	}
	var tenant, project, owner, status string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT tenant_id,project_id,owner_tenant_id,status FROM suma_execution_pool_grants WHERE id=? AND pool_id=?`), a.PoolGrantID, a.PoolID).Scan(&tenant, &project, &owner, &status); err != nil {
		return forbidden()
	}
	if tenant != a.TenantID || project != a.ProjectID || owner != identity.owner || status != "approved" && status != "withdrawn" && status != "revoked" {
		return forbidden()
	}
	if err = s.poolOrderedOrganizationsTx(ctx, tx, []string{identity.owner, tenant}); err != nil {
		return err
	}
	identity, err = s.poolMachineIdentityTx(ctx, tx, token, a.PoolRunnerID, true)
	if err != nil {
		return err
	}
	if identity.runner.PoolID != a.PoolID || identity.runner.WorkerID != a.WorkerID || identity.credentialID != a.PoolCredentialID || identity.runner.CredentialVersion != a.PoolCredentialVersion {
		return forbidden()
	}
	return dag.ValidatePoolAssignmentTx(ctx, tx, s.Dialect, a)
}

func (s *Service) GetExecutionPoolRunnerSession(ctx context.Context, token, runnerID string) (ExecutionPoolRunnerSession, error) {
	var out ExecutionPoolRunnerSession
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	identity, err := s.poolMachineIdentityTx(ctx, tx, token, runnerID, false)
	if err != nil {
		return out, err
	}
	out.Runner, out.PoolActive, out.Scopes = identity.runner, identity.poolStatus == "active", []dag.PoolScope{}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT g.tenant_id,g.project_id,g.id,g.version,g.project_weight FROM suma_execution_pool_grants g JOIN suma_projects p ON p.tenant_id=g.tenant_id AND p.id=g.project_id WHERE g.pool_id=? AND g.status='approved' AND p.status='active' ORDER BY g.tenant_id,g.project_id,g.id LIMIT 257`), identity.runner.PoolID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		scope := dag.PoolScope{PoolID: identity.runner.PoolID, Status: "active"}
		if err = rows.Scan(&scope.TenantID, &scope.ProjectID, &scope.GrantID, &scope.GrantVersion, &scope.ProjectWeight); err != nil {
			return out, err
		}
		out.Scopes = append(out.Scopes, scope)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Scopes) > 256 {
		return out, conflict("资源池授权项目超出当前单节点 256 范围限制，请拆分执行节点配置")
	}
	return out, nil
}
