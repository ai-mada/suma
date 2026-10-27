package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func (s *Service) CreateWorkspaceWrite(ctx context.Context, actor Actor, project string, req wcc.CreateRequest) (out wcc.Command, err error) {
	if req.ProjectID != project || !handoffID(project, false) || wcc.ValidateCreate(req) != nil || forbiddenPath(req.Path) || !planningKey(req.IdempotencyKey) {
		return out, invalid(workspaceWriteInvalid)
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
	var id, previous string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,request_hash FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND caller_hash=? AND operation_id=?`), actor.credential.tenantID, project, actor.credential.digest, req.IdempotencyKey).Scan(&id, &previous)
	if err == nil {
		r, e := s.workspaceWriteTx(ctx, tx, actor.credential.tenantID, project, id)
		if e != nil {
			return out, e
		}
		// Recovering an existing receipt grants no execution or source body.
		// Current read authority is sufficient even after write permission ends.
		if e = s.workspaceWriteReadAuthorityTx(ctx, tx, actor, project, r); e != nil {
			return out, e
		}
		if previous != wcc.RequestDigest(req) {
			return out, conflict(workspaceWriteChanged)
		}
		clock, e := s.artifactClockTx(ctx, tx)
		if e != nil {
			return out, e
		}
		if e = s.refreshWorkspaceWriteTx(ctx, tx, actor.credential.tenantID, project, &r, clock); e != nil {
			return out, e
		}
		return r.Command, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	basis, basisErr := s.workspaceObservationBasisTx(ctx, tx, actor.credential.tenantID, project, req.Scope)
	bases := []workspaceObservationBasis{}
	if basisErr == nil {
		bases = append(bases, basis)
	}
	if _, err = s.workspaceScopeBrowserTx(ctx, tx, actor, project, bases, purposeProjectWrite); err != nil {
		return out, err
	}
	// A concurrent caller may have created the same operation while this
	// transaction waited for its human authority lock.
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,request_hash FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND caller_hash=? AND operation_id=?`), actor.credential.tenantID, project, actor.credential.digest, req.IdempotencyKey).Scan(&id, &previous)
	if err == nil {
		if previous != wcc.RequestDigest(req) {
			return out, conflict(workspaceWriteChanged)
		}
		r, e := s.workspaceWriteTx(ctx, tx, actor.credential.tenantID, project, id)
		if e != nil {
			return out, e
		}
		clock, e := s.artifactClockTx(ctx, tx)
		if e == nil {
			e = s.refreshWorkspaceWriteTx(ctx, tx, actor.credential.tenantID, project, &r, clock)
		}
		if e != nil {
			return out, e
		}
		return r.Command, tx.Commit()
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
	if err = s.workspaceWriteTaskBasisTx(ctx, tx, actor.credential.tenantID, project, req.Scope.RunID); err != nil {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	var pending int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND state IN ('requested','leased','reconciling')`), actor.credential.tenantID, project).Scan(&pending); err != nil {
		return out, err
	}
	if pending >= 200 {
		return out, conflict("Workspace save queue is full")
	}
	out = wcc.Command{ID: newID(), ProjectID: project, Version: 1, State: "requested", Scope: req.Scope, WorkspacePolicySHA256: req.WorkspacePolicySHA256, Path: req.Path, ExpectedSHA256: req.ExpectedSHA256, ContentSHA256: req.ContentSHA256, RequestSHA256: wcc.RequestDigest(req), CreatedAt: workspaceObservationTime(clock), ExpiresAt: workspaceObservationTime(clock + wcc.RequestTTLSeconds)}
	r := workspaceWriteRecord{Command: out, Caller: expertCredentialSnapshot(actor), Assignment: basis.Assignment, PoolOwner: basis.PoolOwner, Claims: map[string]workspaceWriteClaim{}, Content: req.Content, Expires: clock + wcc.RequestTTLSeconds, NextExpiry: clock + wcc.RequestTTLSeconds}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_workspace_writes(id,tenant_id,project_id,caller_hash,operation_id,request_hash,run_id,node_id,attempt,assignment_fence,worker_id,policy_sha256,state,inspection_state,version,created_at,expires_at,execute_before,next_expiry,payload,content)VALUES(?,?,?,?,?,?,?,?,?,?,?,?,'requested','',1,?,?,0,?,?,?)`), out.ID, actor.credential.tenantID, project, actor.credential.digest, req.IdempotencyKey, out.RequestSHA256, req.Scope.RunID, req.Scope.NodeID, req.Scope.Attempt, req.Scope.Fence, req.Scope.WorkerID, req.WorkspacePolicySHA256, clock, r.Expires, r.NextExpiry, stringify(r), r.Content)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) GetWorkspaceWrite(ctx context.Context, actor Actor, project, id string) (out wcc.Command, err error) {
	if !handoffID(project, false) || !handoffID(id, false) {
		return out, invalid(workspaceWriteInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	r, loadErr := s.workspaceWriteTx(ctx, tx, actor.credential.tenantID, project, id)
	if loadErr != nil {
		if _, err = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead); err != nil {
			return out, err
		}
		return out, loadErr
	}
	if err = s.workspaceWriteReadAuthorityTx(ctx, tx, actor, project, r); err != nil {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = s.refreshWorkspaceWriteTx(ctx, tx, actor.credential.tenantID, project, &r, clock); err != nil {
		return out, err
	}
	return r.Command, tx.Commit()
}

func (s *Service) ListWorkspaceWrites(ctx context.Context, actor Actor, project string) (out wcc.Page, err error) {
	out = wcc.Page{ProjectID: project, Items: []wcc.Command{}, Limit: 50}
	if !handoffID(project, false) || (actor.kind != actorBrowser && actor.kind != actorProjectAPI) {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	// Discovery collects all authority participants before taking the same
	// globally ordered locks as individual metadata reads.
	rows, err := tx.QueryContext(ctx, s.q(`SELECT id,payload FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND caller_hash=? ORDER BY created_at DESC,id DESC LIMIT 50`), actor.credential.tenantID, project, actor.credential.digest)
	if err != nil {
		return out, err
	}
	ids, organizations := []string{}, []string{actor.credential.tenantID}
	for rows.Next() {
		var id, raw string
		var record workspaceWriteRecord
		if err = rows.Scan(&id, &raw); err == nil {
			err = json.Unmarshal([]byte(raw), &record)
		}
		if err != nil {
			rows.Close()
			return out, err
		}
		ids = append(ids, id)
		organizations = append(organizations, record.PoolOwner)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	if err = s.workspaceObservationLockActorsTx(ctx, tx, []Actor{actor}, nil, organizations); err != nil {
		return out, err
	}
	if _, err = s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, organizations); err != nil {
		return out, err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_workspace_writes WHERE tenant_id=? AND project_id=? AND caller_hash=?`), actor.credential.tenantID, project, actor.credential.digest).Scan(&out.Total); err != nil {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		r, e := s.workspaceWriteTx(ctx, tx, actor.credential.tenantID, project, id)
		if e == nil {
			e = s.workspaceWriteReadAuthorityTx(ctx, tx, actor, project, r)
		}
		if e == nil {
			e = s.refreshWorkspaceWriteTx(ctx, tx, actor.credential.tenantID, project, &r, clock)
		}
		if e != nil {
			return out, e
		}
		out.Items = append(out.Items, r.Command)
	}
	out.Truncated = out.Total > len(out.Items)
	return out, tx.Commit()
}

func (s *Service) RequestWorkspaceWriteInspection(ctx context.Context, actor Actor, project, id string, req wcc.InspectionRequest) (out wcc.Command, err error) {
	if req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningKey(req.IdempotencyKey) {
		return out, invalid(workspaceWriteInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	r, loadErr := s.workspaceWriteTx(ctx, tx, actor.credential.tenantID, project, id)
	if loadErr != nil {
		if _, err = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead); err != nil {
			return out, err
		}
		return out, loadErr
	}
	if err = s.workspaceWriteReadAuthorityTx(ctx, tx, actor, project, r); err != nil {
		return out, err
	}
	op := digest("workspace-write-inspection/" + actor.credential.digest + "/" + req.IdempotencyKey)
	requestSHA := digest(stringify([]any{id, req}))
	var previous, response string
	err = tx.QueryRowContext(ctx, s.q(`SELECT request_hash,response FROM suma_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), actor.credential.tenantID, project, op).Scan(&previous, &response)
	if err == nil {
		if previous != requestSHA {
			return out, conflict(workspaceWriteChanged)
		}
		if err = json.Unmarshal([]byte(response), &out); err != nil {
			return out, err
		}
		return out, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return out, err
	}
	if err = s.refreshWorkspaceWriteTx(ctx, tx, actor.credential.tenantID, project, &r, clock); err != nil {
		return out, err
	}
	if r.Command.Version != req.ExpectedVersion || r.Command.State != "reconciling" || r.Command.ExecutionFence != 1 || r.Command.InspectionState == "requested" || r.Command.InspectionState == "leased" {
		return out, conflict(workspaceWriteChanged)
	}
	if len(r.Claims) >= 65 {
		return out, invalid(workspaceWriteInvalid)
	}
	old := r.Command.Version
	r.Command.Version++
	r.Command.InspectionID = newID()
	r.Command.InspectionState = "requested"
	r.InspectionBefore = clock + wcc.RequestTTLSeconds
	workspaceWriteNextExpiry(&r)
	if err = s.saveWorkspaceWriteTx(ctx, tx, actor.credential.tenantID, project, &r, old); err != nil {
		return out, err
	}
	out = r.Command
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_operations(tenant_id,project_id,operation_id,request_hash,response)VALUES(?,?,?,?,?)`), actor.credential.tenantID, project, op, requestSHA, stringify(out))
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
}
