package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type actorKind uint8

const (
	actorInvalid actorKind = iota
	actorBrowser
	actorProjectAPI
	actorProjectRunner
	actorAgentRead
	actorDurableAutomation
	actorDurableToolActivation
	actorSignedIngress
)

type credentialRef struct {
	digest, subjectID, tenantID, projectID string
	sourceID, familyID, grantID, slotID    string
	sourceRevision, familyRevision         int64
}

type delegationRef struct {
	recordID, tenantID, projectID, subjectID, secondarySubjectID string
	sourceDigest, bodyDigest, deliveryID, linkedRecordID         string
	manualOccurrenceID, manualInvokerID                          string
	revision, generation                                         int
}

// Actor contains only a reference obtained from a real credential. The display
// projection is never a substitute for the transaction's current SQL authority.
type Actor struct {
	kind       actorKind
	credential credentialRef
	delegation delegationRef
	projection Principal
}

func (a Actor) Projection() Principal      { return a.projection }
func (a Actor) IsBrowser() bool            { return a.kind == actorBrowser }
func (Actor) MarshalJSON() ([]byte, error) { return nil, errors.New("Actor cannot be serialized") }
func (*Actor) UnmarshalJSON([]byte) error  { return errors.New("Actor cannot be constructed from JSON") }

func unauthenticated() error { return &APIError{401, "unauthenticated", "登录已失效", nil} }

type credentialState struct {
	ref     credentialRef
	kind    actorKind
	p       Principal
	expires time.Time
	label   string
}

type authorityQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Service) currentSQL(query string) string {
	if s.Dialect == "postgres" || s.Dialect == "oceanbase" {
		return query + " FOR UPDATE"
	}
	return query
}

func kindName(k actorKind) string {
	switch k {
	case actorBrowser:
		return "browser-session"
	case actorProjectAPI:
		return "user-api"
	case actorProjectRunner:
		return "scoped-runner"
	case actorAgentRead:
		return "scoped-agent"
	}
	return "invalid"
}

func scopeKind(scope string) actorKind {
	switch scope {
	case "api":
		return actorProjectAPI
	case "runner":
		return actorProjectRunner
	case "agent":
		return actorAgentRead
	}
	return actorInvalid
}

func validCredentialSourceKind(kind string) bool {
	return kind == "setup" || kind == "password" || kind == "invitation"
}

func (s *Service) readCredential(ctx context.Context, q authorityQuery, hash string, current bool) (credentialState, error) {
	var c credentialState
	var kind, expiry string
	var revoked int
	query := `SELECT t.user_id,t.tenant_id,t.expires_at,t.label,a.credential_kind,a.source_id,a.source_generation,a.family_id,a.family_generation,a.grant_id,a.slot_id,a.revoked FROM suma_sessions t JOIN suma_credential_authority a ON a.token_hash=t.token_hash WHERE t.token_hash=?`
	if current {
		query = s.currentSQL(query)
	}
	err := q.QueryRowContext(ctx, s.q(query), hash).Scan(&c.ref.subjectID, &c.ref.tenantID, &expiry, &c.label, &kind, &c.ref.sourceID, &c.ref.sourceRevision, &c.ref.familyID, &c.ref.familyRevision, &c.ref.grantID, &c.ref.slotID, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return c, unauthenticated()
	}
	if err != nil {
		return c, err
	}
	for _, k := range []actorKind{actorBrowser, actorProjectAPI, actorProjectRunner, actorAgentRead} {
		if kind == kindName(k) {
			c.kind = k
		}
	}
	if c.kind == actorInvalid || revoked != 0 {
		return c, unauthenticated()
	}
	c.ref.digest = hash
	c.expires, err = time.Parse(time.RFC3339Nano, expiry)
	if err != nil {
		return c, unauthenticated()
	}
	var sourceUser, sourceKind, authenticatedAt string
	var sourceGeneration int64
	query = `SELECT user_id,generation,revoked,source_kind,authenticated_at FROM suma_auth_sources WHERE id=?`
	if current {
		query = s.currentSQL(query)
	}
	err = q.QueryRowContext(ctx, s.q(query), c.ref.sourceID).Scan(&sourceUser, &sourceGeneration, &revoked, &sourceKind, &authenticatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return c, unauthenticated()
	}
	if err != nil {
		return c, err
	}
	if sourceUser != c.ref.subjectID || sourceGeneration != c.ref.sourceRevision || revoked != 0 || authenticatedAt == "" || !validCredentialSourceKind(sourceKind) {
		return c, unauthenticated()
	}
	var scope, project string
	query = `SELECT kind,project_id FROM suma_token_scopes WHERE token_hash=?`
	if current {
		query = s.currentSQL(query)
	}
	err = q.QueryRowContext(ctx, s.q(query), hash).Scan(&scope, &project)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return c, err
	}
	if c.kind == actorBrowser {
		if err == nil || c.ref.familyID == "" || c.ref.grantID != "" {
			return c, unauthenticated()
		}
		var user, source, familyExpiry string
		var generation, sourceRevision int64
		query = `SELECT user_id,source_id,source_generation,generation,expires_at,revoked FROM suma_session_families WHERE id=?`
		if current {
			query = s.currentSQL(query)
		}
		err = q.QueryRowContext(ctx, s.q(query), c.ref.familyID).Scan(&user, &source, &sourceRevision, &generation, &familyExpiry, &revoked)
		if errors.Is(err, sql.ErrNoRows) {
			return c, unauthenticated()
		}
		if err != nil {
			return c, err
		}
		if user != c.ref.subjectID || source != c.ref.sourceID || sourceRevision != c.ref.sourceRevision || generation != c.ref.familyRevision || revoked != 0 || expiry != familyExpiry {
			return c, unauthenticated()
		}
	} else {
		if err != nil || scopeKind(scope) != c.kind || project == "" || c.ref.grantID == "" || c.ref.familyID != "" {
			return c, unauthenticated()
		}
		c.ref.projectID = project
		var user, tenant, gProject, gKind, gScope, gExpiry, approved, source string
		var revision int64
		query = `SELECT user_id,tenant_id,project_id,credential_kind,scope_kind,expires_at,approved_by,source_id,source_generation,revoked FROM suma_credential_grants WHERE id=?`
		if current {
			query = s.currentSQL(query)
		}
		err = q.QueryRowContext(ctx, s.q(query), c.ref.grantID).Scan(&user, &tenant, &gProject, &gKind, &gScope, &gExpiry, &approved, &source, &revision, &revoked)
		if errors.Is(err, sql.ErrNoRows) {
			return c, unauthenticated()
		}
		if err != nil {
			return c, err
		}
		if user != c.ref.subjectID || tenant != c.ref.tenantID || gProject != project || gKind != kind || gScope != scope || gExpiry != expiry || approved != user || source != c.ref.sourceID || revision != c.ref.sourceRevision || revoked != 0 {
			return c, unauthenticated()
		}
	}
	var slotHash, slotExpiry, issuedSlot, issuedFamily, issuedGrant, slotUser, slotKind, slotProject, slotSource, slotFamily, slotGrant string
	query = `SELECT current_digest,expires_at,revoked,user_id,credential_kind,project_id,source_id,family_id,grant_id FROM suma_credential_slots WHERE id=?`
	if current {
		query = s.currentSQL(query)
	}
	if err = q.QueryRowContext(ctx, s.q(query), c.ref.slotID).Scan(&slotHash, &slotExpiry, &revoked, &slotUser, &slotKind, &slotProject, &slotSource, &slotFamily, &slotGrant); errors.Is(err, sql.ErrNoRows) {
		return c, unauthenticated()
	}
	if err != nil {
		return c, err
	}
	if slotHash != hash || slotExpiry != expiry || revoked != 0 || slotUser != c.ref.subjectID || slotKind != kind || slotProject != c.ref.projectID || slotSource != c.ref.sourceID || slotFamily != c.ref.familyID || slotGrant != c.ref.grantID {
		return c, unauthenticated()
	}
	query = `SELECT slot_id,family_id,grant_id FROM suma_credential_issuance WHERE token_hash=?`
	if current {
		query = s.currentSQL(query)
	}
	err = q.QueryRowContext(ctx, s.q(query), hash).Scan(&issuedSlot, &issuedFamily, &issuedGrant)
	if errors.Is(err, sql.ErrNoRows) {
		return c, unauthenticated()
	}
	if err != nil {
		return c, err
	}
	if issuedSlot != c.ref.slotID || issuedFamily != c.ref.familyID || issuedGrant != c.ref.grantID {
		return c, unauthenticated()
	}
	return c, nil
}

func (s *Service) AuthenticateActor(ctx context.Context, token string) (Actor, error) {
	if token == "" {
		var details any
		if s.SetupRequired(ctx) {
			details = map[string]any{"setup_required": true}
		}
		return Actor{}, &APIError{401, "unauthenticated", "请登录", details}
	}
	c, err := s.readCredential(ctx, s.DB, digest(token), false)
	if err != nil {
		return Actor{}, err
	}
	at, err := s.authorityNow(ctx, s.DB)
	if err != nil {
		return Actor{}, err
	}
	if c.expires.Unix() <= at.Unix() {
		return Actor{}, unauthenticated()
	}
	err = s.DB.QueryRowContext(ctx, s.q(`SELECT u.id,u.name,u.email,m.tenant_id,m.role FROM suma_users u JOIN suma_org_members m ON m.user_id=u.id WHERE u.id=? AND m.tenant_id=?`), c.ref.subjectID, c.ref.tenantID).Scan(&c.p.ID, &c.p.Name, &c.p.Email, &c.p.TenantID, &c.p.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return Actor{}, forbidden()
	}
	if err != nil {
		return Actor{}, err
	}
	return Actor{kind: c.kind, credential: c.ref, projection: c.p}, nil
}
