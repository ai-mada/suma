package platform

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// The additive authority ledger preserves old session rows and scopes as audit
// facts. Missing issuance evidence never becomes a new approval during upgrade.
func (s *Service) MigrateCredentialAuthority(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suma_authority_sqlite_guard(id INTEGER PRIMARY KEY,sequence BIGINT NOT NULL)`,
		`INSERT INTO suma_authority_sqlite_guard(id,sequence) SELECT 1,0 WHERE NOT EXISTS(SELECT 1 FROM suma_authority_sqlite_guard WHERE id=1)`,
		`CREATE TABLE IF NOT EXISTS suma_identity_authority_locks(user_id VARCHAR(64) PRIMARY KEY,sequence BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_authority_credential_locks(lock_key VARCHAR(160) PRIMARY KEY,sequence BIGINT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_auth_sources(id VARCHAR(64) PRIMARY KEY,user_id VARCHAR(64) NOT NULL,source_kind VARCHAR(40) NOT NULL,generation BIGINT NOT NULL,revoked INTEGER NOT NULL,authenticated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_session_families(id VARCHAR(64) PRIMARY KEY,user_id VARCHAR(64) NOT NULL,source_id VARCHAR(64) NOT NULL,source_generation BIGINT NOT NULL,generation BIGINT NOT NULL,authenticated_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,revoked INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_credential_grants(id VARCHAR(64) PRIMARY KEY,user_id VARCHAR(64) NOT NULL,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,scope_kind VARCHAR(30) NOT NULL,credential_kind VARCHAR(40) NOT NULL,approved_by VARCHAR(64) NOT NULL,approved_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,policy_revision INTEGER NOT NULL,source_id VARCHAR(64) NOT NULL,source_generation BIGINT NOT NULL,revoked INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_credential_slots(id VARCHAR(64) PRIMARY KEY,user_id VARCHAR(64) NOT NULL,tenant_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_hash VARCHAR(64) NOT NULL,credential_kind VARCHAR(40) NOT NULL,project_id VARCHAR(64) NOT NULL,source_id VARCHAR(64) NOT NULL,grant_id VARCHAR(64) NOT NULL,family_id VARCHAR(64) NOT NULL,expires_at VARCHAR(40) NOT NULL,current_digest VARCHAR(64) NOT NULL,revoked INTEGER NOT NULL,UNIQUE(user_id,tenant_id,operation_id))`,
		`CREATE TABLE IF NOT EXISTS suma_credential_issuance(token_hash VARCHAR(64) PRIMARY KEY,slot_id VARCHAR(64) NOT NULL,grant_id VARCHAR(64) NOT NULL,family_id VARCHAR(64) NOT NULL,issued_at VARCHAR(40) NOT NULL,operation_id VARCHAR(128) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_credential_authority(token_hash VARCHAR(64) PRIMARY KEY,credential_kind VARCHAR(40) NOT NULL,migration_status VARCHAR(40) NOT NULL,source_id VARCHAR(64) NOT NULL,source_generation BIGINT NOT NULL,family_id VARCHAR(64) NOT NULL,family_generation BIGINT NOT NULL,grant_id VARCHAR(64) NOT NULL,slot_id VARCHAR(64) NOT NULL,revoked INTEGER NOT NULL)`,
		`INSERT INTO suma_credential_authority(token_hash,credential_kind,migration_status,source_id,source_generation,family_id,family_generation,grant_id,slot_id,revoked) SELECT t.token_hash,'legacy-unclassified','reissue-required','',0,'',0,'','',0 FROM suma_sessions t WHERE NOT EXISTS(SELECT 1 FROM suma_credential_authority a WHERE a.token_hash=t.token_hash)`,
	}
	for _, query := range statements {
		if _, err := s.DB.ExecContext(ctx, s.ddl(query)); err != nil {
			return err
		}
	}
	// Structural checks deliberately exclude expiry/revocation: complete expired
	// or withdrawn credentials keep their true kind and immutable issuance facts.
	_, err := s.DB.ExecContext(ctx, `UPDATE suma_credential_authority SET credential_kind='legacy-unclassified',migration_status='reissue-required' WHERE migration_status='current' AND NOT EXISTS(
 SELECT 1 FROM suma_sessions t
 JOIN suma_auth_sources src ON src.id=suma_credential_authority.source_id AND src.user_id=t.user_id AND src.generation>=suma_credential_authority.source_generation AND src.source_kind IN ('setup','password','invitation') AND src.authenticated_at<>''
 JOIN suma_credential_issuance i ON i.token_hash=t.token_hash AND i.slot_id=suma_credential_authority.slot_id AND i.family_id=suma_credential_authority.family_id AND i.grant_id=suma_credential_authority.grant_id AND i.issued_at<>'' AND i.operation_id<>''
 JOIN suma_credential_slots sl ON sl.id=i.slot_id AND sl.user_id=t.user_id AND sl.credential_kind=suma_credential_authority.credential_kind AND sl.source_id=src.id AND sl.family_id=i.family_id AND sl.grant_id=i.grant_id AND sl.expires_at=t.expires_at
 WHERE t.token_hash=suma_credential_authority.token_hash AND suma_credential_authority.source_generation>0 AND (
 (suma_credential_authority.credential_kind='browser-session' AND i.grant_id='' AND suma_credential_authority.family_generation>0
 AND NOT EXISTS(SELECT 1 FROM suma_token_scopes sc WHERE sc.token_hash=t.token_hash)
 AND EXISTS(SELECT 1 FROM suma_session_families f WHERE f.id=i.family_id AND f.user_id=t.user_id AND f.source_id=src.id AND f.source_generation=suma_credential_authority.source_generation AND f.generation>=suma_credential_authority.family_generation AND f.expires_at=t.expires_at AND f.authenticated_at<>''))
 OR (suma_credential_authority.credential_kind IN ('user-api','scoped-runner','scoped-agent') AND i.family_id='' AND suma_credential_authority.family_generation=0
 AND EXISTS(SELECT 1 FROM suma_credential_grants g JOIN suma_token_scopes sc ON sc.token_hash=t.token_hash AND sc.project_id=g.project_id AND sc.kind=g.scope_kind
 WHERE g.id=i.grant_id AND g.user_id=t.user_id AND g.tenant_id=t.tenant_id AND g.project_id=sl.project_id AND g.credential_kind=suma_credential_authority.credential_kind AND g.source_id=src.id AND g.source_generation=suma_credential_authority.source_generation AND g.approved_by=g.user_id AND g.approved_at<>'' AND g.policy_revision>0 AND g.expires_at=t.expires_at
 AND ((g.scope_kind='api' AND g.credential_kind='user-api') OR (g.scope_kind='runner' AND g.credential_kind='scoped-runner') OR (g.scope_kind='agent' AND g.credential_kind='scoped-agent'))))))`)
	return err
}

func (s *Service) persistCredential(ctx context.Context, tx *sql.Tx, p Principal, kind actorKind, ref credentialRef, label, expiry, issuedAt, operation string) (string, error) {
	token := newID() + newID()
	ref.digest = digest(token)
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO suma_sessions(token_hash,user_id,tenant_id,expires_at,label) VALUES(?,?,?,?,?)`, []any{ref.digest, p.ID, p.TenantID, expiry, label}},
		{`INSERT INTO suma_credential_authority(token_hash,credential_kind,migration_status,source_id,source_generation,family_id,family_generation,grant_id,slot_id,revoked) VALUES(?,?,'current',?,?,?,?,?,?,0)`, []any{ref.digest, kindName(kind), ref.sourceID, ref.sourceRevision, ref.familyID, ref.familyRevision, ref.grantID, ref.slotID}},
		{`INSERT INTO suma_credential_issuance(token_hash,slot_id,grant_id,family_id,issued_at,operation_id) VALUES(?,?,?,?,?,?)`, []any{ref.digest, ref.slotID, ref.grantID, ref.familyID, issuedAt, operation}},
		{`UPDATE suma_credential_slots SET current_digest=? WHERE id=? AND revoked=0`, []any{ref.digest, ref.slotID}},
	}
	for _, q := range queries {
		if _, err := tx.ExecContext(ctx, s.q(q.query), q.args...); err != nil {
			return "", err
		}
	}
	return token, nil
}

func (s *Service) newBrowserSessionTx(ctx context.Context, tx *sql.Tx, p Principal, sourceKind string) (string, error) {
	// Only Setup/password Login/one-time invitation call this private issuer,
	// after taking the complete organization/account coordination set. New
	// source/family/slot IDs are invisible to any other transaction until commit.
	at, err := s.authorityNow(ctx, tx)
	if err != nil {
		return "", err
	}
	expiry := at.Add(7 * 24 * time.Hour).Format(time.RFC3339Nano)
	ref := credentialRef{subjectID: p.ID, tenantID: p.TenantID, sourceID: newID(), sourceRevision: 1, familyID: newID(), familyRevision: 1, slotID: newID()}
	var present string
	if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT role FROM suma_org_members WHERE tenant_id=? AND user_id=?`)), p.TenantID, p.ID).Scan(&present); err != nil {
		return "", forbidden()
	}
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO suma_auth_sources(id,user_id,source_kind,generation,revoked,authenticated_at) VALUES(?,?,?,1,0,?)`, []any{ref.sourceID, p.ID, sourceKind, at.Format(time.RFC3339Nano)}},
		{`INSERT INTO suma_session_families(id,user_id,source_id,source_generation,generation,authenticated_at,expires_at,revoked) VALUES(?,?,?,1,1,?,?,0)`, []any{ref.familyID, p.ID, ref.sourceID, at.Format(time.RFC3339Nano), expiry}},
		{`INSERT INTO suma_credential_slots(id,user_id,tenant_id,operation_id,request_hash,credential_kind,project_id,source_id,grant_id,family_id,expires_at,current_digest,revoked) VALUES(?,?,?,?,?,'browser-session','',?,'',?,?,?,0)`, []any{ref.slotID, p.ID, p.TenantID, ref.slotID, digest(ref.slotID), ref.sourceID, ref.familyID, expiry, ""}},
	}
	for _, q := range queries {
		if _, err = tx.ExecContext(ctx, s.q(q.query), q.args...); err != nil {
			return "", err
		}
	}
	return s.persistCredential(ctx, tx, p, actorBrowser, ref, "browser", expiry, at.Format(time.RFC3339Nano), ref.slotID)
}

type ProjectCredentialIssueRequest struct {
	ProjectID      string `json:"project_id"`
	Scope          string `json:"scope,omitempty"`
	Label          string `json:"label,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}
type ProjectCredentialIssueResult struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
	Label     string `json:"label"`
	Scope     string `json:"scope"`
	ProjectID string `json:"project_id"`
}

// IssueProjectCredential explicitly approves one independent 24-hour Grant.
// A browser's remaining TTL never silently shortens that approved policy.
func (s *Service) IssueProjectCredential(ctx context.Context, actor Actor, req ProjectCredentialIssueRequest) (ProjectCredentialIssueResult, error) {
	var out ProjectCredentialIssueResult
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		out, err = s.issueProjectCredential(ctx, actor, req)
		if !authoritySetRetry(err) {
			return out, err
		}
	}
	return out, err
}

func (s *Service) issueProjectCredential(ctx context.Context, actor Actor, req ProjectCredentialIssueRequest) (ProjectCredentialIssueResult, error) {
	var out ProjectCredentialIssueResult
	kind := scopeKind(req.Scope)
	if kind == actorInvalid || req.ProjectID == "" {
		return out, invalid("无效凭证范围")
	}
	label := strings.TrimSpace(req.Label)
	if label == "" {
		label = "Runner"
	}
	if len(label) > 200 {
		return out, invalid("凭证名称过长")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	a, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: req.ProjectID}, purposeIssueCredential)
	if err != nil {
		return out, err
	}
	if !orgAdmin(a.currentPrincipal) {
		return out, forbidden()
	}
	body := map[string]any{"action": "credential.issue", "project_id": req.ProjectID, "scope": req.Scope, "label": label}
	hash, replay, err := s.commandReceipt(ctx, a, req.IdempotencyKey, body)
	if err != nil {
		return out, err
	}
	ref := credentialRef{subjectID: a.currentPrincipal.ID, tenantID: a.currentPrincipal.TenantID, projectID: req.ProjectID, sourceID: actor.credential.sourceID, sourceRevision: actor.credential.sourceRevision, grantID: newID(), slotID: newID()}
	expiry := a.checkedAt.Add(24 * time.Hour).Format(time.RFC3339Nano)
	if replay {
		var stored, slotKind, project, source string
		var revoked int
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT id,request_hash,credential_kind,project_id,source_id,grant_id,expires_at,revoked FROM suma_credential_slots WHERE user_id=? AND tenant_id=? AND operation_id=?`)), ref.subjectID, ref.tenantID, req.IdempotencyKey).Scan(&ref.slotID, &stored, &slotKind, &project, &source, &ref.grantID, &expiry, &revoked)
		if err != nil {
			return out, err
		}
		if stored != digest(a.currentPrincipal.ID+stringify(body)) || slotKind != kindName(kind) || project != req.ProjectID || source != ref.sourceID || revoked != 0 {
			return out, forbidden()
		}
		expires, parseErr := time.Parse(time.RFC3339Nano, expiry)
		if parseErr != nil || expires.Unix() <= a.checkedAt.Unix() {
			return out, unauthenticated()
		}
		var grantRevoked int
		if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT revoked FROM suma_credential_grants WHERE id=?`)), ref.grantID).Scan(&grantRevoked); err != nil {
			return out, err
		}
		if grantRevoked != 0 {
			return out, forbidden()
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_authority SET revoked=1 WHERE slot_id=? AND revoked=0`), ref.slotID); err != nil {
			return out, err
		}
	} else {
		if err = s.insertIndependentGrantTx(ctx, tx, a, kind, ref, req.ProjectID, req.Scope, req.IdempotencyKey, hash, expiry); err != nil {
			return out, err
		}
	}
	out, err = s.issueScopedSecretTx(ctx, tx, a, kind, ref, label, expiry, req.Scope, req.ProjectID, req.IdempotencyKey)
	if err != nil {
		return out, err
	}
	if !replay {
		if err = s.saveCommandReceipt(ctx, a, req.IdempotencyKey, hash, map[string]any{"project_id": req.ProjectID, "scope": req.Scope, "label": label, "expires_at": expiry}); err != nil {
			return ProjectCredentialIssueResult{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return ProjectCredentialIssueResult{}, err
	}
	return out, nil
}

func (s *Service) insertIndependentGrantTx(ctx context.Context, tx *sql.Tx, a *authorizedCommand, kind actorKind, ref credentialRef, project, scope, operation, hash, expiry string) error {
	if a == nil {
		return forbidden()
	}
	if err := a.assertTx(tx, a.target, a.purpose); err != nil {
		return err
	}
	switch a.purpose {
	case purposeIssueCredential, purposeProjectWrite, purposeProjectApprove, purposeBrowserProjectWrite, purposeBrowserProjectApprove:
	default:
		return forbidden()
	}
	if a.actor.kind != actorBrowser || a.target.projectID != project || scopeKind(scope) != kind || kind == actorInvalid || ref.sourceID != a.actor.credential.sourceID || ref.sourceRevision != a.actor.credential.sourceRevision || ref.subjectID != a.currentPrincipal.ID || ref.tenantID != a.currentPrincipal.TenantID || ref.projectID != project || ref.familyID != "" || ref.grantID == "" || ref.slotID == "" {
		return forbidden()
	}
	if _, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_credential_grants(id,user_id,tenant_id,project_id,scope_kind,credential_kind,approved_by,approved_at,expires_at,policy_revision,source_id,source_generation,revoked) VALUES(?,?,?,?,?,?,?,?,?,1,?,?,0)`), ref.grantID, ref.subjectID, ref.tenantID, project, scope, kindName(kind), a.currentPrincipal.ID, a.checkedAt.Format(time.RFC3339Nano), expiry, ref.sourceID, ref.sourceRevision); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_credential_slots(id,user_id,tenant_id,operation_id,request_hash,credential_kind,project_id,source_id,grant_id,family_id,expires_at,current_digest,revoked) VALUES(?,?,?,?,?,?,?,?,?,'',?,'',0)`), ref.slotID, ref.subjectID, ref.tenantID, operation, hash, kindName(kind), project, ref.sourceID, ref.grantID, expiry)
	return err
}

func (s *Service) issueScopedSecretTx(ctx context.Context, tx *sql.Tx, a *authorizedCommand, kind actorKind, ref credentialRef, label, expiry, scope, project, operation string) (ProjectCredentialIssueResult, error) {
	if a == nil {
		return ProjectCredentialIssueResult{}, forbidden()
	}
	if err := a.assertTx(tx, a.target, a.purpose); err != nil {
		return ProjectCredentialIssueResult{}, err
	}
	if a.actor.kind != actorBrowser || a.target.projectID != project || scopeKind(scope) != kind || kind == actorInvalid || ref.projectID != project || ref.sourceID != a.actor.credential.sourceID || ref.subjectID != a.currentPrincipal.ID {
		return ProjectCredentialIssueResult{}, forbidden()
	}
	token, err := s.persistCredential(ctx, tx, a.currentPrincipal, kind, ref, label, expiry, a.checkedAt.Format(time.RFC3339Nano), operation)
	if err != nil {
		return ProjectCredentialIssueResult{}, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_token_scopes(token_hash,kind,project_id) VALUES(?,?,?)`), digest(token), scope, project); err != nil {
		return ProjectCredentialIssueResult{}, err
	}
	return ProjectCredentialIssueResult{Token: token, ExpiresAt: expiry, Label: label, Scope: scope, ProjectID: project}, nil
}

// Registration shares the same real Grant/slot/issuance implementation as the
// public issuer, but its owning domain keeps its own metadata-only receipt.
// C has already checked the registration's current administrative rule.
func (s *Service) issueRegisteredRunnerTx(ctx context.Context, tx *sql.Tx, a *authorizedCommand, label, operation string) (ProjectCredentialIssueResult, error) {
	if a == nil {
		return ProjectCredentialIssueResult{}, forbidden()
	}
	if err := a.assertTx(tx, a.target, a.purpose); err != nil {
		return ProjectCredentialIssueResult{}, err
	}
	if a.actor.kind != actorBrowser || a.target.projectID == "" || a.target.projectID == organizationScope {
		return ProjectCredentialIssueResult{}, forbidden()
	}
	switch a.purpose {
	case purposeProjectWrite, purposeProjectApprove, purposeBrowserProjectWrite, purposeBrowserProjectApprove:
	default:
		return ProjectCredentialIssueResult{}, forbidden()
	}
	if len(operation) < 8 || len(operation) > 128 || len(label) > 200 {
		return ProjectCredentialIssueResult{}, invalid("无效凭证范围")
	}
	ref := credentialRef{subjectID: a.currentPrincipal.ID, tenantID: a.currentPrincipal.TenantID, projectID: a.target.projectID, sourceID: a.actor.credential.sourceID, sourceRevision: a.actor.credential.sourceRevision, grantID: newID(), slotID: newID()}
	expiry := a.checkedAt.Add(24 * time.Hour).Format(time.RFC3339Nano)
	slotOperation := digest("registered-runner:" + ref.tenantID + ":" + ref.projectID + ":" + operation)
	hash := digest(a.currentPrincipal.ID + stringify(map[string]any{"action": "registered-runner.issue", "project_id": ref.projectID, "label": label, "operation_id": operation}))
	if err := s.insertIndependentGrantTx(ctx, tx, a, actorProjectRunner, ref, ref.projectID, "runner", slotOperation, hash, expiry); err != nil {
		return ProjectCredentialIssueResult{}, err
	}
	return s.issueScopedSecretTx(ctx, tx, a, actorProjectRunner, ref, label, expiry, "runner", ref.projectID, operation)
}

// This constructor is only a candidate from a provider ticket's persisted
// digest. It deliberately takes no authority locks: the caller must use the
// normal earliest authorizeTx runner purpose before reading a receipt/effect.
func (s *Service) runnerActorByHashTx(ctx context.Context, tx *sql.Tx, tokenHash string) (Actor, error) {
	c, err := s.readCredential(ctx, tx, tokenHash, false)
	if err != nil {
		return Actor{}, err
	}
	if c.kind != actorProjectRunner {
		return Actor{}, forbidden()
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT u.id,u.name,u.email,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE u.id=? AND m.tenant_id=?`), c.ref.subjectID, c.ref.tenantID).Scan(&c.p.ID, &c.p.Name, &c.p.Email, &c.p.TenantID, &c.p.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Actor{}, forbidden()
	}
	if err != nil {
		return Actor{}, err
	}
	return Actor{kind: c.kind, credential: c.ref, projection: c.p}, nil
}

// logoutTarget has withdrawal authority only. It has no public writable fields,
// cannot be supplied to authorizeTx and is never populated from JSON input.
type logoutTarget struct {
	credentialDigest, subjectID, tenantID, familyID, slotID string
	kind                                                    actorKind
}

func (*logoutTarget) UnmarshalJSON([]byte) error {
	return errors.New("logout target cannot be constructed from JSON")
}
func (logoutTarget) MarshalJSON() ([]byte, error) {
	return nil, errors.New("logout target cannot be serialized")
}

func (s *Service) ResolveLogoutTarget(ctx context.Context, token string) (logoutTarget, error) {
	var out logoutTarget
	if token == "" {
		return out, nil
	}
	var kind string
	err := s.DB.QueryRowContext(ctx, s.q(`SELECT t.user_id,t.tenant_id,a.credential_kind,a.family_id,a.slot_id FROM suma_sessions t JOIN suma_credential_authority a ON a.token_hash=t.token_hash JOIN suma_credential_issuance i ON i.token_hash=t.token_hash AND i.slot_id=a.slot_id AND i.family_id=a.family_id AND i.grant_id=a.grant_id JOIN suma_credential_slots sl ON sl.id=a.slot_id AND sl.user_id=t.user_id AND sl.family_id=a.family_id AND sl.grant_id=a.grant_id JOIN suma_auth_sources src ON src.id=a.source_id AND src.user_id=t.user_id WHERE t.token_hash=? AND a.migration_status='current' AND ((a.credential_kind='browser-session' AND EXISTS(SELECT 1 FROM suma_session_families f WHERE f.id=a.family_id AND f.user_id=t.user_id AND f.source_id=src.id)) OR (a.credential_kind IN ('user-api','scoped-runner','scoped-agent') AND EXISTS(SELECT 1 FROM suma_credential_grants g WHERE g.id=a.grant_id AND g.user_id=t.user_id AND g.source_id=src.id)))`), digest(token)).Scan(&out.subjectID, &out.tenantID, &kind, &out.familyID, &out.slotID)
	if errors.Is(err, sql.ErrNoRows) {
		return logoutTarget{}, nil
	}
	if err != nil {
		return logoutTarget{}, err
	}
	for _, k := range []actorKind{actorBrowser, actorProjectAPI, actorProjectRunner, actorAgentRead} {
		if kind == kindName(k) {
			out.kind = k
		}
	}
	if out.kind == actorInvalid {
		return logoutTarget{}, nil
	}
	out.credentialDigest = digest(token)
	return out, nil
}

func (s *Service) Logout(ctx context.Context, target logoutTarget) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.logout(ctx, target)
		if !authoritySetRetry(err) {
			return err
		}
	}
	return err
}

func (s *Service) logout(ctx context.Context, target logoutTarget) error {
	if target.kind == actorInvalid || target.credentialDigest == "" {
		return nil
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return err
	}
	orgs, err := s.authorityOrganizations(ctx, tx, target.subjectID, target.tenantID)
	if err != nil {
		return err
	}
	keys := credentialLockKeys(credentialRef{digest: target.credentialDigest, slotID: target.slotID, familyID: target.familyID})
	if err = s.lockHumanAuthority(ctx, tx, target.subjectID, orgs, keys); err != nil {
		return err
	}
	var family, slot, user, kind string
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT a.family_id,a.slot_id,t.user_id,a.credential_kind FROM suma_credential_authority a JOIN suma_sessions t ON t.token_hash=a.token_hash WHERE a.token_hash=?`)), target.credentialDigest).Scan(&family, &slot, &user, &kind)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if user != target.subjectID || family != target.familyID || slot != target.slotID || kind != kindName(target.kind) {
		return nil
	}
	if target.kind == actorBrowser {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_session_families SET revoked=1,generation=generation+1 WHERE id=? AND user_id=? AND revoked=0`), family, user); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_authority SET revoked=1 WHERE family_id=? AND revoked=0`), family); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_slots SET revoked=1 WHERE family_id=? AND revoked=0`), family)
	} else {
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_authority SET revoked=1 WHERE slot_id=? AND revoked=0`), slot); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_slots SET revoked=1 WHERE id=? AND revoked=0`), slot)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Governance withdrawal retains historical sessions/scopes and their issuance
// evidence. It never revokes a cross-organization browser family or independent
// activation; only the validated target subject's exact tenant/project is hit.
// authorizeTx has already coordinated all target accounts and credential keys.
func (s *Service) withdrawSubjectCredentialsTx(ctx context.Context, a *authorizedCommand, userID, projectID string) error {
	if a == nil || userID == "" || a.target.subjectID != userID || a.purpose != purposeOrganizationWrite {
		return forbidden()
	}
	expectedResource := userID
	if projectID != "" {
		expectedResource = projectID + ":" + userID
	}
	if a.target.projectID != organizationScope || a.target.resourceID != expectedResource {
		return forbidden()
	}
	if err := a.assertTx(a.tx, a.target, purposeOrganizationWrite); err != nil {
		return err
	}
	ctx, cancel := context.WithDeadline(ctx, a.deadline)
	defer cancel()
	where := `t.tenant_id=? AND t.user_id=?`
	args := []any{a.target.tenantID, userID}
	if projectID != "" {
		where += ` AND EXISTS(SELECT 1 FROM suma_token_scopes sc WHERE sc.token_hash=t.token_hash AND sc.project_id=?)`
		args = append(args, projectID)
	}
	slotQuery := `UPDATE suma_credential_slots SET revoked=1 WHERE revoked=0 AND id IN (SELECT ca.slot_id FROM suma_credential_authority ca JOIN suma_sessions t ON t.token_hash=ca.token_hash WHERE ` + where + `)`
	if _, err := a.tx.ExecContext(ctx, s.q(slotQuery), args...); err != nil {
		return err
	}
	authQuery := `UPDATE suma_credential_authority SET revoked=1 WHERE revoked=0 AND token_hash IN (SELECT t.token_hash FROM suma_sessions t WHERE ` + where + `)`
	if _, err := a.tx.ExecContext(ctx, s.q(authQuery), args...); err != nil {
		return err
	}
	grantQuery := `UPDATE suma_credential_grants SET revoked=1 WHERE revoked=0 AND tenant_id=? AND user_id=?`
	grantArgs := []any{a.target.tenantID, userID}
	if projectID != "" {
		grantQuery += ` AND project_id=?`
		grantArgs = append(grantArgs, projectID)
	}
	_, err := a.tx.ExecContext(ctx, s.q(grantQuery), grantArgs...)
	return err
}

func (s *Service) SwitchOrganization(ctx context.Context, actor Actor, m map[string]any) (Principal, string, error) {
	var p Principal
	var token string
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		p, token, err = s.switchOrganization(ctx, actor, m)
		if !authoritySetRetry(err) {
			return p, token, err
		}
	}
	return p, token, err
}

func (s *Service) switchOrganization(ctx context.Context, actor Actor, m map[string]any) (Principal, string, error) {
	target := str(m, "organization_id")
	if target == "" {
		return Principal{}, "", invalid("请选择组织")
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return Principal{}, "", err
	}
	defer cancel()
	defer tx.Rollback()
	a, err := s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeSwitchOrganization)
	if err != nil {
		return Principal{}, "", err
	}
	var chosen Principal
	err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT u.id,u.name,u.email,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE u.id=? AND m.tenant_id=?`)), a.currentPrincipal.ID, target).Scan(&chosen.ID, &chosen.Name, &chosen.Email, &chosen.TenantID, &chosen.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, "", forbidden()
	}
	if err != nil {
		return Principal{}, "", err
	}
	body := map[string]any{"action": "organization.switch", "organization_id": target}
	hash, replay, err := s.commandReceipt(ctx, a, str(m, "idempotency_key"), body)
	if err != nil {
		return Principal{}, "", err
	}
	ref := actor.credential
	ref.tenantID = target
	ref.slotID = newID()
	ref.digest = ""
	var expiry string
	if err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT expires_at FROM suma_session_families WHERE id=?`)), ref.familyID).Scan(&expiry); err != nil {
		return Principal{}, "", err
	}
	if replay {
		var family, source, stored string
		var revoked int
		err = tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT id,family_id,source_id,request_hash,expires_at,revoked FROM suma_credential_slots WHERE user_id=? AND tenant_id=? AND operation_id=?`)), chosen.ID, a.currentPrincipal.TenantID, str(m, "idempotency_key")).Scan(&ref.slotID, &family, &source, &stored, &expiry, &revoked)
		if err != nil {
			return Principal{}, "", err
		}
		if revoked != 0 || family != ref.familyID || source != ref.sourceID || stored != digest(a.currentPrincipal.ID+stringify(body)) {
			return Principal{}, "", forbidden()
		}
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_credential_authority SET revoked=1 WHERE slot_id=? AND revoked=0`), ref.slotID); err != nil {
			return Principal{}, "", err
		}
	} else {
		_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_credential_slots(id,user_id,tenant_id,operation_id,request_hash,credential_kind,project_id,source_id,grant_id,family_id,expires_at,current_digest,revoked) VALUES(?,?,?,?,?,'browser-session','',?,'',?,?,?,0)`), ref.slotID, chosen.ID, a.currentPrincipal.TenantID, str(m, "idempotency_key"), hash, ref.sourceID, ref.familyID, expiry, "")
		if err != nil {
			return Principal{}, "", err
		}
		if err = s.saveCommandReceipt(ctx, a, str(m, "idempotency_key"), hash, map[string]any{"organization_id": target, "user_id": chosen.ID}); err != nil {
			return Principal{}, "", err
		}
	}
	token, err := s.persistCredential(ctx, tx, chosen, actorBrowser, ref, "browser", expiry, a.checkedAt.Format(time.RFC3339Nano), str(m, "idempotency_key"))
	if err != nil {
		return Principal{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return Principal{}, "", err
	}
	return chosen, token, nil
}

// AcceptInvitation is intentionally not a login shortcut. Existing users prove
// their current password; a consumed token never starts a second acceptance.
func (s *Service) AcceptInvitation(ctx context.Context, m map[string]any) (Principal, string, error) {
	var p Principal
	var token string
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		p, token, err = s.acceptInvitation(ctx, m)
		if !authoritySetRetry(err) {
			return p, token, err
		}
	}
	return p, token, err
}

func (s *Service) acceptInvitation(ctx context.Context, m map[string]any) (Principal, string, error) {
	email, err := invitationEmail(str(m, "email"))
	if err != nil {
		return Principal{}, "", err
	}
	password, name, secret := str(m, "password"), strings.TrimSpace(str(m, "name")), str(m, "token")
	if len(password) < 12 || len(password) > 1024 || name == "" || len(name) > 200 {
		return Principal{}, "", invalid("姓名应为 1–200 字节，密码应为 12–1024 字节")
	}
	if len(secret) != 64 {
		return Principal{}, "", invalid("无效邀请密钥")
	}
	if _, err = hex.DecodeString(secret); err != nil {
		return Principal{}, "", invalid("无效邀请密钥")
	}
	if key := str(m, "idempotency_key"); len(key) < 8 || len(key) > 128 {
		return Principal{}, "", invalid("请提供 8–128 字符的 idempotency_key")
	}
	var tenant string
	if err = s.DB.QueryRowContext(ctx, s.q(`SELECT tenant_id FROM suma_invitations WHERE token_hash=?`), digest(secret)).Scan(&tenant); errors.Is(err, sql.ErrNoRows) {
		return Principal{}, "", invalid("邀请不存在或密钥无效")
	}
	if err != nil {
		return Principal{}, "", err
	}
	candidateUser := newID()
	var existingUser string
	lookupErr := s.DB.QueryRowContext(ctx, s.q(`SELECT id FROM suma_users WHERE email=?`), email).Scan(&existingUser)
	if lookupErr == nil {
		candidateUser = existingUser
	} else if !errors.Is(lookupErr, sql.ErrNoRows) {
		return Principal{}, "", lookupErr
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return Principal{}, "", err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return Principal{}, "", err
	}
	organizations, err := s.authorityOrganizations(ctx, tx, candidateUser, tenant)
	if err != nil {
		return Principal{}, "", err
	}
	if err = s.lockHumanAuthority(ctx, tx, candidateUser, organizations, []string{"invitation:" + digest(secret)}); err != nil {
		return Principal{}, "", err
	}
	item, err := scanInvitation(tx.QueryRowContext(ctx, s.q(`SELECT `+invitationColumns+` FROM suma_invitations WHERE token_hash=?`), digest(secret)))
	if err != nil {
		return Principal{}, "", err
	}
	var stored string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT token_hash FROM suma_invitations WHERE id=? AND tenant_id=?`), item.ID, tenant).Scan(&stored); err != nil {
		return Principal{}, "", err
	}
	if subtle.ConstantTimeCompare([]byte(stored), []byte(digest(secret))) != 1 {
		return Principal{}, "", invalid("无效邀请密钥")
	}
	if subtle.ConstantTimeCompare([]byte(item.Email), []byte(email)) != 1 {
		return Principal{}, "", invalid("邀请邮箱不匹配")
	}
	expiry, err := time.Parse(time.RFC3339Nano, item.ExpiresAt)
	dbAt, clockErr := s.authorityNow(ctx, tx)
	if clockErr != nil {
		return Principal{}, "", clockErr
	}
	if err != nil || expiry.Unix() <= dbAt.Unix() || item.Status != "pending" {
		return Principal{}, "", conflict("邀请已使用、已撤销或已过期；若先前已接受，请使用普通登录")
	}
	issuer, err := s.liveOrganization(ctx, tx, Principal{ID: item.CreatedBy, TenantID: tenant})
	if err != nil || !orgAdmin(issuer) {
		return Principal{}, "", conflict("邀请人已无组织管理权限，请重新申请邀请")
	}
	if item.Role != "admin" && item.Role != "member" {
		return Principal{}, "", invalid("无效邀请角色")
	}
	if err = s.validateInvitationGrants(ctx, tx, tenant, item.Grants); err != nil {
		return Principal{}, "", err
	}
	p := Principal{TenantID: tenant, Role: item.Role}
	var hash string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,name,email,password_hash FROM suma_users WHERE email=?`), email).Scan(&p.ID, &p.Name, &p.Email, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		p.ID, p.Name, p.Email = candidateUser, name, email
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_users(id,name,email,password_hash,created_at) VALUES(?,?,?,?,?)`), p.ID, p.Name, p.Email, passwordHash(password, newID()), now()); err != nil {
			return Principal{}, "", err
		}
	} else if err != nil {
		return Principal{}, "", err
	} else if !passwordMatches(password, hash) {
		return Principal{}, "", &APIError{401, "invalid_credentials", "现有账号必须提供正确密码", nil}
	}
	if p.ID != candidateUser {
		return Principal{}, "", &APIError{409, "authority_retry", "账号状态已改变，请重试", nil}
	}
	var membership int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_org_members WHERE tenant_id=? AND user_id=?`), tenant, p.ID).Scan(&membership); err != nil {
		return Principal{}, "", err
	}
	if membership != 0 {
		return Principal{}, "", conflict("账号已是组织成员；邀请不会覆盖其角色或密码")
	}
	// Never persist a password-derived request digest: with known metadata it
	// would become a fast offline verifier beside the slow account password KDF.
	// Credentials are verified above; all subsequent uses of this invitation
	// fail its consumed-state check, including requests with the same key.
	body := map[string]any{"action": "invitation.accept", "invitation_id": item.ID, "email": email, "name": name}
	// This bootstrap path has just verified the real one-time invitation,
	// current issuer, target grants and password. It cannot construct a human
	// Actor or share a general Principal-based command receipt capability.
	requestHash := digest(p.ID + stringify(body))
	var priorHash string
	err = tx.QueryRowContext(ctx, s.q(`SELECT request_hash FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), tenant, organizationScope, str(m, "idempotency_key")).Scan(&priorHash)
	if err == nil {
		if priorHash != requestHash {
			return Principal{}, "", &APIError{409, "idempotency_conflict", "该操作标识已用于其他内容", nil}
		}
		return Principal{}, "", conflict("邀请已接受，请使用普通登录")
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Principal{}, "", err
	}
	version, err := s.orgMemberVersion(ctx, tx, tenant, p.ID)
	if err != nil {
		return Principal{}, "", err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_org_members(tenant_id,user_id,role) VALUES(?,?,?)`), tenant, p.ID, item.Role); err != nil {
		return Principal{}, "", err
	}
	if err = s.setOrgMemberVersion(ctx, tx, tenant, p.ID, version+1); err != nil {
		return Principal{}, "", err
	}
	for _, grant := range item.Grants {
		v, err := s.projectGrantVersion(ctx, tx, tenant, grant.ProjectID, p.ID)
		if err != nil {
			return Principal{}, "", err
		}
		if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_project_members(project_id,user_id,role) VALUES(?,?,?)`), grant.ProjectID, p.ID, grant.Role); err != nil {
			return Principal{}, "", err
		}
		if err = s.setProjectGrantVersion(ctx, tx, tenant, grant.ProjectID, p.ID, v+1); err != nil {
			return Principal{}, "", err
		}
	}
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_invitations SET status='accepted',accepted_by=?,accepted_at=?,version=version+1 WHERE id=? AND tenant_id=? AND status='pending' AND version=?`), p.ID, now(), item.ID, tenant, item.Version)
	if err != nil {
		return Principal{}, "", err
	}
	changed, _ := result.RowsAffected()
	if changed != 1 {
		return Principal{}, "", conflict("邀请已使用")
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_activity(id,tenant_id,project_id,actor_id,kind,object_id,title,created_at) VALUES(?,?,?,?,?,?,?,?)`), newID(), tenant, organizationScope, p.ID, "organization.invitation.accept", item.ID, "接受组织邀请", dbAt.Format(time.RFC3339Nano)); err != nil {
		return Principal{}, "", err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response) VALUES(?,?,?,?,?)`), tenant, organizationScope, str(m, "idempotency_key"), requestHash, stringify(map[string]any{"user_id": p.ID, "organization_id": tenant, "invitation_id": item.ID, "status": "accepted"})); err != nil {
		return Principal{}, "", err
	}
	token, err := s.newBrowserSessionTx(ctx, tx, p, "invitation")
	if err != nil {
		return Principal{}, "", err
	}
	if err = tx.Commit(); err != nil {
		return Principal{}, "", err
	}
	return p, token, nil
}
