package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type executionPreviewRecord struct {
	id, tenant, project, pool, kind                 string
	version, applied                                int
	state, createdBy, createdAt, expiresAt, payload string
}

func (s *Service) executionPreviewRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, pool, kind, id string) (executionPreviewRecord, error) {
	var row executionPreviewRecord
	query := `SELECT id,tenant_id,project_id,pool_id,kind,version,state,applied_version,created_by,created_at,expires_at,payload FROM suma_execution_pool_previews WHERE id=? AND tenant_id=? AND project_id=? AND kind=?`
	args := []any{id, tenant, project, kind}
	if kind != "route" {
		query += ` AND pool_id=?`
		args = append(args, pool)
	}
	err := tx.QueryRowContext(ctx, s.q(query), args...).Scan(&row.id, &row.tenant, &row.project, &row.pool, &row.kind, &row.version, &row.state, &row.applied, &row.createdBy, &row.createdAt, &row.expiresAt, &row.payload)
	if errors.Is(err, sql.ErrNoRows) {
		return row, missing()
	}
	return row, err
}

func (s *Service) currentExecutionPreviewRecordTx(ctx context.Context, tx *sql.Tx, tenant, project, pool, kind, id string) (executionPreviewRecord, error) {
	var row executionPreviewRecord
	query := `SELECT id,tenant_id,project_id,pool_id,kind,version,state,applied_version,created_by,created_at,expires_at,payload FROM suma_execution_pool_previews WHERE id=? AND tenant_id=? AND project_id=? AND kind=?`
	args := []any{id, tenant, project, kind}
	if kind != "route" {
		query += ` AND pool_id=?`
		args = append(args, pool)
	}
	err := tx.QueryRowContext(ctx, s.q(s.currentSQL(query)), args...).Scan(&row.id, &row.tenant, &row.project, &row.pool, &row.kind, &row.version, &row.state, &row.applied, &row.createdBy, &row.createdAt, &row.expiresAt, &row.payload)
	if errors.Is(err, sql.ErrNoRows) {
		return row, missing()
	}
	return row, err
}

// Preview payloads retain immutable inputs; only lifecycle columns change.
// Expiry is evaluated from database time without a background status rewrite.
func (s *Service) saveExecutionPreviewTx(ctx context.Context, tx *sql.Tx, row executionPreviewRecord) error {
	var active int
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_execution_pool_previews WHERE tenant_id=? AND project_id=? AND pool_id=? AND kind=? AND state='ready' AND expires_at>?`), row.tenant, row.project, row.pool, row.kind, automationStamp(clock)).Scan(&active); err != nil {
		return err
	}
	if active >= 1000 {
		return conflict("当前未过期预览过多，请复用已有预览或等待过期")
	}
	_, err = tx.ExecContext(ctx, s.q(`INSERT INTO suma_execution_pool_previews(id,tenant_id,project_id,pool_id,kind,version,state,applied_version,created_by,created_at,expires_at,payload)VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`), row.id, row.tenant, row.project, row.pool, row.kind, 1, "ready", 0, row.createdBy, row.createdAt, row.expiresAt, row.payload)
	return err
}

func (s *Service) commitExecutionPreviewTx(ctx context.Context, tx *sql.Tx, row executionPreviewRecord, applied int) error {
	result, err := tx.ExecContext(ctx, s.q(`UPDATE suma_execution_pool_previews SET state='committed',version=version+1,applied_version=? WHERE id=? AND tenant_id=? AND project_id=? AND kind=? AND version=? AND state='ready'`), applied, row.id, row.tenant, row.project, row.kind, row.version)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return conflict("预览已被其他操作采用，请刷新")
	}
	return nil
}

func previewExpiryReason(clock time.Time, expires string) string {
	deadline, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !clock.Before(deadline) {
		return "预览已过期，请重新生成"
	}
	return ""
}
