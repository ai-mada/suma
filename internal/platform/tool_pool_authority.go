package platform

import (
	"context"
	"database/sql"

	"github.com/ai-mada/suma/internal/dag"
)

func (s *Service) toolProfileForPoolTx(ctx context.Context, tx *sql.Tx, proof *poolMachineProof, scope dag.PoolScope, p ToolProfile) (ToolProfile, error) {
	if err := proof.assertScope(tx, scope); err != nil {
		return ToolProfile{}, err
	}
	if p.WorkerID != proof.runner.WorkerID {
		return ToolProfile{}, forbidden()
	}
	current, err := scanToolProfile(tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT `+toolProfileColumns+` FROM suma_tool_profiles WHERE tenant_id=? AND project_id=? AND id=?`)), scope.TenantID, scope.ProjectID, p.ID))
	if err != nil {
		return ToolProfile{}, err
	}
	if current.Version != p.Version || current.WorkerID != p.WorkerID || current.Profile.ProfileSHA256 != p.Profile.ProfileSHA256 {
		return ToolProfile{}, forbidden()
	}
	return s.toolProfileObservationLockedTx(ctx, tx, scope.TenantID, scope.ProjectID, current)
}
