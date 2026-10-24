package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
)

// A project credential remains a project credential even when it supplies the
// name/fence of a pool worker. Read the immutable server route rather than
// trusting the submitted assignment (whose pool fields can be stripped).
func (s *Service) projectRunnerExecutionAuthorityTx(ctx context.Context, tx *sql.Tx, p Principal, project, token, worker, run string) error {
	if err := s.runnerIdentityTx(ctx, tx, p, project, token, worker); err != nil {
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
