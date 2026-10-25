package platform

import "context"

// Public protected reads carry their original credential and read through the
// same current-proof transaction; private readers never open another connection.

func (s *Service) PreviewAutomation(ctx context.Context, actor Actor, project string, a AutomationSpec) (AutomationPreview, error) {
	var out AutomationPreview
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.previewAutomationCurrentTx(auth.ctx, auth, project, a)
		return e
	})
	return out, err
}

func (s *Service) GetAutomation(ctx context.Context, actor Actor, project, id string) (AutomationRule, error) {
	var out AutomationRule
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.getAutomationCurrentTx(auth.ctx, auth, project, id)
		return e
	})
	return out, err
}

func (s *Service) ListAutomations(ctx context.Context, actor Actor, project string, limit int, cursor string) (AutomationList, error) {
	var out AutomationList
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.listAutomationsCurrentTx(auth.ctx, auth, project, limit, cursor)
		return e
	})
	return out, err
}

func (s *Service) AutomationOccurrences(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (AutomationOccurrences, error) {
	var out AutomationOccurrences
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.automationOccurrencesCurrentTx(auth.ctx, auth, project, id, limit, cursor)
		return e
	})
	return out, err
}

func (s *Service) AutomationRevisions(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (AutomationRevisions, error) {
	var out AutomationRevisions
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.automationRevisionsCurrentTx(auth.ctx, auth, project, id, limit, cursor)
		return e
	})
	return out, err
}

func (s *Service) GetAutomationIngress(ctx context.Context, actor Actor, project, id string) (AutomationIngress, error) {
	var out AutomationIngress
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.getAutomationIngressCurrentTx(auth.ctx, auth, project, id)
		return e
	})
	return out, err
}

func (s *Service) ListAutomationIngresses(ctx context.Context, actor Actor, project, rule string, limit int, cursor string) (AutomationIngressList, error) {
	var out AutomationIngressList
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: rule}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.listAutomationIngressesCurrentTx(auth.ctx, auth, project, rule, limit, cursor)
		return e
	})
	return out, err
}

func (s *Service) AutomationIngressRevisions(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (AutomationIngressRevisions, error) {
	var out AutomationIngressRevisions
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.automationIngressRevisionsCurrentTx(auth.ctx, auth, project, id, limit, cursor)
		return e
	})
	return out, err
}

func (s *Service) AutomationIngressDeliveries(ctx context.Context, actor Actor, project, id string, limit int, cursor string) (AutomationIngressDeliveries, error) {
	var out AutomationIngressDeliveries
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.automationIngressDeliveriesCurrentTx(auth.ctx, auth, project, id, limit, cursor)
		return e
	})
	return out, err
}

func (s *Service) GetAutomationIngressDelivery(ctx context.Context, actor Actor, project, id, delivery string) (AutomationIngressDelivery, error) {
	var out AutomationIngressDelivery
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.getAutomationIngressDeliveryCurrentTx(auth.ctx, auth, project, id, delivery)
		return e
	})
	return out, err
}

func (s *Service) GetToolProfile(ctx context.Context, actor Actor, project, id string) (ToolProfile, error) {
	var out ToolProfile
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: id}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.getToolProfileCurrentTx(auth.ctx, auth, project, id)
		return e
	})
	return out, err
}

func (s *Service) ListToolProfiles(ctx context.Context, actor Actor, project string, limit int, cursor string) (ToolProfileList, error) {
	var out ToolProfileList
	err := s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project, resourceID: ""}, purposeBrowserProjectRead, func(auth *authorizedCommand) error {
		var e error
		out, e = s.listToolProfilesCurrentTx(auth.ctx, auth, project, limit, cursor)
		return e
	})
	return out, err
}
