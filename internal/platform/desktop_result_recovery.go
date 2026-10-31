package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	rc "github.com/ai-mada/suma/internal/desktoprecoverycontract"
	"github.com/ai-mada/suma/internal/strictjson"
)

// DesktopResultRecoveryRequest is private to the existing loopback/HMAC bridge.
// It carries digests and original identities, never a new result or credential.
type DesktopResultRecoveryRequest struct {
	Scope               rc.Scope `json:"scope"`
	RecoveryOperationID string   `json:"recovery_operation_id"`
	Plan                rc.Plan  `json:"plan"`
}

func (r *DesktopResultRecoveryRequest) UnmarshalJSON(raw []byte) error {
	type wire DesktopResultRecoveryRequest
	var value wire
	if strictjson.Decode(raw, &value, rc.InputLimit) != nil {
		return desktopRunnerInvalid()
	}
	*r = DesktopResultRecoveryRequest(value)
	return nil
}

func (s *Service) desktopResultRecovery(parent context.Context, actor Actor, req DesktopResultRecoveryRequest, proof desktopRunnerProof) (rc.Proof, error) {
	var out rc.Proof
	if rc.ValidateScope(req.Scope) != nil || rc.ValidatePlan(req.Plan) != nil || !desktopWorkspaceHex(req.RecoveryOperationID, 32) || req.Scope.ProjectID != req.Plan.ProjectID || req.Scope.WorkerID != req.Plan.WorkerID {
		return out, desktopRunnerInvalid()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(parent)
	if err != nil {
		return out, err
	}
	defer cancel()
	// Current human authority uses its existing coordination locks. Nothing in
	// this fact lookup, including those locks' counters, is committed.
	defer tx.Rollback()
	auth, err := s.artifactAdminTx(ctx, tx, actor, req.Scope.ProjectID)
	if err != nil {
		return out, err
	}
	v := req.Scope
	if err = s.validateDesktopRunnerProof(auth, proof, "recovery-receipts", v.ProjectID, v.UserID, v.TenantID, v.WorkerID, v.AdapterID, v.InstanceID, v.ExpectedBindingSHA256, v.BaseCommit, v.RegistrationOperationID, v.CredentialSHA256, req.RecoveryOperationID, rc.PlanSHA256(req.Plan)); err != nil {
		return out, err
	}
	current, err := s.desktopResultRecoverySourceTx(ctx, tx, auth, v)
	if err != nil {
		return out, err
	}
	out = rc.Proof{Protocol: rc.ProofProtocol, Scope: v, PlanSHA256: rc.PlanSHA256(req.Plan), IssuedAt: auth.checkedAt.UTC().Format(time.RFC3339Nano), ExpiresAt: auth.checkedAt.UTC().Add(time.Minute).Format(time.RFC3339Nano), Items: make([]rc.Receipt, 0, len(req.Plan.Items))}
	for _, item := range req.Plan.Items {
		status := "source_changed"
		if current {
			status, err = s.desktopResultRecoveryReceiptTx(ctx, tx, v, item)
			if err != nil {
				return rc.Proof{}, err
			}
		}
		out.Items = append(out.Items, rc.Receipt{JournalKey: item.JournalKey, OperationID: item.OperationID, Status: status})
	}
	if err = ctx.Err(); err != nil {
		return rc.Proof{}, err
	}
	return rc.SignProof(out, s.DesktopWorkspaceKey), nil
}

func (s *Service) desktopResultRecoverySourceTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, scope rc.Scope) (bool, error) {
	p := auth.currentPrincipal
	var id, createdAt, currentDigest, holder, createdBy string
	err := tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT w.id,w.created_at,w.token_hash,x.user_id,w.created_by FROM suma_release_workers w JOIN suma_sessions x ON x.token_hash=w.token_hash WHERE w.tenant_id=? AND w.project_id=? AND w.worker_id=?`)), p.TenantID, scope.ProjectID, scope.WorkerID).Scan(&id, &createdAt, &currentDigest, &holder, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return false, forbidden()
	}
	if err != nil {
		return false, err
	}
	if holder != p.ID || createdBy != p.ID {
		return false, forbidden()
	}
	if currentDigest != scope.CredentialSHA256 {
		return false, nil
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), p.TenantID, scope.ProjectID, scope.RegistrationOperationID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var original DesktopRunnerRegistration
	if err = json.Unmarshal([]byte(raw), &original); err != nil {
		return false, err
	}
	if original.OperationID != scope.RegistrationOperationID || original.ProjectID != scope.ProjectID || original.UserID != p.ID || original.TenantID != p.TenantID || original.AdapterID != scope.AdapterID || original.InstanceID != scope.InstanceID || original.BindingSHA256 != scope.ExpectedBindingSHA256 || original.BaseCommit != scope.BaseCommit || original.Worker.ID != id || original.Worker.ProjectID != scope.ProjectID || original.Worker.WorkerID != scope.WorkerID || original.Worker.CreatedAt != createdAt {
		return false, nil
	}
	local, err := s.desktopRunnerContextTx(ctx, tx, auth)
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 404 || api.Status == 409 || api.Status == 422) {
			return false, nil
		}
		return false, err
	}
	return local.InstanceID == scope.InstanceID && local.BindingSHA256 == scope.ExpectedBindingSHA256 && local.BaseCommit == scope.BaseCommit, nil
}

func (s *Service) desktopResultRecoveryReceiptTx(ctx context.Context, tx *sql.Tx, scope rc.Scope, item rc.Item) (string, error) {
	var worker string
	var fence int64
	err := tx.QueryRowContext(ctx, s.q(`SELECT a.worker_id,a.fence FROM dag_attempts a JOIN dag_runs r ON r.id=a.run_id WHERE r.tenant_id=? AND r.project_id=? AND a.run_id=? AND a.node_id=? AND a.attempt=?`), scope.TenantID, scope.ProjectID, item.RunID, item.NodeID, item.Attempt).Scan(&worker, &fence)
	if errors.Is(err, sql.ErrNoRows) {
		return "source_changed", nil
	}
	if err != nil {
		return "", err
	}
	if worker != scope.WorkerID || strconv.FormatInt(fence, 10) != item.Fence {
		return "source_changed", nil
	}
	var fingerprint, resultID string
	err = tx.QueryRowContext(ctx, s.q(`SELECT fingerprint,result_id FROM dag_operations WHERE tenant_id=? AND project_id=? AND scope=? AND operation_id=?`), scope.TenantID, scope.ProjectID, "result:"+item.RunID, item.OperationID).Scan(&fingerprint, &resultID)
	if errors.Is(err, sql.ErrNoRows) {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if fingerprint != item.ResultSHA256 || resultID != item.RunID {
		return "mismatch", nil
	}
	return "matched", nil
}
