package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	ti "github.com/ai-mada/suma/internal/toolinstallationcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
)

func (s *Service) ClaimToolPreparation(ctx context.Context, actor Actor, project string, request tp.ClaimRequest) (out tp.Claim, err error) {
	if request.ProjectID != project || tp.ValidateClaimRequest(request) != nil {
		return out, toolPreparationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	machine, err := s.toolInspectionNormalMachineTx(ctx, tx, actor, project, request.WorkerID)
	if err != nil {
		return out, err
	}
	out, err = s.claimToolPreparationTx(ctx, tx, machine, request.ClaimID, request)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) ClaimPoolToolPreparation(ctx context.Context, token string, request tp.PoolClaimRequest) (out tp.Claim, err error) {
	if tp.ValidatePoolClaimRequest(request) != nil {
		return out, toolPreparationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	machine, err := s.toolInspectionPoolMachineTx(ctx, tx, token, request.TenantID, request.ProjectID, request.RunnerID)
	if err != nil {
		return out, err
	}
	out, err = s.claimToolPreparationTx(ctx, tx, machine, request.ClaimID, request)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func preparationSourceChanged(err error) bool {
	if automationAuthorityFailure(err) {
		return true
	}
	var api *APIError
	return errors.As(err, &api) && (api.Code == "tool_preparation_source_changed" || api.Code == "tool_installation_source_changed" || api.Code == "tool_maintenance_unavailable")
}

func (s *Service) claimToolPreparationTx(ctx context.Context, tx *sql.Tx, machine toolInspectionMachine, key string, request any) (out tp.Claim, err error) {
	out = tp.Claim{TenantID: machine.tenant, ProjectID: machine.project, WorkerID: machine.worker}
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, machine, "managed-preparation/claim", key, request)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(raw), &out) != nil || out.TenantID != machine.tenant || out.ProjectID != machine.project || out.WorkerID != machine.worker {
			return out, toolPreparationChanged()
		}
		if out.Preparation == nil {
			return out, nil
		}
		row, e := s.toolPreparationReadTx(ctx, tx, machine.tenant, machine.project, out.Preparation.ID)
		if e != nil {
			return out, e
		}
		if row.Value.WorkerID != machine.worker || row.Value.ClaimID != key || row.Value.Fence != out.Preparation.Fence || row.Value.SourceSHA256 != out.Preparation.SourceSHA256 {
			return out, toolPreparationChanged()
		}
		// Authentication above is current. This is only the same original
		// operation's projection, not renewed permission, lease or opt-in.
		value := toolPreparationEffective(row.Value, clock)
		out.Preparation = &value
		return out, nil
	}
	status, _, err := s.toolMaintenanceStatusTx(ctx, tx, machine.tenant, machine.project, machine.worker, clock)
	if err != nil {
		return out, err
	}
	// A scope-only null reply lets a restarted accepted head verify its real
	// tenant without guessing. Lack of opt-in cannot acquire pending work.
	if status.State != "supported" || status.ManagedPreparation == nil {
		if err = s.toolInspectionReceiptTx(ctx, tx, machine, op, hash, out); err != nil {
			return out, err
		}
		return out, nil
	}
	if err = s.toolInstallationBusyTx(ctx, tx, machine.tenant, machine.project, machine.worker, clock); err != nil {
		return out, err
	}
	if err = s.toolInstallationInspectionBusyTx(ctx, tx, machine.tenant, machine.project, machine.worker, clock); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_preparations WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','awaiting_inventory','unknown') ORDER BY created_at,id LIMIT 10001`), machine.tenant, machine.project, machine.worker)
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
	if len(ids) > tp.MaxProjectRequests {
		return out, toolPreparationChanged()
	}
	for _, id := range ids {
		row, e := s.toolPreparationReadTx(ctx, tx, machine.tenant, machine.project, id)
		if e != nil {
			return out, e
		}
		value := toolPreparationEffective(row.Value, clock)
		if value.State == "claimed" || value.State == "awaiting_inventory" || value.State == "unknown" {
			break
		}
		old := row.Value.Version
		if value.State == "expired" {
			row.Value = value
			row.Value.Version++
			if e = s.toolPreparationSaveTx(ctx, tx, row, old); e != nil {
				return out, e
			}
			continue
		}
		if e = s.toolPreparationCurrentTx(ctx, tx, row, true, false); e != nil {
			if !preparationSourceChanged(e) {
				return out, e
			}
			row.Value.State, row.Value.ResultCode = "superseded", "source_changed"
			row.Value.Version++
			if e = s.toolPreparationSaveTx(ctx, tx, row, old); e != nil {
				return out, e
			}
			continue
		}
		row.Value.State = "claimed"
		row.Value.Version++
		row.Value.Fence++
		row.Value.ClaimID = key
		row.Value.ClaimedAt = clock.UTC().Format(time.RFC3339Nano)
		row.Value.LeaseUntil = clock.Add(tp.ClaimLeaseSeconds * time.Second).UTC().Format(time.RFC3339Nano)
		if e = s.toolPreparationSaveTx(ctx, tx, row, old); e != nil {
			return out, e
		}
		value = row.Value
		out.Preparation = &value
		break
	}
	if err = s.toolInspectionReceiptTx(ctx, tx, machine, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Service) CompleteToolPreparation(ctx context.Context, actor Actor, project string, request tp.ResultRequest) (out tp.Operation, err error) {
	if request.ProjectID != project || tp.ValidateResultRequest(request) != nil {
		return out, toolPreparationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	machine, err := s.toolInspectionNormalMachineTx(ctx, tx, actor, project, request.WorkerID)
	if err != nil {
		return out, err
	}
	out, err = s.completeToolPreparationTx(ctx, tx, machine, request)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) CompletePoolToolPreparation(ctx context.Context, token string, request tp.PoolResultRequest) (out tp.Operation, err error) {
	if !ti.ID(request.TenantID) || !ti.ID(request.ProjectID) || !ti.ID(request.RunnerID) {
		return out, toolPreparationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	machine, err := s.toolInspectionPoolMachineTx(ctx, tx, token, request.TenantID, request.ProjectID, request.RunnerID)
	if err != nil {
		return out, err
	}
	ordinary := tp.ResultRequest{ProjectID: request.ProjectID, WorkerID: machine.worker, Preparation: request.Preparation, SourceSHA256: request.SourceSHA256, IdempotencyKey: request.IdempotencyKey, Outcome: request.Outcome, FailureCode: request.FailureCode, Receipt: request.Receipt, StopProof: request.StopProof}
	if tp.ValidateResultRequest(ordinary) != nil {
		return out, toolPreparationInvalid()
	}
	out, err = s.completeToolPreparationTx(ctx, tx, machine, ordinary)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func preparationReference(value tp.Operation) tp.Reference {
	return tp.Reference{PreparationID: value.ID, ClaimID: value.ClaimID, Fence: value.Fence}
}

func preparationReceiptMatches(value tp.Operation, receipt *tp.GenerationReceipt) bool {
	if receipt == nil || tp.ValidateGenerationReceipt(*receipt) != nil || receipt.ParentGenerationSHA256 != value.ExpectedGenerationSHA256 || receipt.GenerationSHA256 == value.ExpectedGenerationSHA256 || receipt.CommandID != tp.CommandID(value.ID) || receipt.Profile.CommandID != receipt.CommandID || receipt.Profile.ExecutableSHA256 != value.InstallationItem.ExecutableSHA256 || receipt.Profile.ToolVersion != value.InstallationItem.Version {
		return false
	}
	if !tp.ProfileMatchesConfiguration(value.AdapterID, value.Configuration, receipt.Profile) {
		return false
	}
	return receipt.StopProof != nil && receipt.StopProof.Preparation == preparationReference(value) && receipt.StopProof.SourceSHA256 == value.SourceSHA256
}

func (s *Service) completeToolPreparationTx(ctx context.Context, tx *sql.Tx, machine toolInspectionMachine, request tp.ResultRequest) (out tp.Operation, err error) {
	row, err := s.toolPreparationReadTx(ctx, tx, machine.tenant, machine.project, request.Preparation.PreparationID)
	if err != nil {
		return out, err
	}
	if row.Value.WorkerID != machine.worker || preparationReference(row.Value) != request.Preparation || row.Value.SourceSHA256 != request.SourceSHA256 {
		return out, toolPreparationChanged()
	}
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, machine, "managed-preparation/result", request.IdempotencyKey, request)
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(raw), &out) != nil || out.ID != row.Value.ID || out.SourceSHA256 != row.Value.SourceSHA256 {
			return out, toolPreparationChanged()
		}
		return out, nil
	}
	if row.Value.State != "claimed" && row.Value.State != "unknown" {
		return out, toolPreparationChanged()
	}
	if err = s.toolPreparationReconcileTx(ctx, tx, row, false); err != nil {
		return out, err
	}
	if request.StopProof != nil && (request.StopProof.Preparation != request.Preparation || request.StopProof.SourceSHA256 != request.SourceSHA256) {
		return out, toolPreparationChanged()
	}
	if request.Outcome == "prepared" && !preparationReceiptMatches(row.Value, request.Receipt) {
		return out, toolPreparationChanged()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	old := row.Value.Version
	effective := toolPreparationEffective(row.Value, clock)
	if request.Outcome == "closed_unknown" && (effective.State != "unknown" || effective.ResultCode != request.FailureCode) {
		return out, toolPreparationChanged()
	}
	row.Value.Version++
	row.Value.ResultCode = request.FailureCode
	row.Value.State = request.Outcome
	if request.Outcome == "prepared" {
		row.Value.State, row.Value.ResultCode = "awaiting_inventory", "prepared"
		// JSON cloning keeps the caller's mutable receipt/profile pointers out of
		// the stored response and the subsequent inventory correlation.
		rawReceipt := stringify(request.Receipt)
		if json.Unmarshal([]byte(rawReceipt), &row.Value.Receipt) != nil {
			return out, toolPreparationInvalid()
		}
	} else if request.Outcome == "unknown" {
		row.Value.Receipt = nil
		row.Value.StopProof = nil
	} else {
		row.Value.StopProof = request.StopProof
		row.Value.FinishedAt = clock.UTC().Format(time.RFC3339Nano)
		if request.Outcome == "closed_unknown" && effective.ResultCode != "" {
			row.Value.ResultCode = effective.ResultCode
		}
	}
	if err = s.toolPreparationSaveTx(ctx, tx, row, old); err != nil {
		return out, err
	}
	out = row.Value
	if err = s.toolInspectionReceiptTx(ctx, tx, machine, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}
