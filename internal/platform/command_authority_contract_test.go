package platform

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func requireAuthorityStatus(t *testing.T, err error, status int) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != status {
		t.Fatalf("want status %d, got %v", status, err)
	}
}

func TestAuthorityActorRejectsZeroAndJSONConstruction(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil || !actor.IsBrowser() {
		t.Fatalf("real setup Actor: %v", err)
	}
	_, login, err := s.Login(ctx, "authority@example.test", "authority-test-password-42")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AuthenticateActor(ctx, login); err != nil {
		t.Fatal(err)
	}
	var forged Actor
	if err = json.Unmarshal([]byte(`{"kind":"browser-session","internal":true,"delegation":"trusted"}`), &forged); err == nil {
		t.Fatal("JSON constructed Actor")
	}
	if _, err = json.Marshal(actor); err == nil {
		t.Fatal("Actor was serialized")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = s.authorizeTx(ctx, tx, forged, commandTarget{tenantID: actor.projection.TenantID, projectID: project}, purposeProjectWrite)
	requireAuthorityStatus(t, err, 401)
}

func TestAuthorityTransactionProofBindsTxScopePurpose(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	target := commandTarget{tenantID: actor.projection.TenantID, projectID: project, resourceID: "task-1"}
	a, err := s.authorizeTx(ctx, tx, actor, target, purposeProjectWrite)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.assertTx(tx, target, purposeProjectWrite); err != nil {
		t.Fatal(err)
	}
	for _, wrong := range []commandTarget{{tenantID: target.tenantID, projectID: "other", resourceID: target.resourceID}, {tenantID: target.tenantID, projectID: project, resourceID: "other"}} {
		if err = a.assertTx(tx, wrong, purposeProjectWrite); err == nil {
			t.Fatal("proof accepted another target")
		}
	}
	if err = a.assertTx(tx, target, purposeProjectApprove); err == nil {
		t.Fatal("proof accepted another purpose")
	}
	if err = a.assertTx(nil, target, purposeProjectWrite); err == nil {
		t.Fatal("proof accepted another transaction")
	}
	hash, replay, err := s.commandReceipt(ctx, a, "proof-operation", map[string]any{"task": "one"})
	if err != nil || replay {
		t.Fatalf("fresh receipt: %v", err)
	}
	if err = s.saveCommandReceipt(ctx, a, "proof-operation", hash, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	tx, err = s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	_, err = s.authorizeTx(ctx, tx, actor, target, purposeProjectWrite)
	requireAuthorityStatus(t, err, 401)
}

func TestAuthorityKindIgnoresLabelAndTransport(t *testing.T) {
	s, _, token, project := authorityFixture(t)
	ctx := context.Background()
	browser, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	api, err := s.IssueProjectCredential(ctx, browser, ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", Label: "browser", IdempotencyKey: "api-kind-check"})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := s.AuthenticateActor(ctx, api.Token)
	if err != nil || actor.IsBrowser() {
		t.Fatalf("label elevated scoped kind: %v", err)
	}
	for _, tc := range []struct {
		a      Actor
		p      commandPurpose
		status int
	}{{actor, purposeProjectWrite, 0}, {actor, purposeBrowserProjectWrite, 403}, {browser, purposeProjectWrite, 0}} {
		tx, e := s.DB.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		_, e = s.authorizeTx(ctx, tx, tc.a, commandTarget{tenantID: browser.projection.TenantID, projectID: project}, tc.p)
		tx.Rollback()
		if tc.status == 0 && e != nil {
			t.Fatal(e)
		}
		if tc.status != 0 {
			requireAuthorityStatus(t, e, tc.status)
		}
	}
	if _, err = s.DB.Exec(`UPDATE suma_token_scopes SET kind='runner' WHERE token_hash=?`, digest(api.Token)); err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, api.Token)
	requireAuthorityStatus(t, err, 401)
}

func TestAuthorityDBTimeAndDeadline(t *testing.T) {
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
	// Use database-selected seconds, including a deliberately subsecond future
	// expiry. No application clock or sleep decides the expiry boundary.
	for _, expiry := range []time.Time{dbAt.Add(-time.Second), dbAt, dbAt.Add(999 * time.Millisecond)} {
		value := expiry.Format(time.RFC3339Nano)
		if _, err = s.DB.Exec(`UPDATE suma_sessions SET expires_at=? WHERE token_hash=?`, value, digest(token)); err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE suma_session_families SET expires_at=? WHERE id=?`, value, actor.credential.familyID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.DB.Exec(`UPDATE suma_credential_slots SET expires_at=? WHERE id=?`, value, actor.credential.slotID); err != nil {
			t.Fatal(err)
		}
		_, err = s.AuthenticateActor(ctx, token)
		requireAuthorityStatus(t, err, 401)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, tx, done, e := s.beginAuthorityTx(canceled); e == nil {
		done()
		tx.Rollback()
		t.Fatal("canceled transaction started")
	}
	bounded, tx, done, err := s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	defer tx.Rollback()
	deadline, ok := bounded.Deadline()
	if !ok || time.Until(deadline) > authorityTransactionBudget {
		t.Fatal("missing five second budget")
	}
	_ = project
}

func TestAuthorityAccountParticipantsAndMembershipSetChange(t *testing.T) {
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	otherOrg, otherUser := newID(), newID()
	if _, err = s.DB.Exec(`INSERT INTO suma_organizations(id,name,created_at) VALUES(?,?,?)`, otherOrg, "Other organization", now()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO suma_users(id,name,email,password_hash,created_at) VALUES(?,?,?,?,?)`, otherUser, "Target", "target@example.test", passwordHash("target-password-42", newID()), now()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`INSERT INTO suma_org_members(tenant_id,user_id,role) VALUES(?,?,'member')`, otherOrg, otherUser); err != nil {
		t.Fatal(err)
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project, subjectID: otherUser}, purposeProjectWrite)
	if err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	for _, user := range []string{p.ID, otherUser} {
		var seq int
		if err = tx.QueryRow(`SELECT sequence FROM suma_identity_authority_locks WHERE user_id=?`, user).Scan(&seq); err != nil || seq < 1 {
			t.Fatalf("missing sorted participant %s: %v", user, err)
		}
	}
	tx.Rollback()
	s.endAuthorityTx(tx)
	tx, err = s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = s.lockHumanAuthorities(ctx, tx, []string{p.ID, otherUser}, []string{p.TenantID}, credentialLockKeys(actor.credential))
	requireAuthorityStatus(t, err, 409)
	tx.Rollback()
	var count int
	if err = s.DB.QueryRow(`SELECT COUNT(*) FROM suma_identity_authority_locks WHERE user_id=?`, otherUser).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed candidate set left coordination mutation")
	}
	// A real issued source generation remains authoritative after a normal
	// browser logout, but an explicit auth-source generation revoke rejects it.
	issued, err := s.IssueProjectCredential(ctx, actor, ProjectCredentialIssueRequest{ProjectID: project, Scope: "api", IdempotencyKey: "source-generation-test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DB.Exec(`UPDATE suma_auth_sources SET generation=generation+1 WHERE id=?`, actor.credential.sourceID); err != nil {
		t.Fatal(err)
	}
	_, err = s.AuthenticateActor(ctx, issued.Token)
	requireAuthorityStatus(t, err, 401)
	_, err = s.AuthenticateActor(ctx, token)
	requireAuthorityStatus(t, err, 401)
}
