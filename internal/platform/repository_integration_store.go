package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

const (
	repositoryInvalid     = "Invalid repository integration request"
	repositoryChanged     = "Repository integration version changed; refresh the original plan"
	repositoryBasis       = "Accepted task, review, tests or sealed candidate basis changed"
	repositoryTarget      = "Repository target binding or observed Git state changed"
	repositoryBusy        = "Repository target has an unresolved operation; reconcile the original operation"
	repositoryApproval    = "Current browser approval of the exact prepared diff is required"
	repositoryAcknowledge = "Explicit repository integration acknowledgement is required"
	repositoryProvider    = "Repository integration provider is unavailable or not configured"
	repositoryUnknown     = "Repository effect is unknown; inspect and reconcile the original operation"
	repositoryObservation = "Repository provider observation does not match the fixed operation"
	repositoryExpired     = "Repository integration plan or authorization expired"
)

func repositoryError(status int, code, message string) error {
	return &APIError{status, "repository_integration_" + code, message, nil}
}
func repositoryConflict(code, message string) error { return repositoryError(409, code, message) }

func (s *Service) InitializeRepositoryIntegration(ctx context.Context) error {
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS suma_repository_targets(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,data TEXT NOT NULL,UNIQUE(tenant_id,project_id,id))`,
		`CREATE TABLE IF NOT EXISTS suma_repository_plans(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,task_id VARCHAR(64) NOT NULL,target_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,created_at VARCHAR(40) NOT NULL,data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_repository_approvals(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,data TEXT NOT NULL,UNIQUE(plan_id,version))`,
		`CREATE TABLE IF NOT EXISTS suma_repository_operations(operation_id VARCHAR(128) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,plan_id VARCHAR(64) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,node_id VARCHAR(150) NOT NULL,version INTEGER NOT NULL,data TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_repository_authorizations(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,ticket_hash VARCHAR(64) NOT NULL UNIQUE,expires_at VARCHAR(40) NOT NULL,state VARCHAR(20) NOT NULL,assignment TEXT NOT NULL,request_sha256 VARCHAR(64) NOT NULL,grant_data TEXT NOT NULL,effect_grant_id VARCHAR(64) NOT NULL,worker_token_hash VARCHAR(64) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_repository_observations(tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,version INTEGER NOT NULL,sha256 VARCHAR(64) NOT NULL,data TEXT NOT NULL,PRIMARY KEY(operation_id,version))`,
		`CREATE TABLE IF NOT EXISTS suma_repository_reconciliation_grants(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,operation_id VARCHAR(128) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,ticket_hash VARCHAR(64) NOT NULL UNIQUE,expires_at VARCHAR(40) NOT NULL,state VARCHAR(20) NOT NULL,actor_credential VARCHAR(64) NOT NULL,data TEXT NOT NULL)`,
	} {
		if _, err := s.DB.ExecContext(ctx, s.ddl(ddl)); err != nil {
			return err
		}
	}
	for _, index := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_repository_target_scope", "suma_repository_targets", []string{"tenant_id", "project_id", "id"}},
		{"suma_repository_plan_scope", "suma_repository_plans", []string{"tenant_id", "project_id", "task_id", "created_at", "id"}},
		{"suma_repository_operation_plan", "suma_repository_operations", []string{"tenant_id", "project_id", "plan_id"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, index.table, index.columns...); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) repositoryTargetTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out RepositoryIntegrationTarget, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT data FROM suma_repository_targets WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &out)
	}
	return
}
func (s *Service) repositoryPlanTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out RepositoryIntegrationPlan, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT data FROM suma_repository_plans WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &out)
	}
	return
}
func (s *Service) repositoryOperationTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (out repositoryOperation, err error) {
	var raw string
	err = tx.QueryRowContext(ctx, s.q(`SELECT data FROM suma_repository_operations WHERE tenant_id=? AND project_id=? AND operation_id=?`), tenant, project, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err == nil {
		err = json.Unmarshal([]byte(raw), &out)
	}
	return
}
func (s *Service) saveRepositoryTargetTx(ctx context.Context, tx *sql.Tx, tenant string, old int, out RepositoryIntegrationTarget) error {
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_repository_targets SET version=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, stringify(out), tenant, out.ProjectID, out.ID, old)
	if err != nil {
		return err
	}
	if n, e := res.RowsAffected(); e != nil {
		return e
	} else if n != 1 {
		return repositoryConflict("changed", repositoryChanged)
	}
	return nil
}
func (s *Service) saveRepositoryPlanTx(ctx context.Context, tx *sql.Tx, tenant string, old int, out RepositoryIntegrationPlan) error {
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_repository_plans SET version=?,data=? WHERE tenant_id=? AND project_id=? AND id=? AND version=?`), out.Version, stringify(out), tenant, out.ProjectID, out.ID, old)
	if err != nil {
		return err
	}
	if n, e := res.RowsAffected(); e != nil {
		return e
	} else if n != 1 {
		return repositoryConflict("changed", repositoryChanged)
	}
	return nil
}
func (s *Service) saveRepositoryOperationTx(ctx context.Context, tx *sql.Tx, tenant, project string, old int, out repositoryOperation) error {
	res, err := tx.ExecContext(ctx, s.q(`UPDATE suma_repository_operations SET version=?,data=? WHERE tenant_id=? AND project_id=? AND operation_id=? AND version=?`), out.Version, stringify(out), tenant, project, out.Execution.OperationID, old)
	if err != nil {
		return err
	}
	if n, e := res.RowsAffected(); e != nil {
		return e
	} else if n != 1 {
		return repositoryConflict("changed", repositoryChanged)
	}
	return nil
}
