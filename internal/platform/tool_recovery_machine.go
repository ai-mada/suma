package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	tic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func (s *Service) withToolRecoveryMachineTx(ctx context.Context, actor Actor, token, tenant, project, worker, runner string, work func(context.Context, *sql.Tx, toolInspectionMachine) error) error {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	defer tx.Rollback()
	var m toolInspectionMachine
	if runner == "" {
		m, err = s.toolInspectionNormalMachineTx(ctx, tx, actor, project, worker)
	} else {
		m, err = s.toolInspectionPoolMachineTx(ctx, tx, token, tenant, project, runner)
	}
	if err != nil {
		return err
	}
	if err = work(ctx, tx, m); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Service) PollToolInspectionRecovery(ctx context.Context, actor Actor, project string, req tic.ToolInspectionRecoveryPollRequest) (out tic.ToolInspectionRecoveryPoll, err error) {
	if req.ProjectID != project || !tic.ID(req.WorkerID) || tic.ValidateReference(req.Inspection) != nil {
		return out, toolInspectionInvalid()
	}
	err = s.withToolRecoveryMachineTx(ctx, actor, "", "", project, req.WorkerID, "", func(ctx context.Context, tx *sql.Tx, m toolInspectionMachine) error {
		var e error
		out, e = s.pollToolRecoveryTx(ctx, tx, m, req.Inspection)
		return e
	})
	return
}
func (s *Service) PollPoolToolInspectionRecovery(ctx context.Context, token string, req tic.PoolToolInspectionRecoveryPollRequest) (out tic.ToolInspectionRecoveryPoll, err error) {
	if !tic.ID(req.TenantID) || !tic.ID(req.ProjectID) || !tic.ID(req.RunnerID) || tic.ValidateReference(req.Inspection) != nil {
		return out, toolInspectionInvalid()
	}
	err = s.withToolRecoveryMachineTx(ctx, Actor{}, token, req.TenantID, req.ProjectID, "", req.RunnerID, func(ctx context.Context, tx *sql.Tx, m toolInspectionMachine) error {
		var e error
		out, e = s.pollToolRecoveryTx(ctx, tx, m, req.Inspection)
		return e
	})
	return
}
func (s *Service) pollToolRecoveryTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, ref tic.Reference) (out tic.ToolInspectionRecoveryPoll, err error) {
	original, err := s.toolInspectionReadTx(ctx, tx, m.tenant, m.project, ref.InspectionID)
	if err != nil {
		return out, err
	}
	if original.Value.WorkerID != m.worker || original.Value.ClaimID != ref.ClaimID || original.Value.Fence != ref.Fence {
		return out, toolRecoveryChanged()
	}
	var id string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_tool_inspection_recoveries WHERE tenant_id=? AND project_id=? AND inspection_id=? ORDER BY created_at DESC,id DESC LIMIT 1`), m.tenant, m.project, ref.InspectionID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	row, err := s.toolRecoveryReadTx(ctx, tx, m.tenant, m.project, id)
	if err != nil {
		return out, err
	}
	if _, err = s.toolRecoveryGuardTx(ctx, tx, row); err != nil {
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	v := toolRecoveryEffective(row.Value, clock)
	out.Recovery = &v
	return out, nil
}
func recoveryProofRequestValid(req tic.ToolInspectionRecoveryProofRequest) bool {
	return tic.ID(req.ProjectID) && tic.ID(req.WorkerID) && tic.ID(req.RecoveryID) && ingressVersion(req.ExpectedVersion) && tic.Operation(req.IdempotencyKey) && tic.ValidateStopProof(req.Proof) == nil
}
func (s *Service) StoreToolInspectionRecoveryProof(ctx context.Context, actor Actor, project string, req tic.ToolInspectionRecoveryProofRequest) (out tic.ToolInspectionRecovery, err error) {
	if req.ProjectID != project || !recoveryProofRequestValid(req) {
		return out, toolInspectionInvalid()
	}
	err = s.withToolRecoveryMachineTx(ctx, actor, "", "", project, req.WorkerID, "", func(ctx context.Context, tx *sql.Tx, m toolInspectionMachine) error {
		var e error
		out, e = s.storeToolRecoveryProofTx(ctx, tx, m, req)
		return e
	})
	return
}
func (s *Service) StorePoolToolInspectionRecoveryProof(ctx context.Context, token string, req tic.PoolToolInspectionRecoveryProofRequest) (out tic.ToolInspectionRecovery, err error) {
	if !tic.ID(req.TenantID) || !tic.ID(req.ProjectID) || !tic.ID(req.RunnerID) {
		return out, toolInspectionInvalid()
	}
	err = s.withToolRecoveryMachineTx(ctx, Actor{}, token, req.TenantID, req.ProjectID, "", req.RunnerID, func(ctx context.Context, tx *sql.Tx, m toolInspectionMachine) error {
		normal := tic.ToolInspectionRecoveryProofRequest{ProjectID: req.ProjectID, WorkerID: m.worker, RecoveryID: req.RecoveryID, ExpectedVersion: req.ExpectedVersion, IdempotencyKey: req.IdempotencyKey, Proof: req.Proof}
		if !recoveryProofRequestValid(normal) {
			return toolInspectionInvalid()
		}
		var e error
		out, e = s.storeToolRecoveryProofTx(ctx, tx, m, normal)
		return e
	})
	return
}
func (s *Service) storeToolRecoveryProofTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, req tic.ToolInspectionRecoveryProofRequest) (out tic.ToolInspectionRecovery, err error) {
	row, err := s.toolRecoveryReadTx(ctx, tx, m.tenant, m.project, req.RecoveryID)
	if err != nil {
		return out, err
	}
	if row.Value.WorkerID != m.worker {
		return out, toolRecoveryChanged()
	}
	original, err := s.toolRecoveryGuardTx(ctx, tx, row)
	if err != nil {
		return out, err
	}
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, m, "recovery-proof", req.IdempotencyKey, req)
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(raw), &out)
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if toolRecoveryEffective(row.Value, clock).State != "requested" || row.Value.Version != req.ExpectedVersion || toolInspectionEffective(original.Value, clock).State != "unknown" || req.Proof.Inspection != row.Value.Inspection || req.Proof.SourceSHA256 != row.Value.SourceSHA256 {
		return out, toolRecoveryChanged()
	}
	observed, _ := time.Parse(time.RFC3339Nano, req.Proof.ObservedAt)
	claimed, _ := time.Parse(time.RFC3339Nano, original.Value.ClaimedAt)
	if observed.Before(claimed) || observed.After(clock.Add(5*time.Second)) {
		return out, toolInspectionInvalid()
	}
	old := row.Value.Version
	proof := req.Proof
	row.Value.Proof = &proof
	row.Value.ProofSHA256 = tic.StopProofDigest(proof)
	row.Value.ProofStoredAt = clock.UTC().Format(time.RFC3339Nano)
	row.Value.State = "proof_stored"
	row.Value.Version++
	if err = s.toolRecoverySaveTx(ctx, tx, row, old); err != nil {
		return out, err
	}
	out = row.Value
	if err = s.toolInspectionReceiptTx(ctx, tx, m, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}
func recoveryAcknowledgementValid(req tic.ToolInspectionRecoveryAcknowledgementRequest) bool {
	return tic.ID(req.ProjectID) && tic.ID(req.WorkerID) && tic.ID(req.RecoveryID) && ingressVersion(req.ExpectedVersion) && tic.Operation(req.IdempotencyKey) && contentDigest.MatchString(req.ProofSHA256)
}
func (s *Service) AcknowledgeToolInspectionRecovery(ctx context.Context, actor Actor, project string, req tic.ToolInspectionRecoveryAcknowledgementRequest) (out tic.ToolInspectionRecovery, err error) {
	if req.ProjectID != project || !recoveryAcknowledgementValid(req) {
		return out, toolInspectionInvalid()
	}
	err = s.withToolRecoveryMachineTx(ctx, actor, "", "", project, req.WorkerID, "", func(ctx context.Context, tx *sql.Tx, m toolInspectionMachine) error {
		var e error
		out, e = s.acknowledgeToolRecoveryTx(ctx, tx, m, req)
		return e
	})
	return
}
func (s *Service) AcknowledgePoolToolInspectionRecovery(ctx context.Context, token string, req tic.PoolToolInspectionRecoveryAcknowledgementRequest) (out tic.ToolInspectionRecovery, err error) {
	if !tic.ID(req.TenantID) || !tic.ID(req.ProjectID) || !tic.ID(req.RunnerID) {
		return out, toolInspectionInvalid()
	}
	err = s.withToolRecoveryMachineTx(ctx, Actor{}, token, req.TenantID, req.ProjectID, "", req.RunnerID, func(ctx context.Context, tx *sql.Tx, m toolInspectionMachine) error {
		normal := tic.ToolInspectionRecoveryAcknowledgementRequest{ProjectID: req.ProjectID, WorkerID: m.worker, RecoveryID: req.RecoveryID, ExpectedVersion: req.ExpectedVersion, IdempotencyKey: req.IdempotencyKey, ProofSHA256: req.ProofSHA256}
		if !recoveryAcknowledgementValid(normal) {
			return toolInspectionInvalid()
		}
		var e error
		out, e = s.acknowledgeToolRecoveryTx(ctx, tx, m, normal)
		return e
	})
	return
}
func (s *Service) acknowledgeToolRecoveryTx(ctx context.Context, tx *sql.Tx, m toolInspectionMachine, req tic.ToolInspectionRecoveryAcknowledgementRequest) (out tic.ToolInspectionRecovery, err error) {
	row, err := s.toolRecoveryReadTx(ctx, tx, m.tenant, m.project, req.RecoveryID)
	if err != nil {
		return out, err
	}
	if row.Value.WorkerID != m.worker {
		return out, toolRecoveryChanged()
	}
	original, err := s.toolRecoveryGuardTx(ctx, tx, row)
	if err != nil {
		return out, err
	}
	op, hash, raw, replay, err := s.toolInspectionOperationTx(ctx, tx, m, "recovery-ack", req.IdempotencyKey, req)
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(raw), &out)
		return out, err
	}
	clock, err := s.toolInventoryClock(ctx, tx)
	if err != nil {
		return out, err
	}
	if toolRecoveryEffective(row.Value, clock).State != "proof_stored" || row.Value.Version != req.ExpectedVersion || row.Value.Proof == nil || row.Value.ProofSHA256 != req.ProofSHA256 || toolInspectionEffective(original.Value, clock).State != "unknown" {
		return out, toolRecoveryChanged()
	}
	// The machine's durable acceptance names exactly the stored native receipt.
	// Closing releases only maintenance occupancy and preserves the unknown code.
	oldRecovery, oldInspection := row.Value.Version, original.Value.Version
	closed := clock.UTC().Format(time.RFC3339Nano)
	row.Value.State = "closed_unknown"
	row.Value.ClosedAt = closed
	row.Value.Version++
	original.Value = toolInspectionEffective(original.Value, clock)
	original.Value.State = "closed_unknown"
	original.Value.RecoveryID = row.Value.ID
	original.Value.ClosedAt = closed
	original.Value.Version++
	if err = s.toolInspectionSaveTx(ctx, tx, original, oldInspection); err != nil {
		return out, err
	}
	if err = s.toolRecoverySaveTx(ctx, tx, row, oldRecovery); err != nil {
		return out, err
	}
	out = row.Value
	if err = s.toolInspectionReceiptTx(ctx, tx, m, op, hash, out); err != nil {
		return out, err
	}
	return out, nil
}
