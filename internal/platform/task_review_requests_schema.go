package platform

import (
	"context"
	"fmt"

	"github.com/ai-mada/suma/internal/store"
)

const taskReviewRequestsDDL = `CREATE TABLE IF NOT EXISTS suma_task_review_requests(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,task_version INTEGER NOT NULL,run_id VARCHAR(100) NOT NULL,run_version INTEGER NOT NULL,task_basis_sha256 VARCHAR(64) NOT NULL,requester_user_id VARCHAR(64) NOT NULL,requester_name_at_request VARCHAR(200) NOT NULL,reviewer_resource_id VARCHAR(64) NOT NULL,reviewer_user_id VARCHAR(64) NOT NULL,reviewer_name_at_request VARCHAR(500) NOT NULL,note TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL,cancelled_by_user_id VARCHAR(64) NOT NULL,cancel_reason TEXT NOT NULL,closed_review_id VARCHAR(64) NOT NULL,notification_id VARCHAR(64) NOT NULL UNIQUE)`
const taskReviewPendingDDL = `CREATE TABLE IF NOT EXISTS suma_task_review_pending(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,request_id VARCHAR(64) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,task_id))`

// This initial domain is independent of task/run/review/inbox history. Reject
// partial or incompatible existing tables before any initializer writes.
func (s *Service) validateTaskReviewRequestsInitialSchema(ctx context.Context) error {
	tables := []struct {
		name, columns, primary string
		unique                 []string
	}{
		{"suma_task_review_requests", "tenant_id," + taskReviewRequestColumns, "id", []string{"notification_id"}},
		{"suma_task_review_pending", "tenant_id,project_id,task_id,request_id", "tenant_id,project_id,task_id", []string{"request_id"}},
	}
	var existence string
	switch s.Dialect {
	case "sqlite":
		existence = `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?`
	case "postgres":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name=?`
	case "oceanbase":
		existence = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?`
	default:
		return fmt.Errorf("%w: unsupported task review requests schema dialect", store.ErrInitialSchema)
	}
	present := 0
	for _, table := range tables {
		var count int
		if err := s.DB.QueryRowContext(ctx, s.q(existence), table.name).Scan(&count); err != nil {
			return err
		}
		present += count
	}
	if present == 0 {
		return nil
	}
	if present != len(tables) {
		return fmt.Errorf("%w: complete task review requests domain is required", store.ErrInitialSchema)
	}
	for _, table := range tables {
		rows, err := s.DB.QueryContext(ctx, `SELECT `+table.columns+` FROM `+table.name+` WHERE 1=0`)
		if err != nil {
			return fmt.Errorf("%w: current %s columns are required", store.ErrInitialSchema, table.name)
		}
		if err = rows.Close(); err != nil {
			return err
		}
		primary, unique, err := s.notificationInboxSchemaKeys(ctx, table.name)
		if err != nil {
			return err
		}
		if primary != table.primary {
			return fmt.Errorf("%w: current %s primary key is required", store.ErrInitialSchema, table.name)
		}
		for _, required := range table.unique {
			if !unique[required] {
				return fmt.Errorf("%w: current %s uniqueness is required", store.ErrInitialSchema, table.name)
			}
		}
	}
	return nil
}

func (s *Service) InitializeTaskReviewRequests(ctx context.Context) error {
	if err := s.validateTaskReviewRequestsInitialSchema(ctx); err != nil {
		return err
	}
	for _, ddl := range []string{taskReviewRequestsDDL, taskReviewPendingDDL} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_task_review_history", "suma_task_review_requests", "tenant_id", "project_id", "task_id", "created_at", "id")
}
