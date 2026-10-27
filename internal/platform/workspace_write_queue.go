package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func workspaceWriteOperationKey(subject workspaceObservationSubject, kind, key string) string {
	return digest("workspace-write/" + kind + "/" + workspaceObservationSubjectHash(subject) + "/" + key)
}
func (s *Service) workspaceWriteOperationTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, kind, key string, body any) (string, bool, error) {
	if !planningKey(key) {
		return "", false, invalid(workspaceWriteInvalid)
	}
	var previous, response string
	err := tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), subject.Tenant, subject.Project, workspaceWriteOperationKey(subject, kind, key)).Scan(&previous, &response)
	hash := digest(stringify(body))
	if errors.Is(err, sql.ErrNoRows) {
		return hash, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if previous != hash {
		return "", false, conflict(workspaceWriteChanged)
	}
	return response, true, nil
}
func (s *Service) saveWorkspaceWriteOperationTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, kind, key, hash string, response any) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response)VALUES(?,?,?,?,?)`), subject.Tenant, subject.Project, workspaceWriteOperationKey(subject, kind, key), hash, stringify(response))
	return err
}
func (s *Service) ClaimWorkspaceWrite(ctx context.Context, actor Actor, project string, req wcc.ClaimRequest) (*wcc.Lease, error) {
	return s.claimWorkspaceWrite(ctx, workspaceObservationSubject{Actor: actor, Tenant: actor.credential.tenantID, Project: project}, req)
}
func (s *Service) ClaimPoolWorkspaceWrite(ctx context.Context, token string, req PoolWorkspaceWriteClaimRequest) (*wcc.Lease, error) {
	if req.TenantID != req.Assignment.TenantID || req.ProjectID != req.Assignment.ProjectID || req.RunnerID != req.Assignment.PoolRunnerID {
		return nil, forbidden()
	}
	return s.claimWorkspaceWrite(ctx, workspaceObservationSubject{Pool: true, Token: token, Tenant: req.TenantID, Project: req.ProjectID, Runner: req.RunnerID, Assignment: req.Assignment}, req.ClaimRequest)
}
func (s *Service) claimWorkspaceWrite(ctx context.Context, subject workspaceObservationSubject, req wcc.ClaimRequest) (out *wcc.Lease, err error) {
	if req.ProjectID != subject.Project || req.WorkerID != req.Scope.WorkerID || woc.ValidateScope(req.Scope) != nil || !woc.ValidDigest(req.WorkspacePolicySHA256) || !planningKey(req.OperationID) {
		return nil, invalid(workspaceWriteInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer tx.Rollback()
	previous, replay, err := s.workspaceWriteOperationTx(ctx, tx, subject, "claim", req.OperationID, req)
	if err != nil {
		return nil, err
	}
	var id string
	if replay {
		var saved wcc.Lease
		if err = json.Unmarshal([]byte(previous), &saved); err != nil {
			return nil, err
		}
		id = saved.Command.ID
	} else {
		err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND worker_id=? AND run_id=? AND node_id=? AND attempt=? AND assignment_fence=? AND policy_sha256=? AND (inspection_state='requested' OR state='requested') ORDER BY CASE WHEN inspection_state='requested' THEN 0 ELSE 1 END,id LIMIT 1`), subject.Tenant, subject.Project, req.WorkerID, req.Scope.RunID, req.Scope.NodeID, req.Scope.Attempt, req.Scope.Fence, req.WorkspacePolicySHA256).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	if id == "" {
		basis, e := s.workspaceObservationBasisTx(ctx, tx, subject.Tenant, subject.Project, req.Scope)
		if e != nil {
			return nil, e
		}
		if e = s.workspaceObservationMachineAuthorityTx(ctx, tx, subject, basis, nil); e != nil {
			return nil, e
		}
		if e = s.requireWorkspaceWriteTransportTx(ctx, tx, subject.Tenant, subject.Project, basis); e != nil {
			return nil, e
		}
		return nil, tx.Commit()
	}
	r, err := s.workspaceWriteTx(ctx, tx, subject.Tenant, subject.Project, id)
	if err != nil {
		return nil, err
	}
	if !workspaceWriteScopeMatches(r, req) {
		return nil, forbidden()
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	mode := "execute"
	if r.Command.InspectionState == "requested" {
		mode = "inspect"
	}
	var saved wcc.Lease
	if replay {
		if err = json.Unmarshal([]byte(previous), &saved); err != nil {
			return nil, err
		}
		mode = saved.Mode
	}
	if mode == "execute" && r.Command.State == "requested" || mode == "execute" && r.Command.State == "leased" && r.ExecuteBefore > clock {
		basis, e := s.workspaceObservationBasisTx(ctx, tx, subject.Tenant, subject.Project, req.Scope)
		if e != nil {
			return nil, e
		}
		if e = s.workspaceWriteExecuteAuthorityTx(ctx, tx, subject, basis, r); e != nil {
			return nil, e
		}
	} else {
		if err = s.workspaceWriteFactAuthorityTx(ctx, tx, subject, r, true); err != nil {
			return nil, err
		}
	}
	clock, err = s.artifactClockTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err = s.refreshWorkspaceWriteTx(ctx, tx, subject.Tenant, subject.Project, &r, clock); err != nil {
		return nil, err
	}
	if replay {
		proof, ok := r.Claims[saved.ClaimID]
		if !ok || proof.Owner != workspaceObservationSubjectHash(subject) || proof.Mode != mode {
			return nil, forbidden()
		}
		if proof.LeaseUntil <= clock {
			return nil, tx.Commit()
		}
		if mode == "execute" {
			if r.Command.State != "leased" || r.Command.ExecuteClaimID != saved.ClaimID {
				return nil, tx.Commit()
			}
			if wcc.ContentDigest(r.Content) != r.Command.ContentSHA256 {
				return nil, conflict(workspaceWriteChanged)
			}
			content := r.Content
			saved.Content = &content
		} else if r.Command.InspectionID != saved.ClaimID || r.Command.InspectionState != "leased" {
			return nil, tx.Commit()
		}
		return &saved, tx.Commit()
	}
	old := r.Command.Version
	claimID := ""
	until := int64(0)
	if mode == "inspect" {
		if r.Command.InspectionState != "requested" || r.Command.ExecutionFence != 1 {
			return nil, tx.Commit()
		}
		claimID = r.Command.InspectionID
		until = clock + wcc.LeaseSeconds
		r.Command.InspectionState = "leased"
		r.InspectionBefore = until
	} else {
		if r.Command.State != "requested" || r.Command.ExecutionFence != 0 {
			return nil, tx.Commit()
		}
		if wcc.ContentDigest(r.Content) != r.Command.ContentSHA256 {
			return nil, conflict(workspaceWriteChanged)
		}
		r.Command.State = "leased"
		r.Command.ExecutionFence = 1
		r.Command.ExecuteClaimID = newID()
		claimID = r.Command.ExecuteClaimID
		until = min(clock+wcc.LeaseSeconds, r.Expires)
		r.ExecuteBefore = until
		r.Command.ExecuteBefore = workspaceObservationTime(until)
	}
	r.Command.Version++
	r.Claims[claimID] = workspaceWriteClaim{Mode: mode, Owner: workspaceObservationSubjectHash(subject), IssuedAt: clock, LeaseUntil: until}
	workspaceWriteNextExpiry(&r)
	if err = s.saveWorkspaceWriteTx(ctx, tx, subject.Tenant, subject.Project, &r, old); err != nil {
		return nil, err
	}
	out = &wcc.Lease{Command: r.Command, ClaimID: claimID, Mode: mode, LeaseUntil: workspaceObservationTime(until)}
	metadata := *out
	if mode == "execute" {
		content := r.Content
		out.Content = &content
	}
	// Operation receipts contain metadata only; source text has one bounded
	// delivery column, which is removed on timeout or a returned effect fact.
	if err = s.saveWorkspaceWriteOperationTx(ctx, tx, subject, "claim", req.OperationID, digest(stringify(req)), metadata); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func (s *Service) CompleteWorkspaceWrite(ctx context.Context, actor Actor, project string, req wcc.ResultRequest) (wcc.Receipt, error) {
	return s.completeWorkspaceWrite(ctx, workspaceObservationSubject{Actor: actor, Tenant: actor.credential.tenantID, Project: project}, req)
}
func (s *Service) CompletePoolWorkspaceWrite(ctx context.Context, token string, req PoolWorkspaceWriteResultRequest) (wcc.Receipt, error) {
	if req.TenantID != req.Assignment.TenantID || req.ProjectID != req.Assignment.ProjectID || req.RunnerID != req.Assignment.PoolRunnerID {
		return wcc.Receipt{}, forbidden()
	}
	return s.completeWorkspaceWrite(ctx, workspaceObservationSubject{Pool: true, Token: token, Tenant: req.TenantID, Project: req.ProjectID, Runner: req.RunnerID, Assignment: req.Assignment}, req.ResultRequest)
}
func (s *Service) completeWorkspaceWrite(ctx context.Context, subject workspaceObservationSubject, req wcc.ResultRequest) (out wcc.Receipt, err error) {
	if req.ProjectID != subject.Project || !handoffID(req.CommandID, false) || !planningKey(req.OperationID) {
		return out, invalid(workspaceWriteInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	r, err := s.workspaceWriteTx(ctx, tx, subject.Tenant, subject.Project, req.CommandID)
	if err != nil {
		return out, err
	}
	if wcc.ValidateResult(r.Command, req) != nil {
		return out, invalid(workspaceWriteInvalid)
	}
	if err = s.workspaceWriteFactAuthorityTx(ctx, tx, subject, r, false); err != nil {
		return out, err
	}
	claim, ok := r.Claims[req.ClaimID]
	if !ok || claim.Mode != req.Mode || claim.Owner != workspaceObservationSubjectHash(subject) {
		return out, forbidden()
	}
	if req.Mode == "execute" && req.ClaimID != r.Command.ExecuteClaimID {
		return out, forbidden()
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	observed, e := time.Parse(time.RFC3339Nano, req.ObservedAt)
	if e != nil || observed.Unix() < claim.IssuedAt-5 || observed.Unix() > clock+5 {
		return out, invalid(workspaceWriteInvalid)
	}
	hash, replay, err := s.workspaceWriteOperationTx(ctx, tx, subject, "result", req.OperationID, req)
	if err != nil {
		return out, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if claim.ResultSHA != "" && claim.ResultSHA != req.ResultSHA256 {
		return out, conflict(workspaceWriteChanged)
	}
	if r.Command.State == "applied" && req.Outcome == "applied" && woc.Digest(r.Command.Result) != woc.Digest(req.Effect) {
		return out, conflict(workspaceWriteChanged)
	}
	old := r.Command.Version
	r.Command.Version++
	r.Content = ""
	claim.ResultSHA = req.ResultSHA256
	r.Claims[req.ClaimID] = claim
	if req.Mode == "inspect" && r.Command.InspectionID == req.ClaimID {
		r.Command.InspectionState = "completed"
	}
	if req.Outcome == "applied" {
		r.Command.State = "applied"
		effect := *req.Effect
		r.Command.Result = &effect
		r.Command.ErrorCode = ""
	} else if r.Command.State != "applied" && r.Command.State != "not_applied" {
		if req.Outcome == "not_applied" {
			r.Command.State = "not_applied"
		} else {
			r.Command.State = "reconciling"
		}
		r.Command.ErrorCode = req.ErrorCode
	}
	workspaceWriteNextExpiry(&r)
	if err = s.saveWorkspaceWriteTx(ctx, tx, subject.Tenant, subject.Project, &r, old); err != nil {
		return out, err
	}
	out = wcc.Receipt{CommandID: req.CommandID, ClaimID: req.ClaimID, ResultSHA256: req.ResultSHA256, Acknowledged: true}
	if err = s.saveWorkspaceWriteOperationTx(ctx, tx, subject, "result", req.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
