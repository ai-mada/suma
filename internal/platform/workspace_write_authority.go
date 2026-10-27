package platform

import (
	"context"
	"database/sql"
	"errors"

	wcc "github.com/ai-mada/suma/internal/workspacewritecontract"
)

func (s *Service) workspaceWriteTaskBasisTx(ctx context.Context, tx *sql.Tx, tenant, project, run string) error {
	var task string
	err := tx.QueryRowContext(ctx, s.q(`SELECT task_id FROM suma_task_run_provenance WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&task)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_tasks WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, run).Scan(&task)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	r, err := scanRecord(tx.QueryRowContext(ctx, s.q(s.executionReadSQL(`SELECT `+recordColumns+` FROM suma_tasks WHERE tenant_id=? AND project_id=? AND id=?`, true)), tenant, project, task))
	if err != nil {
		return err
	}
	if r.RunID != run || r.Status == "accepted" || r.Data["run_task_basis"] != taskBasis(r) {
		return conflict(workspaceWriteTaskChanged)
	}
	return nil
}

// canWorkspaceWriteTargetTx derives only human permission/task eligibility.
// Native availability remains independently checked by the observation path.
func (s *Service) canWorkspaceWriteTargetTx(ctx context.Context, tx *sql.Tx, actor Actor, project string, b workspaceObservationBasis) (bool, error) {
	_, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectWrite, []string{b.PoolOwner})
	if err == nil {
		err = s.workspaceWriteTaskBasisTx(ctx, tx, actor.credential.tenantID, project, b.Scope.RunID)
	}
	if err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 401 || api.Status == 403 || api.Status == 404 || api.Status == 409) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Service) workspaceWriteCallerTx(ctx context.Context, tx *sql.Tx, r workspaceWriteRecord) (Actor, error) {
	return s.workspaceObservationCallerActorTx(ctx, tx, workspaceObservationRecord{Caller: r.Caller})
}
func (s *Service) workspaceWriteReadAuthorityTx(ctx context.Context, tx *sql.Tx, actor Actor, project string, r workspaceWriteRecord) error {
	if actor.kind != actorBrowser && actor.kind != actorProjectAPI {
		return forbidden()
	}
	if _, err := s.authorizeTxWithParticipants(ctx, tx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, []string{r.PoolOwner}); err != nil {
		return err
	}
	if actor.credential.digest != r.Caller.Digest || actor.credential.subjectID != r.Caller.SubjectID {
		return forbidden()
	}
	return nil
}

// New execution is deliberately separate from accepted-effect fact draining.
func (s *Service) workspaceWriteExecuteAuthorityTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, b workspaceObservationBasis, r workspaceWriteRecord) error {
	caller, err := s.workspaceWriteCallerTx(ctx, tx, r)
	if err != nil {
		return err
	}
	actors := []Actor{caller}
	if !subject.Pool {
		actors = append(actors, subject.Actor)
	}
	users, err := s.workspaceObservationBasisUsersTx(ctx, tx, subject.Tenant, subject.Project, b)
	if err != nil {
		return err
	}
	if err = s.workspaceObservationLockActorsTx(ctx, tx, actors, users, []string{subject.Tenant, b.PoolOwner}); err != nil {
		return err
	}
	if _, err = s.authorizeTxWithParticipants(ctx, tx, caller, commandTarget{tenantID: subject.Tenant, projectID: subject.Project}, purposeProjectWrite, []string{b.PoolOwner}); err != nil {
		return err
	}
	if err = s.workspaceObservationMachineAuthorityTx(ctx, tx, subject, b, nil); err != nil {
		return err
	}
	return s.workspaceWriteTaskBasisTx(ctx, tx, subject.Tenant, subject.Project, b.Scope.RunID)
}

// This authorizes a fixed old operation's receipt only. No current DAG state,
// caller membership, profile activation or Pool grant can turn it into a write.
func (s *Service) workspaceWriteFactAuthorityTx(ctx context.Context, tx *sql.Tx, subject workspaceObservationSubject, r workspaceWriteRecord, requireCaller bool) error {
	if r.Command.ExecutionFence != 1 || !workspaceObservationSameScope(r.Assignment, r.Command.Scope) {
		return forbidden()
	}
	actors := []Actor{}
	var caller Actor
	var err error
	if requireCaller {
		caller, err = s.workspaceWriteCallerTx(ctx, tx, r)
		if err != nil {
			return err
		}
		actors = append(actors, caller)
	}
	if !subject.Pool {
		actors = append(actors, subject.Actor)
	}
	if err = s.workspaceObservationLockActorsTx(ctx, tx, actors, nil, []string{subject.Tenant, r.PoolOwner}); err != nil {
		return err
	}
	if requireCaller {
		if _, err = s.authorizeTxWithParticipants(ctx, tx, caller, commandTarget{tenantID: subject.Tenant, projectID: subject.Project}, purposeProjectRead, []string{r.PoolOwner}); err != nil {
			return err
		}
	}
	if subject.Pool {
		if subject.Assignment.DispatchID != r.Assignment.DispatchID || subject.Assignment.PoolID != r.Assignment.PoolID || subject.Assignment.PoolGrantID != r.Assignment.PoolGrantID || subject.Assignment.PoolGrantVersion != r.Assignment.PoolGrantVersion || subject.Assignment.PoolRunnerID != subject.Runner || !workspaceObservationSameScope(subject.Assignment, r.Command.Scope) {
			return forbidden()
		}
		proof, e := s.PoolRunnerAssignmentAuthorityTx(ctx, tx, subject.Token, subject.Assignment)
		if e != nil {
			return e
		}
		return proof.assertAssignment(tx, subject.Assignment)
	}
	if r.Assignment.PoolID != "" {
		return forbidden()
	}
	auth, err := s.runnerIdentityTx(ctx, tx, subject.Actor, subject.Project, r.Command.Scope.WorkerID)
	if err != nil {
		return err
	}
	return s.projectRunnerExecutionAuthorityTx(ctx, tx, auth, r.Command.Scope.WorkerID, r.Command.Scope.RunID)
}

func workspaceWriteScopeMatches(r workspaceWriteRecord, req wcc.ClaimRequest) bool {
	return r.Command.ProjectID == req.ProjectID && r.Command.Scope == req.Scope && r.Command.WorkspacePolicySHA256 == req.WorkspacePolicySHA256
}
