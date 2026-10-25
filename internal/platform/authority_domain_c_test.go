package platform

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func domainCUnauthenticated(t *testing.T, err error) {
	t.Helper()
	var api *APIError
	if !errors.As(err, &api) || api.Status != 401 {
		t.Fatalf("current credential refusal = %v, want HTTP401", err)
	}
}

func TestAuthorityDomainCPoolFamilyBeforeFreshAndReplay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := seedCredentialPool(t, f)
	request := ExecutionPoolRunnerCreateRequest{Name: "current credential registration", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()}
	issued, err := f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool, request)
	if err != nil {
		t.Fatal(err)
	}
	var before int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AuthenticateActor(ctx, f.cookie.Value)
	domainCUnauthenticated(t, err)
	machine, err := f.s.GetExecutionPoolRunnerSession(ctx, issued.Token, issued.Runner.ID)
	if err != nil || machine.Runner.ID != issued.Runner.ID {
		t.Fatal("ordinary browser logout revoked independent pool credential", err)
	}
	_, err = f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool, request)
	domainCUnauthenticated(t, err)
	request.IdempotencyKey = newID()
	_, err = f.s.RegisterExecutionPoolRunner(ctx, f.actor, pool, request)
	domainCUnauthenticated(t, err)
	var after int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_operations`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatal("revoked caller added operation receipt")
	}
}

func TestAuthorityDomainCRunnerChecksLedgerBeforeDispatch(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "runner", Label: "ledger-bound", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	actor, err := f.s.AuthenticateActor(ctx, issued.Token)
	if err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, f.s, issued.Token); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.AuthenticateActor(ctx, issued.Token)
	domainCUnauthenticated(t, err)
	bound, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cancel()
	defer tx.Rollback()
	_, err = f.s.runnerIdentityTx(bound, tx, actor, f.project, "ledger-worker")
	domainCUnauthenticated(t, err)
}

func domainCActor(t *testing.T, s *Service, token string) Actor {
	t.Helper()
	actor, err := s.AuthenticateActor(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return actor
}

func TestAuthorityDomainCWorkspaceInteractiveRechecksAndAcceptedRecoverySurvivesLogout(t *testing.T) {
	f, root := workspaceFixture(t)
	ctx := context.Background()
	body := f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "accepted durable content"})
	key := str(body, "idempotency_key")
	hash := digest(f.user.ID + stringify(map[string]any{"kind": "workspace.file", "body": body}))
	bound, tx, cancel, err := f.s.beginAuthorityTx(ctx)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := f.s.authorizeTx(bound, tx, f.actor, commandTarget{tenantID: f.user.TenantID, projectID: f.project}, purposeProjectWrite)
	if err == nil {
		err = f.s.prepareWorkspaceOperationTx(bound, auth, key, hash, root, "main.go", "original", "accepted durable content", fileResponse("main.go", "accepted durable content"))
	}
	if err == nil {
		err = tx.Commit()
	}
	tx.Rollback()
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.saveWorkspace(ctx, f.actor, f.project, body)
	domainCUnauthenticated(t, err)
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatal("interactive revoked finalize changed file", err)
	}
	// A private accepted-operation recovery uses only its persisted exact scope/hash.
	recovery, err := f.s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer recovery.Rollback()
	if err = f.s.workspaceLock(ctx, recovery, f.user, f.project); err == nil {
		err = f.s.recoverWorkspace(ctx, recovery, f.user, f.project)
	}
	if err == nil {
		err = recovery.Commit()
	}
	if err != nil {
		t.Fatal(err)
	}
	actual, err = os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "accepted durable content" {
		t.Fatal("trusted accepted recovery lost", err)
	}
}
func TestAuthorityDomainCWorkspaceLegacyPreparedIsNotAccepted(t *testing.T) {
	f, root := workspaceFixture(t)
	key := newID()
	_, err := f.s.DB.Exec(`INSERT INTO suma_workspace_file_operations(tenant_id,project_id,operation_id,request_hash,root_path,path,expected_hash,new_hash,content,response,state,created_at)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, f.user.TenantID, f.project, key, digest("legacy"), root, "main.go", digest("original"), digest("untrusted"), "untrusted", "{}", "prepared", now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.saveWorkspace(context.Background(), f.actor, f.project, f.body(map[string]any{"path": "main.go", "expected_hash": digest("original"), "content": "next"}))
	var api *APIError
	if !errors.As(err, &api) || api.Status != 409 {
		t.Fatalf("legacy recovery = %v", err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "main.go"))
	if err != nil || string(actual) != "original" {
		t.Fatal("legacy recovery changed file", err)
	}
	var accepted int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_workspace_accepted_operations WHERE operation_id=?`, key).Scan(&accepted); err != nil || accepted != 0 {
		t.Fatal("legacy accepted marker fabricated", err)
	}
}

func TestAuthorityDomainCHumanReadsRejectRevokedCredential(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := seedCredentialPool(t, f)
	if err := logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		read func() error
	}{
		{"workers", func() error { _, e := f.s.ReleaseWorkers(ctx, f.actor, f.project); return e }},
		{"artifact", func() error { _, e := f.s.ReleaseArtifact(ctx, f.actor, f.project, "missing"); return e }},
		{"candidate", func() error { _, e := f.s.ReleaseCandidate(ctx, f.actor, f.project, "missing"); return e }},
		{"candidate-list", func() error { _, _, _, e := f.s.ReleaseCandidates(ctx, f.actor, f.project, 20, ""); return e }},
		{"environments", func() error { _, e := f.s.ReleaseEnvironments(ctx, f.actor, f.project); return e }},
		{"plans", func() error { _, e := f.s.ReleasePlans(ctx, f.actor, f.project); return e }},
		{"deployments", func() error { _, e := f.s.ReleaseDeployments(ctx, f.actor, f.project); return e }},
		{"pool", func() error { _, e := f.s.GetExecutionPool(ctx, f.e, f.actor, pool); return e }},
		{"pool-list", func() error { _, e := f.s.ListExecutionPools(ctx, f.e, f.actor, 20, ""); return e }},
		{"workspace", func() error { _, e := f.s.workspaceRoot(ctx, f.actor, f.project); return e }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) { domainCUnauthenticated(t, check.read()) })
	}
}

func TestAuthorityDomainCProjectAPIReadScopeIsPreserved(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: "api", Label: "browser", IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	api := domainCActor(t, f.s, issued.Token)
	if api.IsBrowser() {
		t.Fatal("display label upgraded API credential")
	}
	if _, err = f.s.ReleaseWorkers(ctx, api, f.project); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ReleaseEnvironments(ctx, api, f.project); err != nil {
		t.Fatal(err)
	}
	actions, err := f.s.ReleaseEnvironmentActions(ctx, api, f.project)
	if err != nil || len(actions) != 0 {
		t.Fatal("API read advertised browser-only environment command", actions, err)
	}
	if _, err = f.s.ReleasePlans(ctx, api, f.project); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = f.s.ReleaseCandidates(ctx, api, f.project, 20, ""); err != nil {
		t.Fatal(err)
	}
	other, err := f.s.CreateProject(ctx, f.actor, map[string]any{"title": "other scope", "idempotency_key": newID()})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.ReleasePlans(ctx, api, other.ID)
	releaseWantStatus(t, err, 403)
	_, err = f.s.ListExecutionPools(ctx, f.e, api, 20, "")
	releaseWantStatus(t, err, 403)
}
