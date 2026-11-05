package platform

import "context"

// SharedTaskViewSync identifies the current view directory of one authorized
// account and project. Clients compare the revision for equality.
type SharedTaskViewSync struct {
	ProjectID string `json:"project_id"`
	Revision  string `json:"revision"`
}

func (s *Service) SyncSharedTaskViews(ctx context.Context, actor Actor, project string) (out SharedTaskViewSync, err error) {
	if !myWorkHuman(actor) {
		return out, forbidden()
	}
	if !handoffID(project, false) {
		return out, invalid(sharedTaskViewInvalid)
	}
	err = s.withCurrentRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: project}, purposeProjectRead, func(a *authorizedCommand) error {
		write, _, _, err := s.repositoryCapabilitiesTx(a.ctx, a)
		if err != nil {
			return err
		}
		// Visible records enter at version one. Every edit and archive increases
		// the persisted version and archives retain the row. The sum therefore
		// changes on each accepted mutation, including changes beyond any page.
		var versions int64
		if err := a.tx.QueryRowContext(a.ctx, s.q(`SELECT COALESCE(SUM(version), 0)
			FROM suma_shared_task_views WHERE tenant_id=? AND project_id=?
			AND (visibility='project' OR (visibility='private' AND owner_id=?))`),
			a.target.tenantID, project, a.currentPrincipal.ID).Scan(&versions); err != nil {
			return err
		}
		out = SharedTaskViewSync{
			ProjectID: project,
			Revision: digest(stringify([]any{
				"task-view-directory-v1", a.target.tenantID, project, a.currentPrincipal.ID,
				write, orgAdmin(a.currentPrincipal), versions,
			})),
		}
		return nil
	})
	return
}
