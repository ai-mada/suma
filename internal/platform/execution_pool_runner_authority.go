package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
)

// A project credential remains a project credential even when it supplies the
// name/fence of a pool worker. Read the immutable server route rather than
// trusting the submitted assignment (whose pool fields can be stripped).
func (s *Service) projectRunnerExecutionAuthorityTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, worker, run string) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(tx, auth.target, purposeRunnerWrite); err != nil {
		return err
	}
	p, project := auth.currentPrincipal, auth.target.projectID

	if err := s.runnerIdentityBindingTx(ctx, tx, auth, worker); err != nil {
		return err
	}
	route, err := dag.ReadRunExecutionRouteTx(ctx, tx, s.Dialect, p.TenantID, project, run)
	if err != nil {
		return err
	}
	if route.PoolID != "" {
		return forbidden()
	}
	return nil
}
