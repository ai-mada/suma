package platform

import (
	"context"
	"testing"
)

func TestBoundSubworkflowScopeProvesExactLockWithoutExpandingOrganizations(t *testing.T) {
	f := newFixture(t)
	ctx, tx, close, err := f.s.beginAuthorityTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	defer tx.Rollback()
	if _, err = f.s.persistedReleaseAdminLockedTx(ctx, tx, f.actor.credential.digest, f.user.ID, f.user.TenantID, f.project); err == nil {
		t.Fatal("an authority budget alone became an organization lock")
	}
	cleanup, err := f.s.BeforeBoundSubworkflowScope(ctx, tx, f.user.TenantID, f.project)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err = tx.QueryRowContext(ctx, `SELECT sequence FROM suma_governance_locks WHERE tenant_id=?`, f.user.TenantID).Scan(&before); err != nil {
		t.Fatal(err)
	}
	again, err := f.s.BeforeBoundSubworkflowScope(ctx, tx, f.user.TenantID, f.project)
	defer again()
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.QueryRowContext(ctx, `SELECT sequence FROM suma_governance_locks WHERE tenant_id=?`, f.user.TenantID).Scan(&after); err != nil || before != after {
		t.Fatal("a repeated exact scope took another lock/write", before, after, err)
	}
	other := newID()
	deny, err := f.s.BeforeBoundSubworkflowScope(ctx, tx, other, f.project)
	defer deny()
	if err == nil {
		t.Fatal("a late organization expanded the lock set")
	}
	var count int
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM suma_governance_locks WHERE tenant_id=?`, other).Scan(&count); err != nil || count != 0 {
		t.Fatal("refused scope acquired another organization", count, err)
	}
}

func TestPersistedReleaseScopeRequiresActualBrowserCredentialAndCurrentRights(t *testing.T) {
	f := newFixture(t)
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "scope-machine125", "profiles": []any{}, "deployment": false}), 201)
	ctx, tx, close, err := f.s.beginAuthorityTx(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	defer tx.Rollback()
	cleanup, err := f.s.BeforeBoundSubworkflowScope(ctx, tx, f.user.TenantID, f.project)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	prove := func(hash, subject string) error {
		auth, e := f.s.persistedReleaseAdminLockedTx(ctx, tx, hash, subject, f.user.TenantID, f.project)
		if e == nil && (auth.actor.kind != actorBrowser || auth.currentPrincipal.ID != f.user.ID) {
			t.Fatal("wrong authenticated identity")
		}
		return e
	}
	if err = prove(f.actor.credential.digest, f.user.ID); err != nil {
		t.Fatal("original actual browser credential failed", err)
	}
	if prove(f.actor.credential.digest, newID()) == nil || prove(digest(worker["token"].(string)), f.user.ID) == nil {
		t.Fatal("subject hint or machine credential manufactured a release administrator")
	}
	// This protocol-level fault is rolled back; it proves current database rights
	// are re-read rather than trusting the earlier authenticated projection.
	if _, err = tx.ExecContext(ctx, f.s.q(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`), f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, f.s.q(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`), f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if prove(f.actor.credential.digest, f.user.ID) == nil {
		t.Fatal("old browser projection survived current rights removal")
	}
}
