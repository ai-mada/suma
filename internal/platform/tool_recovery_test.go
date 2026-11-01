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
)

// This is an owned authenticated machine contract fixture. Native process
// containment is verified separately by Root's single real Worker journey.
func inspectionMaintenance194(t *testing.T, f *fixture, token string, p ToolProfile) ToolInventoryReceipt {
	t.Helper()
	capability := &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-continuous-journal194"}
	ctx := context.Background()
	var ack ToolInventoryReceipt
	var err error
	if strings.HasPrefix(p.WorkerID, "pool-") {
		ack, err = f.s.ReportPoolToolInventory(ctx, token, PoolToolInventoryRequest{TenantID: f.user.TenantID, ProjectID: f.project, RunnerID: strings.TrimPrefix(p.WorkerID, "pool-"), OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}, Maintenance: capability})
	} else {
		ack, err = f.s.ReportToolInventory(ctx, toolActorForTest(t, f, token), f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}, Maintenance: capability})
	}
	if err != nil || ack.Maintenance == nil || *ack.Maintenance != *capability {
		t.Fatal("authenticated continuous declaration lost exact ACK", ack, err)
	}
	return ack
}

func TestToolRecoveryPoolCurrentGrantExpiryAndInitialSchema(t *testing.T) {
	h := newPoolMachineFixture(t)
	f := h.f
	ctx := context.Background()
	credential := h.credential
	scope := h.scopes[0]
	profile, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: credential.Runner.WorkerID, Name: "Owned Pool recovery contract", Profile: toolTestProfile(), IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	inspectionMaintenance194(t, f, credential.Token, profile)
	created := inspectionCreate194(t, f, profile)
	claim, e := f.s.ClaimPoolToolInspection(ctx, credential.Token, tic.PoolToolInspectionClaimRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, ClaimID: newID()})
	if e != nil || claim.Inspection == nil {
		t.Fatal(claim, e)
	}
	ref := tic.Reference{InspectionID: created.ID, ClaimID: claim.Inspection.ClaimID, Fence: claim.Inspection.Fence}
	unknown, e := f.s.FailPoolToolInspection(ctx, credential.Token, tic.PoolToolInspectionFailureRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, Inspection: ref, IdempotencyKey: newID(), Outcome: "unknown", FailureCode: "probe_failed"})
	if e != nil {
		t.Fatal(e)
	}
	approver := invitedReviewer(t, f)
	intent := tic.ToolInspectionRecoveryCreateRequest{ProjectID: f.project, ExpectedVersion: unknown.Version, Inspection: ref, IdempotencyKey: newID()}
	recovery, e := f.s.CreateToolInspectionRecovery(ctx, approver, f.project, created.ID, intent)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.PollPoolToolInspectionRecovery(ctx, credential.Token, tic.PoolToolInspectionRecoveryPollRequest{TenantID: h.scopes[1].TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, Inspection: ref}); e == nil {
		t.Fatal("another tenant's grant read the original recovery")
	}
	var raw string
	if e = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_inspection_recoveries WHERE id=?`, recovery.ID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var expired toolRecoveryRow
	json.Unmarshal([]byte(raw), &expired)
	at, _ := time.Parse(time.RFC3339Nano, expired.Value.CreatedAt)
	expired.Value.ExpiresAt = at.Add(time.Nanosecond).Format(time.RFC3339Nano)
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_inspection_recoveries SET payload=? WHERE id=?`, stringify(expired), recovery.ID); e != nil {
		t.Fatal(e)
	}
	proof := tic.ToolInspectionStopProof{Protocol: tic.ProbeStopProtocol, Inspection: ref, SourceSHA256: tic.InspectionSourceSHA256(unknown), ReceiptSHA256: digest("owned-pool-stop-proof195"), ProbeReturned: true, AllNativeTreesStopped: true, NoPendingInventory: true, NativeProcessCount: 0, StoppedProcessCount: 0, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	proofReq := tic.PoolToolInspectionRecoveryProofRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, RecoveryID: recovery.ID, ExpectedVersion: recovery.Version, IdempotencyKey: newID(), Proof: proof}
	if _, e = f.s.StorePoolToolInspectionRecoveryProof(ctx, credential.Token, proofReq); e == nil {
		t.Fatal("expired conditional authorization accepted stop proof")
	}
	read, e := f.s.GetToolInspectionRecovery(ctx, f.actor, f.project, recovery.ID)
	if e != nil || read.State != "expired" {
		t.Fatal(read, e)
	}
	intent.IdempotencyKey = newID()
	recovery, e = f.s.CreateToolInspectionRecovery(ctx, approver, f.project, created.ID, intent)
	if e != nil {
		t.Fatal("expired recovery did not permit a new explicit grant", e)
	}
	proofReq.RecoveryID = recovery.ID
	proofReq.ExpectedVersion = recovery.Version
	proofReq.IdempotencyKey = newID()
	body := map[string]any{"tenant_id": scope.TenantID, "project_id": scope.ProjectID, "runner_id": credential.Runner.ID, "recovery_id": recovery.ID, "expected_version": recovery.Version, "idempotency_key": newID()}
	var missing map[string]any
	json.Unmarshal([]byte(stringify(proof)), &missing)
	delete(missing, "native_process_count")
	delete(missing, "stopped_process_count")
	body["proof"] = missing
	f.bearer("POST", "/pool-runner/tool-inspections/recovery/proofs", credential.Token, body, 400)
	body["proof"] = nil
	f.bearer("POST", "/pool-runner/tool-inspections/recovery/proofs", credential.Token, body, 400)
	f.bearer("POST", "/pool-runner/tool-inventory", credential.Token, map[string]any{"tenant_id": scope.TenantID, "project_id": scope.ProjectID, "runner_id": credential.Runner.ID, "operation_id": newID(), "items": []tc.Observation{}, "maintenance": nil}, 400)
	stored, e := f.s.StorePoolToolInspectionRecoveryProof(ctx, credential.Token, proofReq)
	if e != nil || stored.State != "proof_stored" {
		t.Fatal(stored, e)
	}
	ack := tic.PoolToolInspectionRecoveryAcknowledgementRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, RecoveryID: stored.ID, ExpectedVersion: stored.Version, IdempotencyKey: newID(), ProofSHA256: stored.ProofSHA256}
	if _, e = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, approver.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.AcknowledgePoolToolInspectionRecovery(ctx, credential.Token, ack); e == nil {
		t.Fatal("withdrawn current recovery approval closed unknown")
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_project_members SET role='reviewer' WHERE project_id=? AND user_id=?`, f.project, approver.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	closed, e := f.s.AcknowledgePoolToolInspectionRecovery(ctx, credential.Token, ack)
	if e != nil || closed.State != "closed_unknown" {
		t.Fatal(closed, e)
	}
	inspectionMaintenance194(t, f, credential.Token, profile)
	fresh := inspectionCreate194(t, f, profile)
	if fresh.ID == created.ID {
		t.Fatal("Pool reused an unknown execution")
	}
	var before, after string
	if e = f.s.DB.QueryRow(`SELECT items FROM suma_tool_inventories WHERE project_id=? AND worker_id=?`, f.project, profile.WorkerID).Scan(&before); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReportPoolToolInventory(ctx, credential.Token, PoolToolInventoryRequest{TenantID: scope.TenantID, ProjectID: scope.ProjectID, RunnerID: credential.Runner.ID, OperationID: newID(), Inspection: &ref, Items: []tc.Observation{}}); e == nil {
		t.Fatal("closed reference overwrote inventory despite unchanged profile/current initiator")
	}
	if e = f.s.DB.QueryRow(`SELECT items FROM suma_tool_inventories WHERE project_id=? AND worker_id=?`, f.project, profile.WorkerID).Scan(&after); e != nil || before != after {
		t.Fatal("late report escaped rollback", e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_execution_pool_grants SET version=version+1 WHERE id=?`, h.grants[0].ID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.AcknowledgePoolToolInspectionRecovery(ctx, credential.Token, ack); e == nil {
		t.Fatal("old closure ACK bypassed current exact Pool route/grant")
	}
	if _, e = f.s.DB.Exec(`ALTER TABLE suma_tool_maintenance ADD COLUMN unsupported TEXT`); e != nil {
		t.Fatal(e)
	}
	if e = f.s.Initialize(ctx); !errors.Is(e, store.ErrInitialSchema) {
		t.Fatal("unknown maintenance schema was reset or upgraded", e)
	}
	var retained string
	if e = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_inspection_recoveries WHERE id=?`, closed.ID).Scan(&retained); e != nil || !strings.Contains(retained, closed.ProofSHA256) {
		t.Fatal("initial rejection lost original evidence", e)
	}
}
func TestToolRecoveryCapabilitiesUseIndependentWatermarkAndGateCreation(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	worker := toolActorForTest(t, f, token)
	current, e := f.s.GetToolProfile(ctx, f.actor, f.project, p.ID)
	if e != nil || current.Maintenance == nil || current.Maintenance.State != "unknown" || current.Maintenance.CheckedAt == "" || current.Maintenance.Protocol != "" {
		t.Fatal(current.Maintenance, e)
	}
	if _, e = f.s.CreateToolInspection(ctx, f.actor, f.project, p.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}); e == nil {
		t.Fatal("registered tool implied maintenance support")
	}
	req := ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}}
	if _, e = f.s.ReportToolInventory(ctx, worker, f.project, req); e != nil {
		t.Fatal(e)
	}
	current, e = f.s.GetToolProfile(ctx, f.actor, f.project, p.ID)
	if e != nil || current.Maintenance.State != "unknown" {
		t.Fatal("one-off inventory advertised continuous maintenance", current.Maintenance, e)
	}
	req.OperationID = newID()
	req.Maintenance = &tic.ToolMaintenanceCapability{Protocol: tic.MaintenanceProtocol, InstanceID: "owned-capability195"}
	ack, e := f.s.ReportToolInventory(ctx, worker, f.project, req)
	if e != nil || ack.Maintenance == nil {
		t.Fatal(ack, e)
	}
	current, e = f.s.GetToolProfile(ctx, f.actor, f.project, p.ID)
	if e != nil || current.Maintenance.State != "supported" {
		t.Fatal(current.Maintenance, e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET observed_at='2000-01-01T00:00:00Z'`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.ReportToolInventory(ctx, worker, f.project, req); e != nil {
		t.Fatal(e)
	}
	current, e = f.s.GetToolProfile(ctx, f.actor, f.project, p.ID)
	if e != nil || current.Maintenance.State != "expired" || current.Maintenance.ObservedAt != "2000-01-01T00:00:00Z" {
		t.Fatal("old ACK renewed independent capability watermark", current.Maintenance, e)
	}
	if _, e = f.s.CreateToolInspection(ctx, f.actor, f.project, p.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()}); e == nil {
		t.Fatal("expired declaration authorized new inspection")
	}
	req.OperationID = newID()
	req.Maintenance = &tic.ToolMaintenanceCapability{Protocol: "suma-tool-maintenance-v2", InstanceID: "owned-capability195"}
	if _, e = f.s.ReportToolInventory(ctx, worker, f.project, req); e != nil {
		t.Fatal(e)
	}
	current, e = f.s.GetToolProfile(ctx, f.actor, f.project, p.ID)
	if e != nil || current.Maintenance.State != "unsupported" {
		t.Fatal(current.Maintenance, e)
	}
	f.bearer("POST", "/runner/tool-inventory", token, map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "operation_id": newID(), "items": []tc.Observation{}, "maintenance": nil}, 400)
}

func TestToolRecoveryUnknownClosesOnlyAfterExactProofAndMachineAcceptance(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	worker := toolActorForTest(t, f, token)
	inspectionMaintenance194(t, f, token, p)
	initiator := invitedReviewer(t, f)
	created, e := f.s.CreateToolInspection(ctx, initiator, f.project, p.ID, tic.CreateRequest{ProjectID: f.project, ExpectedVersion: p.Version, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	claim, e := f.s.ClaimToolInspection(ctx, worker, f.project, tic.ClaimRequest{ProjectID: f.project, WorkerID: p.WorkerID, ClaimID: newID()})
	if e != nil || claim.Inspection == nil {
		t.Fatal(claim, e)
	}
	ref := tic.Reference{InspectionID: created.ID, ClaimID: claim.Inspection.ClaimID, Fence: claim.Inspection.Fence}
	inspectionChangeClock194(t, f, created.ID, func(v *tic.Inspection) {
		at, _ := time.Parse(time.RFC3339Nano, v.ClaimedAt)
		v.LeaseUntil = at.Add(time.Nanosecond).Format(time.RFC3339Nano)
	})
	if _, e = f.s.DB.Exec(`UPDATE suma_project_members SET role='viewer' WHERE project_id=? AND user_id=?`, f.project, initiator.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_profiles SET version=version+1 WHERE id=?`, p.ID); e != nil {
		t.Fatal(e)
	}
	p.Version++
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET observed_at='2000-01-01T00:00:00Z'`); e != nil {
		t.Fatal(e)
	}
	head, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || head.State != "unknown" || head.ResultCode != "probe_timeout" || !head.SourceChanged {
		t.Fatal("projection hid unresolved original effects", head, e)
	}
	recoveryReq := tic.ToolInspectionRecoveryCreateRequest{ProjectID: f.project, ExpectedVersion: head.Version, Inspection: ref, IdempotencyKey: newID()}
	recovery, e := f.s.CreateToolInspectionRecovery(ctx, f.actor, f.project, created.ID, recoveryReq)
	if e != nil || recovery.State != "requested" || recovery.InspectionVersion != head.Version+1 {
		t.Fatal("current replacement manager could not reconcile original lease timeout using stale last-known capability", recovery, e)
	}
	if _, e = f.s.CreateToolInspectionRecovery(ctx, initiator, f.project, created.ID, recoveryReq); e == nil {
		t.Fatal("revoked original initiator created a new recovery grant")
	}
	recovered, e := f.s.CreateToolInspectionRecovery(ctx, f.actor, f.project, created.ID, recoveryReq)
	if e != nil || stringify(recovered) != stringify(recovery) {
		t.Fatal("original recovery creation ACK changed", recovered, e)
	}
	poll := f.bearer("POST", "/runner/tool-inspections/recovery", token, tic.ToolInspectionRecoveryPollRequest{ProjectID: f.project, WorkerID: p.WorkerID, Inspection: ref}, 200)
	if poll["recovery"] == nil {
		t.Fatal("independent reconciliation was blocked by old initiator/profile")
	}
	original, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || original.Version != recovery.InspectionVersion || original.ResultCode != "probe_timeout" {
		t.Fatal(original, e)
	}
	proof := tic.ToolInspectionStopProof{Protocol: tic.ProbeStopProtocol, Inspection: ref, SourceSHA256: tic.InspectionSourceSHA256(original), ReceiptSHA256: digest("owned-no-native-process-contract-receipt195"), ProbeReturned: true, AllNativeTreesStopped: true, NoPendingInventory: true, NativeProcessCount: 0, StoppedProcessCount: 0, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	body := map[string]any{"project_id": f.project, "worker_id": p.WorkerID, "recovery_id": recovery.ID, "expected_version": recovery.Version, "idempotency_key": newID(), "proof": proof}
	for _, field := range []string{"native_process_count", "stopped_process_count", "probe_returned", "all_native_trees_stopped", "no_pending_inventory"} {
		var bad map[string]any
		json.Unmarshal([]byte(stringify(proof)), &bad)
		delete(bad, field)
		body["proof"] = bad
		f.bearer("POST", "/runner/tool-inspections/recovery/proofs", token, body, 400)
	}
	body["proof"] = nil
	f.bearer("POST", "/runner/tool-inspections/recovery/proofs", token, body, 400)
	bad := proof
	bad.AllNativeTreesStopped = false
	body["proof"] = bad
	f.bearer("POST", "/runner/tool-inspections/recovery/proofs", token, body, 400)
	bad = proof
	bad.Inspection.Fence++
	body["proof"] = bad
	f.bearer("POST", "/runner/tool-inspections/recovery/proofs", token, body, 409)
	body["proof"] = proof
	storedMap := f.bearer("POST", "/runner/tool-inspections/recovery/proofs", token, body, 200)
	var stored tic.ToolInspectionRecovery
	json.Unmarshal([]byte(stringify(storedMap)), &stored)
	if stored.State != "proof_stored" || stored.ProofSHA256 != tic.StopProofDigest(proof) {
		t.Fatal(stored)
	}
	unchanged, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || unchanged.State != "unknown" {
		t.Fatal("proof alone released maintenance occupancy", unchanged, e)
	}
	ack := tic.ToolInspectionRecoveryAcknowledgementRequest{ProjectID: f.project, WorkerID: p.WorkerID, RecoveryID: stored.ID, ExpectedVersion: stored.Version, IdempotencyKey: newID(), ProofSHA256: stored.ProofSHA256}
	if _, e = f.s.DB.Exec(`CREATE TRIGGER owned_recovery_atomic195 BEFORE UPDATE OF state ON suma_tool_inspection_recoveries WHEN NEW.state='closed_unknown' BEGIN SELECT RAISE(ABORT,'owned closure interruption'); END`); e != nil {
		t.Fatal(e)
	}
	if _, e = f.s.AcknowledgeToolInspectionRecovery(ctx, worker, f.project, ack); e == nil {
		t.Fatal("closure transaction fault was swallowed")
	}
	unchanged, e = f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || unchanged.State != "unknown" {
		t.Fatal("closure partially committed original request", unchanged, e)
	}
	if _, e = f.s.DB.Exec(`DROP TRIGGER owned_recovery_atomic195`); e != nil {
		t.Fatal(e)
	}
	closed, e := f.s.AcknowledgeToolInspectionRecovery(ctx, worker, f.project, ack)
	if e != nil || closed.State != "closed_unknown" {
		t.Fatal(closed, e)
	}
	again, e := f.s.AcknowledgeToolInspectionRecovery(ctx, worker, f.project, ack)
	if e != nil || stringify(again) != stringify(closed) {
		t.Fatal("lost closure ACK did not restore same receipt", again, e)
	}
	final, e := f.s.GetToolInspection(ctx, f.actor, f.project, created.ID)
	if e != nil || final.State != "closed_unknown" || final.ResultCode != "probe_timeout" || final.RecoveryID != closed.ID {
		t.Fatal(final, e)
	}
	inspectionMaintenance194(t, f, token, p)
	fresh := inspectionCreate194(t, f, p)
	if fresh.ID == created.ID {
		t.Fatal("closed unknown was re-probed instead of creating new request")
	}
	var before string
	f.s.DB.QueryRow(`SELECT items FROM suma_tool_inventories WHERE worker_id=?`, p.WorkerID).Scan(&before)
	if _, e = f.s.ReportToolInventory(ctx, worker, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Inspection: &ref, Items: []tc.Observation{}}); e == nil {
		t.Fatal("late old reference overwrote new inventory")
	}
	var after string
	f.s.DB.QueryRow(`SELECT items FROM suma_tool_inventories WHERE worker_id=?`, p.WorkerID).Scan(&after)
	if before != after {
		t.Fatal("late inventory changed fresh snapshot")
	}
}
