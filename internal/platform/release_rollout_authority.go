package platform

import (
	"context"
	"database/sql"
)

// BeforeBoundSubworkflowScope is a transaction-lifecycle hook, not an actor
// factory. The DAG calls it before route/project/family locks, including Tick.
// Existing HTTP/Pool authority executes first and already locks the complete
// participant set. A system wake locks only its exact existing tenant; it may
// never append another organization after that set has been established.
func (s *Service) BeforeBoundSubworkflowScope(ctx context.Context, tx *sql.Tx, tenant, project string) (func(), error) {
	noop := func() {}
	if tx == nil || !handoffID(tenant, false) || !handoffID(project, false) || project == organizationScope {
		return noop, forbidden()
	}
	budget := s.txAuthorityBudget(ctx, tx)
	cleanup := func() { s.endAuthorityTx(tx) }
	if err := budget.ctx.Err(); err != nil {
		return cleanup, err
	}
	s.authorityBudgetMu.Lock()
	locked := budget.lockedOrganizations[tenant]
	otherScopes := len(budget.lockedOrganizations) > 0
	s.authorityBudgetMu.Unlock()
	if locked {
		return cleanup, nil
	}
	if otherScopes {
		return cleanup, &APIError{Status: 409, Code: "authority_retry", Message: "权限集合已改变，请重试"}
	}
	if err := s.authorityWriter(budget.ctx, tx); err != nil {
		return cleanup, err
	}
	if err := s.lockOrganization(budget.ctx, tx, tenant); err != nil {
		return cleanup, err
	}
	return cleanup, nil
}

// persistedReleaseAdminLockedTx authenticates the original stored browser
// credential under the exact tenant serialization lock and then rechecks the
// current release administrator. Subject IDs are comparison inputs only. No
// Actor can be manufactured from an ID, and this proof grants no other project.
func (s *Service) persistedReleaseAdminLockedTx(ctx context.Context, tx *sql.Tx, credentialHash, subject, tenant, project string) (*authorizedCommand, error) {
	value, ok := s.authorityBudgets.Load(tx)
	if !ok {
		return nil, forbidden()
	}
	s.authorityBudgetMu.Lock()
	locked := value.(*authorityTxBudget).lockedOrganizations[tenant]
	s.authorityBudgetMu.Unlock()
	if !locked {
		return nil, forbidden()
	}
	auth, err := s.persistedBrowserProjectWriteLockedTx(ctx, tx, credentialHash, subject, commandTarget{tenantID: tenant, projectID: project})
	if err != nil {
		return nil, err
	}
	if err = s.releaseActorAdminTx(auth.ctx, tx, tenant, project, auth.currentPrincipal.ID); err != nil {
		return nil, err
	}
	return auth, nil
}
