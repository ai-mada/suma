package platform

import (
	"context"
	"database/sql"
	"errors"
)

// Managed execution and provider failures share one durable operation issue.
// The same organization/scheduler lock order serializes both projections.
func (s *Service) projectManagedReleaseFailure(ctx context.Context, tenant, project, runID string) (bool, error) {
	ctx, tx, cancel, err := s.beginAuthorityTx(ctx)
	if err != nil {
		return false, err
	}
	defer cancel()
	defer tx.Rollback()
	// Scope hints become authority only after matching a real persisted failed
	// execution in this transaction. There is no general system Principal.
	proof, err := s.failureSourceTx(ctx, tx, tenant, project, runID)
	if err != nil {
		return false, err
	}
	if err = proof.assertTx(tx); err != nil {
		return false, err
	}
	tenant, project, runID = proof.tenant, proof.project, proof.run
	if err = s.releaseSchedulerLock(ctx, tx, tenant, project); err != nil {
		return false, err
	}
	var operation string
	err = tx.QueryRowContext(ctx, s.q(`SELECT operation_id FROM suma_release_deployments WHERE tenant_id=? AND project_id=? AND run_id=?`), tenant, project, runID).Scan(&operation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	d, err := s.currentReleaseDeploymentTx(ctx, tx, tenant, project, operation)
	if err != nil {
		return true, err
	}
	if d.IssueID != "" {
		return true, nil
	}
	// This path proves execution failure only. An earlier healthy provider
	// observation remains attached to the deployment, never relabeled as failed.
	if err = s.ensureReleaseIssueTx(ctx, tx, tenant, project, &d, nil); err != nil {
		return true, err
	}
	old := d.Version
	d.Version++
	d.UpdatedAt = now()
	if d.Observation == nil {
		d.State = "reconciling"
	}
	if err = s.saveReleaseJSON(ctx, tx, "deployments", tenant, project, operation, old, d); err != nil {
		return true, err
	}
	if err = proof.assertTx(tx); err != nil {
		return true, err
	}
	if _, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_activity(id,tenant_id,project_id,actor_id,kind,object_id,title,created_at) VALUES(?,?,?,?,?,?,?,?)`), newID(), tenant, project, "system", "release.issue_projected", d.IssueID, "发布执行失败关联原操作问题", now()); err != nil {
		return true, err
	}
	return true, tx.Commit()
}
