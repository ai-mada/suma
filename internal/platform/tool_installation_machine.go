package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	tii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

// Reuse original authenticated ordinary/Pool machine authority and durable SQL
// receipts. The original inventory owner consumes these requests, not a second
// task dispatcher. A scan never reserves or executes a business task.
func (s *Service) ClaimToolInstallationScan(ctx context.Context, actor Actor, project string, req tii.ClaimRequest) (out tii.ClaimReply, err error) {
	if req.ProjectID != project || tii.ValidateClaimRequest(req) != nil {
		return out, toolInstallationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	m, err := s.toolInspectionNormalMachineTx(ctx, tx, actor, project, req.WorkerID)
	if err != nil {
		return out, err
	}
	out, err = s.claimToolInstallationTx(ctx, tx, m, req.ClaimID, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ClaimPoolToolInstallationScan(ctx context.Context, token string, req tii.PoolClaimRequest) (out tii.ClaimReply, err error) {
	if !tii.ID(req.TenantID) || !tii.ID(req.ProjectID) || !tii.ID(req.RunnerID) || !tii.Operation(req.ClaimID) {
		return out, toolInstallationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	m, err := s.toolInspectionPoolMachineTx(ctx, tx, token, req.TenantID, req.ProjectID, req.RunnerID)
	if err != nil {
		return out, err
	}
	out, err = s.claimToolInstallationTx(ctx, tx, m, req.ClaimID, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) claimToolInstallationTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, key string, request any) (out tii.ClaimReply, err error) {
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, m, "installation-scan/claim", key, request)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(raw), &out) != nil {
			return out, toolInstallationChanged()
		}
		if out.Scan == nil {
			return out, nil
		}
		row, e := s.toolInstallationReadTx(ctx, tx, m.tenant, m.project, out.Scan.ID)
		if e != nil {
			return out, e
		}
		if row.Value.WorkerID != m.worker || row.Value.ClaimID != key || row.Value.Fence != out.Scan.Fence {
			return out, toolInstallationChanged()
		}
		if e = s.toolInstallationCurrentTx(ctx, tx, row, false); e != nil {
			return out, e
		}
		value, e := s.toolInstallationPublicTx(ctx, tx, row, clock)
		out.Scan = &value
		return out, e
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_installation_scans WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','unknown') ORDER BY created_at,id LIMIT 10001`), m.tenant, m.project, m.worker)
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
	if len(ids) > tii.MaxProjectRequests {
		return out, toolInstallationChanged()
	}
	for _, id := range ids {
		row, e := s.toolInstallationReadTx(ctx, tx, m.tenant, m.project, id)
		if e != nil {
			return out, e
		}
		value := installationEffective(row.Value, clock)
		if value.State == "claimed" || value.State == "unknown" {
			break
		}
		old := row.Value.Version
		if value.State == "expired" {
			row.Value = value
			row.Value.Version++
			if e = s.toolInstallationSaveTx(ctx, tx, row, old); e != nil {
				return out, e
			}
			continue
		}
		if e = s.toolInstallationCurrentTx(ctx, tx, row, true); e != nil {
			var api *APIError
			if !automationAuthorityFailure(e) && !(errors.As(e, &api) && (api.Code == "tool_installation_source_changed" || api.Code == "tool_maintenance_unavailable")) {
				return out, e
			}
			row.Value.State = "superseded"
			row.Value.ResultCode = "source_changed"
			row.Value.Version++
			if e = s.toolInstallationSaveTx(ctx, tx, row, old); e != nil {
				return out, e
			}
			continue
		}
		row.Value.State = "claimed"
		row.Value.Version++
		row.Value.Fence++
		row.Value.ClaimID = key
		row.Value.ClaimedAt = clock.UTC().Format(time.RFC3339Nano)
		row.Value.LeaseUntil = clock.Add(tii.ClaimLeaseSeconds * time.Second).UTC().Format(time.RFC3339Nano)
		if e = s.toolInstallationSaveTx(ctx, tx, row, old); e != nil {
			return out, e
		}
		value = row.Value
		out.Scan = &value
		break
	}
	if err = s.toolInspectionReceiptTx(ctx, tx, m, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) CompleteToolInstallationScan(ctx context.Context, actor Actor, project string, req tii.ResultRequest) (out tii.Scan, err error) {
	if req.ProjectID != project || tii.ValidateResultRequest(req) != nil {
		return out, toolInstallationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	m, err := s.toolInspectionNormalMachineTx(ctx, tx, actor, project, req.WorkerID)
	if err != nil {
		return out, err
	}
	out, err = s.completeToolInstallationTx(ctx, tx, m, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) CompletePoolToolInstallationScan(ctx context.Context, token string, req tii.PoolResultRequest) (out tii.Scan, err error) {
	if !tii.ID(req.TenantID) || !tii.ID(req.ProjectID) || !tii.ID(req.RunnerID) {
		return out, toolInstallationInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	m, err := s.toolInspectionPoolMachineTx(ctx, tx, token, req.TenantID, req.ProjectID, req.RunnerID)
	if err != nil {
		return out, err
	}
	normal := tii.ResultRequest{ProjectID: req.ProjectID, WorkerID: m.worker, Scan: req.Scan, IdempotencyKey: req.IdempotencyKey, Outcome: req.Outcome, Items: req.Items, FailureCode: req.FailureCode, StopProof: req.StopProof}
	if tii.ValidateResultRequest(normal) != nil {
		return out, toolInstallationInvalid()
	}
	out, err = s.completeToolInstallationTx(ctx, tx, m, normal)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) completeToolInstallationTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, req tii.ResultRequest) (out tii.Scan, err error) {
	row, err := s.toolInstallationReadTx(ctx, tx, m.tenant, m.project, req.Scan.ScanID)
	if err != nil {
		return out, err
	}
	if row.Value.WorkerID != m.worker || row.Value.ClaimID != req.Scan.ClaimID || row.Value.Fence != req.Scan.Fence {
		return out, toolInstallationChanged()
	}
	if err = s.toolInstallationCurrentTx(ctx, tx, row, false); err != nil {
		return out, err
	}
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, m, "installation-scan/result", req.IdempotencyKey, req)
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(raw), &out) != nil || tii.ValidateScan(out) != nil {
			return out, toolInstallationChanged()
		}
		return out, nil
	}
	if row.Value.State != "claimed" && row.Value.State != "unknown" {
		return out, toolInstallationChanged()
	}
	if req.StopProof != nil && req.StopProof.SourceSHA256 != row.Value.SourceSHA256 {
		return out, toolInstallationChanged()
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	old := row.Value.Version
	effective := installationEffective(row.Value, clock)
	row.Value.State = req.Outcome
	row.Value.Version++
	row.Value.ResultCode = req.FailureCode
	if req.Outcome == "completed" {
		row.Value.ResultCode = "scan_completed"
	}
	if req.Outcome == "closed_unknown" && effective.ResultCode != "" {
		row.Value.ResultCode = effective.ResultCode
	}
	if req.Outcome != "unknown" {
		row.Value.FinishedAt = clock.UTC().Format(time.RFC3339Nano)
		row.Value.Result = &tii.Result{Items: req.Items, StopProof: req.StopProof}
	}
	if err = s.toolInstallationSaveTx(ctx, tx, row, old); err != nil {
		return out, err
	}
	out = row.Value
	if err = s.toolInspectionReceiptTx(ctx, tx, m, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}
