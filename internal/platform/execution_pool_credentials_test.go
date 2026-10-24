package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func seedCredentialPool(t *testing.T, f *fixture) string {
	t.Helper()
	id := newID()
	_, err := f.s.DB.Exec(`INSERT INTO suma_execution_pools(id,owner_tenant_id,name,description,version,status,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?)`, id, f.user.TenantID, "凭证边界夹具", "仅验证机器凭证，不代表资源池调度已实现", 1, "active", now(), now())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestExecutionPoolCredentialIsOneTimeAndNeverAUserSession(t *testing.T) {
	f := newFixture(t)
	pool := seedCredentialPool(t, f)
	request := ExecutionPoolRunnerCreateRequest{Name: "共享开发节点", MaxSlots: 2, TTLSeconds: 86400, IdempotencyKey: newID()}
	out, err := f.s.RegisterExecutionPoolRunner(context.Background(), f.user, f.cookie.Value, pool, request)
	if err != nil {
		t.Fatal(err)
	}
	if out.Token == "" || out.Replayed || out.Runner.WorkerID != "pool-"+out.Runner.ID || out.Runner.CredentialVersion != 1 {
		t.Fatal("missing canonical machine identity or first secret")
	}
	if _, err = f.s.Authenticate(context.Background(), out.Token); err == nil {
		t.Fatal("machine credential became a human browser session")
	}
	replayed, err := f.s.RegisterExecutionPoolRunner(context.Background(), f.user, f.cookie.Value, pool, request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Token != "" || !replayed.Replayed || replayed.Runner.ID != out.Runner.ID || replayed.RecoveryAction != "rotate_credential" {
		t.Fatal("duplicate issue returned another secret or identity")
	}
	var response string
	if err = f.s.DB.QueryRow(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`, f.user.TenantID, organizationScope, request.IdempotencyKey).Scan(&response); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(response, out.Token) {
		t.Fatal("raw credential persisted in business receipt")
	}
	if raw, _ := json.Marshal(replayed); strings.Contains(string(raw), out.Token) {
		t.Fatal("replay leaked the original secret")
	}
	var sessions, credentials int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_sessions WHERE token_hash=?`, digest(out.Token)).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_execution_pool_credentials WHERE token_hash=? AND status='active'`, digest(out.Token)).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 || credentials != 1 {
		t.Fatal("credential crossed storage boundary")
	}
}

func TestExecutionPoolCredentialCommandsRecheckCurrentHumanAndVersion(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	pool := seedCredentialPool(t, f)
	r, err := f.s.RegisterExecutionPoolRunner(ctx, f.user, f.cookie.Value, pool, ExecutionPoolRunnerCreateRequest{Name: "凭证轮换", MaxSlots: 1, TTLSeconds: 3600, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	rotation := ExecutionPoolCredentialRequest{ExpectedVersion: 1, TTLSeconds: 86400, IdempotencyKey: newID()}
	rotated, err := f.s.RotateExecutionPoolCredential(ctx, f.user, f.cookie.Value, pool, r.Runner.ID, rotation)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Token == "" || rotated.Token == r.Token || rotated.Runner.CredentialVersion != 2 || rotated.Runner.Version != 2 {
		t.Fatal("rotation failed to advance exact identity")
	}
	var old int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_execution_pool_credentials WHERE token_hash=? AND status='active'`, digest(r.Token)).Scan(&old); err != nil || old != 0 {
		t.Fatal("old secret remained active", err)
	}
	if _, err = f.s.RotateExecutionPoolCredential(ctx, f.user, f.cookie.Value, pool, r.Runner.ID, ExecutionPoolCredentialRequest{ExpectedVersion: 1, TTLSeconds: 86400, IdempotencyKey: newID()}); err == nil {
		t.Fatal("stale registration version rotated secret")
	}
	if _, err = f.s.RotateExecutionPoolCredential(ctx, f.user, f.cookie.Value, pool, r.Runner.ID, rotation); err != nil {
		t.Fatal("same rotation operation did not recover metadata", err)
	}
	if err = f.s.Logout(ctx, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.RotateExecutionPoolCredential(ctx, f.user, f.cookie.Value, pool, r.Runner.ID, rotation); err == nil {
		t.Fatal("logged out human replayed privileged operation")
	}
}
