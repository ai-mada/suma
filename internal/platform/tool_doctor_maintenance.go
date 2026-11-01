package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// Maintenance is a separate read of the existing worker's maintenance slot,
// never a replacement for dispatch availability or a grant to perform an action.
// The original create/claim/recovery commands remain the only write authority.
func (s *Service) toolDoctorMaintenanceTx(ctx context.Context, auth *authorizedCommand, report *ToolProfileDiagnostics) error {
	profile := report.Tool
	if err := assertProjectRead(auth, profile.ProjectID); err != nil {
		return err
	}
	clock, err := time.Parse(time.RFC3339Nano, report.CheckedAt)
	if err != nil || profile.Maintenance == nil {
		return toolInspectionChanged()
	}
	expiry, err := time.Parse(time.RFC3339Nano, report.ExpiresAt)
	if err != nil || !expiry.After(clock) {
		return toolInspectionChanged()
	}
	clamp := func(value string) error {
		if value == "" {
			return nil
		}
		until, e := time.Parse(time.RFC3339Nano, value)
		if e != nil {
			return toolInspectionChanged()
		}
		if until.After(clock) && until.Before(expiry) {
			expiry = until
			report.ExpiresAt = until.UTC().Format(time.RFC3339Nano)
		}
		return nil
	}
	if err = clamp(profile.Maintenance.ExpiresAt); err != nil {
		return err
	}
	resolution := ToolProfileMaintenanceResolution{ProfileID: profile.ID, ProfileVersion: profile.Version, WorkerID: profile.WorkerID}
	finish := func(action, reason, inspection, recovery string) error {
		resolution.Action, resolution.Reason, resolution.InspectionID, resolution.RecoveryID = action, reason, inspection, recovery
		report.MaintenanceResolution = resolution
		return nil
	}

	// The original writer materializes expired predecessors before creating a
	// new request. Two stored occupants violate that single-slot invariant;
	// reject the ambiguous source instead of hiding unknown effects or scanning
	// arbitrary project history. This query uses the existing worker/state index.
	rows, err := auth.tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','unknown') LIMIT 2`), auth.target.tenantID, profile.ProjectID, profile.WorkerID)
	if err != nil {
		return err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) > 1 {
		return toolInspectionChanged()
	}
	if len(ids) == 1 {
		row, e := s.toolInspectionReadTx(ctx, auth.tx, auth.target.tenantID, profile.ProjectID, ids[0])
		if e != nil {
			return e
		}
		if row.Value.WorkerID != profile.WorkerID {
			return toolInspectionChanged()
		}
		original := toolInspectionEffective(row.Value, clock)
		switch original.State {
		case "pending":
			if err = clamp(original.ExpiresAt); err != nil {
				return err
			}
			return finish("view_inspection", "inspection_pending", original.ID, "")
		case "claimed":
			if err = clamp(original.LeaseUntil); err != nil {
				return err
			}
			return finish("view_inspection", "inspection_claimed", original.ID, "")
		case "unknown":
			// Reading an existing recovery is safe even when it can no longer
			// progress. A recommendation never renews its old approval or owner.
			var recoveryID string
			e = auth.tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=? AND inspection_id=? ORDER BY created_at DESC,id DESC LIMIT 1`), auth.target.tenantID, profile.ProjectID, original.ID).Scan(&recoveryID)
			if e != nil && !errors.Is(e, sql.ErrNoRows) {
				return e
			}
			reason := "inspection_unknown"
			if e == nil {
				recovery, e := s.toolRecoveryReadTx(ctx, auth.tx, auth.target.tenantID, profile.ProjectID, recoveryID)
				if e != nil {
					return e
				}
				if recovery.Value.WorkerID != profile.WorkerID || recovery.Value.Inspection.InspectionID != original.ID || recovery.Value.Inspection.ClaimID != original.ClaimID || recovery.Value.Inspection.Fence != original.Fence || recovery.Value.SourceSHA256 != tic.InspectionSourceSHA256(original) {
					return toolRecoveryChanged()
				}
				head := toolRecoveryEffective(recovery.Value, clock)
				switch head.State {
				case "requested", "proof_stored":
					if err = clamp(head.ExpiresAt); err != nil {
						return err
					}
					return finish("view_recovery", "recovery_"+head.State, original.ID, head.ID)
				case "expired":
					reason = "recovery_expired"
				default:
					return toolRecoveryChanged()
				}
			}
			maintenance := profile.Maintenance
			if maintenance.Protocol == tic.MaintenanceProtocol && maintenance.InstanceID != "" && (maintenance.State == "supported" || maintenance.State == "expired") {
				return finish("recover_inspection", reason, original.ID, "")
			}
			return finish("view_inspection", "inspection_unknown", original.ID, "")
		case "expired":
			// No probe was claimed. Do not persist this projection in a GET;
			// a future explicitly authorized create handles the old row itself.
		default:
			return toolInspectionChanged()
		}
	}
	if profile.State == "revoked" {
		return finish("none", "profile_revoked", "", "")
	}
	for _, check := range report.Checks {
		if check.Code == "worker" && check.Status != "pass" {
			return finish("open_worker", "worker_unavailable", "", "")
		}
	}
	switch profile.Maintenance.State {
	case "supported":
		return finish("inspect", "maintenance_supported", "", "")
	case "expired":
		return finish("open_worker", "maintenance_expired", "", "")
	case "unsupported":
		return finish("open_worker", "maintenance_unsupported", "", "")
	case "unknown":
		return finish("open_worker", "maintenance_unknown", "", "")
	default:
		return toolRecoveryChanged()
	}
}
