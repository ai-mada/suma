package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

// This maintenance protocol uses the existing outbound worker identity and SQL
// receipts. It has no task dispatcher, process slot, model call or config writer.
type toolInspectionMachine struct{ tenant, project, worker, identity string }

func (s *Service) toolInspectionNormalMachineTx(ctx context.Context, tx *sql.Tx, actor Actor, project, worker string) (toolInspectionMachine, error) {
	auth, err := s.releaseWorkerTx(ctx, tx, actor, project, worker, "", "", "identity")
	if err != nil {
		return toolInspectionMachine{}, err
	}
	return toolInspectionMachine{auth.target.tenantID, project, worker, digest(actor.credential.digest)}, nil
}
func (s *Service) toolInspectionPoolMachineTx(ctx context.Context, tx *sql.Tx, token, tenant, project, runner string) (toolInspectionMachine, error) {
	proof, _, err := s.authorizePoolRunnerTx(ctx, tx, token, runner, []dag.PoolCapabilityScope{{TenantID: tenant, ProjectID: project, Capabilities: []string{}}})
	if err != nil {
		return toolInspectionMachine{}, err
	}
	if len(proof.scopes) != 1 {
		return toolInspectionMachine{}, forbidden()
	}
	if err = proof.assertScope(tx, proof.scopes[0]); err != nil {
		return toolInspectionMachine{}, err
	}
	return toolInspectionMachine{tenant, project, proof.runner.WorkerID, digest(stringify([]any{runner, proof.runner.CredentialVersion, digest(token)}))}, nil
}
func (s *Service) toolInspectionOperationTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, kind, key string, body any) (op, hash, raw string, replay bool, err error) {
	op = digest("tool-inspection/" + kind + "/" + m.worker + "/" + key)
	hash = digest(stringify([]any{m.identity, body}))
	var saved string
	err = tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), m.tenant, m.project, op).Scan(&saved, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return op, hash, "", false, nil
	}
	if err != nil {
		return
	}
	if saved != hash {
		err = toolInspectionChanged()
		return
	}
	replay = true
	return
}
func (s *Service) toolInspectionReceiptTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, op, hash string, value any) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response)VALUES(?,?,?,?,?)`), m.tenant, m.project, op, hash, stringify(value))
	return err
}
func (s *Service) ClaimToolInspection(ctx context.Context, actor Actor, project string, req tic.ClaimRequest) (out tic.ClaimReply, err error) {
	if req.ProjectID != project || tic.ValidateClaimRequest(req) != nil {
		return out, toolInspectionInvalid()
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
	out, err = s.claimToolInspectionTx(ctx, tx, m, req.ClaimID, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) ClaimPoolToolInspection(ctx context.Context, token string, req tic.PoolToolInspectionClaimRequest) (out tic.ClaimReply, err error) {
	if !tic.ID(req.TenantID) || !tic.ID(req.ProjectID) || !tic.ID(req.RunnerID) || !tic.Operation(req.ClaimID) {
		return out, toolInspectionInvalid()
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
	out, err = s.claimToolInspectionTx(ctx, tx, m, req.ClaimID, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) claimToolInspectionTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, key string, req any) (out tic.ClaimReply, err error) {
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, m, "claim", key, req)
	if err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if replay {
		if json.Unmarshal([]byte(raw), &out) != nil {
			return out, toolInspectionChanged()
		}
		if out.Inspection == nil {
			return out, nil
		}
		r, e := s.toolInspectionReadTx(ctx, tx, m.tenant, m.project, out.Inspection.ID)
		if e != nil {
			return out, e
		}
		if r.Value.WorkerID != m.worker || r.Value.ClaimID != key || r.Value.Fence != out.Inspection.Fence {
			return out, toolInspectionChanged()
		}
		if e = s.toolInspectionCurrentTx(ctx, tx, r); e != nil {
			return out, e
		}
		v := toolInspectionEffective(r.Value, clock)
		out.Inspection = &v
		return out, nil
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id FROM suma_tool_inspections WHERE tenant_id=? AND project_id=? AND worker_id=? AND state IN ('pending','claimed','unknown') ORDER BY created_at,id LIMIT 10001`), m.tenant, m.project, m.worker)
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
	if len(ids) > tic.MaxProjectRequests {
		return out, toolInspectionChanged()
	}
	for _, id := range ids {
		r, e := s.toolInspectionReadTx(ctx, tx, m.tenant, m.project, id)
		if e != nil {
			return out, e
		}
		v := toolInspectionEffective(r.Value, clock)
		// An already-started/unknown inspection is never assigned again. A new
		// nonce cannot acquire a new fence or cause another external probe.
		if v.State == "claimed" || v.State == "unknown" {
			break
		}
		if v.State == "expired" {
			old := r.Value.Version
			r.Value = v
			r.Value.Version++
			if e = s.toolInspectionSaveTx(ctx, tx, r, old); e != nil {
				return out, e
			}
			continue
		}
		if e = s.toolInspectionCurrentTx(ctx, tx, r); e != nil {
			var api *APIError
			if !automationAuthorityFailure(e) && !(errors.As(e, &api) && api.Code == "tool_inspection_source_changed") {
				return out, e
			}
			old := r.Value.Version
			r.Value.State = "superseded"
			r.Value.ResultCode = "source_changed"
			r.Value.Version++
			if e = s.toolInspectionSaveTx(ctx, tx, r, old); e != nil {
				return out, e
			}
			continue
		}
		old := r.Value.Version
		r.Value.State = "claimed"
		r.Value.Version++
		r.Value.Fence++
		r.Value.ClaimID = key
		r.Value.ClaimedAt = clock.UTC().Format(time.RFC3339Nano)
		r.Value.LeaseUntil = clock.Add(tic.ClaimLeaseSeconds * time.Second).UTC().Format(time.RFC3339Nano)
		if e = s.toolInspectionSaveTx(ctx, tx, r, old); e != nil {
			return out, e
		}
		v = r.Value
		out.Inspection = &v
		break
	}
	if err = s.toolInspectionReceiptTx(ctx, tx, m, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}
func (s *Service) FailToolInspection(ctx context.Context, actor Actor, project string, req tic.FailureRequest) (out tic.Inspection, err error) {
	if req.ProjectID != project || tic.ValidateFailureRequest(req) != nil {
		return out, toolInspectionInvalid()
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
	out, err = s.failToolInspectionTx(ctx, tx, m, req)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) FailPoolToolInspection(ctx context.Context, token string, req tic.PoolToolInspectionFailureRequest) (out tic.Inspection, err error) {
	if !tic.ID(req.TenantID) || !tic.ID(req.ProjectID) || !tic.ID(req.RunnerID) {
		return out, toolInspectionInvalid()
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
	normal := tic.FailureRequest{ProjectID: req.ProjectID, WorkerID: m.worker, Inspection: req.Inspection, IdempotencyKey: req.IdempotencyKey, Outcome: req.Outcome, FailureCode: req.FailureCode}
	if tic.ValidateFailureRequest(normal) != nil {
		return out, toolInspectionInvalid()
	}
	out, err = s.failToolInspectionTx(ctx, tx, m, normal)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) toolInspectionReferenceTx(ctx context.Context, tx *sql.Tx, tenant, project, worker string, ref *tic.Reference) (row toolInspectionRow, err error) {
	if ref == nil || tic.ValidateReference(*ref) != nil {
		return row, toolInspectionInvalid()
	}
	row, err = s.toolInspectionReadTx(ctx, tx, tenant, project, ref.InspectionID)
	if err != nil {
		return row, err
	}
	if row.Value.WorkerID != worker || row.Value.ClaimID != ref.ClaimID || row.Value.Fence != ref.Fence {
		return row, toolInspectionChanged()
	}
	if err = s.toolInspectionCurrentTx(ctx, tx, row); err != nil {
		return row, err
	}
	return row, nil
}
func (s *Service) failToolInspectionTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, req tic.FailureRequest) (out tic.Inspection, err error) {
	row, err := s.toolInspectionReferenceTx(ctx, tx, m.tenant, m.project, m.worker, &req.Inspection)
	if err != nil {
		return out, err
	}
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, m, "failure", req.IdempotencyKey, req)
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(raw), &out)
		return out, err
	}
	if row.Value.State != "claimed" && row.Value.State != "unknown" {
		return out, toolInspectionChanged()
	}
	old := row.Value.Version
	row.Value.State = req.Outcome
	row.Value.ResultCode = req.FailureCode
	row.Value.Version++
	if err = s.toolInspectionSaveTx(ctx, tx, row, old); err != nil {
		return out, err
	}
	out = row.Value
	if err = s.toolInspectionReceiptTx(ctx, tx, m, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}

// Called only inside the original inventory-report transaction, after its
// identity guard and before its operation receipt. Uncorrelated periodic reports
// never reach this seam, regardless of how fresh their timestamps appear.
func (s *Service) completeToolInspectionTx(ctx context.Context, tx *sql.Tx, tenant, project, worker, operation, observed string, ref *tic.Reference, items []tc.Observation) error {
	row, err := s.toolInspectionReferenceTx(ctx, tx, tenant, project, worker, ref)
	if err != nil {
		return err
	}
	if row.Value.State == "completed" {
		if row.Value.InventoryOperationID == operation {
			return nil
		}
		return toolInspectionChanged()
	}
	if row.Value.State != "claimed" && row.Value.State != "unknown" {
		return toolInspectionChanged()
	}
	old := row.Value.Version
	row.Value.State = "completed"
	row.Value.Version++
	row.Value.ObservedAt = observed
	row.Value.InventoryOperationID = operation
	row.Value.ResultCode = "command_missing"
	for _, item := range items {
		if item.Profile.CommandID != row.Value.CommandID {
			continue
		}
		row.Value.ResultCode = "reported_unavailable"
		if stringify(item.Profile) != stringify(row.Profile) {
			row.Value.ResultCode = "profile_changed"
		} else if item.Available {
			row.Value.ResultCode = "reported_available"
		}
		break
	}
	return s.toolInspectionSaveTx(ctx, tx, row, old)
}
