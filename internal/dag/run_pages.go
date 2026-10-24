package dag

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type runCursor struct {
	Tenant  string `json:"tenant"`
	Project string `json:"project"`
	At      string `json:"at"`
	ID      string `json:"id"`
}

// ListRunsPage returns bounded run metadata without loading node graphs or
// taking the scheduler mutation lock. Open one run for its complete snapshot.
func (e *Engine) ListRunsPage(ctx context.Context, tenant, project string, limit int, cursor string) ([]Run, int, string, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, 0, "", err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	c := runCursor{}
	if cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || len(raw) > 2048 || json.Unmarshal(raw, &c) != nil || c.Tenant != tenant || c.Project != project || c.At == "" || c.ID == "" {
			return nil, 0, "", dagDiagnostic(fmt.Errorf("%w: invalid run cursor", ErrInvalid), "dag.runs.cursor", nil)
		}
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, "", err
	}
	defer tx.Rollback()
	var total int
	if err = tx.QueryRowContext(ctx, e.q(`SELECT COUNT(*) FROM dag_runs WHERE tenant_id=? AND project_id=?`), tenant, project).Scan(&total); err != nil {
		return nil, 0, "", err
	}
	query := `SELECT r.id,r.tenant_id,r.project_id,r.workflow_id,r.workflow_name,r.workflow_version,r.version,r.state,r.created_at,r.updated_at,COALESCE(c.compensation_run_id,''),COALESCE(cr.state,'') FROM dag_runs r LEFT JOIN dag_compensations c ON c.original_run_id=r.id LEFT JOIN dag_runs cr ON cr.id=c.compensation_run_id WHERE r.tenant_id=? AND r.project_id=?`
	args := []any{tenant, project}
	if cursor != "" {
		query += ` AND (r.created_at<? OR (r.created_at=? AND r.id>?))`
		args = append(args, c.At, c.At, c.ID)
	}
	query += ` ORDER BY r.created_at DESC,r.id ASC LIMIT ?`
	args = append(args, limit+1)
	rows, err := tx.QueryContext(ctx, e.q(query), args...)
	if err != nil {
		return nil, 0, "", err
	}
	items := []Run{}
	for rows.Next() {
		var r Run
		if err = rows.Scan(&r.ID, &r.TenantID, &r.ProjectID, &r.WorkflowID, &r.WorkflowName, &r.WorkflowVersion, &r.Version, &r.State, &r.CreatedAt, &r.UpdatedAt, &r.CompensationRunID, &r.CompensationState); err != nil {
			rows.Close()
			return nil, 0, "", err
		}
		r.Nodes = []NodeRun{}
		items = append(items, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, 0, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		raw, _ := json.Marshal(runCursor{Tenant: tenant, Project: project, At: last.CreatedAt, ID: last.ID})
		next = base64.RawURLEncoding.EncodeToString(raw)
	}
	if err = tx.Commit(); err != nil {
		return nil, 0, "", err
	}
	return items, total, next, nil
}
