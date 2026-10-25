package platform

import (
	"context"
	"database/sql"
	"errors"
)

// Historical design text remains reviewable after eligibility changes. This
// read is deliberately independent from dispatch's current-only resolver.
func (s *Service) GetTaskDesignContext(ctx context.Context, actor Actor, project, taskID string) (out TaskDesignContextView, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: taskID}, purposeProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.taskDesignContextViewTx(auth.ctx, auth, project, taskID)
		return e
	})
	if err != nil {
		return TaskDesignContextView{}, err
	}
	return
}

func (s *Service) taskDesignContextViewTx(ctx context.Context, auth *authorizedCommand, project, taskID string) (out TaskDesignContextView, err error) {
	task, err := s.getRecordTx(ctx, auth, project, "tasks", taskID)
	if err != nil {
		return out, err
	}
	out = TaskDesignContextView{ProjectID: project, TaskID: taskID, TaskVersion: task.Version, Eligibility: "none"}
	var requirement string
	var link designTaskLink
	err = auth.tx.QueryRowContext(ctx, s.q(`SELECT requirement_id,task_key,task_id,active,revision,requirement_version,task_basis,adoption_id FROM suma_requirement_design_task_links WHERE tenant_id=? AND project_id=? AND task_id=?`), auth.target.tenantID, project, taskID).Scan(&requirement, &link.Key, &link.TaskID, &link.Active, &link.Revision, &link.RequirementVersion, &link.TaskBasis, &link.AdoptionID)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Bound, out.Eligibility = true, "source_unavailable"
	aggregate, err := s.designAggregateTx(ctx, auth, project, requirement)
	if err != nil {
		return out, err
	}
	out.CurrentDesignVersion, out.CurrentPublishedRevision = aggregate.Version, aggregate.Published
	source, sourceErr := s.getRecordTx(ctx, auth, project, "requirements", requirement)
	if sourceErr != nil {
		var api *APIError
		if !errors.As(sourceErr, &api) || api.Status != 404 {
			return out, sourceErr
		}
	} else {
		out.CurrentRequirementVersion = source.Version
	}
	adoption, revision, proposal, err := s.taskDesignOriginalTx(ctx, auth, project, requirement, link)
	if errors.Is(err, errTaskDesignSource) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Context = &RequirementDesignContext{RequirementID: requirement, RequirementVersion: adoption.RequirementVersion, Revision: adoption.Revision, SHA256: adoption.SHA256, TaskKey: link.Key, AdoptionID: adoption.ID, Content: revision.Content}
	out.Requirement, out.AdoptedTask = &revision.Requirement, &proposal
	out.AdoptedBy, out.AdoptedAt = adoption.CreatedBy, adoption.CreatedAt
	switch {
	case sourceErr != nil || source.Status == "archived" || source.Status == "deleted" || aggregate.Version < 1:
		out.Eligibility = "source_unavailable"
	case link.Active == 0:
		out.Eligibility = "excluded"
	case aggregate.State == "retired":
		out.Eligibility = "retired"
	case aggregate.Published != adoption.Revision:
		out.Eligibility = "publication_changed"
	case source.Version != adoption.RequirementVersion || !designSourceCurrent(source, revision.Requirement):
		out.Eligibility = "requirement_changed"
	case task.RequirementID != requirement || task.Status == "archived" || task.Status == "deleted" || taskBasis(task) != link.TaskBasis:
		out.Eligibility = "task_changed"
	default:
		out.Eligibility = "current"
	}
	return out, nil
}

func taskDesignAdoptionTask(a RequirementDesignAdoption, link designTaskLink) (RequirementDesignAdoptedTask, error) {
	for _, task := range a.Tasks {
		if task.Key == link.Key && task.TaskID == link.TaskID {
			return task, nil
		}
	}
	return RequirementDesignAdoptedTask{}, errTaskDesignSource
}

func (s *Service) taskDesignOriginalTx(ctx context.Context, auth *authorizedCommand, project, requirement string, link designTaskLink) (adoption RequirementDesignAdoption, revision RequirementDesignRevision, proposal RequirementDesignTask, err error) {
	if link.Active != 0 && link.Active != 1 || requirement == "" || !designKey(link.Key) || link.Revision < 1 || link.RequirementVersion < 1 || len(link.TaskBasis) != 64 || link.AdoptionID == "" {
		return adoption, revision, proposal, errTaskDesignSource
	}
	adoption, err = s.taskDesignAdoptionTx(ctx, auth, project, requirement, link.AdoptionID)
	if err != nil {
		return
	}
	var adopted RequirementDesignAdoptedTask
	adopted, err = taskDesignAdoptionTask(adoption, link)
	if err != nil {
		return
	}
	if adoption.Revision != link.Revision || adoption.RequirementVersion != link.RequirementVersion {
		return adoption, revision, proposal, errTaskDesignSource
	}
	revision, err = s.taskDesignRevisionTx(ctx, auth, project, requirement, adoption)
	if err != nil {
		return
	}
	if link.Active == 0 {
		if adopted.Action != "exclude" {
			return adoption, revision, proposal, errTaskDesignSource
		}
		for _, task := range revision.Tasks {
			if task.Key == link.Key {
				return adoption, revision, proposal, errTaskDesignSource
			}
		}
		var indexed taskDesignIndexedAdoption
		err = auth.tx.QueryRowContext(ctx, s.q(`SELECT adoption_id,adoption_version,task_key,action,task_version FROM suma_requirement_design_adoption_tasks WHERE tenant_id=? AND project_id=? AND requirement_id=? AND task_id=? AND task_key=? AND adoption_version<=? AND action IN ('create','update') ORDER BY adoption_version DESC LIMIT 1`), auth.target.tenantID, project, requirement, link.TaskID, link.Key, adoption.Version).Scan(&indexed.id, &indexed.version, &indexed.key, &indexed.action, &indexed.taskVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return adoption, revision, proposal, errTaskDesignSource
		}
		if err != nil {
			return
		}
		adoption, err = s.taskDesignAdoptionTx(ctx, auth, project, requirement, indexed.id)
		if err != nil {
			return
		}
		adopted, err = taskDesignAdoptionTask(adoption, link)
		if err != nil {
			return
		}
		if adoption.Version != indexed.version || adopted.Key != indexed.key || adopted.Action != indexed.action || adopted.Version != indexed.taskVersion {
			return adoption, revision, proposal, errTaskDesignSource
		}
		revision, err = s.taskDesignRevisionTx(ctx, auth, project, requirement, adoption)
		if err != nil {
			return
		}
	}
	if adopted.Action != "create" && adopted.Action != "update" {
		return adoption, revision, proposal, errTaskDesignSource
	}
	found := false
	for _, task := range revision.Tasks {
		if task.Key == link.Key {
			if found {
				return adoption, revision, proposal, errTaskDesignSource
			}
			proposal, found = task, true
		}
	}
	if !found {
		return adoption, revision, proposal, errTaskDesignSource
	}
	return
}
