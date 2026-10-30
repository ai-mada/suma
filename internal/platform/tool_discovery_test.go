package platform

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
)

// These tests catch disclosure of a different worker/project's inventory,
// historical reports presented as current, and discovery creating execution.
func TestToolDiscoveryPrivateFreshReadHasNoBusinessEffects(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	unavailable := tc.Observation{Profile: tc.Profile{CommandID: "missing-command", Adapter: "command", Protocol: "text-v1"}, Reason: "Executable unavailable"}
	items := []tc.Observation{{Profile: profile.Profile, Available: true}, unavailable}
	receipt, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: items})
	if err != nil {
		t.Fatal(err)
	}
	before := discoveryBusinessCounts(t, f)
	got, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	if err != nil || got.State != "fresh" || got.ProjectID != f.project || got.WorkerID != profile.WorkerID || got.ObservedAt != receipt.ObservedAt || len(got.Items) != 2 {
		t.Fatalf("fresh authorized inventory was not returned: state=%s, items=%d, err=%v", got.State, len(got.Items), err)
	}
	observed, e := time.Parse(time.RFC3339, got.ObservedAt)
	expires, e2 := time.Parse(time.RFC3339, got.ExpiresAt)
	asOf, e3 := time.Parse(time.RFC3339, got.AsOf)
	if e != nil || e2 != nil || e3 != nil || expires.Sub(observed) != 60*time.Second || asOf.Before(observed) || !asOf.Before(expires) {
		t.Fatal("discovery lost the original database timestamp or expiry")
	}
	if !reflect.DeepEqual(got.Items, items) {
		t.Fatal("discovery replaced actual observations")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{token, "token_hash", "created_by", "raw_args", "environment", "executable_path"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatal("discovery disclosed private execution configuration")
		}
	}
	if !reflect.DeepEqual(discoveryBusinessCounts(t, f), before) {
		t.Fatal("discovery mutated business records or started execution")
	}
	var storedAt, storedItems string
	if err = f.s.DB.QueryRow(`SELECT observed_at,items FROM suma_tool_inventories WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, profile.WorkerID).Scan(&storedAt, &storedItems); err != nil || storedAt != receipt.ObservedAt || storedItems != stringify(items) {
		t.Fatal("discovery rewrote the original report", err)
	}
}

func TestToolDiscoveryUnreportedAndFreshEmptyRemainDistinct(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	got, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	if err != nil || got.State != "unreported" || got.ObservedAt != "" || got.ExpiresAt != "" || got.Items == nil || len(got.Items) != 0 {
		t.Fatalf("unreported inventory became an empty successful probe: state=%s, err=%v", got.State, err)
	}
	_, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{}})
	if err != nil {
		t.Fatal(err)
	}
	got, err = f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	if err != nil || got.State != "fresh" || got.ObservedAt == "" || got.Items == nil || len(got.Items) != 0 {
		t.Fatal("actual empty report lost its fresh fact", err)
	}
}

func TestToolDiscoveryStaleNeverReturnsHistoricalItems(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	request := ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true}}}
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, request); err != nil {
		t.Fatal(err)
	}
	for _, observed := range []string{"2000-01-01T00:00:00Z", "2999-01-01T00:00:00Z", "invalid-time"} {
		if _, err := f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at=? WHERE tenant_id=? AND project_id=? AND worker_id=?`, observed, f.user.TenantID, f.project, profile.WorkerID); err != nil {
			t.Fatal(err)
		}
		got, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
		if err != nil || got.State != "stale" || len(got.Items) != 0 || got.Items == nil {
			t.Fatalf("invalid or historical report became a candidate: state=%s, err=%v", got.State, err)
		}
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_tool_inventories SET observed_at=strftime('%Y-%m-%dT%H:%M:%SZ','now','-60 seconds') WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, profile.WorkerID); err != nil {
		t.Fatal(err)
	}
	boundary, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	boundaryExpiry, expiryErr := time.Parse(time.RFC3339Nano, boundary.ExpiresAt)
	boundaryAsOf, asOfErr := time.Parse(time.RFC3339Nano, boundary.AsOf)
	if err != nil || boundary.State != "stale" || len(boundary.Items) != 0 || expiryErr != nil || asOfErr != nil || boundaryExpiry.After(boundaryAsOf) {
		t.Fatal("inventory remained fresh at the sixty-second database boundary", err)
	}
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, request); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	if err != nil || got.State != "stale" || len(got.Items) != 0 {
		t.Fatal("replayed report refreshed discovery expiry", err)
	}
}

func TestToolDiscoveryUnknownAndRevokedWorkerFailClosed(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: profile.Profile, Available: true}}}); err != nil {
		t.Fatal(err)
	}
	for _, worker := range []string{"unknown-worker", "pool-unknown"} {
		got, err := f.s.DiscoverTools(ctx, f.actor, f.project, worker)
		requireAuthorityStatus(t, err, 403)
		if len(got.Items) != 0 {
			t.Fatal("unknown worker disclosed an inventory")
		}
	}
	if _, err := f.s.DB.Exec(`UPDATE suma_credential_authority SET revoked=1 WHERE token_hash=?`, digest(token)); err != nil {
		t.Fatal(err)
	}
	got, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	requireAuthorityStatus(t, err, 403)
	if len(got.Items) != 0 || got.State != "" {
		t.Fatal("revoked worker disclosed cached inventory")
	}
}

func TestToolDiscoveryPrivateExpiredCredentialAndCreatorPermission(t *testing.T) {
	for _, query := range []string{
		`UPDATE suma_sessions SET expires_at='2000-01-01T00:00:00Z' WHERE token_hash=?`,
		`UPDATE suma_release_workers SET status='revoked' WHERE token_hash=?`,
	} {
		f, token, profile := toolFixture(t)
		if _, err := f.s.DB.Exec(query, digest(token)); err != nil {
			t.Fatal(err)
		}
		_, err := f.s.DiscoverTools(context.Background(), f.actor, f.project, profile.WorkerID)
		requireAuthorityStatus(t, err, 403)
	}
	f, _, profile := toolFixture(t)
	// The current browser may still read, but its earlier Runner registration
	// no longer has the creator's project write authority.
	if _, err := f.s.DB.Exec(`UPDATE suma_org_members SET role='member' WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DB.Exec(`INSERT INTO suma_project_members(project_id,user_id,role)VALUES(?,?,'viewer')`, f.project, f.user.ID); err != nil {
		t.Fatal(err)
	}
	_, err := f.s.DiscoverTools(context.Background(), f.actor, f.project, profile.WorkerID)
	requireAuthorityStatus(t, err, 403)
}

func TestToolDiscoveryRequiresCurrentBrowserProjectScope(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	_, err := f.s.DiscoverTools(ctx, toolActorForTest(t, f, token), f.project, profile.WorkerID)
	requireAuthorityStatus(t, err, 403)
	for _, scope := range []string{"api", "agent"} {
		issued, err := f.s.IssueProjectCredential(ctx, f.actor, ProjectCredentialIssueRequest{ProjectID: f.project, Scope: scope, IdempotencyKey: newID()})
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.s.DiscoverTools(ctx, toolActorForTest(t, f, issued.Token), f.project, profile.WorkerID)
		requireAuthorityStatus(t, err, 403)
	}
	other := f.call("POST", "/projects", map[string]any{"name": "Another project", "idempotency_key": newID()}, 201)["id"].(string)
	_, err = f.s.DiscoverTools(ctx, f.actor, other, profile.WorkerID)
	requireAuthorityStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_projects SET status='archived' WHERE id=?`, f.project); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	requireAuthorityStatus(t, err, 403)
	if _, err = f.s.DB.Exec(`UPDATE suma_projects SET status='active' WHERE id=?`, f.project); err != nil {
		t.Fatal(err)
	}
	if err = logoutCredentialForTest(ctx, f.s, f.cookie.Value); err != nil {
		t.Fatal(err)
	}
	_, err = f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
	requireAuthorityStatus(t, err, 401)
}

func TestToolDiscoveryPoolRequiresExactCurrentRouteAndCredential(t *testing.T) {
	for _, test := range []struct{ name, query string }{
		{"route_removed", `DELETE FROM dag_project_execution_routes WHERE tenant_id=? AND project_id=?`},
		{"route_version_changed", `UPDATE dag_project_execution_routes SET grant_version=grant_version+1 WHERE tenant_id=? AND project_id=?`},
		{"grant_withdrawn", `UPDATE suma_execution_pool_grants SET status='withdrawn' WHERE tenant_id=? AND project_id=?`},
		{"pool_paused", `UPDATE suma_execution_pools SET status='paused' WHERE id=?`},
		{"runner_revoked", `UPDATE suma_execution_pool_runners SET status='revoked' WHERE pool_id=?`},
		{"credential_revoked", `UPDATE suma_execution_pool_credentials SET status='revoked' WHERE runner_id=?`},
		{"credential_expired", `UPDATE suma_execution_pool_credentials SET expires_at='2000-01-01T00:00:00Z' WHERE runner_id=?`},
		{"credential_invalid", `UPDATE suma_execution_pool_credentials SET expires_at='invalid' WHERE runner_id=?`},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := newPoolMachineFixture(t)
			f, runner := p.f, p.credential.Runner
			ctx := context.Background()
			_, err := f.s.ReportPoolToolInventory(ctx, p.credential.Token, PoolToolInventoryRequest{RunnerID: runner.ID, TenantID: f.user.TenantID, ProjectID: f.project, OperationID: newID(), Items: []tc.Observation{{Profile: toolTestProfile(), Available: true}}})
			if err != nil {
				t.Fatal(err)
			}
			before := discoveryBusinessCounts(t, f)
			got, err := f.s.DiscoverTools(ctx, f.actor, f.project, runner.WorkerID)
			if err != nil || got.State != "fresh" || len(got.Items) != 1 {
				t.Fatal("current delegated pool inventory unavailable", err)
			}
			if !reflect.DeepEqual(discoveryBusinessCounts(t, f), before) {
				t.Fatal("pool discovery dispatched or mutated business state")
			}
			var args []any
			switch {
			case strings.Contains(test.query, "tenant_id=?"):
				args = []any{f.user.TenantID, f.project}
			case strings.Contains(test.query, "runner_id=?"):
				args = []any{runner.ID}
			default:
				args = []any{p.pool.ID}
			}
			if _, err = f.s.DB.Exec(test.query, args...); err != nil {
				t.Fatal(err)
			}
			got, err = f.s.DiscoverTools(ctx, f.actor, f.project, runner.WorkerID)
			requireAuthorityStatus(t, err, 403)
			if got.State != "" || len(got.Items) != 0 {
				t.Fatal("historical pool consent exposed inventory after current authority failed")
			}
		})
	}
}

func TestToolDiscoveryRejectsInvalidStoredInventory(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	if _, err := f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: profile.WorkerID, OperationID: newID(), Items: []tc.Observation{}}); err != nil {
		t.Fatal(err)
	}
	tooMany := make([]tc.Observation, 129)
	for i := range tooMany {
		tooMany[i] = tc.Observation{Profile: profile.Profile, Available: true}
	}
	for _, raw := range []string{"null", `{}`, `[`, `[] {"raw_args":"must-not-escape"}`, stringify([]any{map[string]any{"profile": profile.Profile, "available": true, "reason": "", "raw_args": "must-not-escape"}}), strings.Repeat(" ", (2<<20)+1), stringify(tooMany), stringify([]tc.Observation{{Profile: profile.Profile, Available: true}, {Profile: profile.Profile, Available: true}})} {
		if _, err := f.s.DB.Exec(`UPDATE suma_tool_inventories SET items=? WHERE tenant_id=? AND project_id=? AND worker_id=?`, raw, f.user.TenantID, f.project, profile.WorkerID); err != nil {
			t.Fatal(err)
		}
		got, err := f.s.DiscoverTools(ctx, f.actor, f.project, profile.WorkerID)
		if err == nil || len(got.Items) != 0 {
			t.Fatal("invalid stored inventory became a fresh candidate list")
		}
	}
}

func discoveryBusinessCounts(t *testing.T, f *fixture) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"suma_operations", "suma_activity", "suma_tool_profiles", "suma_tool_inventories", "dag_runs", "dag_attempts", "dag_dispatches", "dag_execution_pool_dispatches"} {
		var count int
		if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		out[table] = count
	}
	return out
}
