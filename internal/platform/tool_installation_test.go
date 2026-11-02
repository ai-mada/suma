package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

func installationMaintenance205(t *testing.T, f *fixture, token, worker string, pool bool, runner string) {
	t.Helper()
	cap := &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-journal-205", InstalledDiscovery: &tii.Capability{Protocol: tii.Protocol, AdapterIDs: tii.AdapterIDs()}}
	var err error
	if pool {
		_, err = f.s.ReportPoolToolInventory(context.Background(), token, PoolToolInventoryRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: runner, OperationID: newID(), Items: []tc.Observation{}, Maintenance: cap})
	} else {
		_, err = f.s.ReportToolInventory(context.Background(), toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: worker, OperationID: newID(), Items: []tc.Observation{}, Maintenance: cap})
	}
	if err != nil {
		t.Fatal(err)
	}
}
func installationProof205(scan tii.Scan) *tii.StopProof {
	return &tii.StopProof{Protocol: tii.StopProtocol, Scan: tii.Reference{ScanID: scan.ID, ClaimID: scan.ClaimID, Fence: scan.Fence}, SourceSHA256: scan.SourceSHA256, ReceiptSHA256: strings.Repeat("a", 64), ProbeReturned: true, AllNativeTreesStopped: true, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
}
func installationResult205(scan tii.Scan) tii.ResultRequest {
	proof := installationProof205(scan)
	items := []tii.Item{}
	for _, id := range scan.AdapterIDs {
		items = append(items, tii.Item{AdapterID: id, Protocol: tii.AdapterProtocol(id), ReasonCode: "not_installed"})
	}
	return tii.ResultRequest{ProjectID: scan.ProjectID, WorkerID: scan.WorkerID, Scan: proof.Scan, IdempotencyKey: newID(), Outcome: "completed", Items: items, StopProof: proof}
}

func TestInstallationScanCurrentAuthorityDurabilityAndMaintenanceExclusion(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	worker := toolActorForTest(t, f, token)
	installationMaintenance205(t, f, token, profile.WorkerID, false, "")
	request := tii.CreateRequest{ProjectID: f.project, WorkerID: profile.WorkerID, AdapterIDs: []string{"codebuddy-cli", "claude-code-sdk"}, IdempotencyKey: newID()}
	created, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, request)
	if err != nil || created.State != "pending" {
		t.Fatal(created, err)
	}
	replayed, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, request)
	if err != nil || stringify(replayed) != stringify(created) {
		t.Fatal("original creation receipt changed", replayed, err)
	}
	second := request
	second.IdempotencyKey = newID()
	if _, err = f.s.CreateToolInstallationScan(ctx, f.actor, f.project, second); err == nil {
		t.Fatal("parallel maintenance admitted")
	}
	if _, err = f.s.CreateToolInspection(ctx, f.actor, f.project, profile.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: profile.Version, IdempotencyKey: newID()}); err == nil {
		t.Fatal("profile inspection bypassed scan occupancy")
	}
	claimReq := tii.ClaimRequest{ProjectID: f.project, WorkerID: profile.WorkerID, ClaimID: newID()}
	claim, err := f.s.ClaimToolInstallationScan(ctx, worker, f.project, claimReq)
	if err != nil || claim.Scan == nil || claim.Scan.SourceSHA256 != created.SourceSHA256 {
		t.Fatal(claim, err)
	}
	again, err := f.s.ClaimToolInstallationScan(ctx, worker, f.project, claimReq)
	if err != nil || stringify(again) != stringify(claim) {
		t.Fatal("lost claim ACK created another effect", again, err)
	}
	result := installationResult205(*claim.Scan)
	wrong := result
	wrong.Scan.Fence++
	if _, err = f.s.CompleteToolInstallationScan(ctx, worker, f.project, wrong); err == nil {
		t.Fatal("old fence accepted")
	}
	partial := result
	partial.Items = partial.Items[:1]
	if _, err = f.s.CompleteToolInstallationScan(ctx, worker, f.project, partial); err == nil {
		t.Fatal("partial coverage accepted")
	}
	done, err := f.s.CompleteToolInstallationScan(ctx, worker, f.project, result)
	if err != nil || done.State != "completed" {
		t.Fatal(done, err)
	}
	replayed, err = f.s.CompleteToolInstallationScan(ctx, worker, f.project, result)
	if err != nil || stringify(replayed) != stringify(done) {
		t.Fatal("lost result ACK changed outcome", replayed, err)
	}
	page, err := f.s.ListToolInstallationScans(ctx, f.actor, f.project, ToolInstallationScanListQuery{WorkerID: profile.WorkerID, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Maintenance.InstalledDiscovery == nil {
		t.Fatal(page, err)
	}
	if _, err = f.s.GetToolInstallationScan(ctx, worker, f.project, created.ID); err == nil {
		t.Fatal("machine gained browser history authority")
	}
	// A restart with an original stopped receipt may close unknown without a
	// second probe. It does not fabricate successful installation results.
	second.AdapterIDs = []string{"codebuddy-cli"}
	created2, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, second)
	if err != nil {
		t.Fatal(err)
	}
	claimReq.ClaimID = newID()
	next, err := f.s.ClaimToolInstallationScan(ctx, worker, f.project, claimReq)
	if err != nil || next.Scan == nil {
		t.Fatal(next, err)
	}
	unknown := tii.ResultRequest{ProjectID: f.project, WorkerID: profile.WorkerID, Scan: tii.Reference{ScanID: created2.ID, ClaimID: next.Scan.ClaimID, Fence: next.Scan.Fence}, IdempotencyKey: newID(), Outcome: "unknown", Items: []tii.Item{}, FailureCode: "worker_restarted"}
	head, err := f.s.CompleteToolInstallationScan(ctx, worker, f.project, unknown)
	if err != nil || head.State != "unknown" {
		t.Fatal(head, err)
	}
	second.IdempotencyKey = newID()
	if _, err = f.s.CreateToolInstallationScan(ctx, f.actor, f.project, second); err == nil {
		t.Fatal("unknown execution silently released occupancy")
	}
	closed := unknown
	closed.IdempotencyKey = newID()
	closed.Outcome = "closed_unknown"
	closed.StopProof = installationProof205(*next.Scan)
	head, err = f.s.CompleteToolInstallationScan(ctx, worker, f.project, closed)
	if err != nil || head.State != "closed_unknown" || head.ResultCode != "worker_restarted" {
		t.Fatal(head, err)
	}
	if _, err = f.s.CompleteToolInstallationScan(ctx, worker, f.project, closed); err != nil {
		t.Fatal("closure ACK did not recover", err)
	}
	page, err = f.s.ListToolInstallationScans(ctx, f.actor, f.project, ToolInstallationScanListQuery{WorkerID: profile.WorkerID, Limit: 1})
	if err != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	last, err := f.s.ListToolInstallationScans(ctx, f.actor, f.project, ToolInstallationScanListQuery{WorkerID: profile.WorkerID, Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(last.Items) != 1 || last.Items[0].ID == page.Items[0].ID {
		t.Fatal(last, err)
	}
	if _, err = f.s.ListToolInstallationScans(ctx, f.actor, f.project, ToolInstallationScanListQuery{WorkerID: "foreign", Limit: 1, Cursor: page.NextCursor}); err == nil {
		t.Fatal("cursor moved between workers")
	}
	inventory := f.bearer("POST", "/runner/tool-inventory", token, map[string]any{"project_id": f.project, "worker_id": profile.WorkerID, "operation_id": newID(), "items": []tc.Observation{}, "maintenance": map[string]any{"protocol": tic.MaintenanceProtocol, "instance_id": "owned-journal-205", "installed_discovery": nil}}, 400)
	_ = inventory
	var originalMaintenance string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_maintenance WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, profile.WorkerID).Scan(&originalMaintenance); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET payload='{}' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, profile.WorkerID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.GetToolInstallationScan(ctx, f.actor, f.project, created.ID); err == nil {
		t.Fatal("corrupt maintenance source was hidden as an ordinary identity-change hint")
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET payload=? WHERE tenant_id=? AND project_id=? AND worker_id=?`, originalMaintenance, f.user.TenantID, f.project, profile.WorkerID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, profile.WorkerID); err != nil {
		t.Fatal(err)
	}
	page, err = f.s.ListToolInstallationScans(ctx, f.actor, f.project, ToolInstallationScanListQuery{WorkerID: profile.WorkerID})
	if err != nil || len(page.Items) != 2 || page.Maintenance.State != "unknown" || page.Maintenance.InstalledDiscovery != nil || !page.Items[0].SourceChanged || !page.Items[1].SourceChanged {
		t.Fatal("revoked worker hid history or retained current installation eligibility", page, err)
	}
	if _, err = f.s.CreateToolInstallationScan(ctx, f.actor, f.project, second); err == nil {
		t.Fatal("revoked worker admitted a new scan")
	}
	if _, err = f.s.DB.Exec(`DELETE FROM suma_sessions WHERE token_hash=?`, f.actor.credential.digest); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ListToolInstallationScans(ctx, f.actor, f.project, ToolInstallationScanListQuery{WorkerID: profile.WorkerID}); err == nil {
		t.Fatal("revoked browser still read protected scan history")
	}
}

func TestInstallationScanPoolCurrentGrantAndRequiredProofFields(t *testing.T) {
	pool := newPoolMachineFixture(t)
	f := pool.f
	ctx := context.Background()
	c := pool.credential
	scope := pool.scopes[0]
	installationMaintenance205(t, f, c.Token, c.Runner.WorkerID, true, c.Runner.ID)
	scan, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, tii.CreateRequest{ProjectID: f.project, WorkerID: c.Runner.WorkerID, AdapterIDs: []string{"cursor-cli"}, IdempotencyKey: newID()})
	if err != nil {
		t.Fatal(err)
	}
	request := tii.PoolClaimRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: c.Runner.ID, ClaimID: newID()}
	claim, err := f.s.ClaimPoolToolInstallationScan(ctx, c.Token, request)
	if err != nil || claim.Scan == nil || claim.Scan.ID != scan.ID {
		t.Fatal(claim, err)
	}
	ordinary := installationResult205(*claim.Scan)
	result := tii.PoolResultRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: c.Runner.ID, Scan: ordinary.Scan, IdempotencyKey: ordinary.IdempotencyKey, Outcome: ordinary.Outcome, Items: ordinary.Items, StopProof: ordinary.StopProof}
	var raw map[string]any
	if json.Unmarshal([]byte(stringify(result)), &raw) != nil {
		t.Fatal("fixture encoding")
	}
	proof := raw["stop_proof"].(map[string]any)
	delete(proof, "native_process_count")
	delete(proof, "stopped_process_count")
	f.bearer("POST", "/pool-runner/tool-installation-scans/results", c.Token, raw, 400)
	raw["stop_proof"] = nil
	f.bearer("POST", "/pool-runner/tool-installation-scans/results", c.Token, raw, 400)
	out, err := f.s.CompletePoolToolInstallationScan(ctx, c.Token, result)
	if err != nil || out.State != "completed" {
		t.Fatal(out, err)
	}
	if _, err = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET version=version+1 WHERE id=?`, pool.grants[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.CompletePoolToolInstallationScan(ctx, c.Token, result); err == nil {
		t.Fatal("old result receipt bypassed current exact grant")
	}
}

func TestInstallationScanReceiptRollbackAndInitialSchema(t *testing.T) {
	f, token, profile := toolFixture(t)
	ctx := context.Background()
	installationMaintenance205(t, f, token, profile.WorkerID, false, "")
	req := tii.CreateRequest{ProjectID: f.project, WorkerID: profile.WorkerID, AdapterIDs: []string{"codebuddy-cli"}, IdempotencyKey: newID()}
	if _, err := f.s.DB.Exec(`CREATE TRIGGER owned_scan_receipt_failure BEFORE INSERT ON suma_operations WHEN NEW.response LIKE '%"adapter_ids"%' BEGIN SELECT RAISE(ABORT,'owned receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, req); err == nil {
		t.Fatal("failed receipt committed scan")
	}
	var count int
	if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_tool_installation_scans`).Scan(&count); err != nil || count != 0 {
		t.Fatal("scan survived rolled-back receipt", count, err)
	}
	if _, err := f.s.DB.Exec(`DROP TRIGGER owned_scan_receipt_failure`); err != nil {
		t.Fatal(err)
	}
	scan, err := f.s.CreateToolInstallationScan(ctx, f.actor, f.project, req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DB.Exec(`ALTER TABLE suma_tool_installation_scans ADD COLUMN unsupported TEXT`); err != nil {
		t.Fatal(err)
	}
	if err = f.s.Initialize(ctx); !errors.Is(err, store.ErrInitialSchema) {
		t.Fatal("unregistered layout was reset or repaired", err)
	}
	var saved string
	if err = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_installation_scans WHERE id=?`, scan.ID).Scan(&saved); err != nil || !strings.Contains(saved, scan.ID) {
		t.Fatal("schema refusal changed original data", err)
	}
}
