package platform

import (
	"context"
	"database/sql"
	"errors"
	"sort"
	"time"

	"github.com/ai-mada/suma/internal/dag"
)

// poolMachineProof is private and transaction-bound. Only the machine protocol
// can construct it after current credential and bilateral scope verification.
// It carries no human Principal and cannot authorize a human command.
type poolMachineProof struct {
	tx         *sql.Tx
	owner      string
	runner     ExecutionPoolRunner
	scopes     []dag.PoolScope
	assignment *dag.Assignment
	deadline   time.Time
}

func (p *poolMachineProof) assertScope(tx *sql.Tx, scope dag.PoolScope) error {
	if p == nil || p.tx != tx || tx == nil || p.runner.ID == "" || p.owner == "" {
		return forbidden()
	}
	if !time.Now().Before(p.deadline) {
		tx.Rollback()
		return context.DeadlineExceeded
	}
	for _, current := range p.scopes {
		if current.PoolID == scope.PoolID && current.TenantID == scope.TenantID && current.ProjectID == scope.ProjectID && current.GrantID == scope.GrantID && current.GrantVersion == scope.GrantVersion {
			return nil
		}
	}
	return forbidden()
}
func (p *poolMachineProof) assertAssignment(tx *sql.Tx, a dag.Assignment) error {
	if p == nil || p.tx != tx || p.assignment == nil || !releaseSame(*p.assignment, a) {
		return forbidden()
	}
	return p.assertScope(tx, dag.PoolScope{PoolID: a.PoolID, TenantID: a.TenantID, ProjectID: a.ProjectID, GrantID: a.PoolGrantID, GrantVersion: a.PoolGrantVersion})
}

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

func (s *Service) currentPoolActiveScopeTx(ctx context.Context, tx *sql.Tx, pool, tenant, project string) (dag.PoolScope, error) {
	out := dag.PoolScope{PoolID: pool, TenantID: tenant, ProjectID: project, Status: "active"}
	err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT g.id,g.version,g.project_weight FROM suma_execution_pool_grants g JOIN suma_projects p ON p.id=g.project_id AND p.tenant_id=g.tenant_id WHERE g.pool_id=? AND g.tenant_id=? AND g.project_id=? AND g.status='approved' AND p.status='active' ORDER BY g.id LIMIT 1`)), pool, tenant, project).Scan(&out.GrantID, &out.GrantVersion, &out.ProjectWeight)
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
	proof, authority, err := s.authorizePoolRunnerTx(ctx, tx, token, runnerID, requested)
	if err != nil {
		return ExecutionPoolRunner{}, authority, err
	}
	return proof.runner, authority, nil
}
func (s *Service) authorizePoolRunnerTx(ctx context.Context, tx *sql.Tx, token, runnerID string, requested []dag.PoolCapabilityScope) (*poolMachineProof, dag.PoolClaimAuthority, error) {
	deadline := s.txAuthorityBudget(ctx, tx).deadline
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := s.authorityWriter(ctx, tx); err != nil {
		return nil, dag.PoolClaimAuthority{}, err
	}

	var authority dag.PoolClaimAuthority
	identity, err := s.poolMachineIdentityTx(ctx, tx, token, runnerID, false)
	if err != nil {
		return nil, authority, err
	}
	if len(requested) > 256 {
		return nil, authority, invalid("最多声明 256 个项目的本地执行能力")
	}
	tenants := []string{identity.owner}
	seen := map[string]bool{}
	for _, local := range requested {
		key := local.TenantID + "/" + local.ProjectID
		if !executionPoolID.MatchString(local.TenantID) || !executionPoolID.MatchString(local.ProjectID) || seen[key] {
			return nil, authority, invalid("本地项目范围无效或重复")
		}
		seen[key] = true
		if _, err = s.poolActiveScopeTx(ctx, tx, identity.runner.PoolID, local.TenantID, local.ProjectID); err != nil {
			return nil, authority, err
		}
		tenants = append(tenants, local.TenantID)
	}
	if err = s.poolOrderedOrganizationsTx(ctx, tx, tenants); err != nil {
		return nil, authority, err
	}
	discoveredOwner, discoveredPool := identity.owner, identity.runner.PoolID
	identity, err = s.poolMachineIdentityTx(ctx, tx, token, runnerID, true)
	if err != nil {
		return nil, authority, err
	}
	if identity.owner != discoveredOwner || identity.runner.PoolID != discoveredPool {
		return nil, authority, conflict("资源池归属已变化，请重试")
	}
	authority = dag.PoolClaimAuthority{CredentialID: identity.credentialID, CredentialVersion: identity.runner.CredentialVersion, MaxSlots: identity.runner.MaxSlots, PoolActive: identity.poolStatus == "active", Scopes: []dag.PoolScope{}}
	for _, local := range requested {
		scope, err := s.currentPoolActiveScopeTx(ctx, tx, identity.runner.PoolID, local.TenantID, local.ProjectID)
		if err != nil {
			return nil, dag.PoolClaimAuthority{}, err
		}
		authority.Scopes = append(authority.Scopes, scope)
	}
	return &poolMachineProof{tx: tx, owner: identity.owner, runner: identity.runner, scopes: authority.Scopes, deadline: deadline}, authority, nil
}

// PoolRunnerAssignmentAuthorityTx admits only evidence for the exact frozen
// dispatch. A withdrawn/revoked project grant can drain an already dispatched
// attempt; it cannot create or recover an executable new dispatch. Machine
// revocation and credential expiry still reject every privileged response.
func (s *Service) PoolRunnerAssignmentAuthorityTx(ctx context.Context, tx *sql.Tx, token string, a dag.Assignment) (*poolMachineProof, error) {
	deadline := s.txAuthorityBudget(ctx, tx).deadline
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	if err := s.authorityWriter(ctx, tx); err != nil {
		return nil, err
	}

	identity, err := s.poolMachineIdentityTx(ctx, tx, token, a.PoolRunnerID, false)
	if err != nil {
		return nil, err
	}
	if identity.runner.PoolID != a.PoolID || identity.runner.WorkerID != a.WorkerID || identity.credentialID != a.PoolCredentialID || identity.runner.CredentialVersion != a.PoolCredentialVersion {
		return nil, forbidden()
	}
	if err = dag.ValidatePoolAssignmentTx(ctx, tx, s.Dialect, a); err != nil {
		return nil, err
	}
	var tenant, project, owner, status string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT tenant_id,project_id,owner_tenant_id,status FROM suma_execution_pool_grants WHERE id=? AND pool_id=?`), a.PoolGrantID, a.PoolID).Scan(&tenant, &project, &owner, &status); err != nil {
		return nil, forbidden()
	}
	if tenant != a.TenantID || project != a.ProjectID || owner != identity.owner || status != "approved" && status != "withdrawn" && status != "revoked" {
		return nil, forbidden()
	}
	if err = s.poolOrderedOrganizationsTx(ctx, tx, []string{identity.owner, tenant}); err != nil {
		return nil, err
	}
	identity, err = s.poolMachineIdentityTx(ctx, tx, token, a.PoolRunnerID, true)
	if err != nil {
		return nil, err
	}
	if identity.runner.PoolID != a.PoolID || identity.runner.WorkerID != a.WorkerID || identity.credentialID != a.PoolCredentialID || identity.runner.CredentialVersion != a.PoolCredentialVersion {
		return nil, forbidden()
	}
	if identity.owner != owner {
		return nil, conflict("资源池归属已变化，请重试")
	}
	if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT tenant_id,project_id,owner_tenant_id,status FROM suma_execution_pool_grants WHERE id=? AND pool_id=?`)), a.PoolGrantID, a.PoolID).Scan(&tenant, &project, &owner, &status); err != nil {
		return nil, forbidden()
	}
	if tenant != a.TenantID || project != a.ProjectID || owner != identity.owner || status != "approved" && status != "withdrawn" && status != "revoked" {
		return nil, forbidden()
	}
	if err = dag.ValidatePoolAssignmentTx(ctx, tx, s.Dialect, a); err != nil {
		return nil, err
	}
	captured := a
	return &poolMachineProof{tx: tx, owner: identity.owner, runner: identity.runner, scopes: []dag.PoolScope{{PoolID: a.PoolID, TenantID: a.TenantID, ProjectID: a.ProjectID, GrantID: a.PoolGrantID, GrantVersion: a.PoolGrantVersion}}, assignment: &captured, deadline: deadline}, nil
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
