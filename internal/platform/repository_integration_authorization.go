package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func repositoryAssignmentEqual(a, b cc.CandidateAssignmentRef) bool {
	return stringify(ri.RepositoryStableAssignment(a)) == stringify(ri.RepositoryStableAssignment(b))
}

// Called only by the authenticated Core claim decorator with its real returned
// assignment. This preserves recovery identity before any provider ticket exists.
func (s *Service) bindRepositoryAssignmentTx(ctx context.Context, tx *sql.Tx, tenant, project string, a dag.Assignment) error {
	execution, err := s.readRepositoryExecutionTx(ctx, tx, tenant, project, a.RunID, a.NodeID)
	if err != nil || execution == nil {
		return err
	}
	op, err := s.repositoryOperationTx(ctx, tx, tenant, project, execution.OperationID)
	if err != nil {
		return err
	}
	ref := cc.CandidateAssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, TenantID: a.TenantID, ProjectID: a.ProjectID, PoolID: a.PoolID, PoolRunnerID: a.PoolRunnerID, PoolSlotID: a.PoolSlotID, PoolGrantID: a.PoolGrantID, PoolGrantVersion: a.PoolGrantVersion, PoolCredentialID: a.PoolCredentialID, PoolCredentialVersion: a.PoolCredentialVersion, DispatchID: a.DispatchID, DispatchState: a.DispatchState, LeaseUntil: a.LeaseUntil}
	if a.PoolID == "" {
		// Ordinary scheduler projections also carry tenant/project metadata;
		// candidate callback identities reserve those fields for full Pool scope.
		ref = cc.CandidateAssignmentRef{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID}
	}
	if !ref.Valid() {
		return repositoryError(400, "invalid", repositoryInvalid)
	}
	if op.Assignment != nil {
		if !repositoryAssignmentEqual(*op.Assignment, ref) {
			return repositoryConflict("unknown", repositoryUnknown)
		}
		return nil
	}
	requestSHA, err := ri.RepositoryRequestDigest(ri.RepositoryRequest{RepositoryExecution: op.Execution, Assignment: ref})
	if err != nil {
		return err
	}
	old := op.Version
	op.Version++
	op.Assignment = &ref
	op.RequestSHA256 = requestSHA
	return s.saveRepositoryOperationTx(ctx, tx, tenant, project, old, op)
}

func (s *Service) repositoryCreatorTx(ctx context.Context, tx *sql.Tx, tenant, project string, op repositoryOperation) (Principal, error) {
	c, err := s.readCredential(ctx, tx, op.CreatorCredential, true)
	if err != nil {
		return Principal{}, err
	}
	clock, err := s.authorityNow(ctx, tx)
	if err != nil {
		return Principal{}, err
	}
	if c.kind != actorBrowser && c.kind != actorProjectAPI || c.ref.subjectID != op.CreatedBy || c.ref.tenantID != tenant || c.kind == actorProjectAPI && c.ref.projectID != project || !c.expires.After(clock) {
		return Principal{}, forbidden()
	}
	return s.currentProjectMembershipLockedTx(ctx, tx, tenant, op.CreatedBy, project, true, op.Execution.Phase == "apply")
}
func (s *Service) repositoryFreshOperationTx(ctx context.Context, tx *sql.Tx, tenant, project string, op repositoryOperation) error {
	p, err := s.repositoryCreatorTx(ctx, tx, tenant, project, op)
	if err != nil {
		return err
	}
	plan, err := s.repositoryPlanTx(ctx, tx, tenant, project, op.Execution.PlanID)
	if err != nil {
		return err
	}
	if plan.OperationID != op.Execution.OperationID || op.Execution.Phase == "prepare" && plan.State != "preparing" && plan.State != "reconciling" || op.Execution.Phase == "apply" && plan.State != "applying" && plan.State != "reconciling" {
		return repositoryConflict("changed", repositoryChanged)
	}
	basis, err := s.repositoryBasisReadTx(ctx, tx, p, project, plan.TaskID, plan.Basis.SourceNodeID)
	if err != nil {
		return err
	}
	if basis != plan.Basis {
		return repositoryConflict("basis", repositoryBasis)
	}
	target, err := s.repositoryTargetTx(ctx, tx, tenant, project, plan.TargetID)
	if err != nil {
		return err
	}
	if target.Status != "active" || target.ProviderID != op.Execution.ProviderID || target.TargetAlias != op.Execution.TargetAlias || target.Ref != op.Execution.TargetRef || target.ConfigSHA256 != op.Execution.TargetConfigSHA256 {
		return repositoryConflict("target", repositoryTarget)
	}
	if op.Execution.Phase == "apply" {
		if target.ActiveOperationID != op.Execution.OperationID || target.Generation != op.Execution.Generation || plan.Approval == nil || plan.Approval.ActorID != op.ApprovedBy || plan.Approval.Decision != "approved" || plan.Approval.PreparedSHA256 != op.Execution.PreparedSHA256 {
			return repositoryConflict("approval", repositoryApproval)
		}
		if _, err = s.currentProjectMembershipLockedTx(ctx, tx, tenant, op.ApprovedBy, project, false, true); err != nil {
			return err
		}
	} else if target.ActiveOperationID != "" || target.Generation != op.Execution.ExpectedGitGeneration {
		return repositoryConflict("target", repositoryTarget)
	}
	if _, err = s.repositoryProvider(tenant, project, target.ProviderID, target.TargetAlias); err != nil {
		return err
	}
	current, err := dag.ReadProjectExecutionRouteTx(ctx, tx, s.Dialect, tenant, project)
	if err != nil {
		return err
	}
	bound, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, op.RunID)
	if err != nil {
		return err
	}
	if current != op.Route || bound != op.Route {
		return repositoryConflict("target", repositoryTarget)
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= plan.ExpiresAt {
		return repositoryConflict("expired", repositoryExpired)
	}
	return nil
}
func (s *Service) readRepositoryExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run, node string) (*ri.RepositoryExecution, error) {
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_repository_operations WHERE tenant_id=? AND project_id=? AND run_id=? AND node_id=?`), tenant, project, run, node).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	op, err := s.repositoryOperationTx(ctx, tx, tenant, project, id)
	if err != nil {
		return nil, err
	}
	return &op.Execution, nil
}
func (s *Service) RepositoryIntegrationRunTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_operations WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&n)
	return n == 1, err
}
func (s *Service) checkRepositoryIntegrationExecutionTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) error {
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_repository_operations WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	op, err := s.repositoryOperationTx(ctx, tx, tenant, project, id)
	if err != nil {
		return err
	}
	return s.repositoryFreshOperationTx(ctx, tx, tenant, project, op)
}

func (s *Service) AuthorizeRepositoryIntegration(ctx context.Context, actor Actor, req ri.RepositoryAuthorizationRequest) (ri.RepositoryAuthorization, error) {
	return s.issueRepositoryAuthorization(ctx, projectCandidateSubject(actor, req.ProjectID), req)
}
func (s *Service) AuthorizePoolRepositoryIntegration(ctx context.Context, token string, req ri.RepositoryAuthorizationRequest) (ri.RepositoryAuthorization, error) {
	return s.issueRepositoryAuthorization(ctx, candidateSubject{pool: true, token: token, tenant: req.Assignment.TenantID, project: req.ProjectID}, req)
}
func (s *Service) issueRepositoryAuthorization(ctx context.Context, subject candidateSubject, req ri.RepositoryAuthorizationRequest) (out ri.RepositoryAuthorization, err error) {
	if !req.Assignment.Valid() || !ri.RepositoryValidOperationID(req.OperationID) || req.ProjectID != subject.project {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	proof, err := s.candidateAuthorityTx(ctx, tx, subject, req.Assignment)
	if err != nil {
		return out, err
	}
	if proof.spec.Type != "external_workflow" || proof.spec.Config["workflow"] != ri.RepositoryProtocol {
		return out, forbidden()
	}
	op, err := s.repositoryOperationTx(ctx, tx, subject.tenant, subject.project, req.OperationID)
	if err != nil {
		return out, err
	}
	if op.RunID != req.Assignment.RunID || op.NodeID != req.Assignment.NodeID || proof.spec.Config["plugin"] != op.Execution.ProviderID {
		return out, forbidden()
	}
	if err = s.repositoryFreshOperationTx(ctx, tx, subject.tenant, subject.project, op); err != nil {
		return out, err
	}
	if op.Assignment != nil && !repositoryAssignmentEqual(*op.Assignment, req.Assignment) {
		return out, repositoryConflict("unknown", repositoryUnknown)
	}
	var consumed int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_repository_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='consumed'`), subject.tenant, subject.project, req.OperationID).Scan(&consumed); err != nil {
		return out, err
	}
	if consumed > 0 {
		return out, repositoryConflict("unknown", repositoryUnknown)
	}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_repository_authorizations SET state='revoked' WHERE tenant_id=? AND project_id=? AND operation_id=? AND state='issued'`), subject.tenant, subject.project, req.OperationID); err != nil {
		return out, err
	}
	requestSHA, err := ri.RepositoryRequestDigest(ri.RepositoryRequest{RepositoryExecution: op.Execution, Assignment: req.Assignment})
	if err != nil {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	ticket := newID() + newID()
	out = ri.RepositoryAuthorization{AuthorizationID: newID(), AuthorizationTicket: ticket, ExpiresAt: clock.Add(120 * time.Second).Format(time.RFC3339Nano), OperationID: req.OperationID, RequestSHA256: requestSHA}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_authorizations(id,tenant_id,project_id,operation_id,ticket_hash,expires_at,state,assignment,request_sha256,grant_data,effect_grant_id,worker_token_hash)VALUES(?,?,?,?,?,?,'issued',?,?,'','',?)`), out.AuthorizationID, subject.tenant, subject.project, req.OperationID, digest(ticket), out.ExpiresAt, stringify(req.Assignment), requestSHA, subject.hash()); err != nil {
		return out, err
	}
	old := op.Version
	op.Version++
	op.Assignment = &req.Assignment
	op.WorkerTokenHash = subject.hash()
	op.RequestSHA256 = requestSHA
	if err = s.saveRepositoryOperationTx(ctx, tx, subject.tenant, subject.project, old, op); err != nil {
		return ri.RepositoryAuthorization{}, err
	}
	return out, tx.Commit()
}

type repositoryAuthorizationRow struct {
	ID, OperationID, ExpiresAt, State, RequestSHA256, GrantData, EffectGrantID, WorkerHash string
	Assignment                                                                             cc.CandidateAssignmentRef
}

func (s *Service) repositoryAuthorizationTx(ctx context.Context, tx *sql.Tx, cfg RepositoryProviderConfig, where, arg string) (a repositoryAuthorizationRow, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT id,operation_id,expires_at,state,assignment,request_sha256,grant_data,effect_grant_id,worker_token_hash FROM suma_repository_authorizations WHERE tenant_id=? AND project_id=? AND `+where), cfg.TenantID, cfg.ProjectID, arg).Scan(&a.ID, &a.OperationID, &a.ExpiresAt, &a.State, &raw, &a.RequestSHA256, &a.GrantData, &a.EffectGrantID, &a.WorkerHash)
	if errors.Is(err, sql.ErrNoRows) {
		return a, forbidden()
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &a.Assignment)
	}
	return
}

func (s *Service) repositoryAuthorizationCurrentTx(ctx context.Context, tx *sql.Tx, cfg RepositoryProviderConfig, a repositoryAuthorizationRow) (op repositoryOperation, err error) {
	if candidateHasPoolFields(a.Assignment) {
		proof, e := s.poolRunnerAssignmentAuthorityByHashTx(ctx, tx, a.WorkerHash, candidateDAGAssignment(a.Assignment))
		if e != nil {
			return op, e
		}
		identity, e := s.poolMachineIdentityByHashTx(ctx, tx, a.WorkerHash, a.Assignment.PoolRunnerID, true)
		if e != nil {
			return op, e
		}
		scope, e := s.currentPoolActiveScopeTx(ctx, tx, a.Assignment.PoolID, cfg.TenantID, cfg.ProjectID)
		if e != nil {
			return op, e
		}
		if identity.poolStatus != "active" || scope.GrantID != a.Assignment.PoolGrantID || scope.GrantVersion != a.Assignment.PoolGrantVersion {
			return op, forbidden()
		}
		if e = proof.assertAssignment(tx, candidateDAGAssignment(a.Assignment)); e != nil {
			return op, e
		}
	} else {
		actor, e := s.runnerActorByHashTx(ctx, tx, a.WorkerHash)
		if e != nil {
			return op, e
		}
		if _, e = s.candidateAuthorityTx(ctx, tx, projectCandidateSubject(actor, cfg.ProjectID), a.Assignment); e != nil {
			return op, e
		}
	}
	if err = dag.GuardProjectTx(ctx, tx, s.Dialect, cfg.TenantID, cfg.ProjectID); err != nil {
		return op, err
	}
	op, err = s.repositoryOperationTx(ctx, tx, cfg.TenantID, cfg.ProjectID, a.OperationID)
	if err != nil {
		return op, err
	}
	if op.Execution.ProviderID != cfg.ID || op.Assignment == nil || !repositoryAssignmentEqual(*op.Assignment, a.Assignment) || op.WorkerTokenHash != a.WorkerHash || op.RequestSHA256 != a.RequestSHA256 {
		return op, forbidden()
	}
	if err = s.repositoryFreshOperationTx(ctx, tx, cfg.TenantID, cfg.ProjectID, op); err != nil {
		return op, err
	}
	return op, nil
}

func (s *Service) ConsumeRepositoryAuthorization(ctx context.Context, token string, req ri.RepositoryConsumeAuthorization) (out ri.RepositoryAuthorizationGrant, err error) {
	cfg, err := s.authenticateRepositoryProvider(token)
	if err != nil {
		return out, err
	}
	if req.ProviderID != cfg.ID || req.AuthorizationTicket == "" || !req.Assignment.Valid() {
		return out, forbidden()
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	a, err := s.repositoryAuthorizationTx(ctx, tx, cfg, "ticket_hash=?", digest(req.AuthorizationTicket))
	if err != nil {
		return out, err
	}
	if a.OperationID != req.OperationID || a.RequestSHA256 != req.RequestSHA256 || !repositoryAssignmentEqual(a.Assignment, req.Assignment) || a.State != "issued" && a.State != "consumed" {
		return out, forbidden()
	}
	op, err := s.repositoryAuthorizationCurrentTx(ctx, tx, cfg, a)
	if err != nil {
		return out, err
	}
	if a.State == "consumed" {
		err = json.Unmarshal([]byte(a.GrantData), &out)
		return out, err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return out, err
	}
	expiry, e := time.Parse(time.RFC3339Nano, a.ExpiresAt)
	if e != nil || !clock.Before(expiry) {
		return out, repositoryConflict("expired", repositoryExpired)
	}
	out = ri.RepositoryAuthorizationGrant{AuthorizationID: a.ID, OperationID: a.OperationID, ProviderID: cfg.ID, Assignment: a.Assignment, Execution: op.Execution, RequestSHA256: a.RequestSHA256, State: "consumed"}
	if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_repository_authorizations SET state='consumed',grant_data=? WHERE id=? AND state='issued'`), stringify(out), a.ID); err != nil {
		return out, err
	}
	return out, tx.Commit()
}
func (s *Service) InspectRepositoryAuthorization(ctx context.Context, token string, req ri.RepositoryInspectAuthorization) (out ri.RepositoryAuthorizationGrant, err error) {
	cfg, err := s.authenticateRepositoryProvider(token)
	if err != nil {
		return out, err
	}
	if !req.Assignment.Valid() || !contentDigest.MatchString(req.RequestSHA256) {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, s.q(`SELECT grant_data FROM suma_repository_authorizations WHERE tenant_id=? AND project_id=? AND operation_id=? AND request_sha256=? AND state='consumed'`), cfg.TenantID, cfg.ProjectID, req.OperationID, req.RequestSHA256)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return out, err
		}
		if json.Unmarshal([]byte(raw), &out) != nil {
			return out, repositoryConflict("observation", repositoryObservation)
		}
		if out.ProviderID == cfg.ID && repositoryAssignmentEqual(out.Assignment, req.Assignment) {
			return out, nil
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	return out, missing()
}
func (s *Service) BeginRepositoryEffect(ctx context.Context, token string, req ri.RepositoryBeginEffect) (out ri.RepositoryEffectGrant, err error) {
	cfg, err := s.authenticateRepositoryProvider(token)
	if err != nil {
		return out, err
	}
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return out, err
	}
	defer cancel()
	defer tx.Rollback()
	a, err := s.repositoryAuthorizationTx(ctx, tx, cfg, "id=?", req.AuthorizationID)
	if err != nil {
		return out, err
	}
	if a.State != "consumed" || a.OperationID != req.OperationID || a.RequestSHA256 != req.RequestSHA256 || !repositoryAssignmentEqual(a.Assignment, req.Assignment) {
		return out, forbidden()
	}
	op, err := s.repositoryAuthorizationCurrentTx(ctx, tx, cfg, a)
	if err != nil {
		return out, err
	}
	if op.Execution.Phase != "apply" || op.Execution.Generation != req.Generation {
		return out, forbidden()
	}
	grant := a.EffectGrantID
	if grant == "" {
		grant = newID()
		if _, err = tx.ExecContext(ctx, s.q(`UPDATE suma_repository_authorizations SET effect_grant_id=? WHERE id=? AND effect_grant_id=''`), grant, a.ID); err != nil {
			return out, err
		}
	}
	out = ri.RepositoryEffectGrant{EffectGrantID: grant, OperationID: req.OperationID, Generation: req.Generation, State: "granted"}
	return out, tx.Commit()
}

func (s *Service) ProviderRepositoryBundle(ctx context.Context, token string, req ri.RepositoryBundleRequest) ([]byte, error) {
	cfg, err := s.authenticateRepositoryProvider(token)
	if err != nil {
		return nil, err
	}
	resolve := func() (candidateTransfer, error) {
		ctx, tx, cancel, e := s.beginAuthorityTx(ctx)
		if e != nil {
			return candidateTransfer{}, e
		}
		defer cancel()
		defer tx.Rollback()
		a, e := s.repositoryAuthorizationTx(ctx, tx, cfg, "id=?", req.AuthorizationID)
		if e != nil {
			return candidateTransfer{}, e
		}
		if a.State != "consumed" || a.OperationID != req.OperationID || a.RequestSHA256 != req.RequestSHA256 || !repositoryAssignmentEqual(a.Assignment, req.Assignment) {
			return candidateTransfer{}, forbidden()
		}
		op, e := s.repositoryAuthorizationCurrentTx(ctx, tx, cfg, a)
		if e != nil {
			return candidateTransfer{}, e
		}
		transfer, e := s.candidateTransferTx(ctx, tx, candidateSubject{tenant: cfg.TenantID, project: cfg.ProjectID}, op.Execution.Source.TransferID)
		if e != nil {
			return transfer, e
		}
		if transfer.binding != op.Execution.Source {
			return transfer, repositoryConflict("basis", repositoryBasis)
		}
		return transfer, nil
	}
	before, err := resolve()
	if err != nil {
		return nil, err
	}
	raw, err := s.readCandidateBundle(before)
	if err != nil {
		return nil, err
	}
	after, err := resolve()
	if err != nil {
		return nil, err
	}
	if before != after {
		return nil, repositoryConflict("basis", repositoryBasis)
	}
	return raw, nil
}
