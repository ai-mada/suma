package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	ri "github.com/ai-mada/suma/internal/repositoryintegrationcontract"
)

func repositoryWorkflow(provider, phase string) dag.Workflow {
	return dag.Workflow{Name: "Repository integration " + phase, Nodes: []dag.Node{{ID: phase, Name: "Governed repository " + phase, Type: "external_workflow", TimeoutSeconds: 180, MaxRetries: 0, Idempotent: false, Config: map[string]any{"plugin": provider, "workflow": ri.RepositoryProtocol, "purpose": "repository_integration", "input": map[string]any{}}}}, Edges: []dag.Edge{}}
}
func repositoryExecution(plan RepositoryIntegrationPlan, tenant, phase, operation string) ri.RepositoryExecution {
	o := plan.Target.Observation
	out := ri.RepositoryExecution{Protocol: ri.RepositoryProtocol, TenantID: tenant, ProjectID: plan.ProjectID, ProviderID: plan.Target.ProviderID, TargetID: plan.TargetID, TargetAlias: plan.Target.TargetAlias, TargetRef: plan.Target.Ref, TargetConfigSHA256: plan.Target.ConfigSHA256, PlanID: plan.ID, PlanVersion: plan.Version, Phase: phase, OperationID: operation, Source: plan.Basis.Source, ExpectedTargetHead: o.HeadCommit, ExpectedFenceOID: o.FenceOID, ExpectedGitGeneration: o.Generation, Generation: o.Generation + 1}
	if phase == "apply" {
		out.PreparedSHA256 = plan.PreparedSHA256
	}
	return out
}
func (s *Service) checkRepositoryPlanBasisTx(ctx context.Context, auth *authorizedCommand, plan RepositoryIntegrationPlan, barrier string) error {
	if err := s.checkRepositoryBasisTx(ctx, auth, plan.Basis); err != nil {
		return err
	}
	target, err := s.repositoryTargetTx(ctx, auth.tx, auth.target.tenantID, plan.ProjectID, plan.TargetID)
	if err != nil {
		return err
	}
	if target.Status != "active" || target.ProviderID != plan.Target.ProviderID || target.TargetAlias != plan.Target.TargetAlias || target.ConfigSHA256 != plan.Target.ConfigSHA256 || target.Ref != plan.Target.Ref {
		return repositoryConflict("target", repositoryTarget)
	}
	if target.ActiveOperationID != barrier {
		return repositoryConflict("busy", repositoryBusy)
	}
	if barrier == "" && target.Version != plan.Target.Version {
		return repositoryConflict("target", repositoryTarget)
	}
	if _, err = s.repositoryProvider(auth.target.tenantID, plan.ProjectID, target.ProviderID, target.TargetAlias); err != nil {
		return err
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return err
	}
	if automationStamp(clock) >= plan.ExpiresAt {
		return repositoryConflict("expired", repositoryExpired)
	}
	return nil
}
func (s *Service) CreateRepositoryIntegrationPlan(ctx context.Context, engine *dag.Engine, actor Actor, project string, req RepositoryIntegrationPlanCreateRequest) (out RepositoryIntegrationPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(req.TaskID, false) || !handoffID(req.SourceNodeID, false) || !handoffID(req.TargetID, false) || req.ExpectedTaskVersion < 1 || req.ExpectedTargetVersion < 1 || !planningKey(req.IdempotencyKey) {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	first, auth, close, err := s.planningAuthority(ctx, actor, project, req.TaskID, purposeProjectWrite)
	if err != nil {
		return out, err
	}
	body := map[string]any{"repository_plan_create": req}
	_, replay, err := s.commandReceipt(first, auth, req.IdempotencyKey, body)
	if err != nil {
		close()
		return out, err
	}
	if replay {
		var raw string
		raw, _, err = s.commandReceipt(first, auth, req.IdempotencyKey, body)
		if err == nil {
			err = json.Unmarshal([]byte(raw), &out)
		}
		close()
		if err != nil {
			return out, err
		}
		return s.GetRepositoryIntegrationPlan(ctx, engine, actor, project, out.ID)
	}
	basis, err := s.repositoryBasisTx(first, auth, req.TaskID, req.SourceNodeID)
	if err != nil {
		close()
		return out, err
	}
	if basis.TaskVersion != req.ExpectedTaskVersion {
		close()
		return out, repositoryConflict("basis", repositoryBasis)
	}
	target, err := s.repositoryTargetTx(first, auth.tx, auth.target.tenantID, project, req.TargetID)
	if err != nil {
		close()
		return out, err
	}
	if target.Version != req.ExpectedTargetVersion || target.Status != "active" || target.ActiveOperationID != "" {
		close()
		return out, repositoryConflict("target", repositoryTarget)
	}
	cfg, err := s.repositoryProvider(auth.target.tenantID, project, target.ProviderID, target.TargetAlias)
	close()
	if err != nil {
		return out, err
	}
	observed, err := repositoryProbe(ctx, cfg, target.TargetAlias)
	if err != nil {
		return out, err
	}
	if observed.Ref != target.Ref || observed.ConfigSHA256 != target.ConfigSHA256 || observed.Generation != target.Generation {
		return out, repositoryConflict("target", repositoryTarget)
	}
	target.Observation = &observed
	target.CanManage = false
	target.Blockers = []string{}
	id := digest(actor.credential.tenantID + ":" + project + ":" + actor.Projection().ID + ":" + req.IdempotencyKey)[:32]
	runID, operation := "repository-prepare-"+id, "repo-prepare-"+id
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: req.TaskID}, purposeProjectWrite)
	defer authority.close()
	ctx = authority.ctx
	var hash string
	var duplicate bool
	_, err = engine.CreateManagedRunBound(ctx, actor.credential.tenantID, project, runID, "repository-create-"+digest(actor.Projection().ID+":"+req.IdempotencyKey), repositoryWorkflow(target.ProviderID, "prepare"), func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		if e := dag.GuardProjectTx(ctx, tx, s.Dialect, actor.credential.tenantID, project); e != nil {
			return e
		}
		hash, duplicate, err = s.commandReceipt(ctx, authority.proof, req.IdempotencyKey, body)
		if err != nil {
			return err
		}
		if duplicate {
			return json.Unmarshal([]byte(hash), &out)
		}
		return s.checkRepositoryBasisTx(ctx, authority.proof, basis)
	}, func(tx *sql.Tx, actual string) error {
		if duplicate {
			return repositoryConflict("changed", repositoryChanged)
		}
		current, e := s.repositoryTargetTx(ctx, tx, authority.proof.target.tenantID, project, target.ID)
		if e != nil {
			return e
		}
		if current.Version != target.Version || current.Status != "active" || current.ActiveOperationID != "" {
			return repositoryConflict("target", repositoryTarget)
		}
		clock, e := s.automationClock(ctx, tx)
		if e != nil {
			return e
		}
		out = RepositoryIntegrationPlan{ID: id, ProjectID: project, Version: 1, TaskID: req.TaskID, TargetID: target.ID, State: "preparing", Basis: basis, Target: target, PrepareRunID: actual, OperationID: operation, CreatedAt: automationStamp(clock), ExpiresAt: automationStamp(clock.Add(30 * time.Minute)), Blockers: []string{}}
		execution := repositoryExecution(out, authority.proof.target.tenantID, "prepare", operation)
		if e = ri.RepositoryValidateExecution(execution); e != nil {
			return repositoryError(400, "invalid", repositoryInvalid)
		}
		op := repositoryOperation{Execution: execution, RunID: actual, NodeID: "prepare", CreatedBy: authority.proof.currentPrincipal.ID, CreatorCredential: actor.credential.digest, Version: 1}
		if _, e = tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_plans(id,tenant_id,project_id,task_id,target_id,version,created_at,data)VALUES(?,?,?,?,?,?,?,?)`), id, authority.proof.target.tenantID, project, out.TaskID, target.ID, out.Version, out.CreatedAt, stringify(out)); e != nil {
			return e
		}
		if e = s.insertRepositoryOperationTx(ctx, tx, authority.proof.target.tenantID, project, op); e != nil {
			return e
		}
		if e = s.saveCommandReceipt(ctx, authority.proof, req.IdempotencyKey, hash, out); e != nil {
			return e
		}
		return s.commandActivity(ctx, authority.proof, "repository.preparation_created", id, target.Name)
	})
	if err != nil {
		return out, err
	}
	return s.GetRepositoryIntegrationPlan(ctx, engine, actor, project, out.ID)
}
func (s *Service) insertRepositoryOperationTx(ctx context.Context, tx *sql.Tx, tenant, project string, out repositoryOperation) error {
	var err error
	out.Route, err = dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, tenant, project, out.RunID)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_operations(operation_id,tenant_id,project_id,plan_id,run_id,node_id,version,data)VALUES(?,?,?,?,?,?,?,?)`), out.Execution.OperationID, tenant, project, out.Execution.PlanID, out.RunID, out.NodeID, out.Version, stringify(out))
	return err
}

func (s *Service) CommandRepositoryIntegrationPlan(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req RepositoryIntegrationPlanCommandRequest) (out RepositoryIntegrationPlan, err error) {
	if engine == nil || req.ProjectID != project || !handoffID(project, false) || !handoffID(id, false) || req.ExpectedVersion < 1 || !planningText(req.Reason, 2000) || !planningKey(req.IdempotencyKey) || req.Action != "approve" && req.Action != "reject" && req.Action != "execute" && req.Action != "reconcile" {
		return out, repositoryError(400, "invalid", repositoryInvalid)
	}
	if req.Action != "reject" && !req.Acknowledge {
		return out, repositoryError(400, "acknowledge", repositoryAcknowledge)
	}
	if req.Action == "execute" {
		return s.executeRepositoryIntegration(ctx, engine, actor, project, id, req)
	}
	if req.Action == "reconcile" {
		return s.reconcileRepositoryIntegration(ctx, engine, actor, project, id, req)
	}
	if req.Action == "approve" {
		snapshot, e := s.GetRepositoryIntegrationPlan(ctx, engine, actor, project, id)
		if e != nil {
			return out, e
		}
		if snapshot.State == "ready" {
			if e = s.repositoryCurrentTargetProbe(ctx, actor.credential.tenantID, snapshot); e != nil {
				return out, e
			}
		}
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, id, purposeBrowserProjectApprove)
	if err != nil {
		return out, err
	}
	defer close()
	hash, replay, err := s.commandReceipt(ctx, auth, req.IdempotencyKey, map[string]any{"repository_plan_command": id, "request": req})
	if err != nil {
		return out, err
	}
	if replay {
		err = json.Unmarshal([]byte(hash), &out)
		if err != nil {
			return out, err
		}
		return s.repositoryPlanProjectionTx(ctx, auth, out)
	}
	out, err = s.repositoryPlanTx(ctx, auth.tx, auth.target.tenantID, project, id)
	if err != nil {
		return out, err
	}
	if out.Version != req.ExpectedVersion || out.State != "ready" && out.State != "approved" {
		return out, repositoryConflict("changed", repositoryChanged)
	}
	if req.Action == "approve" {
		if out.State != "ready" || out.Preview == nil || out.PreparedSHA256 == "" {
			return out, repositoryConflict("approval", repositoryApproval)
		}
		if err = s.checkRepositoryPlanBasisTx(ctx, auth, out, ""); err != nil {
			return out, err
		}
	}
	clock, err := s.automationClock(ctx, auth.tx)
	if err != nil {
		return out, err
	}
	old := out.Version
	decision := "approved"
	if req.Action == "reject" {
		decision = "rejected"
	}
	approval := RepositoryIntegrationApproval{ActorID: auth.currentPrincipal.ID, Decision: decision, Reason: req.Reason, PlanVersion: out.Version, PreparedSHA256: out.PreparedSHA256, BasisSHA256: out.Basis.BasisSHA256, At: automationStamp(clock)}
	out.Version++
	out.Approval = &approval
	if req.Action == "approve" {
		out.State = "approved"
	} else {
		out.State = "rejected"
	}
	if _, err = auth.tx.ExecContext(ctx, s.q(`INSERT INTO suma_repository_approvals(id,tenant_id,project_id,plan_id,version,data)VALUES(?,?,?,?,?,?)`), newID(), auth.target.tenantID, project, id, out.Version, stringify(approval)); err != nil {
		return out, err
	}
	if err = s.saveRepositoryPlanTx(ctx, auth.tx, auth.target.tenantID, old, out); err != nil {
		return out, err
	}
	if err = s.saveCommandReceipt(ctx, auth, req.IdempotencyKey, hash, out); err != nil {
		return out, err
	}
	if err = s.commandActivity(ctx, auth, "repository."+req.Action, id, req.Reason); err != nil {
		return out, err
	}
	out, err = s.repositoryPlanProjectionTx(ctx, auth, out)
	if err != nil {
		return out, err
	}
	return out, auth.tx.Commit()
}

func (s *Service) executeRepositoryIntegration(ctx context.Context, engine *dag.Engine, actor Actor, project, id string, req RepositoryIntegrationPlanCommandRequest) (out RepositoryIntegrationPlan, err error) {
	plan, err := s.GetRepositoryIntegrationPlan(ctx, engine, actor, project, id)
	if err != nil {
		return out, err
	}
	if plan.State == "approved" {
		if err = s.repositoryCurrentTargetProbe(ctx, actor.credential.tenantID, plan); err != nil {
			return out, err
		}
	}
	run, operation := "repository-apply-"+id, "repo-apply-"+id
	authority := s.engineAuthority(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeProjectApprove)
	defer authority.close()
	ctx = authority.ctx
	var hash string
	var duplicate bool
	_, err = engine.CreateManagedRunBound(ctx, actor.credential.tenantID, project, run, "repository-apply-"+id, repositoryWorkflow(plan.Target.ProviderID, "apply"), func(tx *sql.Tx) error {
		if e := authority.before(tx); e != nil {
			return e
		}
		if e := dag.GuardProjectTx(ctx, tx, s.Dialect, actor.credential.tenantID, project); e != nil {
			return e
		}
		hash, duplicate, err = s.commandReceipt(ctx, authority.proof, req.IdempotencyKey, map[string]any{"repository_plan_command": id, "request": req})
		if err != nil {
			return err
		}
		if duplicate {
			return json.Unmarshal([]byte(hash), &out)
		}
		plan, err = s.repositoryPlanTx(ctx, tx, authority.proof.target.tenantID, project, id)
		if err != nil {
			return err
		}
		if plan.Version != req.ExpectedVersion || plan.State != "approved" || plan.Approval == nil || plan.Approval.Decision != "approved" || plan.Approval.PreparedSHA256 != plan.PreparedSHA256 || plan.Approval.BasisSHA256 != plan.Basis.BasisSHA256 {
			return repositoryConflict("approval", repositoryApproval)
		}
		if _, e := s.currentProjectMembershipLockedTx(ctx, tx, authority.proof.target.tenantID, plan.Approval.ActorID, project, false, true); e != nil {
			return e
		}
		return s.checkRepositoryPlanBasisTx(ctx, authority.proof, plan, "")
	}, func(tx *sql.Tx, actual string) error {
		if duplicate {
			return repositoryConflict("changed", repositoryChanged)
		}
		execution := repositoryExecution(plan, authority.proof.target.tenantID, "apply", operation)
		target, e := s.repositoryTargetTx(ctx, tx, authority.proof.target.tenantID, project, plan.TargetID)
		if e != nil {
			return e
		}
		if target.ActiveOperationID != "" || target.Generation != execution.ExpectedGitGeneration {
			return repositoryConflict("busy", repositoryBusy)
		}
		oldTarget := target.Version
		target.Version++
		target.Generation = execution.Generation
		target.ActiveOperationID = operation
		if e = s.saveRepositoryTargetTx(ctx, tx, authority.proof.target.tenantID, oldTarget, target); e != nil {
			return e
		}
		op := repositoryOperation{Execution: execution, RunID: actual, NodeID: "apply", CreatedBy: authority.proof.currentPrincipal.ID, ApprovedBy: plan.Approval.ActorID, CreatorCredential: actor.credential.digest, Version: 1}
		if e = s.insertRepositoryOperationTx(ctx, tx, authority.proof.target.tenantID, project, op); e != nil {
			return e
		}
		old := plan.Version
		plan.Version++
		plan.State = "applying"
		plan.ApplyRunID = actual
		plan.OperationID = operation
		if e = s.saveRepositoryPlanTx(ctx, tx, authority.proof.target.tenantID, old, plan); e != nil {
			return e
		}
		out = plan
		if e = s.saveCommandReceipt(ctx, authority.proof, req.IdempotencyKey, hash, out); e != nil {
			return e
		}
		return s.commandActivity(ctx, authority.proof, "repository.apply_created", id, req.Reason)
	})
	if err != nil {
		return out, err
	}
	return s.GetRepositoryIntegrationPlan(ctx, engine, actor, project, id)
}
