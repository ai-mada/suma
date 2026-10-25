package platform

import (
	"context"
	"database/sql"
	"testing"

	"github.com/ai-mada/suma/internal/store"
)

func TestRunnerClaimRechecksCredentialAfterOuterAuthentication(t *testing.T) {
	for _, change := range []string{"logout", "expired"} {
		t.Run(change, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			w := f.call("POST", "/workflows", f.body(map[string]any{"name": "事务内重新核验执行凭证", "nodes": []any{map[string]any{"id": "work", "type": "command", "command": []string{"inspect"}}}}), 201)
			f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
			token := f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
			operation := newID()
			first := f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": "generic-worker", "capabilities": []string{"command:inspect"}, "idempotency_key": operation}, 200)
			if first["assignment"] == nil {
				t.Fatal("no original dispatch")
			}
			// Reproduce authentication having finished before another connection
			// revokes/expires the credential, without forging a Principal.
			actor, err := f.s.AuthenticateActor(ctx, token)
			if err != nil {
				t.Fatal(err)
			}
			var sequence int
			var name, path string
			if err = f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
				t.Fatal(err)
			}
			other, err := store.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			if change == "logout" {
				err = logoutCredentialForTest(ctx, &Service{DB: other, Dialect: "sqlite"}, token)
			} else {
				_, err = other.ExecContext(ctx, `UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`, digest(token))
			}
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.e.ClaimWithOperation(ctx, actor.Projection().TenantID, f.project, "generic-worker", operation, []string{"command:inspect"}, func(tx *sql.Tx) error {
				_, err := f.s.runnerIdentityTx(ctx, tx, actor, f.project, "generic-worker")
				return err
			}, nil)
			if err == nil {
				t.Fatal("revoked/expired credential recovered an executable receipt using cached outer authentication")
			}
			var count int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts`).Scan(&count); err != nil || count != 1 {
				t.Fatalf("rejected recovery changed original attempt: %d %v", count, err)
			}
		})
	}
}

func TestSchedulingCommitRechecksSessionBeforeSuccessfulReceipt(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	preview := schedulingPreview(f, 2, nil)
	request := SchedulingPolicyCommitRequest{ProjectID: f.project, ExpectedVersion: 1, IdempotencyKey: newID()}
	f.call("POST", "/scheduling/policy-previews/"+preview["id"].(string)+"/commit", request, 200)
	token := f.cookie.Value
	actor, err := f.s.AuthenticateActor(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	var sequence int
	var name, path string
	if err = f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &name, &path); err != nil {
		t.Fatal(err)
	}
	other, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if err = logoutCredentialForTest(ctx, &Service{DB: other, Dialect: "sqlite"}, token); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.CommitSchedulingPreview(ctx, f.e, actor, f.project, preview["id"].(string), request)
	if err == nil {
		t.Fatal("logged-out browser session recovered privileged policy receipt using cached outer authentication")
	}
}
