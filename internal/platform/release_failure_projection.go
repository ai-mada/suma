package platform

import (
	"context"
	"database/sql"
	"errors"
)

// Managed execution and provider failures share one durable operation issue.
// The same organization/scheduler lock order serializes both projections.
func (s *Service) projectManagedReleaseFailure(ctx context.Context, tenant, project, runID string) (bool, error) {
	tx, err := s.DB.BeginTx(ctx, s.executionWriteOptions())
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	p := Principal{ID: "system", TenantID: tenant, internal: true}
	if err = s.authorizeProjectTx(ctx, tx, p, project, true, false); err != nil {
		return false, err
	}
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
	if err = s.activity(ctx, tx, p, project, "release.issue_projected", d.IssueID, "发布执行失败关联原操作问题"); err != nil {
		return true, err
	}
	return true, tx.Commit()
}
