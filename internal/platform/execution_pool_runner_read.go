package platform

import "context"

// GetExecutionPoolRunner reads one exact registered machine in its owning
// organization. It does not infer health, contact a machine, expose a token or
// inspect any consuming project's files, tools or execution state.
func (s *Service) GetExecutionPoolRunner(ctx context.Context, actor Actor, pool, runner string) (ExecutionPoolRunner, error) {
	return s.readExecutionPoolRunner(ctx, actor, pool, runner, nil)
}

// The HTTP parser runs only after the same transaction has established current
// organization administration and pool ownership. A malformed request must not
// disclose whether an inaccessible pool or runner exists.
func (s *Service) readExecutionPoolRunner(ctx context.Context, actor Actor, pool, runner string, validateRequest func() error) (ExecutionPoolRunner, error) {
	var out ExecutionPoolRunner
	err := s.withCurrentCRead(ctx, actor, commandTarget{tenantID: actor.credential.tenantID, projectID: organizationScope}, purposeOrganizationRead, func(ctx context.Context, auth *authorizedCommand) error {
		if _, err := s.poolAdminTx(ctx, auth.tx, auth, pool); err != nil {
			return err
		}
		if validateRequest != nil {
			if err := validateRequest(); err != nil {
				return err
			}
		}
		var err error
		out, err = s.poolRunnerTx(ctx, auth.tx, pool, runner)
		return err
	})
	return out, err
}
