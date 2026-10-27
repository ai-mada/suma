package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func workspaceObservationSubjectHash(s workspaceObservationSubject) string {
	if s.Pool {
		return digest(s.Token)
	}
	return s.Actor.credential.digest
}
func workspaceObservationOperationKey(s workspaceObservationSubject, kind, key string) string {
	return digest("workspace-observation/" + kind + "/" + workspaceObservationSubjectHash(s) + "/" + key)
}
func (s *Service) workspaceObservationOperationTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, kind, key string, body any) (string, bool, error) {
	if !planningKey(key) {
		return "", false, invalid(workspaceObservationInvalid)
	}
	hash := digest(stringify(body))
	var previous, response string
	err := tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), subject.Tenant, subject.Project, workspaceObservationOperationKey(subject, kind, key)).Scan(&previous, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return hash, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if hash != previous {
		return "", false, conflict("Workspace observation operation content changed")
	}
	return response, true, nil
}
func (s *Service) saveWorkspaceObservationOperationTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, kind, key, hash string, response any) error {
	_, err := tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response)VALUES(?,?,?,?,?)`), subject.Tenant, subject.Project, workspaceObservationOperationKey(subject, kind, key), hash, stringify(response))
	return err
}

func (s *Service) ClaimWorkspaceObservation(ctx context.Context, actor Actor, project string, req woc.ClaimRequest) (*woc.Lease, error) {
	return s.claimWorkspaceObservation(ctx, workspaceObservationSubject{Actor: actor, Tenant: actor.credential.tenantID, Project: project}, req)
}
func (s *Service) ClaimPoolWorkspaceObservation(ctx context.Context, token string, req PoolWorkspaceObservationClaimRequest) (*woc.Lease, error) {
	if req.TenantID != req.Assignment.TenantID || req.ProjectID != req.Assignment.ProjectID || req.RunnerID != req.Assignment.PoolRunnerID {
		return nil, forbidden()
	}
	return s.claimWorkspaceObservation(ctx, workspaceObservationSubject{Pool: true, Token: token, Tenant: req.TenantID, Project: req.ProjectID, Runner: req.RunnerID, Assignment: req.Assignment}, req.ClaimRequest)
}
func (s *Service) claimWorkspaceObservation(ctx context.Context, subject workspaceObservationSubject, req woc.ClaimRequest) (out *woc.Lease, err error) {
	if req.ProjectID != subject.Project || req.WorkerID != req.Scope.WorkerID || woc.ValidateScope(req.Scope) != nil || !woc.ValidDigest(req.WorkspacePolicySHA256) || !planningKey(req.OperationID) {
		return nil, invalid(workspaceObservationInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer tx.Rollback()
	basis, err := s.workspaceObservationBasisTx(ctx, tx, subject.Tenant, subject.Project, req.Scope)
	if err != nil {
		return nil, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var id string
	previous, wasReplay, err := s.workspaceObservationOperationTx(ctx, tx, subject, "claim", req.OperationID, req)
	if err != nil {
		return nil, err
	}
	if wasReplay {
		var saved *woc.Lease
		if err = json.Unmarshal([]byte(previous), &saved); err != nil {
			return nil, err
		}
		if saved != nil {
			id = saved.Observation.ID
		}
	} else {
		err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_workspace_observations WHERE tenant_id=? AND project_id=? AND worker_id=? AND run_id=? AND node_id=? AND attempt=? AND assignment_fence=? AND expires_at>? AND (state='requested' OR state='leased' AND lease_until<=?) ORDER BY expires_at,id LIMIT 1`), subject.Tenant, subject.Project, req.WorkerID, req.Scope.RunID, req.Scope.NodeID, req.Scope.Attempt, req.Scope.Fence, clock, clock).Scan(&id)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}
	var record *workspaceObservationRecord
	if id != "" {
		r, e := s.workspaceObservationTx(ctx, tx, subject.Tenant, subject.Project, id)
		if e != nil {
			return nil, e
		}
		record = &r
	}
	// Even an empty queue requires the current machine identity and assignment.
	if err = s.workspaceObservationMachineAuthorityTx(ctx, tx, subject, basis, record); err != nil {
		return nil, err
	}
	clock, err = s.artifactClockTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	hash, replay, err := s.workspaceObservationOperationTx(ctx, tx, subject, "claim", req.OperationID, req)
	if err != nil {
		return nil, err
	}
	if replay {
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return nil, err
		}
		if out != nil {
			current, e := s.workspaceObservationTx(ctx, tx, subject.Tenant, subject.Project, out.Observation.ID)
			if e != nil {
				return nil, e
			}
			if current.Expires <= clock || current.LeaseUntil <= clock || current.LeaseFence != out.LeaseFence || current.Observation.State != "leased" {
				out = nil
			} else {
				if e = s.workspaceObservationMachineAuthorityTx(ctx, tx, subject, basis, &current); e != nil {
					return nil, e
				}
			}
		}
		return out, tx.Commit()
	}
	// A fully authorized poll that acquires no lease has no durable effect.
	// Reusing its operation ID may acquire later work; only an actual lease
	// freezes an idempotent response and consumes an operation receipt row.
	if record == nil {
		return nil, tx.Commit()
	}
	if record != nil {
		if record.Expires <= clock {
			return nil, tx.Commit()
		}
		if record.Observation.Scope != req.Scope {
			return nil, forbidden()
		}
		if record.Observation.WorkspacePolicySHA256 != "" && record.Observation.WorkspacePolicySHA256 != req.WorkspacePolicySHA256 {
			return nil, conflict(workspaceObservationStale)
		}
		old := record.Observation.Version
		record.Observation.State = "leased"
		record.Observation.Version++
		record.LeaseFence++
		record.LeaseUntil = min(clock+woc.LeaseSeconds, record.Expires)
		record.LeaseStarted = clock
		record.LeaseOwner = workspaceObservationSubjectHash(subject)
		record.Observation.WorkspacePolicySHA256 = req.WorkspacePolicySHA256
		if err = s.saveWorkspaceObservationTx(ctx, tx, subject.Tenant, subject.Project, record, old); err != nil {
			return nil, err
		}
		out = &woc.Lease{Observation: record.Observation, LeaseFence: record.LeaseFence, LeaseUntil: workspaceObservationTime(record.LeaseUntil)}
	}
	if err = s.saveWorkspaceObservationOperationTx(ctx, tx, subject, "claim", req.OperationID, hash, out); err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func (s *Service) CompleteWorkspaceObservation(ctx context.Context, actor Actor, project string, req woc.ResultRequest) (woc.Receipt, error) {
	return s.completeWorkspaceObservation(ctx, workspaceObservationSubject{Actor: actor, Tenant: actor.credential.tenantID, Project: project}, req)
}
func (s *Service) CompletePoolWorkspaceObservation(ctx context.Context, token string, req PoolWorkspaceObservationResultRequest) (woc.Receipt, error) {
	if req.TenantID != req.Assignment.TenantID || req.ProjectID != req.Assignment.ProjectID || req.RunnerID != req.Assignment.PoolRunnerID {
		return woc.Receipt{}, forbidden()
	}
	return s.completeWorkspaceObservation(ctx, workspaceObservationSubject{Pool: true, Token: token, Tenant: req.TenantID, Project: req.ProjectID, Runner: req.RunnerID, Assignment: req.Assignment}, req.ResultRequest)
}
func (s *Service) completeWorkspaceObservation(ctx context.Context, subject workspaceObservationSubject, req woc.ResultRequest) (out woc.Receipt, err error) {
	if req.ProjectID != subject.Project || req.WorkerID != req.Scope.WorkerID || woc.ValidateScope(req.Scope) != nil || !handoffID(req.RequestID, false) || req.LeaseFence < 1 || !woc.ValidDigest(req.WorkspacePolicySHA256) || !woc.ValidDigest(req.ResultSHA256) || !planningKey(req.OperationID) {
		return out, invalid(workspaceObservationInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	record, err := s.workspaceObservationTx(ctx, tx, subject.Tenant, subject.Project, req.RequestID)
	if err != nil {
		return out, err
	}
	if record.Observation.Scope != req.Scope {
		return out, forbidden()
	}
	basis, err := s.workspaceObservationBasisTx(ctx, tx, subject.Tenant, subject.Project, req.Scope)
	if err != nil {
		return out, err
	}
	if err = s.workspaceObservationMachineAuthorityTx(ctx, tx, subject, basis, &record); err != nil {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	if record.Expires <= clock || record.LeaseFence != req.LeaseFence || record.LeaseOwner != workspaceObservationSubjectHash(subject) || record.Observation.WorkspacePolicySHA256 != req.WorkspacePolicySHA256 {
		return out, conflict(workspaceObservationStale)
	}
	if req.ResultSHA256 != woc.ResultDigest(req.ObservationResult, req.ErrorCode) || woc.ValidateResult(record.Observation, req.ObservationResult, req.ErrorCode) != nil {
		return out, invalid(workspaceObservationInvalid)
	}
	if req.ObservationResult != nil && req.ObservationResult.List != nil {
		for _, entry := range req.ObservationResult.List.Entries {
			if forbiddenPath(entry.Path) {
				return out, invalid(workspaceObservationInvalid)
			}
		}
	}
	hash, replay, err := s.workspaceObservationOperationTx(ctx, tx, subject, "result", req.OperationID, req)
	if err != nil {
		return out, err
	}
	if replay {
		if record.ResultSHA != req.ResultSHA256 {
			return out, conflict(workspaceObservationStale)
		}
		if err = json.Unmarshal([]byte(hash), &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if record.Observation.State != "leased" || record.LeaseUntil <= clock {
		return out, conflict(workspaceObservationStale)
	}
	observed, e := time.Parse(time.RFC3339Nano, req.ObservedAt)
	created, _ := parseWorkspaceObservationTime(record.Observation.CreatedAt)
	if e != nil || observed.Unix() < max(created, record.LeaseStarted) || observed.Unix() > clock+5 || observed.Unix() > record.LeaseUntil {
		return out, invalid(workspaceObservationInvalid)
	}
	old := record.Observation.Version
	record.Observation.Version++
	record.Observation.State = "completed"
	if req.ErrorCode != "" {
		record.Observation.State = "failed"
	}
	record.Observation.Result = req.ObservationResult
	record.Observation.ErrorCode = req.ErrorCode
	record.Observation.ObservedAt = req.ObservedAt
	record.ResultSHA = req.ResultSHA256
	if err = s.saveWorkspaceObservationTx(ctx, tx, subject.Tenant, subject.Project, &record, old); err != nil {
		return out, err
	}
	out = woc.Receipt{RequestID: req.RequestID, LeaseFence: req.LeaseFence, ResultSHA256: req.ResultSHA256, Acknowledged: true}
	if err = s.saveWorkspaceObservationOperationTx(ctx, tx, subject, "result", req.OperationID, hash, out); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
