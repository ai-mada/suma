package platform

import (
	"context"
	"errors"
	"net/http"
)

const workspaceTaskContextInvalid = "Invalid workspace task context request"

// GetWorkspaceTaskContext composes the original task, design, delivery and
// published-plan authorities under one current project-read transaction. It
// deliberately accepts completed/accepted tasks: this is not dispatch input.
func (s *Service) GetWorkspaceTaskContext(ctx context.Context, actor Actor, project, taskID string) (out WorkspaceTaskContext, err error) {
	if !testPlanActor(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) || !handoffID(taskID, false) {
		return out, invalid(workspaceTaskContextInvalid)
	}
	ctx, auth, close, err := s.planningAuthority(ctx, actor, project, taskID, purposeProjectRead)
	if err != nil {
		return out, err
	}
	defer close()
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	out = WorkspaceTaskContext{ProjectID: project, ObservedAt: now(),
		Task: WorkspaceTask{ID: task.ID, Version: task.Version, Title: task.Title, Description: task.Description,
			AcceptanceCriteria: append([]string{}, task.AcceptanceCriteria...), Status: task.Status,
			Priority: task.Priority, OwnerID: task.OwnerID, RequirementID: task.RequirementID, RunID: task.RunID},
		Design: WorkspaceTaskDesign{State: "none"}}
	if task.RequirementID != "" {
		requirement, e := s.getRecordTx(ctx, auth, project, "requirements", task.RequirementID)
		if e != nil {
			return WorkspaceTaskContext{}, e
		}
		compact := taskContextRecord(requirement)
		out.Requirement = &compact
	}
	if task.OwnerID != "" {
		owner, e := s.getRecordTx(ctx, auth, project, "members", task.OwnerID)
		if e != nil {
			return WorkspaceTaskContext{}, e
		}
		out.Owner = &WorkspaceTaskOwner{ID: owner.ID, Version: owner.Version, Name: owner.Name, Kind: owner.Kind, Status: owner.Status}
	}
	design, err := s.resolveTaskRequirementDesignTx(ctx, auth, project, taskID)
	if workspaceTaskContextStale(err) {
		out.Design.State = "stale"
	} else if err != nil {
		return WorkspaceTaskContext{}, err
	} else if design != nil {
		if out.Requirement == nil || design.RequirementID != out.Requirement.ID || design.RequirementVersion != out.Requirement.Version {
			out.Design.State = "stale"
		} else {
			out.Design = WorkspaceTaskDesign{State: "current", Context: design}
		}
	}
	if out.Delivery, err = s.taskDeliveryTx(ctx, auth, project, taskID); err != nil {
		return WorkspaceTaskContext{}, err
	}
	if out.Delivery.TaskVersion != task.Version || out.Delivery.TaskStatus != task.Status || out.Delivery.TaskID != task.ID {
		return WorkspaceTaskContext{}, conflict(workspaceTaskContextInvalid)
	}
	if out.Quality, err = s.workspaceTaskQualityTx(ctx, auth, project, task, out.Delivery); err != nil {
		return WorkspaceTaskContext{}, err
	}
	return out, auth.assertTx(auth.tx, auth.target, purposeProjectRead)
}

func workspaceTaskContextStale(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.Status == http.StatusConflict
}

func (s *Service) workspaceTaskQualityTx(ctx context.Context, auth *authorizedCommand, project string, task Record, delivery TaskDelivery) (out WorkspaceTaskQuality, err error) {
	out = WorkspaceTaskQuality{RunID: task.RunID, State: "none"}
	if delivery.Run != nil {
		out.RunState = delivery.Run.State
	}
	tenant := auth.target.tenantID
	current, found, err := s.testPlanByTaskTx(ctx, auth.tx, tenant, project, task.ID)
	if err != nil || !found {
		return out, err
	}
	out.PlanID = current.ID
	if task.RunID == "" {
		if current.Published == 0 {
			if current.State == "retired" {
				out.State = "retired"
			}
			return out, nil // A draft is not an executable published test plan.
		}
		revision, e := s.testPlanRevisionTx(ctx, auth.tx, tenant, project, current.ID, current.Published)
		if e != nil {
			return out, e
		}
		out.Revision, out.PlanSHA256 = revision.Revision, revision.SHA256
		out.Total, out.NotRun = len(revision.Cases), len(revision.Cases)
		out.State = "not_run"
		out.BasisCurrent, e = s.testPlanSourceCurrentTx(ctx, auth.tx, tenant, project, revision.Source)
		if workspaceTaskContextStale(e) {
			out.State, out.BasisCurrent = "stale", false
			return out, nil
		}
		if e != nil {
			return out, e
		}
		if !out.BasisCurrent {
			out.State = "source_changed"
		}
		return out, nil
	}
	// A missing run or protected snapshot is unknown, never an inferred pass.
	if delivery.Run == nil {
		out.State = "unknown"
		return out, nil
	}
	fixed, found, err := s.testPlanRunTx(ctx, auth.tx, tenant, project, task.RunID)
	if err != nil {
		return out, err
	}
	if !found || fixed.TaskID != task.ID || fixed.PlanID != current.ID {
		out.State = "unknown"
		return out, nil
	}
	execution, err := s.testPlanExecutionTx(ctx, auth.tx, tenant, project, current, fixed, task.RunID)
	if workspaceTaskContextStale(err) {
		out.State = "stale"
		return out, nil
	}
	if err != nil {
		return out, err
	}
	return WorkspaceTaskQuality{PlanID: execution.PlanID, Revision: execution.Revision, PlanSHA256: execution.PlanSHA256,
		RunID: execution.RunID, RunState: execution.RunState, State: execution.State, BasisCurrent: execution.BasisCurrent,
		Total: execution.Total, Passed: execution.Passed, Failed: execution.Failed, Errors: execution.Errors,
		Skipped: execution.Skipped, NotRun: execution.NotRun}, nil
}
