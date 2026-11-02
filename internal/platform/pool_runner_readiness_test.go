package platform

import (
	"context"
	"encoding/json"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPoolRunnerReadinessHTTPCurrentReportHandoff(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	r := p.credential.Runner
	unreported, e := f.s.GetExecutionPoolRunnerReadiness(context.Background(), f.actor, p.pool.ID, r.ID, f.user.TenantID, f.project)
	if e != nil || unreported.Scope.SourceReason != "inventory_unreported" || unreported.Scope.InventoryItemCount != nil || unreported.Runner.HistoricalLastSeenAt != "" {
		t.Fatal("unreported is not a current empty inventory", unreported, e)
	}
	ack, e := f.s.ReportPoolToolInventory(context.Background(), p.credential.Token, PoolToolInventoryRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: r.ID, OperationID: newID(), Items: []tc.Observation{{Profile: toolTestProfile(), Available: true}}, Maintenance: &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-continuous-readiness212"}})
	if e != nil {
		t.Fatal(e)
	}
	before := discoveryBusinessCounts(t, f)
	out := f.call("GET", "/execution-pools/"+p.pool.ID+"/runners/"+r.ID+"/readiness?tenant_id="+f.user.TenantID+"&project_id="+f.project, nil, 200)
	scope, ok := out["scope"].(map[string]any)
	if !ok || scope["can_open_project"] != true || scope["source_current"] != true || scope["source_reason"] != "current" || scope["inventory_observed_at"] != ack.ObservedAt || scope["inventory_item_count"] != float64(1) {
		t.Fatal("current report did not preserve exact project handoff", out)
	}
	if !reflect.DeepEqual(before, discoveryBusinessCounts(t, f)) {
		t.Fatal("readiness created business effects")
	}
	encoded, e := json.Marshal(out)
	if e != nil {
		t.Fatal(e)
	}
	for _, private := range []string{p.credential.Token, "token_hash", "configuration_sha256", "executable_path", "\"items\""} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("readiness disclosed private content", private)
		}
	}
	if _, e = f.s.ReportPoolToolInventory(context.Background(), p.credential.Token, PoolToolInventoryRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: r.ID, OperationID: newID(), Items: []tc.Observation{}, Maintenance: &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-continuous-readiness212"}}); e != nil {
		t.Fatal(e)
	}
	empty, e := f.s.GetExecutionPoolRunnerReadiness(context.Background(), f.actor, p.pool.ID, r.ID, f.user.TenantID, f.project)
	if e != nil || !empty.Scope.SourceCurrent || empty.Scope.InventoryItemCount == nil || *empty.Scope.InventoryItemCount != 0 {
		t.Fatal("current empty report was lost", empty, e)
	}
}

// A Pool owner's shared Grant is not a browser project proof, including when
// the same human account is also a member of the other tenant.
func TestPoolRunnerReadinessCurrentAuthorityAndRestrictedScope(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	r := p.credential.Runner
	ctx := context.Background()
	foreign := p.scopes[1]
	// Invalid foreign content must never be queried/decoded on this owner route.
	for _, statement := range []string{
		`INSERT INTO suma_tool_inventories(tenant_id,project_id,worker_id,observed_at,items)VALUES(?,?,?,'2026-11-02T00:00:00Z','not-json')`,
		`INSERT INTO suma_tool_maintenance(tenant_id,project_id,worker_id,identity_sha256,observed_at,payload)VALUES(?,?,?,'invalid','bad-time','not-json')`,
	} {
		if _, e := f.s.DB.Exec(statement, foreign.TenantID, foreign.ProjectID, r.WorkerID); e != nil {
			t.Fatal(e)
		}
	}
	got, e := f.s.GetExecutionPoolRunnerReadiness(ctx, f.actor, p.pool.ID, r.ID, foreign.TenantID, foreign.ProjectID)
	if e != nil || got.Scope.CanOpenProject || got.Scope.RouteState != "restricted" || got.Scope.SourceReason != "project_access_required" || got.Scope.InventoryObservedAt != "" || got.Scope.InventoryExpiresAt != "" || got.Scope.InventoryItemCount != nil || got.Scope.GrantID != p.grants[1].ID {
		t.Fatal("pool ownership expanded foreign project access", got, e)
	}
	base := "/execution-pools/" + p.pool.ID + "/runners/" + r.ID + "/readiness"
	for _, query := range []string{"?tenant_id=" + f.user.TenantID, "?tenant_id=" + f.user.TenantID + "&project_id=" + f.project + "&private_path=x", "?tenant_id=" + f.user.TenantID + "&project_id=" + f.project + "&project_id=" + f.project} {
		f.call("GET", base+query, nil, 400)
	}
	f.call("GET", base+"?tenant_id="+f.user.TenantID+"&project_id="+newID(), nil, 404)
	f.call("GET", "/execution-pools/"+p.pool.ID+"/runners/"+newID()+"/readiness?tenant_id="+f.user.TenantID+"&project_id="+f.project, nil, 404)
	before := discoveryBusinessCounts(t, f)
	if _, e = f.s.DB.Exec(`DELETE FROM suma_org_members WHERE tenant_id=? AND user_id=?`, f.user.TenantID, f.user.ID); e != nil {
		t.Fatal(e)
	}
	// Current authority precedes parsing even with a retained cookie/Actor.
	f.call("GET", base+"?private_path=x", nil, 403)
	_, e = f.s.GetExecutionPoolRunnerReadiness(ctx, f.actor, p.pool.ID, r.ID, f.user.TenantID, f.project)
	requireAuthorityStatus(t, e, 403)
	if !reflect.DeepEqual(before, discoveryBusinessCounts(t, f)) {
		t.Fatal("denied read created effects")
	}
}

func TestPoolRunnerReadinessRotationAndOriginalReportProvenance(t *testing.T) {
	p := newPoolMachineFixture(t)
	f := p.f
	r := p.credential.Runner
	ctx := context.Background()
	request := PoolToolInventoryRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: r.ID, OperationID: newID(), Items: []tc.Observation{{Profile: toolTestProfile(), Available: true}}, Maintenance: &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-continuous-readiness212"}}
	ack, e := f.s.ReportPoolToolInventory(ctx, p.credential.Token, request)
	if e != nil {
		t.Fatal(e)
	}
	get := func() PoolRunnerReadiness {
		t.Helper()
		out, e := f.s.GetExecutionPoolRunnerReadiness(ctx, f.actor, p.pool.ID, r.ID, f.user.TenantID, f.project)
		if e != nil {
			t.Fatal(e)
		}
		return out
	}
	current := get()
	if !current.Scope.SourceCurrent || current.Runner.HistoricalLastSeenAt != ack.ObservedAt {
		t.Fatal("missing original report", current)
	}
	rotated, e := f.s.RotateExecutionPoolCredential(ctx, f.actor, p.pool.ID, r.ID, ExecutionPoolCredentialRequest{ExpectedVersion: r.Version, TTLSeconds: 3600, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	old := get()
	if old.Runner.CredentialState != "valid" || old.Runner.CredentialVersion != rotated.Runner.CredentialVersion || old.Runner.HistoricalLastSeenAt != ack.ObservedAt || old.Scope.SourceCurrent || old.Scope.SourceReason != "maintenance_unknown" || old.Scope.InventoryItemCount != nil {
		t.Fatal("rotation promoted historical contact or old inventory", old)
	}
	request.OperationID = newID()
	ack, e = f.s.ReportPoolToolInventory(ctx, rotated.Token, request)
	if e != nil {
		t.Fatal(e)
	}
	if fresh := get(); !fresh.Scope.SourceCurrent || fresh.Scope.InventoryObservedAt != ack.ObservedAt {
		t.Fatal("new authenticated report did not establish source", fresh)
	}
	at, e := time.Parse(time.RFC3339Nano, ack.ObservedAt)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET observed_at=? WHERE tenant_id=? AND project_id=? AND worker_id=?`, at.Add(-time.Nanosecond).Format(time.RFC3339Nano), f.user.TenantID, f.project, r.WorkerID); e != nil {
		t.Fatal(e)
	}
	if mismatch := get(); mismatch.Scope.SourceCurrent || mismatch.Scope.SourceReason != "observation_mismatch" || mismatch.Scope.InventoryItemCount != nil {
		t.Fatal("different reports were paired", mismatch)
	}
	// Replay preserves its exact receipt and does not renew any source clock.
	stale := "2000-01-01T00:00:00.123456789Z"
	for _, table := range []string{"suma_tool_inventories", "suma_tool_maintenance"} {
		if _, e = f.s.DB.Exec(`UPDATE `+table+` SET observed_at=? WHERE tenant_id=? AND project_id=? AND worker_id=?`, stale, f.user.TenantID, f.project, r.WorkerID); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_runners SET last_seen_at=? WHERE id=?`, stale, r.ID); e != nil {
		t.Fatal(e)
	}
	replay, e := f.s.ReportPoolToolInventory(ctx, rotated.Token, request)
	if e != nil || replay.ObservedAt != ack.ObservedAt {
		t.Fatal("replay changed original observation", replay, e)
	}
	if expired := get(); expired.Scope.SourceCurrent || expired.Scope.SourceReason != "inventory_stale" || expired.Runner.HistoricalLastSeenAt != stale || expired.Scope.InventoryObservedAt != stale {
		t.Fatal("old ACK refreshed source", expired)
	}
	request.OperationID = newID()
	if _, e = f.s.ReportPoolToolInventory(ctx, rotated.Token, request); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET payload='not-json' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, r.WorkerID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.GetExecutionPoolRunnerReadiness(ctx, f.actor, p.pool.ID, r.ID, f.user.TenantID, f.project)
	if e == nil {
		t.Fatal("corrupt source was hidden as healthy or unknown")
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_credentials SET expires_at=? WHERE runner_id=?`, stale, r.ID); e != nil {
		t.Fatal(e)
	}
	if expired := get(); expired.Runner.CredentialState != "expired" || expired.Scope.SourceCurrent || expired.Scope.InventoryItemCount != nil {
		t.Fatal("expired credential became current", expired)
	}
}
