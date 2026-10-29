package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"

	"github.com/ai-mada/suma/internal/dag"
	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func originalReleaseExecutionWorkflow(plan ReleasePlan) dag.Workflow {
	return dag.Workflow{Name: "受控发布 " + plan.ID, Nodes: []dag.Node{{ID: plan.Content.ReservedExecution.NodeID, Name: "发布与健康检查", Type: "external_workflow", Idempotent: false, TimeoutSeconds: 300, MaxRetries: 0, Config: map[string]any{"plugin": plan.Content.ProviderID, "workflow": rc.ReleaseProtocol, "input": map[string]any{}}}}}
}

// Both the original Execute endpoint and a protected bound child call this
// same transaction body. It never approves a plan or invokes a provider.
func (s *Service) bindOriginalReleasePlanExecutionTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, project, id, run string) (out ReleaseDeployment, err error) {
	if err = assertProjectCommand(tx, auth, project); err != nil {
		return out, err
	}
	p := auth.currentPrincipal
	if err = s.releaseActorAdminTx(ctx, tx, p.TenantID, project, p.ID); err != nil {
		return out, err
	}
	if err = s.guardReleaseExecutionRouteTx(ctx, tx, p.TenantID, project, run); err != nil {
		return out, err
	}
	current, err := s.currentReleasePlanTx(ctx, tx, p.TenantID, project, id)
	if err != nil {
		return out, err
	}
	if current.Content.ReservedExecution.RunID != run || current.ExecutionOperationID != "" {
		return out, conflict("计划已派发，请读取已有部署")
	}
	if current.State != "approved" || current.Approval == nil || current.Approval.Decision != "approved" || current.Approval.PlanSHA256 != current.SHA256 {
		return out, releaseConflict("release_approval_required", "此固定计划尚未批准")
	}
	if err = s.releaseActorAdminTx(ctx, tx, p.TenantID, project, current.Approval.ActorID); err != nil {
		return out, err
	}
	if err = s.currentReleasePlanBasisTx(ctx, tx, p, project, current, ""); err != nil {
		return out, err
	}
	env, err := s.releaseEnvironmentTx(ctx, tx, p.TenantID, project, current.Content.EnvironmentID, true)
	if err != nil {
		return out, err
	}
	op := "deploy-" + newID()
	oldEnv := env.Version
	env.Version++
	env.Generation = current.Content.Generation
	env.BarrierOperationID = op
	env.UpdatedAt = now()
	if err = s.saveReleaseJSON(ctx, tx, "environments", p.TenantID, project, env.ID, oldEnv, env); err != nil {
		return out, err
	}
	out = ReleaseDeployment{OperationID: op, ProjectID: project, Version: 1, PlanID: id, PlanRevision: current.Revision, PlanSHA256: current.SHA256, EnvironmentID: env.ID, ProviderID: env.ProviderID, Generation: env.Generation, ArtifactID: current.Content.ArtifactID, ArtifactSHA256: current.Content.ArtifactSHA256, RunID: run, NodeID: current.Content.ReservedExecution.NodeID, State: "queued", EffectState: "not_started", HealthState: "not_checked", ApprovedBy: current.Approval.ActorID, ExecutedBy: p.ID, CreatedAt: now(), UpdatedAt: now(), AllowedActions: []string{"reconcile"}, Blockers: []ReleaseBlocker{}}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_release_deployments(operation_id,tenant_id,project_id,version,run_id,node_id,approved_by,executed_by,data)VALUES(?,?,?,?,?,?,?,?,?)`), op, p.TenantID, project, 1, run, out.NodeID, out.ApprovedBy, out.ExecutedBy, stringify(out)); err != nil {
		return out, err
	}
	oldPlan := current.Version
	current.Version++
	current.State = "executing"
	current.ExecutionOperationID = op
	current.AllowedActions = []string{"revoke"}
	err = s.saveReleaseJSON(ctx, tx, "plans", p.TenantID, project, id, oldPlan, current)
	return out, err
}
func (s *Service) guardOriginalReleasePlanOwnershipTx(ctx context.Context, tx *sql.Tx, p Principal, project, plan string) error {
	var owner string
	err := tx.QueryRowContext(ctx, s.q(`SELECT rollout_id FROM suma_application_release_rollout_members WHERE tenant_id=? AND project_id=? AND plan_id=?`), p.TenantID, project, plan).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return releaseConflict("rollout_plan_owned", "This release plan is controlled by its original application rollout")
}

func (s *Service) ResolveBoundSubworkflowTx(ctx context.Context, tx *sql.Tx, ref dag.BoundSubworkflowRef) (out dag.BoundSubworkflowPlan, err error) {
	r, m, hash, starter, err := s.applicationRolloutForBindingTx(ctx, tx, ref.TenantID, ref.ProjectID, ref.BindingID)
	if err != nil {
		return out, err
	}
	if r.ParentRunID != ref.ParentRunID || m.ParentNodeID != ref.NodeID || ref.Attempt != 1 || ref.Fence != 1 {
		return out, forbidden()
	}
	auth, err := s.persistedReleaseAdminLockedTx(ctx, tx, hash, starter, ref.TenantID, ref.ProjectID)
	if err != nil {
		if automationAuthorityFailure(err) {
			out.BlockedReason = "rollout_authority_changed"
			return out, nil
		}
		return out, err
	}
	blockers, err := s.applicationRolloutMemberReadyTx(ctx, tx, auth.currentPrincipal, r, m)
	if err != nil {
		return out, err
	}
	if len(blockers) > 0 {
		out.BlockedReason = blockers[0].Code
		return out, nil
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, ref.TenantID, ref.ProjectID, m.PlanID)
	if err != nil {
		return out, err
	}
	return dag.BoundSubworkflowPlan{RunID: m.ChildRunID, Workflow: originalReleaseExecutionWorkflow(plan)}, nil
}
func (s *Service) BindBoundSubworkflowTx(ctx context.Context, tx *sql.Tx, ref dag.BoundSubworkflowRef, child string) error {
	r, m, hash, starter, err := s.applicationRolloutForBindingTx(ctx, tx, ref.TenantID, ref.ProjectID, ref.BindingID)
	if err != nil {
		return err
	}
	if r.ParentRunID != ref.ParentRunID || m.ParentNodeID != ref.NodeID || m.ChildRunID != child || ref.Attempt != 1 || ref.Fence != 1 {
		return forbidden()
	}
	auth, err := s.persistedReleaseAdminLockedTx(ctx, tx, hash, starter, ref.TenantID, ref.ProjectID)
	if err != nil {
		return err
	}
	blockers, err := s.applicationRolloutMemberReadyTx(ctx, tx, auth.currentPrincipal, r, m)
	if err != nil {
		return err
	}
	if len(blockers) > 0 {
		return conflict(applicationRolloutBlocked)
	}
	_, err = s.bindOriginalReleasePlanExecutionTx(ctx, tx, auth, ref.ProjectID, m.PlanID, child)
	return err
}
func (s *Service) CheckBoundSubworkflowTx(ctx context.Context, tx *sql.Tx, ref dag.BoundSubworkflowRef, child, state string) (out dag.BoundSubworkflowCheck, err error) {
	r, m, _, _, err := s.applicationRolloutForBindingTx(ctx, tx, ref.TenantID, ref.ProjectID, ref.BindingID)
	if err != nil {
		return out, err
	}
	if r.ParentRunID != ref.ParentRunID || m.ParentNodeID != ref.NodeID || m.ChildRunID != child || ref.Attempt != 1 || ref.Fence != 1 {
		return out, forbidden()
	}
	if state == "failed" || state == "cancelled" {
		plan, e := s.currentReleasePlanTx(ctx, tx, ref.TenantID, ref.ProjectID, m.PlanID)
		if e != nil {
			return out, e
		}
		if plan.ExecutionOperationID != "" {
			d, e := s.currentReleaseDeploymentTx(ctx, tx, ref.TenantID, ref.ProjectID, plan.ExecutionOperationID)
			if e != nil {
				return out, e
			}
			if d.Observation != nil && releaseObservationKnownTerminal(*d.Observation) && validateReleaseObservation(d, plan, *d.Observation) == nil {
				return dag.BoundSubworkflowCheck{PauseReason: "rollout_child_failed", StopConfirmed: true}, nil
			}
		}
		return dag.BoundSubworkflowCheck{PauseReason: "rollout_operation_unknown"}, nil
	}
	if state == "succeeded" {
		healthy, err := s.applicationRolloutMemberHealthyTx(ctx, tx, ref.TenantID, ref.ProjectID, m)
		if err != nil {
			return out, err
		}
		if !healthy {
			return dag.BoundSubworkflowCheck{PauseReason: "rollout_operation_unknown"}, nil
		}
		return dag.BoundSubworkflowCheck{StopConfirmed: true}, nil
	}
	var count int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND state='reconciling'`), child).Scan(&count); err != nil {
		return out, err
	}
	if count > 0 {
		return dag.BoundSubworkflowCheck{PauseReason: "rollout_operation_unknown"}, nil
	}
	return out, nil
}

type applicationRolloutCommandPermit struct {
	Service    *Service
	Authority  *authorizedCommand
	ID, Action string
}

var applicationRolloutCommandPermits sync.Map // exact live transaction -> private authority capability
func (s *Service) AuthorizeBoundRunCommandTx(ctx context.Context, tx *sql.Tx, tenant, project, run, action string) error {
	var id string
	err := tx.QueryRowContext(ctx, s.q(`SELECT id FROM suma_application_release_rollouts WHERE tenant_id=? AND project_id=? AND parent_run_id=?`), tenant, project, run).Scan(&id)
	if err == nil {
		value, ok := applicationRolloutCommandPermits.Load(tx)
		if !ok {
			return forbidden()
		}
		permit, ok := value.(applicationRolloutCommandPermit)
		if !ok || permit.Service != s || permit.ID != id || permit.Action != action || permit.Authority == nil || permit.Authority.actor.kind != actorBrowser || permit.Authority.purpose != purposeBrowserProjectWrite {
			return forbidden()
		}
		return assertProjectCommand(tx, permit.Authority, project)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	err = tx.QueryRowContext(ctx, s.q(`SELECT rollout_id FROM suma_application_release_rollout_members WHERE tenant_id=? AND project_id=? AND child_run_id=?`), tenant, project, run).Scan(&id)
	if err == nil {
		return forbidden()
	}
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	return err
}

func (s *Service) applicationRolloutMemberHealthyTx(ctx context.Context, tx *sql.Tx, tenant, project string, m ApplicationReleaseRolloutMember) (bool, error) {
	var state string
	err := tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, m.ChildRunID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if state != "succeeded" {
		return false, nil
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, tenant, project, m.PlanID)
	if err != nil {
		return false, err
	}
	if plan.SHA256 != m.PlanSHA256 || plan.Revision != m.PlanRevision || plan.ExecutionOperationID == "" {
		return false, nil
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, tenant, project, plan.ExecutionOperationID)
	if err != nil {
		return false, err
	}
	if d.RunID != m.ChildRunID || d.NodeID != m.ChildNodeID || d.State != "deployed_healthy" || d.Observation == nil || !releaseObservationKnownTerminal(*d.Observation) || validateReleaseObservation(d, plan, *d.Observation) != nil {
		return false, nil
	}
	env, err := s.releaseEnvironmentTx(ctx, tx, tenant, project, d.EnvironmentID, true)
	if err != nil {
		return false, err
	}
	if env.Generation != d.Generation || env.ActiveGeneration != d.Generation || env.ActiveArtifactID != d.ArtifactID || env.ActiveArtifactSHA256 != d.ArtifactSHA256 || env.BarrierOperationID != "" || env.Status != "active" {
		return false, nil
	}
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT a.result FROM dag_nodes n JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt AND a.fence=n.fence AND a.worker_id=n.worker_id WHERE n.run_id=? AND n.id=? AND n.state='succeeded'`), m.ChildRunID, m.ChildNodeID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var result dag.Result
	if json.Unmarshal([]byte(raw), &result) != nil || result.ReleaseEvidence == nil || result.Stopped || result.OutcomeUnknown || result.ExitCode != 0 || result.ReleaseEvidence.OperationID != d.OperationID || result.ReleaseEvidence.EvidenceSHA256 != d.Observation.EvidenceSHA256 || d.Assignment == nil || !releaseSame(*d.Assignment, artifactRef(result)) {
		return false, nil
	}
	return true, nil
}
func (s *Service) applicationRolloutMemberReadyTx(ctx context.Context, tx *sql.Tx, p Principal, r ApplicationReleaseRollout, m ApplicationReleaseRolloutMember) ([]ApplicationReleaseSetBlocker, error) {
	blockers := []ApplicationReleaseSetBlocker{}
	for _, dep := range m.DependsOn {
		var upstream ApplicationReleaseRolloutMember
		for _, v := range r.Members {
			if v.ApplicationID == dep {
				upstream = v
				break
			}
		}
		if upstream.PlanID == "" {
			return nil, conflict(applicationRolloutChanged)
		}
		healthy, err := s.applicationRolloutMemberHealthyTx(ctx, tx, p.TenantID, r.ProjectID, upstream)
		if err != nil {
			return nil, err
		}
		if !healthy {
			blockers = append(blockers, rolloutBlocker("rollout_dependency_unhealthy", m.ApplicationID, upstream.PlanID))
		}
	}
	plan, err := s.currentReleasePlanTx(ctx, tx, p.TenantID, r.ProjectID, m.PlanID)
	if err != nil {
		return nil, err
	}
	if plan.Version != m.PlanVersion || plan.Revision != m.PlanRevision || plan.SHA256 != m.PlanSHA256 || plan.Content.ReservedExecution.RunID != m.ChildRunID || plan.Content.ReservedExecution.NodeID != m.ChildNodeID || plan.State != "approved" || plan.Approval == nil || plan.Approval.Decision != "approved" || plan.Approval.PlanSHA256 != plan.SHA256 || plan.ExecutionOperationID != "" {
		return append(blockers, rolloutBlocker("rollout_approval_required", m.ApplicationID, m.PlanID)), nil
	}
	if err = s.releaseActorAdminTx(ctx, tx, p.TenantID, r.ProjectID, plan.Approval.ActorID); err != nil {
		if automationAuthorityFailure(err) {
			return append(blockers, rolloutBlocker("rollout_approval_required", m.ApplicationID, m.PlanID)), nil
		}
		return nil, err
	}
	if err = s.currentReleasePlanBasisTx(ctx, tx, p, r.ProjectID, plan, ""); err != nil {
		var api *APIError
		if errors.As(err, &api) && (api.Status == 400 || api.Status == 403 || api.Status == 404 || api.Status == 409) {
			return append(blockers, rolloutBlocker("rollout_plan_changed", m.ApplicationID, m.PlanID)), nil
		}
		return nil, err
	}
	return blockers, nil
}

// The original authorization/effect path calls this through its existing
// current plan basis gate. A queued child cannot use an obsolete dependency
// health result, and cancellation cannot authorize a new external effect.
func (s *Service) applicationReleaseRolloutPlanBasisTx(ctx context.Context, tx *sql.Tx, p Principal, project string, plan ReleasePlan) error {
	var binding string
	err := tx.QueryRowContext(ctx, s.q(`SELECT binding_id FROM suma_application_release_rollout_members WHERE tenant_id=? AND project_id=? AND plan_id=?`), p.TenantID, project, plan.ID).Scan(&binding)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	r, m, _, _, err := s.applicationRolloutForBindingTx(ctx, tx, p.TenantID, project, binding)
	if err != nil {
		return err
	}
	if m.PlanSHA256 != plan.SHA256 || m.PlanRevision != plan.Revision || m.ChildRunID != plan.Content.ReservedExecution.RunID {
		return conflict(applicationRolloutChanged)
	}
	var state string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT state FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), p.TenantID, project, r.ParentRunID).Scan(&state); err != nil {
		return err
	}
	if state == "cancelling" || state == "cancelled" {
		return releaseConflict("rollout_cancel_requested", "The application rollout has requested cancellation")
	}
	for _, dep := range m.DependsOn {
		found := false
		for _, up := range r.Members {
			if up.ApplicationID == dep {
				found = true
				healthy, e := s.applicationRolloutMemberHealthyTx(ctx, tx, p.TenantID, project, up)
				if e != nil {
					return e
				}
				if !healthy {
					return releaseConflict("rollout_dependency_unhealthy", "The required application deployment is not currently healthy")
				}
				break
			}
		}
		if !found {
			return conflict(applicationRolloutChanged)
		}
	}
	return nil
}
