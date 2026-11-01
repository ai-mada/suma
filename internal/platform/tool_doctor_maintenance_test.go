package platform

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func doctorResolution199(t *testing.T, f *fixture, p ToolProfile, action, reason, inspection, recovery string) ToolProfileDiagnostics {
	t.Helper()
	before := []int{expertCount(t, f, "suma_operations"), expertCount(t, f, "suma_tool_inspections"), expertCount(t, f, "suma_tool_inspection_recoveries"), expertCount(t, f, "dag_runs")}
	out, e := f.s.GetToolProfileDiagnostics(context.Background(), f.actor, f.project, p.ID)
	if e != nil {
		t.Fatal(e)
	}
	r := out.MaintenanceResolution
	if r.ProfileID != p.ID || r.ProfileVersion != p.Version || r.WorkerID != p.WorkerID || r.Action != action || r.Reason != reason || r.InspectionID != inspection || r.RecoveryID != recovery {
		t.Fatalf("wrong original maintenance resolution: %+v", r)
	}
	after := []int{expertCount(t, f, "suma_operations"), expertCount(t, f, "suma_tool_inspections"), expertCount(t, f, "suma_tool_inspection_recoveries"), expertCount(t, f, "dag_runs")}
	if stringify(before) != stringify(after) {
		t.Fatal("Doctor produced write effects", before, after)
	}
	if len(out.Checks) != 4 || out.Checks[0].Code != "authorization" || out.Checks[1].Code != "worker" || out.Checks[2].Code != "inventory" || out.Checks[3].Code != "profile" {
		t.Fatal("maintenance replaced dispatch checks", out)
	}
	checked, _ := time.Parse(time.RFC3339Nano, out.CheckedAt)
	expiry, e := time.Parse(time.RFC3339Nano, out.ExpiresAt)
	if e != nil || !expiry.After(checked) || expiry.After(checked.Add(60*time.Second)) {
		t.Fatal("invalid display deadline", out)
	}
	return out
}

func TestToolDoctorMaintenanceFollowsOriginalOccupancyAndRecovery(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	machine := toolActorForTest(t, f, token)
	doctorResolution199(t, f, p, "open_worker", "maintenance_unknown", "", "")
	inspectionMaintenance194(t, f, token, p)
	ready := doctorResolution199(t, f, p, "inspect", "maintenance_supported", "", "")
	if ready.Tool.Available || ready.NextAction != "review_profile" {
		t.Fatal("inspection recommendation enabled paused dispatch", ready)
	}
	original := inspectionCreate194(t, f, p)
	doctorResolution199(t, f, p, "view_inspection", "inspection_pending", original.ID, "")
	// Occupancy belongs to the exact project/worker, even when the selected
	// profile is a different paused configuration for that same worker.
	otherProfile := p.Profile
	otherProfile.CommandID = "doctor-other199"
	other, e := f.s.CreateToolProfile(ctx, f.actor, f.project, ToolProfileCreateRequest{ProjectID: f.project, WorkerID: p.WorkerID, Name: "Other registered configuration", Profile: otherProfile, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, other, "view_inspection", "inspection_pending", original.ID, "")
	claim, e := f.s.ClaimToolInspection(ctx, machine, f.project, tic.ClaimRequest{ProjectID: f.project, WorkerID: p.WorkerID, ClaimID: newID()})
	if e != nil || claim.Inspection == nil {
		t.Fatal(claim, e)
	}
	claimed := doctorResolution199(t, f, other, "view_inspection", "inspection_claimed", original.ID, "")
	if claimed.ExpiresAt > claim.Inspection.LeaseUntil {
		t.Fatal("display outlived original claim", claimed.ExpiresAt, claim.Inspection.LeaseUntil)
	}
	inspectionChangeClock194(t, f, original.ID, func(v *tic.Inspection) {
		at, _ := time.Parse(time.RFC3339Nano, v.ClaimedAt)
		v.LeaseUntil = at.Add(time.Nanosecond).Format(time.RFC3339Nano)
	})
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_maintenance SET observed_at='2000-01-01T00:00:00Z'`); e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "recover_inspection", "inspection_unknown", original.ID, "")
	var storedState string
	if e = f.s.DB.QueryRow(`SELECT state FROM suma_tool_inspections WHERE id=?`, original.ID).Scan(&storedState); e != nil || storedState != "claimed" {
		t.Fatal("read materialized a timed-out claim", storedState, e)
	}
	head, e := f.s.GetToolInspection(ctx, f.actor, f.project, original.ID)
	if e != nil {
		t.Fatal(e)
	}
	ref := tic.Reference{InspectionID: head.ID, ClaimID: head.ClaimID, Fence: head.Fence}
	recovery, e := f.s.CreateToolInspectionRecovery(ctx, f.actor, f.project, head.ID, tic.ToolInspectionRecoveryCreateRequest{ProjectID: f.project, ExpectedVersion: head.Version, Inspection: ref, IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, other, "view_recovery", "recovery_requested", head.ID, recovery.ID)
	proof := tic.ToolInspectionStopProof{Protocol: tic.ProbeStopProtocol, Inspection: ref, SourceSHA256: tic.InspectionSourceSHA256(head), ReceiptSHA256: digest("owned-no-native-stop-receipt199"), ProbeReturned: true, AllNativeTreesStopped: true, NoPendingInventory: true, NativeProcessCount: 0, StoppedProcessCount: 0, ObservedAt: head.ClaimedAt}
	// The effective timeout is part of source identity only through the original
	// immutable lease; the proof uses the persisted original source fields.
	row, e := func() (toolInspectionRow, error) {
		var out toolInspectionRow
		var raw string
		err := f.s.DB.QueryRow(`SELECT payload FROM suma_tool_inspections WHERE id=?`, head.ID).Scan(&raw)
		if err == nil {
			err = json.Unmarshal([]byte(raw), &out)
		}
		return out, err
	}()
	if e != nil {
		t.Fatal(e)
	}
	proof.SourceSHA256 = tic.InspectionSourceSHA256(row.Value)
	stored, e := f.s.StoreToolInspectionRecoveryProof(ctx, machine, f.project, tic.ToolInspectionRecoveryProofRequest{ProjectID: f.project, WorkerID: p.WorkerID, RecoveryID: recovery.ID, ExpectedVersion: recovery.Version, IdempotencyKey: newID(), Proof: proof})
	if e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "view_recovery", "recovery_proof_stored", head.ID, stored.ID)
	var raw string
	if e = f.s.DB.QueryRow(`SELECT payload FROM suma_tool_inspection_recoveries WHERE id=?`, stored.ID).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var recoveryRow toolRecoveryRow
	if e = json.Unmarshal([]byte(raw), &recoveryRow); e != nil {
		t.Fatal(e)
	}
	created, _ := time.Parse(time.RFC3339Nano, recoveryRow.Value.CreatedAt)
	recoveryRow.Value.ExpiresAt = created.Add(time.Nanosecond).Format(time.RFC3339Nano)
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_inspection_recoveries SET payload=? WHERE id=?`, stringify(recoveryRow), stored.ID); e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "recover_inspection", "recovery_expired", head.ID, "")
	if e = f.s.DB.QueryRow(`SELECT state FROM suma_tool_inspection_recoveries WHERE id=?`, stored.ID).Scan(&storedState); e != nil || storedState != "proof_stored" {
		t.Fatal("read rewrote expired recovery", storedState, e)
	}
	recoveryRow.Value.SourceSHA256 = strings.Repeat("e", 64)
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_inspection_recoveries SET payload=? WHERE id=?`, stringify(recoveryRow), stored.ID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.GetToolProfileDiagnostics(ctx, f.actor, f.project, p.ID)
	requireAuthorityStatus(t, e, 409)
}

func TestToolDoctorMaintenanceCapabilityScopeAndCurrentAuthority(t *testing.T) {
	f, token, p := toolFixture(t)
	ctx := context.Background()
	machine := toolActorForTest(t, f, token)
	inspectionMaintenance194(t, f, token, p)
	if _, e := f.s.DB.Exec(`UPDATE suma_tool_maintenance SET observed_at='2000-01-01T00:00:00Z'`); e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "open_worker", "maintenance_expired", "", "")
	_, e := f.s.ReportToolInventory(ctx, machine, f.project, ToolInventoryRequest{ProjectID: f.project, WorkerID: p.WorkerID, OperationID: newID(), Items: []tc.Observation{{Profile: p.Profile, Available: true}}, Maintenance: &tic.ToolMaintenanceCapability{Protocol: "suma-tool-maintenance-v2", InstanceID: "owned-unsupported199"}})
	if e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "open_worker", "maintenance_unsupported", "", "")
	inspectionMaintenance194(t, f, token, p)
	// An unclaimed request whose deadline elapsed does not become an unknown
	// native effect, and a GET must not persist its projected expired state.
	original := inspectionCreate194(t, f, p)
	var shortDeadline string
	inspectionChangeClock194(t, f, original.ID, func(v *tic.Inspection) {
		at, _ := time.Parse(time.RFC3339Nano, v.CreatedAt)
		shortDeadline = at.Add(10 * time.Second).Format(time.RFC3339Nano)
		v.ExpiresAt = shortDeadline
	})
	bounded := doctorResolution199(t, f, p, "view_inspection", "inspection_pending", original.ID, "")
	if bounded.ExpiresAt != shortDeadline {
		t.Fatal("maintenance suggestion outlived its nearer original request deadline", bounded.ExpiresAt, shortDeadline)
	}
	inspectionChangeClock194(t, f, original.ID, func(v *tic.Inspection) {
		at, _ := time.Parse(time.RFC3339Nano, v.CreatedAt)
		v.ExpiresAt = at.Add(time.Nanosecond).Format(time.RFC3339Nano)
	})
	doctorResolution199(t, f, p, "inspect", "maintenance_supported", "", "")
	var state string
	if e = f.s.DB.QueryRow(`SELECT state FROM suma_tool_inspections WHERE id=?`, original.ID).Scan(&state); e != nil || state != "pending" {
		t.Fatal(state, e)
	}
	// Foreign project inventory/maintenance occupancy must not participate.
	if _, e = f.s.DB.Exec(`UPDATE suma_tool_inspections SET tenant_id=? WHERE id=?`, strings.Repeat("f", 32), original.ID); e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "inspect", "maintenance_supported", "", "")
	reader := invitedReviewer(t, f)
	if _, e = f.s.DB.Exec(`DELETE FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, reader.credential.subjectID); e != nil {
		t.Fatal(e)
	}
	_, e = f.s.GetToolProfileDiagnostics(ctx, reader, f.project, p.ID)
	requireAuthorityStatus(t, e, 403)
	if _, e = f.s.DB.Exec(`UPDATE suma_release_workers SET status='revoked' WHERE tenant_id=? AND project_id=? AND worker_id=?`, f.user.TenantID, f.project, p.WorkerID); e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "open_worker", "worker_unavailable", "", "")
	p, e = f.s.CommandToolProfile(ctx, f.actor, f.project, p.ID, ToolProfileCommandRequest{ProjectID: f.project, ExpectedVersion: p.Version, Action: "revoke", IdempotencyKey: newID()})
	if e != nil {
		t.Fatal(e)
	}
	doctorResolution199(t, f, p, "none", "profile_revoked", "", "")
}
