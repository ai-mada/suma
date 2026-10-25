package platform

import (
	"context"
	"testing"
)

func TestAuthorityCurrentReadRejectsRevokedSourceBeforeData(t *testing.T) {
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	target := commandTarget{tenantID: p.TenantID, projectID: project}
	selected := 0
	read := func(a *authorizedCommand) error {
		if err := a.assertTx(a.tx, target, purposeProjectRead); err != nil {
			return err
		}
		return a.tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_projects WHERE tenant_id=? AND id=?`), a.currentPrincipal.TenantID, project).Scan(&selected)
	}
	if err = s.withCurrentRead(ctx, actor, target, purposeProjectRead, read); err != nil || selected != 1 {
		t.Fatalf("same-transaction read failed: %d %v", selected, err)
	}
	if err = logoutCredentialForTest(ctx, s, token); err != nil {
		t.Fatal(err)
	}
	called := false
	err = s.withCurrentRead(ctx, actor, target, purposeProjectRead, func(*authorizedCommand) error { called = true; return nil })
	requireAuthorityStatus(t, err, 401)
	if called {
		t.Fatal("revoked credential reached data callback")
	}
	var operations, activities int
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&operations)
	s.DB.QueryRow(`SELECT COUNT(*) FROM suma_activity`).Scan(&activities)
	if operations != 0 || activities != 0 {
		t.Fatal("protected read/logout created success business receipt or activity")
	}
}

func TestAuthorityParticipantsExpandLocksWithoutAuthority(t *testing.T) {
	s, p, token, project := authorityFixture(t)
	ctx := context.Background()
	actor, err := s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	other := newID()
	if _, err = s.DB.Exec(`INSERT INTO suma_organizations(id,name,created_at) VALUES(?,?,?)`, other, "Pool owner", now()); err != nil {
		t.Fatal(err)
	}
	bounded, tx, cleanup, err := s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer tx.Rollback()
	a, err := s.authorizeTxWithParticipants(bounded, tx, actor, commandTarget{tenantID: p.TenantID, projectID: project}, purposeProjectWrite, []string{other})
	if err != nil {
		t.Fatal(err)
	}
	var sequence int
	if err = tx.QueryRow(`SELECT sequence FROM suma_governance_locks WHERE tenant_id=?`, other).Scan(&sequence); err != nil || sequence < 1 {
		t.Fatal("bilateral participant was not locked")
	}
	if a.currentPrincipal.TenantID != p.TenantID {
		t.Fatal("coordination org became authority tenant")
	}
	tx.Rollback()
	cleanup()
	bounded, tx, cleanup, err = s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	defer tx.Rollback()
	_, err = s.authorizeTxWithParticipants(bounded, tx, actor, commandTarget{tenantID: other}, purposeOrganizationWrite, []string{p.TenantID, other})
	requireAuthorityStatus(t, err, 403)
}
