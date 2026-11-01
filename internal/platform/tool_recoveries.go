package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func toolRecoveryChanged() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_recovery_changed", "核对依据已变化，请读取原记录；无法核实原节点时请隔离旧身份并重新配对", nil}, key: "validation.tool_recovery_changed"}
}
func toolRecoveryBusy() error {
	return &diagnosticAPIError{cause: &APIError{409, "tool_recovery_in_progress", "本次检查已有未结束的核对，请先读取原核对记录", nil}, key: "validation.tool_recovery_busy"}
}
func (s *Service) toolRecoveryBrowserAuthTx(ctx context.Context, tx *sql.Tx, actor Actor, project, inspection string, purpose commandPurpose) (*authorizedCommand, error) {
	if err := s.authorityWriter(ctx, tx); err != nil {
		return nil, err
	}
	var owner string
	err := tx.QueryRowContext(ctx, s.q(`SELECT p.owner_tenant_id FROM suma_tool_inspections i JOIN suma_execution_pool_runners r ON r.worker_id=i.worker_id JOIN suma_execution_pools p ON p.id=r.pool_id WHERE i.tenant_id=? AND i.project_id=? AND i.id=?`), actor.credential.tenantID, project, inspection).Scan(&owner)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: inspection}, purpose, []string{owner})
}
func toolRecoveryEffective(v tic.ToolInspectionRecovery, clock time.Time) tic.ToolInspectionRecovery {
	if v.State == "requested" || v.State == "proof_stored" {
		until, err := time.Parse(time.RFC3339Nano, v.ExpiresAt)
		if err == nil && !clock.Before(until) {
			v.State = "expired"
		}
	}
	return v
}
func (s *Service) CreateToolInspectionRecovery(ctx context.Context, actor Actor, project, id string, req tic.ToolInspectionRecoveryCreateRequest) (out tic.ToolInspectionRecovery, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolRecoveryBrowserAuthTx(ctx, tx, actor, project, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	if req.ProjectID != project || !ingressVersion(req.ExpectedVersion) || tic.ValidateReference(req.Inspection) != nil || req.Inspection.InspectionID != id || !tic.Operation(req.IdempotencyKey) {
		return out, toolInspectionInvalid()
	}
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"tool_recovery": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		return out, err
	}
	original, err := s.toolInspectionReadTx(ctx, tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if original.Value.Version != req.ExpectedVersion || original.Value.ClaimID != req.Inspection.ClaimID || original.Value.Fence != req.Inspection.Fence {
		return out, toolRecoveryChanged()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	effective := toolInspectionEffective(original.Value, clock)
	if effective.State != "unknown" {
		return out, toolRecoveryChanged()
	}
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, auth.target.tenantID, project, original.Value.WorkerID, false)
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=? AND inspection_id=? AND state IN ('requested','proof_stored') ORDER BY id`), auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	var ids []string
	for rows.Next() {
		var x string
		if err = rows.Scan(&x); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	for _, x := range ids {
		r, e := s.toolRecoveryReadTx(ctx, tx, auth.target.tenantID, project, x)
		if e != nil {
			return out, e
		}
		v := toolRecoveryEffective(r.Value, clock)
		if v.State != "expired" {
			return out, toolRecoveryBusy()
		}
		old := r.Value.Version
		r.Value = v
		r.Value.Version++
		if e = s.toolRecoverySaveTx(ctx, tx, r, old); e != nil {
			return out, e
		}
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=?`), auth.target.tenantID, project).Scan(&count); err != nil {
		return out, err
	}
	if count >= tic.MaxProjectRequests {
		return out, toolRecoveryBusy()
	}
	// Materialize a timed-out claimed record only in this explicit CAS command.
	// The original result remains unknown; no stop or completed fact is invented.
	if original.Value.State != effective.State {
		old := original.Value.Version
		original.Value = effective
		original.Value.Version++
		if err = s.toolInspectionSaveTx(ctx, tx, original, old); err != nil {
			return out, err
		}
	}
	out = tic.ToolInspectionRecovery{ID: newID(), ProjectID: project, WorkerID: original.Value.WorkerID, Inspection: req.Inspection, InspectionVersion: original.Value.Version, SourceSHA256: tic.InspectionSourceSHA256(original.Value), Version: 1, State: "requested", RequestedBy: auth.currentPrincipal.ID, CreatedAt: clock.UTC().Format(time.RFC3339Nano), ExpiresAt: clock.Add(tic.RecoveryTTLSeconds * time.Second).UTC().Format(time.RFC3339Nano)}
	row := toolRecoveryRow{Value: out, Tenant: auth.target.tenantID, IdentitySHA256: identity, OwnerID: status.InstanceID}
	if err = s.toolRecoverySaveTx(ctx, tx, row, 0); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "tool_inspection.recovery_requested", out.ID, original.Value.CommandID); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

// A new, finite authorization can retire old unknown effects even after the
// old initiator or old profile revision changed. It cannot revive those rights.
func (s *Service) toolRecoveryGuardTx(ctx context.Context, tx *sql.Tx, row toolRecoveryRow) (toolInspectionRow, error) {
	original, err := s.toolInspectionReadTx(ctx, tx, row.Tenant, row.Value.ProjectID, row.Value.Inspection.InspectionID)
	if err != nil {
		return original, err
	}
	if original.Value.WorkerID != row.Value.WorkerID || original.Value.ClaimID != row.Value.Inspection.ClaimID || original.Value.Fence != row.Value.Inspection.Fence || tic.InspectionSourceSHA256(original.Value) != row.Value.SourceSHA256 {
		return original, toolRecoveryChanged()
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, row.Tenant, row.Value.RequestedBy, row.Value.ProjectID, true, true); err != nil {
		return original, err
	}
	status, identity, err := s.requireToolMaintenanceTx(ctx, tx, row.Tenant, row.Value.ProjectID, row.Value.WorkerID, false)
	if err != nil {
		return original, err
	}
	if identity != row.IdentitySHA256 || status.InstanceID != row.OwnerID {
		return original, toolRecoveryChanged()
	}
	return original, nil
}
func (s *Service) GetToolInspectionRecovery(ctx context.Context, actor Actor, project, id string) (out tic.ToolInspectionRecovery, err error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.authorityWriter(ctx, tx); err != nil {
		return out, err
	}
	var inspection string
	err = tx.QueryRowContext(ctx, s.q(`SELECT inspection_id FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=? AND id=?`), actor.credential.tenantID, project, id).Scan(&inspection)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	auth, err := s.toolRecoveryBrowserAuthTx(ctx, tx, actor, project, inspection, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	r, err := s.toolRecoveryReadTx(ctx, tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	return toolRecoveryEffective(r.Value, clock), tx.Commit()
}
func (s *Service) ListToolInspectionRecoveries(ctx context.Context, actor Actor, project, inspection string) (out tic.ToolInspectionRecoveryList, err error) {
	out.Items = []tic.ToolInspectionRecovery{}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	auth, err := s.toolRecoveryBrowserAuthTx(ctx, tx, actor, project, inspection, purposeBrowserProjectRead)
	if err != nil {
		return out, err
	}
	if _, err = s.toolInspectionReadTx(ctx, tx, auth.target.tenantID, project, inspection); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=? AND inspection_id=? ORDER BY created_at DESC,id DESC LIMIT 10`), auth.target.tenantID, project, inspection)
	if err != nil {
		return out, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		r, e := s.toolRecoveryReadTx(ctx, tx, auth.target.tenantID, project, id)
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, toolRecoveryEffective(r.Value, clock))
	}
	return out, tx.Commit()
}
