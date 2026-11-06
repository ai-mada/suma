package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/dag"
	term "github.com/ai-mada/suma/internal/terminalcontract"
)

const taskDeliveryWorkBuddyOriginChanged = "Cannot verify the original WorkBuddy handoff"

func (s *Service) taskDeliveryWorkBuddyOriginTx(ctx context.Context, auth *authorizedCommand, task Record) (*TaskDeliveryWorkBuddyOrigin, error) {
	tenant, project, run := auth.target.tenantID, task.ProjectID, task.RunID
	var id, repositoryPlan, terminalPlan string
	err := auth.tx.QueryRowContext(ctx, s.q(`SELECT p.id,COALESCE(r.plan_id,''),COALESCE(t.plan_id,'')
		FROM suma_task_dispatch_plans p
		LEFT JOIN suma_repository_source_dispatch_plans r ON r.tenant_id=p.tenant_id AND r.project_id=p.project_id AND r.run_id=p.run_id
		LEFT JOIN suma_workspace_terminal_candidate_plans t ON t.tenant_id=p.tenant_id AND t.project_id=p.project_id AND t.run_id=p.run_id
		WHERE p.tenant_id=? AND p.project_id=? AND p.run_id=? AND (r.plan_id IS NOT NULL OR t.plan_id IS NOT NULL)`), tenant, project, run).
		Scan(&id, &repositoryPlan, &terminalPlan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	changed := conflict(taskDeliveryWorkBuddyOriginChanged)
	if repositoryPlan != "" && terminalPlan != "" || repositoryPlan != "" && repositoryPlan != id || terminalPlan != "" && terminalPlan != id {
		return nil, changed
	}

	plan, err := s.taskDispatchPlanTx(ctx, auth.tx, auth.currentPrincipal, project, id)
	if err != nil {
		return nil, err
	}
	if plan.ID != id || plan.State != "started" || plan.TaskID != task.ID || plan.ProjectID != project ||
		plan.RunID != run || plan.TaskVersion < 1 || plan.TaskVersion >= task.Version {
		return nil, changed
	}

	var origin *term.TerminalCandidateWorkBuddyOrigin
	kind := "repository_source"
	if repositoryPlan != "" {
		source, err := s.checkRepositorySourceDispatchModeTx(ctx, auth.tx, auth, plan)
		if err != nil {
			return nil, err
		}
		if source == nil {
			return nil, changed
		}
		record, _, err := s.loadRepositorySourceIntakeTx(ctx, auth.tx, tenant, project, task.ID, source.IntakeID)
		if err != nil {
			return nil, err
		}
		if !repositoryExecutionMatchesReceipt(*source, record.Receipt) {
			return nil, changed
		}
		origin = &record.Origin
	} else {
		kind = "local_candidate"
		source, err := s.workspaceTerminalCandidatePlanSourceTx(ctx, auth.tx, tenant, project, plan)
		if err != nil {
			return nil, err
		}
		if source == nil {
			return nil, changed
		}
		origin = source.WorkBuddyOrigin
	}
	if origin == nil {
		return nil, nil
	}
	if !origin.Valid() || origin.RunID == run {
		return nil, changed
	}

	// This is a historical reference. The original task pointer, wait reason,
	// lease lifetime and current provider authorization are immaterial to it.
	cloud, found, err := s.workBuddyTaskDispatchRunSourceTx(ctx, auth.tx, tenant, project, origin.RunID)
	if err != nil {
		return nil, err
	}
	if !found || cloud.State != "started" || cloud.ProjectID != project || cloud.TaskID != task.ID ||
		cloud.RunID != origin.RunID || cloud.NodeID != origin.NodeID {
		return nil, changed
	}
	assignment, err := dag.ReadExternalAssignmentTx(ctx, auth.tx, s.Dialect, tenant, project, origin.RunID, origin.NodeID, origin.Attempt)
	if errors.Is(err, dag.ErrNotFound) || errors.Is(err, dag.ErrConflict) {
		return nil, changed
	}
	if err != nil {
		return nil, err
	}
	if assignment.Fence != origin.Fence || assignment.AssignmentSHA256 != origin.AssignmentSHA256 ||
		assignment.Binding.DelegationID != cloud.DelegationID {
		return nil, changed
	}
	return &TaskDeliveryWorkBuddyOrigin{RunID: origin.RunID, NodeID: origin.NodeID, Attempt: origin.Attempt, Kind: kind}, nil
}
