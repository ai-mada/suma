package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	woc "github.com/ai-mada/suma/internal/workspaceobservationcontract"
)

func (s *Service) workspaceObservationBrowserTx(ctx context.Context, tx *sql.Tx, actor Actor, project string, bases []workspaceObservationBasis) (*authorizedCommand, error) {
	users, orgs := []string{}, []string{actor.credential.tenantID}
	for _, basis := range bases {
		more, err := s.workspaceObservationBasisUsersTx(ctx, tx, actor.credential.tenantID, project, basis)
		if err != nil {
			return nil, err
		}
		users = append(users, more...)
		orgs = append(orgs, basis.PoolOwner)
	}
	if err := s.workspaceObservationLockActorsTx(ctx, tx, []Actor{actor}, users, orgs); err != nil {
		return nil, err
	}
	return s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, orgs)
}

func (s *Service) GetWorkspaceObservationTargets(ctx context.Context, actor Actor, project string) (out woc.TargetPage, err error) {
	out = woc.TargetPage{ProjectID: project, Items: []woc.Target{}, Limit: 50}
	if !handoffID(project, false) || (actor.kind != actorBrowser && actor.kind != actorProjectAPI) {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, s.q(`SELECT n.run_id,n.id,n.attempt,n.fence,n.worker_id,n.spec FROM dag_nodes n JOIN dag_runs r ON r.id=n.run_id WHERE r.tenant_id=? AND r.project_id=? AND n.state='running' AND r.state IN ('running','paused') AND n.lease_until>? ORDER BY n.run_id,n.id LIMIT 201`), actor.credential.tenantID, project, clock)
	if err != nil {
		return out, err
	}
	scopes := []woc.Scope{}
	scanned := 0
	for rows.Next() {
		var scope woc.Scope
		var raw string
		if err = rows.Scan(&scope.RunID, &scope.NodeID, &scope.Attempt, &scope.Fence, &scope.WorkerID, &raw); err != nil {
			rows.Close()
			return out, err
		}
		scanned++
		var node dag.Node
		if err = json.Unmarshal([]byte(raw), &node); err != nil {
			rows.Close()
			return out, err
		}
		id, sha, bound, e := dag.ToolBinding(node)
		if e == nil && bound {
			scope.ToolProfileID, scope.ProfileSHA256 = id, sha
			scopes = append(scopes, scope)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	bases := []workspaceObservationBasis{}
	for _, scope := range scopes {
		basis, e := s.workspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, scope)
		if e != nil {
			var api *APIError
			if errors.As(e, &api) {
				continue
			}
			return out, e
		}
		bases = append(bases, basis)
	}
	if _, err = s.workspaceObservationBrowserTx(ctx, tx, actor, project, bases); err != nil {
		return out, err
	}
	out.ObservedAt = workspaceObservationTime(clock)
	out.Truncated = scanned > 200
	for _, basis := range bases {
		if e := s.validateWorkspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, basis); e != nil {
			var api *APIError
			if errors.As(e, &api) {
				continue
			}
			return out, e
		}
		out.Total++
		if len(out.Items) < out.Limit {
			out.Items = append(out.Items, woc.Target{WorkspaceObservationScope: basis.Scope, RunName: basis.RunName, NodeName: basis.NodeName, PoolID: basis.Assignment.PoolID})
		} else {
			out.Truncated = true
		}
	}
	return out, tx.Commit()
}

func (s *Service) CreateWorkspaceObservation(ctx context.Context, actor Actor, project string, req woc.CreateRequest) (out woc.Observation, err error) {
	if req.ProjectID != project || !handoffID(project, false) || woc.ValidateScope(req.Scope) != nil || woc.ValidateOperation(req.Operation, req.Arguments, req.ExpectedSHA256) != nil || forbiddenPath(req.Arguments.Path) || forbiddenPath(req.Arguments.Directory) || !planningKey(req.IdempotencyKey) {
		return out, invalid(workspaceObservationInvalid)
	}
	if actor.kind != actorBrowser && actor.kind != actorProjectAPI {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	basis, basisErr := s.workspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, req.Scope)
	bases := []workspaceObservationBasis{}
	if basisErr == nil {
		bases = append(bases, basis)
	}
	if _, err = s.workspaceObservationBrowserTx(ctx, tx, actor, project, bases); err != nil {
		return out, err
	}
	var id, requestHash string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,request_hash FROM suma_workspace_observations WHERE tenant_id=? AND project_id=? AND caller_hash=? AND operation_id=?`), actor.credential.tenantID, project, actor.credential.digest, req.IdempotencyKey).Scan(&id, &requestHash)
	if err == nil {
		if requestHash != digest(stringify(req)) {
			return out, conflict("Workspace observation operation content changed")
		}
		record, e := s.workspaceObservationTx(ctx, tx, actor.credential.tenantID, project, id)
		if e != nil {
			return out, e
		}
		out, e = s.workspaceObservationPublicTx(ctx, tx, actor, project, &record)
		if e != nil {
			return out, e
		}
		return out, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if basisErr != nil {
		return out, basisErr
	}
	if err = s.validateWorkspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, basis); err != nil {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_observations WHERE tenant_id=? AND project_id=? AND expires_at>?`), actor.credential.tenantID, project, clock).Scan(&pending); err != nil {
		return out, err
	}
	if pending >= 200 {
		return out, conflict("Workspace observation queue is full")
	}
	out = woc.Observation{ID: newID(), ProjectID: project, Version: 1, State: "requested", Scope: req.Scope, Operation: req.Operation, Arguments: req.Arguments, ExpectedSHA256: req.ExpectedSHA256, CreatedAt: workspaceObservationTime(clock), ExpiresAt: workspaceObservationTime(clock + woc.RequestTTLSeconds)}
	record := workspaceObservationRecord{Observation: out, Caller: expertCredentialSnapshot(actor), Assignment: basis.Assignment, PoolOwner: basis.PoolOwner, Expires: clock + woc.RequestTTLSeconds}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_observations(id,tenant_id,project_id,caller_hash,operation_id,request_hash,run_id,node_id,attempt,assignment_fence,worker_id,state,version,lease_fence,lease_until,lease_owner,expires_at,payload,result,result_sha256)VALUES(?,?,?,?,?,?,?,?,?,?,?,'requested',1,0,0,'',?,?,'','')`), out.ID, actor.credential.tenantID, project, actor.credential.digest, req.IdempotencyKey, digest(stringify(req)), req.Scope.RunID, req.Scope.NodeID, req.Scope.Attempt, req.Scope.Fence, req.Scope.WorkerID, record.Expires, stringify(record))
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) workspaceObservationPublicTx(ctx context.Context, tx *sql.Tx, actor Actor, project string, r *workspaceObservationRecord) (woc.Observation, error) {
	if r.Caller.Digest != actor.credential.digest || r.Caller.SubjectID != actor.credential.subjectID {
		return woc.Observation{}, forbidden()
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return woc.Observation{}, err
	}
	old := r.Observation.Version
	if r.Expires <= clock {
		if r.Observation.State != "expired" {
			r.Observation.State = "expired"
			r.Observation.Result = nil
			r.ResultSHA = ""
			r.LeaseUntil = 0
			r.LeaseOwner = ""
			r.Observation.Version++
			if err = s.saveWorkspaceObservationTx(ctx, tx, actor.credential.tenantID, project, r, old); err != nil {
				return woc.Observation{}, err
			}
		}
		return r.Observation, nil
	}
	if r.Observation.State == "cancelled" {
		r.Observation.Result = nil
		return r.Observation, nil
	}
	basis, e := s.workspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, r.Observation.Scope)
	if e == nil {
		e = s.validateWorkspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, basis)
	}
	if e != nil {
		var api *APIError
		if !errors.As(e, &api) && !errors.Is(e, dag.ErrStale) {
			return woc.Observation{}, e
		}
		r.Observation.State = "cancelled"
		r.Observation.ErrorCode = "workspace_closed"
		r.Observation.Result = nil
		r.ResultSHA = ""
		r.LeaseUntil = 0
		r.LeaseOwner = ""
		r.Observation.Version++
		if err = s.saveWorkspaceObservationTx(ctx, tx, actor.credential.tenantID, project, r, old); err != nil {
			return woc.Observation{}, err
		}
	}
	return r.Observation, nil
}

func (s *Service) GetWorkspaceObservation(ctx context.Context, actor Actor, project, id string) (out woc.Observation, err error) {
	if !handoffID(project, false) || !handoffID(id, false) || (actor.kind != actorBrowser && actor.kind != actorProjectAPI) {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	record, loadErr := s.workspaceObservationTx(ctx, tx, actor.credential.tenantID, project, id)
	bases := []workspaceObservationBasis{}
	if loadErr == nil {
		basis, e := s.workspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, record.Observation.Scope)
		if e == nil {
			bases = append(bases, basis)
		}
	}
	if _, err = s.workspaceObservationBrowserTx(ctx, tx, actor, project, bases); err != nil {
		return out, err
	}
	if loadErr != nil {
		return out, loadErr
	}
	out, err = s.workspaceObservationPublicTx(ctx, tx, actor, project, &record)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
