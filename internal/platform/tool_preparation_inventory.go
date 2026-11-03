package platform

import (
	"context"
	"database/sql"

	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

// An in-flight report from the old immutable snapshot remains valid. The new
// accepted command/generation, however, must enter inventory through its own
// correlated acknowledgement rather than a bare periodic report.
func (s *Service) toolPreparationUnboundInventoryTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, maintenance *tic.ToolMaintenanceCapability, items []tc.Observation) error {
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','awaiting_inventory','unknown') LIMIT 10001`), tenant, project, worker)
	if err != nil {
		return err
	}
	var ids []string
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
	if len(ids) > tp.MaxProjectRequests {
		return toolPreparationChanged()
	}
	for _, id := range ids {
		row, e := s.toolPreparationReadTx(ctx, tx, tenant, project, id)
		if e != nil {
			return e
		}
		if row.Value.Receipt != nil && maintenance != nil && maintenance.ManagedPreparation != nil && maintenance.ManagedPreparation.GenerationSHA256 == row.Value.Receipt.GenerationSHA256 {
			return toolPreparationChanged()
		}
		for _, item := range items {
			if item.Profile.CommandID == tp.CommandID(row.Value.ID) {
				return toolPreparationChanged()
			}
		}
	}
	return nil
}

// Reference checks precede the original inventory receipt lookup. A replay is
// still authenticated today, but keeps the original observation time and does
// not need a fresh opt-in declaration or extend a completed operation's lease.
func (s *Service) toolPreparationInventoryReferenceTx(ctx context.Context, tx *sql.Tx, tenant, project, worker, operation string, binding *tp.InventoryBinding) (toolPreparationRow, error) {
	if binding == nil || tp.ValidateInventoryBinding(*binding) != nil {
		return toolPreparationRow{}, toolPreparationInvalid()
	}
	row, err := s.toolPreparationReadTx(ctx, tx, tenant, project, binding.Reference.PreparationID)
	if err != nil {
		return row, err
	}
	v := row.Value
	if v.WorkerID != worker || preparationReference(v) != binding.Reference || v.SourceSHA256 != binding.SourceSHA256 || v.Receipt == nil || v.Receipt.GenerationSHA256 != binding.GenerationSHA256 || v.Receipt.LocalReceiptSHA256 != binding.LocalReceiptSHA256 || !preparationReceiptMatches(v, v.Receipt) {
		return row, toolPreparationChanged()
	}
	if v.State != "awaiting_inventory" && (v.State != "completed" || v.InventoryOperationID != operation) {
		return row, toolPreparationChanged()
	}
	return row, nil
}

// Only the original accepted generation can become current inventory. This
// runs in the inventory transaction, so neither half can survive a failed
// profile, authority, reference or generation check.
func (s *Service) completeToolPreparationInventoryTx(ctx context.Context, tx *sql.Tx, tenant, project, worker, operation, observed string, binding *tp.InventoryBinding, maintenance *tic.ToolMaintenanceCapability, items []tc.Observation) error {
	row, err := s.toolPreparationInventoryReferenceTx(ctx, tx, tenant, project, worker, operation, binding)
	if err != nil {
		return err
	}
	v := row.Value
	if v.State != "awaiting_inventory" || maintenance == nil || tic.ValidateMaintenanceCapability(*maintenance) != nil || maintenance.InstanceID != v.InstanceID {
		return toolPreparationChanged()
	}
	if maintenance.ManagedPreparation != nil && (!preparationSupports(maintenance.ManagedPreparation, v.AdapterID) || maintenance.ManagedPreparation.GenerationSHA256 != v.Receipt.GenerationSHA256) {
		return toolPreparationChanged()
	}
	if err = s.toolPreparationReconcileTx(ctx, tx, row, true); err != nil {
		return err
	}
	matched := false
	for _, item := range items {
		if item.Profile.CommandID == v.Receipt.CommandID {
			if !preparationProfilesEqual(item.Profile, v.Receipt.Profile) {
				return toolPreparationChanged()
			}
			matched = true
		}
	}
	// Configuration acceptance does not grant execution. A maintenance-only
	// node may truthfully report the exact profile with Available=false.
	if !matched {
		return toolPreparationChanged()
	}
	old := v.Version
	row.Value.Version++
	row.Value.State, row.Value.ResultCode = "completed", "completed"
	row.Value.InventoryOperationID, row.Value.InventoryObservedAt = operation, observed
	row.Value.FinishedAt = observed
	return s.toolPreparationSaveTx(ctx, tx, row, old)
}
