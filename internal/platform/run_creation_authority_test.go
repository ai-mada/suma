package platform

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

// A valid session at request ingress does not authorize a delayed scheduler
// transaction after a project administrator has committed revocation.
func TestRunCreationAuthorityPrecedesReceiptReplayAndBinding(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	ownerCookie := f.cookie
	inv := governanceInvite(f, "run-creator@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "developer"}})
	governanceAccept(f, inv, "run-creator@example.test")
	creator, err := f.s.Authenticate(ctx, f.cookie.Value)
	if err != nil {
		t.Fatal(err)
	}
	w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{ID: "authorized-run", Name: "固定流程", Version: 1, Nodes: []dag.Node{{ID: "work", Name: "实际工作", Type: "command", Command: []string{"allowed-command"}, TimeoutSeconds: 30}}})
	if err != nil {
		t.Fatal(err)
	}
	before := func(tx *sql.Tx) error { return f.s.authorizeProjectTx(ctx, tx, creator, f.project, true, false) }
	bound := 0
	bind := func(tx *sql.Tx, runID string) error { bound++; return nil }
	first, err := f.e.CreateRunBoundAuthorized(ctx, creator.TenantID, f.project, w.ID, w.Version, "authorized-run-operation", "binding-v1", before, bind)
	if err != nil || first.ID == "" || bound != 1 {
		t.Fatalf("initial authorized binding: run=%s bound=%d err=%v", first.ID, bound, err)
	}
	f.cookie = ownerCookie
	f.call("POST", "/project-access", map[string]any{"project_id": f.project, "user_id": creator.ID, "action": "remove", "expected_version": 1, "idempotency_key": newID()}, 200)
	for _, op := range []string{"authorized-run-operation", "new-operation-after-revocation"} {
		run, e := f.e.CreateRunBoundAuthorized(ctx, creator.TenantID, f.project, w.ID, w.Version, op, "binding-v1", before, bind)
		var ae *APIError
		if !errors.As(e, &ae) || ae.Status != 403 || run.ID != "" {
			t.Fatalf("cached principal obtained run after revocation: run=%s err=%v", run.ID, e)
		}
	}
	var runs int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_runs WHERE tenant_id=? AND project_id=?`, creator.TenantID, f.project).Scan(&runs); err != nil || runs != 1 || bound != 1 {
		t.Fatalf("revocation changed binding or run count: runs=%d bound=%d err=%v", runs, bound, err)
	}
}
