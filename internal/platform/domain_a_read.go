package platform

import "context"

func assertProjectRead(auth *authorizedCommand, project string) error {
	if auth == nil || auth.target.projectID != project || project == "" {
		return forbidden()
	}
	switch auth.purpose {
	case purposeProjectRead, purposeProjectWrite, purposeProjectApprove, purposeBrowserProjectRead, purposeBrowserProjectWrite, purposeBrowserProjectApprove, purposeAgentRead, purposeRunnerRead, purposeRunnerWrite:
	default:
		return forbidden()
	}
	return auth.assertTx(auth.tx, auth.target, auth.purpose)
}

func assertOrganizationRead(auth *authorizedCommand) error {
	if auth == nil || auth.target.projectID != organizationScope {
		return forbidden()
	}
	if auth.purpose != purposeOrganizationRead && auth.purpose != purposeOrganizationWrite {
		return forbidden()
	}
	return auth.assertTx(auth.tx, auth.target, auth.purpose)
}

func (s *Service) List(ctx context.Context, actor Actor, project, kind string) (out []Record, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.listRecordsTx(bounded, auth, project, kind)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Get(ctx context.Context, actor Actor, project, kind, id string) (out Record, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.getRecordTx(bounded, auth, project, kind, id)
		return err
	})
	if err != nil {
		return Record{}, err
	}
	return out, err
}

func (s *Service) ListPage(ctx context.Context, actor Actor, project, kind string, limit int, cursor string) (items []Record, total int, next string, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		items, total, next, err = s.listPageTx(bounded, auth, project, kind, limit, cursor)
		return err
	})
	if err != nil {
		return nil, 0, "", err
	}
	return items, total, next, err
}

func (s *Service) Organizations(ctx context.Context, actor Actor) (out []map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.organizationsTx(bounded, auth)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) OrganizationMembers(ctx context.Context, actor Actor) (out []map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.organizationMembersTx(bounded, auth)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Invitations(ctx context.Context, actor Actor) (out []map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.invitationsTx(bounded, auth)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Teams(ctx context.Context, actor Actor) (out []map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.teamsTx(bounded, auth)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) TeamMembers(ctx context.Context, actor Actor, team string) (out []map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.teamMembersTx(bounded, auth, team)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) ProjectAccess(ctx context.Context, actor Actor, project string) (out []map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.projectAccessTx(bounded, auth, project)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) PlanningPeriods(ctx context.Context, actor Actor, project, kind string) (out []PlanningPeriod, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.planningPeriodsTx(bounded, auth, project, kind)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Planning(ctx context.Context, actor Actor, project string, limit int, cursor, week, taskID string) (out map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.planningTx(bounded, auth, project, limit, cursor, week, taskID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) PlanningCapacity(ctx context.Context, actor Actor, project, weekValue string) (out []CapacityEntry, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.planningCapacityTx(bounded, auth, project, weekValue)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Baselines(ctx context.Context, actor Actor, project string) (out []BaselineSummary, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.baselinesTx(bounded, auth, project)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Baseline(ctx context.Context, actor Actor, project, id, cursor string, limit int) (out map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.baselineTx(bounded, auth, project, id, cursor, limit)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}

func (s *Service) Overview(ctx context.Context, actor Actor, project string) (out map[string]any, err error) {
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(auth *authorizedCommand) error {
		bounded, cancel := context.WithDeadline(ctx, auth.deadline)
		defer cancel()
		out, err = s.overviewTx(bounded, auth, project)
		return err
	})
	if err != nil {
		return nil, err
	}
	return out, err
}
