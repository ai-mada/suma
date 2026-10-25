package platform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

func logoutCredentialForTest(ctx context.Context, s *Service, token string) error {
	target, err := s.ResolveLogoutTarget(ctx, token)
	if err != nil {
		return err
	}
	return s.Logout(ctx, target)
}

func authorityEmptyFixture(t *testing.T) *Service {
	t.Helper()
	base, err := filepath.Abs("../../.cache/current-authority-tests")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, "fixture-")
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.Open("sqlite", filepath.Join(dir, "authority.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		basePath, baseErr := filepath.EvalSymlinks(base)
		fixturePath, pathErr := filepath.EvalSymlinks(dir)
		if baseErr != nil || pathErr != nil {
			t.Errorf("cannot verify disposable fixture boundary: %v %v", baseErr, pathErr)
			return
		}
		rel, relErr := filepath.Rel(basePath, fixturePath)
		if relErr != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			t.Error("refusing fixture cleanup outside exact .cache task directory")
			return
		}
		if info, err := os.Lstat(dir); err == nil && info.Mode()&os.ModeSymlink != 0 {
			os.Remove(dir)
		} else {
			os.RemoveAll(dir)
		}
	})
	s := &Service{DB: db, Dialect: "sqlite"}
	if err = s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return s
}

func authorityFixture(t *testing.T) (*Service, Principal, string, string) {
	t.Helper()
	s := authorityEmptyFixture(t)
	p, token, err := s.Setup(context.Background(), map[string]any{"email": "authority@example.test", "name": "Authority", "password": "authority-test-password-42"})
	if err != nil {
		t.Fatal(err)
	}
	var project string
	if err = s.DB.QueryRow(`SELECT id FROM suma_projects WHERE tenant_id=?`, p.TenantID).Scan(&project); err != nil {
		t.Fatal(err)
	}
	return s, p, token, project
}

func TestAuthorityLegacyKindsMigration(t *testing.T) {
	s, p, _, project := authorityFixture(t)
	ctx := context.Background()
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	for _, scope := range []string{"", "api", "runner", "agent"} {
		token := "legacy-" + scope
		if _, err := s.DB.Exec(`INSERT INTO suma_sessions(token_hash,user_id,tenant_id,expires_at,label) VALUES(?,?,?,?,?)`, digest(token), p.ID, p.TenantID, expires, "browser"); err != nil {
			t.Fatal(err)
		}
		if scope != "" {
			if _, err := s.DB.Exec(`INSERT INTO suma_token_scopes(token_hash,kind,project_id) VALUES(?,?,?)`, digest(token), scope, project); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, scope := range []string{"", "api", "runner", "agent"} {
		if _, err := s.Authenticate(ctx, "legacy-"+scope); err == nil {
			t.Errorf("legacy %q session remained authoritative", scope)
		}
		var gotExpiry, kind, status string
		if err := s.DB.QueryRow(`SELECT t.expires_at,a.credential_kind,a.migration_status FROM suma_sessions t JOIN suma_credential_authority a ON a.token_hash=t.token_hash WHERE t.token_hash=?`, digest("legacy-"+scope)).Scan(&gotExpiry, &kind, &status); err != nil || gotExpiry != expires || kind != "legacy-unclassified" || status != "reissue-required" {
			t.Fatalf("legacy audit changed %q: %v", scope, err)
		}
	}
	_, fresh, err := s.Login(ctx, "authority@example.test", "authority-test-password-42")
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateActor(ctx, fresh)
	if err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueProjectCredential(ctx, actor, ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", IdempotencyKey: "new-model-issued"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateActor(ctx, fresh); err != nil {
		t.Fatalf("complete browser downgraded on repeat migration: %v", err)
	}
	if _, err = s.AuthenticateActor(ctx, issued.Token); err != nil {
		t.Fatalf("complete Grant downgraded on repeat migration: %v", err)
	}
}

func TestAuthorityIssueAtomicAndGrantLifetime(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	dbAt, err := s.authorityNow(ctx, s.DB)
	if err != nil {
		t.Fatal(err)
	}
	expires := dbAt.Add(time.Minute).Format(time.RFC3339Nano)
	for _, query := range []string{`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`, `UPDATE suma_session_families SET expires_at=? WHERE id=?`, `UPDATE suma_credential_slots SET expires_at=? WHERE id=?`} {
		key := digest(token)
		if strings.Contains(query, "families") {
			key = actor.credential.familyID
		}
		if strings.Contains(query, "slots") {
			key = actor.credential.slotID
		}
		if _, err = s.DB.Exec(query, expires, key); err != nil {
			t.Fatal(err)
		}
	}
	req := ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", Label: "browser", IdempotencyKey: "independent-lifetime"}
	first, err := s.IssueProjectCredential(ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	issuedExpiry, err := time.Parse(time.RFC3339Nano, first.ExpiresAt)
	if err != nil {
		t.Fatal(err)
	}
	if issuedExpiry.Sub(dbAt) < 23*time.Hour {
		t.Fatal("independent Grant capped to browser TTL")
	}
	second, err := s.IssueProjectCredential(ctx, actor, req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Token == second.Token || first.ExpiresAt != second.ExpiresAt {
		t.Fatal("replay did not rotate one slot with fixed expiry")
	}
	_, err = s.AuthenticateActor(ctx, first.Token)
	requireAuthorityStatus(t, err, 401)
	if _, err = s.AuthenticateActor(ctx, second.Token); err != nil {
		t.Fatal(err)
	}
	var receipts string
	if err = s.DB.QueryRow(`SELECT response FROM suma_operations WHERE operation_id=?`, req.IdempotencyKey).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receipts, first.Token) || strings.Contains(receipts, second.Token) {
		t.Fatal("receipt contains raw secret")
	}
	var before int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM suma_credential_grants`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`CREATE TRIGGER reject_authority_scope BEFORE INSERT ON suma_token_scopes BEGIN SELECT RAISE(FAIL,'injected scope failure'); END`); err != nil {
		t.Fatal(err)
	}
	req.IdempotencyKey = "rollback-issuance"
	if _, err = s.IssueProjectCredential(ctx, actor, req); err == nil {
		t.Fatal("scope failure accepted issuance")
	}
	var after int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM suma_credential_grants`).Scan(&after); err != nil || before != after {
		t.Fatalf("partial Grant persisted: %d -> %d %v", before, after, err)
	}
	if _, err = s.DB.Exec(`DROP TRIGGER reject_authority_scope`); err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateActor(ctx, second.Token); err != nil {
		t.Fatalf("browser logout revoked independent Grant: %v", err)
	}
}

func TestAuthoritySwitchPreservesSourceLifetime(t *testing.T) {
	s, p, token, _ := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	// Add an actual target membership fixture; browser credentials themselves
	// always come from real bootstrap/login/switch, never a fake Actor.
	other := newID()
	if _, err = s.DB.Exec(`INSERT INTO suma_organizations(id,name,created_at) VALUES(?,?,?)`, other, "Target", now()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO suma_org_members(tenant_id,user_id,role) VALUES(?,?,'member')`, other, p.ID); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{"organization_id": other, "idempotency_key": "family-switch-operation"}
	_, switched, err := s.SwitchOrganization(ctx, actor, body)
	if err != nil {
		t.Fatal(err)
	}
	switchedActor, err := s.AuthenticateActor(ctx, switched)
	if err != nil {
		t.Fatal(err)
	}
	if switchedActor.credential.familyID != actor.credential.familyID || switchedActor.credential.sourceID != actor.credential.sourceID {
		t.Fatal("switch replaced source or family")
	}
	var originalExpiry, newExpiry string
	s.DB.QueryRow(`SELECT expires_at FROM suma_sessions WHERE token_hash=?`, digest(token)).Scan(&originalExpiry)
	s.DB.QueryRow(`SELECT expires_at FROM suma_sessions WHERE token_hash=?`, digest(switched)).Scan(&newExpiry)
	if originalExpiry != newExpiry {
		t.Fatal("switch extended browser family")
	}
	_, replacement, err := s.SwitchOrganization(ctx, actor, body)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, switched)
	requireAuthorityStatus(t, err, 401)
	if _, err = s.AuthenticateActor(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateActor(ctx, token); err != nil {
		t.Fatal("switch rotation revoked original source slot")
	}
	if err = logoutCredentialForTest(ctx, s, replacement); err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, token)
	requireAuthorityStatus(t, err, 401)
}

func TestAuthorityLogoutKindSpecificAndIdempotent(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	_, otherFamily, err := s.Login(ctx, "authority@example.test", "authority-test-password-42")
	if err != nil {
		t.Fatal(err)
	}
	var issued []ProjectCredentialIssueResult
	for _, scope := range []string{"api", "runner", "agent"} {
		out, e := s.IssueProjectCredential(ctx, actor, ProjectCredentialIssueRequest{ProjectID: project, Scope: scope, IdempotencyKey: "logout-kind-" + scope})
		if e != nil {
			t.Fatal(e)
		}
		issued = append(issued, out)
	}
	target, err := s.ResolveLogoutTarget(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Logout(ctx, target); err != nil {
		t.Fatal(err)
	}
	var generation int
	if err = s.DB.QueryRow(`SELECT generation FROM suma_session_families WHERE id=?`, actor.credential.familyID).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{token, "unknown-token", ""} {
		if err = logoutCredentialForTest(ctx, s, raw); err != nil {
			t.Fatal(err)
		}
	}
	var again int
	s.DB.QueryRow(`SELECT generation FROM suma_session_families WHERE id=?`, actor.credential.familyID).Scan(&again)
	if generation != again {
		t.Fatal("repeated logout changed generation")
	}
	if _, err = s.AuthenticateActor(ctx, otherFamily); err != nil {
		t.Fatal("logout revoked another login family")
	}
	for _, out := range issued {
		if _, err = s.AuthenticateActor(ctx, out.Token); err != nil {
			t.Fatal("family logout revoked independent token")
		}
	}
	if err = logoutCredentialForTest(ctx, s, issued[0].Token); err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, issued[0].Token)
	requireAuthorityStatus(t, err, 401)
	for _, out := range issued[1:] {
		if _, err = s.AuthenticateActor(ctx, out.Token); err != nil {
			t.Fatal("scoped logout revoked another slot")
		}
	}
}

func TestAuthorityGovernanceWithdrawalCannotReviveSlot(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	owner, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.CreateInvitation(ctx, owner, map[string]any{"email": "issuer@example.test", "role": "admin", "grants": []any{map[string]any{"project_id": project, "role": "developer"}}, "idempotency_key": "invite-real-issuer"})
	if err != nil {
		t.Fatal(err)
	}
	p, browser, err := s.AcceptInvitation(ctx, map[string]any{"email": "issuer@example.test", "name": "Credential issuer", "password": "issuer-real-password-42", "token": inv["token"], "idempotency_key": "accept-real-issuer"})
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := s.AuthenticateActor(ctx, browser)
	if err != nil {
		t.Fatal(err)
	}
	req := ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", IdempotencyKey: "cannot-revive-slot"}
	issued, err := s.IssueProjectCredential(ctx, issuer, req)
	if err != nil {
		t.Fatal(err)
	}
	var sessions, scopes int
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_sessions`).Scan(&sessions)
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_token_scopes`).Scan(&scopes)
	_, err = s.SaveProjectAccess(ctx, owner, map[string]any{"project_id": project, "user_id": p.ID, "action": "remove", "expected_version": float64(1), "idempotency_key": "remove-issued-project"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, issued.Token)
	requireAuthorityStatus(t, err, 401)
	_, err = s.SaveProjectAccess(ctx, owner, map[string]any{"project_id": project, "user_id": p.ID, "role": "developer", "expected_version": float64(2), "idempotency_key": "regrant-issued-project"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.IssueProjectCredential(ctx, issuer, req)
	requireAuthorityStatus(t, err, 403)
	if _, err = s.AuthenticateActor(ctx, browser); err != nil {
		t.Fatalf("project withdrawal revoked browser family: %v", err)
	}
	_, err = s.UpdateOrganizationMember(ctx, owner, p.ID, map[string]any{"action": "remove", "expected_version": float64(1), "idempotency_key": "remove-issued-organization"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, browser)
	requireAuthorityStatus(t, err, 401)
	var afterSessions, afterScopes int
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_sessions`).Scan(&afterSessions)
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_token_scopes`).Scan(&afterScopes)
	if sessions != afterSessions || scopes != afterScopes {
		t.Fatal("withdrawal deleted historical session/scope evidence")
	}
}

func TestAuthorityRegisteredRunnerUsesIndependentGrant(t *testing.T) {
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	browser, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	var issued ProjectCredentialIssueResult
	for _, commit := range []bool{false, true} {
		bounded, tx, cleanup, e := s.beginAuthorityTx(ctx)
		if e != nil {
			t.Fatal(e)
		}
		a, e := s.authorizeTx(bounded, tx, browser, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: "registered-worker"}, purposeBrowserProjectWrite)
		if e != nil {
			cleanup()
			t.Fatal(e)
		}
		issued, e = s.issueRegisteredRunnerTx(bounded, tx, a, "browser", "registered-runner-operation")
		if e != nil {
			cleanup()
			t.Fatal(e)
		}
		var operations int
		if e = tx.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&operations); e != nil || operations != 0 {
			cleanup()
			t.Fatal("registration issuer wrote a second business receipt")
		}
		if commit {
			if e = tx.Commit(); e != nil {
				cleanup()
				t.Fatal(e)
			}
		}
		cleanup()
		if !commit {
			_, e = s.AuthenticateActor(ctx, issued.Token)
			requireAuthorityStatus(t, e, 401)
		}
	}
	runner, err := s.AuthenticateActor(ctx, issued.Token)
	if err != nil || runner.kind != actorProjectRunner {
		t.Fatalf("registration did not issue real runner kind: %v", err)
	}
	bounded, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer tx.Rollback()
	candidate, err := s.runnerActorByHashTx(bounded, tx, digest(issued.Token))
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.authorizeTx(bounded, tx, candidate, commandTarget{tenantID: p.TenantID, projectID: project, resourceID: "registered-worker"}, purposeRunnerWrite)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.issueRegisteredRunnerTx(bounded, tx, a, "forged", "runner-cannot-issue")
	requireAuthorityStatus(t, err, 403)
	_, err = s.runnerActorByHashTx(bounded, tx, digest(token))
	requireAuthorityStatus(t, err, 403)
	cleanup()
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateActor(ctx, issued.Token); err != nil {
		t.Fatalf("registered independent Grant followed browser logout: %v", err)
	}
}
