package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func (s *Service) repositoryRecoveryEligibleTx(ctx context.Context, tx *sql.Tx, op repositoryOperation) (bool, error) {
	if op.Assignment == nil {
		return false, repositoryConflict("unknown", repositoryUnknown)
	}
	var state, worker string
	var attempt int
	var fence, lease int64
	err := tx.QueryRowContext(ctx, s.q(`SELECT state,attempt,fence,worker_id,lease_until FROM dag_nodes WHERE run_id=? AND id=?`), op.RunID, op.NodeID).Scan(&state, &attempt, &fence, &worker, &lease)
	if err != nil {
		return false, err
	}
	a := op.Assignment
	if attempt != a.Attempt || fence != a.Fence || worker != a.WorkerID {
		return false, repositoryConflict("unknown", repositoryUnknown)
	}
	clock, err := s.artifactClockTx(ctx, tx)
	if err != nil {
		return false, err
	}
	return state == "reconciling" || state == "running" && lease <= clock, nil
}
func (s *Service) issueRepositoryReconciliation(ctx context.Context, actor Actor, project string, op repositoryOperation) (ticket string, err error) {
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, op.Execution.PlanID, purposeProjectApprove)
	if err != nil {
		return "", err
	}
	defer close()
	current, err := s.repositoryOperationTx(ctx, auth.tx, auth.target.tenantID, project, op.Execution.OperationID)
	if err != nil {
		return "", err
	}
	if current.Assignment == nil || op.Assignment == nil || current.RequestSHA256 != op.RequestSHA256 || !repositoryAssignmentEqual(*current.Assignment, *op.Assignment) {
		return "", repositoryConflict("unknown", repositoryUnknown)
	}
	ok, err := s.repositoryRecoveryEligibleTx(ctx, auth.tx, current)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", repositoryConflict("unknown", repositoryUnknown)
	}
	if op.Execution.Phase == "apply" {
		target, e := s.repositoryTargetTx(ctx, auth.tx, auth.target.tenantID, project, op.Execution.TargetID)
		if e != nil {
			return "", e
		}
		if target.ActiveOperationID != op.Execution.OperationID || target.Generation != op.Execution.Generation {
			return "", repositoryConflict("busy", repositoryBusy)
		}
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return "", err
	}
	ticket = newID() + newID()
	grant := ri.RepositoryReconciliationGrant{ReconciliationID: newID(), OperationID: op.Execution.OperationID, RequestSHA256: op.RequestSHA256, Generation: op.Execution.Generation, State: "granted", Execution: op.Execution, Assignment: *op.Assignment}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_reconciliation_grants(id,tenant_id,project_id,operation_id,request_sha256,ticket_hash,expires_at,state,actor_credential,data)VALUES(?,?,?,?,?,?,?,'issued',?,?)`), grant.ReconciliationID, auth.target.tenantID, project, grant.OperationID, grant.RequestSHA256, digest(ticket), clock.Add(120*time.Second).Format(time.RFC3339Nano), actor.credential.digest, stringify(grant)); err != nil {
		return "", err
	}
	return ticket, auth.tx.Commit()
}
func (s *Service) ConsumeRepositoryReconciliation(ctx context.Context, token string, req ri.RepositoryConsumeReconciliation) (out ri.RepositoryReconciliationGrant, err error) {
	cfg, err := s.authenticateRepositoryProvider(token)
	if err != nil {
		return out, err
	}
	if req.ProviderID != cfg.ID || req.ReconciliationTicket == "" || !req.Assignment.Valid() {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	if err = s.lockOrganization(ctx, tx, cfg.TenantID); err != nil {
		return out, err
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, cfg.TenantID, cfg.ProjectID); err != nil {
		return out, err
	}
	var raw, state, expiry, credential string
	err = tx.QueryRowContext(ctx, s.q(`SELECT data,state,expires_at,actor_credential FROM suma_repository_reconciliation_grants WHERE tenant_id=? AND project_id=? AND ticket_hash=?`), cfg.TenantID, cfg.ProjectID, digest(req.ReconciliationTicket)).Scan(&raw, &state, &expiry, &credential)
	if err != nil {
		return out, forbidden()
	}
	if json.Unmarshal([]byte(raw), &out) != nil || out.OperationID != req.OperationID || out.RequestSHA256 != req.RequestSHA256 || !ri.RepositoryExecutionEqual(out.Execution, req.Execution) || !repositoryAssignmentEqual(out.Assignment, req.Assignment) {
		return out, forbidden()
	}
	if state == "consumed" {
		return out, nil
	}
	if state != "issued" {
		return out, forbidden()
	}
	c, err := s.readCredential(ctx, tx, credential, true)
	if err != nil {
		return out, err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return out, err
	}
	until, e := time.Parse(time.RFC3339Nano, expiry)
	if e != nil || !clock.Before(until) || !c.expires.After(clock) || c.kind != actorBrowser && c.kind != actorProjectAPI || c.ref.tenantID != cfg.TenantID || c.kind == actorProjectAPI && c.ref.projectID != cfg.ProjectID {
		return out, forbidden()
	}
	if _, err = s.currentProjectMembershipLockedTx(ctx, tx, cfg.TenantID, c.ref.subjectID, cfg.ProjectID, false, true); err != nil {
		return out, err
	}
	op, err := s.repositoryOperationTx(ctx, tx, cfg.TenantID, cfg.ProjectID, out.OperationID)
	if err != nil {
		return out, err
	}
	if op.Assignment == nil || !repositoryAssignmentEqual(*op.Assignment, out.Assignment) || op.RequestSHA256 != out.RequestSHA256 || !ri.RepositoryExecutionEqual(op.Execution, out.Execution) {
		return out, forbidden()
	}
	ok, err := s.repositoryRecoveryEligibleTx(ctx, tx, op)
	if err != nil {
		return out, err
	}
	if !ok {
		return out, repositoryConflict("unknown", repositoryUnknown)
	}
	if op.Execution.Phase == "apply" {
		target, e := s.repositoryTargetTx(ctx, tx, cfg.TenantID, cfg.ProjectID, op.Execution.TargetID)
		if e != nil {
			return out, e
		}
		if target.ActiveOperationID != op.Execution.OperationID || target.Generation != op.Execution.Generation {
			return out, repositoryConflict("busy", repositoryBusy)
		}
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_repository_reconciliation_grants SET state='consumed' WHERE id=? AND state='issued'`), out.ReconciliationID); err != nil {
		return out, err
	}
	return out, tx.Commit()
}

func (s *Service) reconcileRepositoryIntegration(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req RepositoryIntegrationPlanCommandRequest) (out RepositoryIntegrationPlan, err error) {
	first, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeProjectApprove)
	if err != nil {
		return out, err
	}
	body := map[string]any{"repository_plan_command": id, "request": req}
	hash, replay, err := s.commandReceipt(first, auth, req.IdempotencyKey, body)
	if err != nil {
		close()
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		if err == nil {
			out, err = s.repositoryPlanProjectionTx(first, auth, out)
		}
		close()
		return out, err
	}
	plan, err := s.repositoryPlanTx(first, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		close()
		return out, err
	}
	if plan.Version != req.ExpectedVersion {
		close()
		return out, repositoryConflict("changed", repositoryChanged)
	}
	op, err := s.repositoryOperationTx(first, auth.tx, auth.target.tenantID, project, plan.OperationID)
	if err != nil {
		close()
		return out, err
	}
	cfg, err := s.repositoryProvider(auth.target.tenantID, project, op.Execution.ProviderID, op.Execution.TargetAlias)
	close()
	if err != nil {
		return out, err
	}
	var observed ri.RepositoryObservation
	err = repositoryProviderHTTP(ctx, cfg, "GET", "/operations/"+url.PathEscape(op.Execution.OperationID), nil, &observed)
	missing := false
	if err != nil {
		var api *APIError
		if !errors.As(err, &api) || api.Code != "repository_integration_operation_missing" {
			return out, err
		}
		missing = true
	}
	if !missing && ri.RepositoryValidateObservation(observed, op.Execution, op.RequestSHA256) != nil {
		return out, repositoryConflict("observation", repositoryObservation)
	}
	if missing || !repositoryKnownObservation(observed) {
		ticket, e := s.issueRepositoryReconciliation(ctx, actor, project, op)
		if e != nil {
			return out, e
		}
		if op.Assignment == nil {
			return out, repositoryConflict("unknown", repositoryUnknown)
		}
		packet := ri.RepositoryReconcileRequest{OperationID: op.Execution.OperationID, RequestSHA256: op.RequestSHA256, ReconciliationTicket: ticket, Request: &ri.RepositoryNonSecretRequest{RepositoryExecution: op.Execution, Assignment: *op.Assignment}}
		if err = repositoryProviderHTTP(ctx, cfg, "POST", "/operations/"+url.PathEscape(op.Execution.OperationID)+"/reconcile", packet, &observed); err != nil {
			return out, err
		}
	}
	if ri.RepositoryValidateObservation(observed, op.Execution, op.RequestSHA256) != nil || !repositoryKnownObservation(observed) {
		return out, repositoryConflict("unknown", repositoryUnknown)
	}
	if op.Assignment == nil {
		return out, repositoryConflict("unknown", repositoryUnknown)
	}
	a := *op.Assignment
	result := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "repository-observed-" + digest(op.Execution.OperationID+":"+strconv.Itoa(observed.Version)), IntegrationEvidence: &ri.RepositoryResult{OperationID: op.Execution.OperationID, RequestSHA256: op.RequestSHA256, ObservationSHA256: observed.ObservationSHA256, Version: observed.Version}}
	if observed.State != "prepared" && observed.State != "applied" {
		result.ExitCode = 1
		result.Error = repositoryUnknown
	}
	var proof *authorizedCommand
	var receipt string
	var duplicate bool
	err = engine.CompleteManagedObservedBound(ctx, cfg.TenantID, project, result, func(tx *sql.Tx) error {
		var e error
		proof, e = s.authorizeTx(ctx, tx, actor, commandTarget{tenantID: cfg.TenantID, projectID: project, resourceID: id}, purposeProjectApprove)
		if e != nil {
			return e
		}
		if e = dag.GuardExecutionScopeTx(ctx, tx, s.Dialect, cfg.TenantID, project, op.RunID); e != nil {
			return e
		}
		receipt, duplicate, e = s.commandReceipt(ctx, proof, req.IdempotencyKey, body)
		if e != nil {
			return e
		}
		if duplicate {
			return json.Unmarshal([]byte(receipt), &out)
		}
		current, e := s.repositoryPlanTx(ctx, tx, cfg.TenantID, project, id)
		if e != nil {
			return e
		}
		if current.OperationID != op.Execution.OperationID || current.Basis != plan.Basis || current.PreparedSHA256 != plan.PreparedSHA256 {
			return repositoryConflict("changed", repositoryChanged)
		}
		_, e = s.storeRepositoryObservationTx(ctx, tx, cfg, observed)
		return e
	}, func(tx *sql.Tx, state string) error {
		if duplicate {
			return nil
		}
		if e := s.validateRepositoryIntegrationResultTx(ctx, tx, cfg.TenantID, project, result); e != nil {
			return e
		}
		if e := s.recordRepositoryIntegrationOutcomeTx(ctx, tx, cfg.TenantID, project, result); e != nil {
			return e
		}
		var e error
		out, e = s.repositoryPlanTx(ctx, tx, cfg.TenantID, project, id)
		if e != nil {
			return e
		}
		return s.saveCommandReceipt(ctx, proof, req.IdempotencyKey, receipt, out)
	})
	if err != nil {
		return out, err
	}
	return s.GetRepositoryIntegrationPlan(ctx, engine, actor, project, id)
}
