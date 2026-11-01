package platform

import (
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// ValidToolProfileMaintenanceResolution validates the public observation shape,
// not execution authority. Every write still reauthorizes and reads the original
// operation, version and owner. An inspection may belong to another profile on
// this worker; consumers must read that original object before navigating to it.
func ValidToolProfileMaintenanceResolution(r ToolProfileMaintenanceResolution, profile ToolProfile) bool {
	if r.ProfileID != profile.ID || r.ProfileVersion != profile.Version || r.ProfileVersion < 1 ||
		r.WorkerID != profile.WorkerID || !tc.ValidCommandID(r.ProfileID) || !tc.ValidCommandID(r.WorkerID) {
		return false
	}
	inspection := tic.ID(r.InspectionID)
	recovery := tic.ID(r.RecoveryID)
	switch r.Action {
	case "inspect":
		return r.Reason == "maintenance_supported" && r.InspectionID == "" && r.RecoveryID == ""
	case "view_inspection":
		return inspection && r.RecoveryID == "" && (r.Reason == "inspection_pending" || r.Reason == "inspection_claimed" || r.Reason == "inspection_unknown")
	case "recover_inspection":
		return inspection && r.RecoveryID == "" && (r.Reason == "inspection_unknown" || r.Reason == "recovery_expired")
	case "view_recovery":
		return inspection && recovery && (r.Reason == "recovery_requested" || r.Reason == "recovery_proof_stored")
	case "open_worker":
		return r.InspectionID == "" && r.RecoveryID == "" && (r.Reason == "maintenance_expired" || r.Reason == "maintenance_unsupported" || r.Reason == "maintenance_unknown" || r.Reason == "worker_unavailable")
	case "none":
		return r.Reason == "profile_revoked" && r.InspectionID == "" && r.RecoveryID == ""
	}
	return false
}
