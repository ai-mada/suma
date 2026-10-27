package platform

import (
	"context"
	"database/sql"
	"errors"

	"github.com/ai-mada/suma/internal/store"
)

// InitializeExecutionPools stores ownership and delegation separately from the
// scheduler's resource ledger. No machine credential is a human session.
func (s *Service) InitializeExecutionPools(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS suma_execution_pools(id VARCHAR(64) PRIMARY KEY,owner_tenant_id VARCHAR(64) NOT NULL,name VARCHAR(200) NOT NULL,description TEXT NOT NULL,version INTEGER NOT NULL,status VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_execution_pool_grants(id VARCHAR(64) PRIMARY KEY,pool_id VARCHAR(64) NOT NULL,owner_tenant_id VARCHAR(64) NOT NULL,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,shared_project_name VARCHAR(500) NOT NULL,version INTEGER NOT NULL,status VARCHAR(20) NOT NULL,project_weight INTEGER NOT NULL,request_sha256 VARCHAR(64) NOT NULL,requested_by VARCHAR(64) NOT NULL,decided_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_execution_pool_runners(id VARCHAR(64) PRIMARY KEY,pool_id VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL UNIQUE,name VARCHAR(200) NOT NULL,version INTEGER NOT NULL,status VARCHAR(20) NOT NULL,max_slots INTEGER NOT NULL,credential_version INTEGER NOT NULL,last_seen_at VARCHAR(40) NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_execution_pool_credentials(id VARCHAR(64) PRIMARY KEY,runner_id VARCHAR(64) NOT NULL UNIQUE,version INTEGER NOT NULL,token_hash VARCHAR(64) NOT NULL UNIQUE,status VARCHAR(20) NOT NULL,expires_at VARCHAR(40) NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS suma_execution_pool_previews(id VARCHAR(64) PRIMARY KEY,tenant_id VARCHAR(64) NOT NULL,project_id VARCHAR(64) NOT NULL,pool_id VARCHAR(64) NOT NULL,kind VARCHAR(20) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,applied_version INTEGER NOT NULL,created_by VARCHAR(64) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,payload TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := s.DB.ExecContext(ctx, s.ddl(statement)); err != nil {
			return err
		}
	}
	for _, index := range []struct {
		name, table string
		columns     []string
	}{
		{"suma_pool_owner_seek", "suma_execution_pools", []string{"owner_tenant_id", "id"}},
		{"suma_pool_grant_inbox", "suma_execution_pool_grants", []string{"pool_id", "status", "id"}},
		{"suma_pool_grant_project", "suma_execution_pool_grants", []string{"tenant_id", "project_id", "pool_id", "status", "id"}},
		{"suma_pool_runner_seek", "suma_execution_pool_runners", []string{"pool_id", "id"}},
		{"suma_pool_preview_quota", "suma_execution_pool_previews", []string{"tenant_id", "project_id", "pool_id", "kind", "state", "expires_at"}},
	} {
		if err := store.EnsureIndex(ctx, s.DB, s.Dialect, index.name, index.table, index.columns...); err != nil {
			return err
		}
	}
	return nil
}

const executionPoolColumns = `id,owner_tenant_id,name,description,version,status,created_at,updated_at`

func scanExecutionPool(row interface{ Scan(...any) error }) (ExecutionPool, error) {
	var out ExecutionPool
	err := row.Scan(&out.ID, &out.OwnerTenantID, &out.Name, &out.Description, &out.Version, &out.Status, &out.CreatedAt, &out.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	out.AllowedActions = []string{}
	return out, err
}

func (s *Service) executionPoolMetadataTx(ctx context.Context, tx *sql.Tx, poolID string) (ExecutionPool, error) {
	return scanExecutionPool(tx.QueryRowContext(ctx, s.q(`SELECT `+executionPoolColumns+` FROM suma_execution_pools WHERE id=?`), poolID))
}

func (s *Service) currentExecutionPoolMetadataTx(ctx context.Context, tx *sql.Tx, poolID string) (ExecutionPool, error) {
	return scanExecutionPool(tx.QueryRowContext(ctx, s.q(s.currentSQL(`SELECT `+executionPoolColumns+` FROM suma_execution_pools WHERE id=?`)), poolID))
}

// poolHumanWriteTx rechecks current organization membership and exact browser
// credential before even recovering an operation receipt. It must run before
// taking any pool/project scheduler locks.
func (s *Service) poolHumanWriteTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand) error {
	if auth == nil {
		return unauthenticated()
	}
	if err := auth.assertTx(tx, auth.target, auth.purpose); err != nil {
		return err
	}
	if !auth.actor.IsBrowser() || !orgAdmin(auth.currentPrincipal) {
		return forbidden()
	}
	return nil
}
func (s *Service) poolAdminTx(ctx context.Context, tx *sql.Tx, auth *authorizedCommand, poolID string) (ExecutionPool, error) {
	var out ExecutionPool
	if err := s.poolHumanWriteTx(ctx, tx, auth); err != nil {
		return out, err
	}
	out, err := s.currentExecutionPoolMetadataTx(ctx, tx, poolID)
	if err != nil {
		return out, err
	}
	if out.OwnerTenantID != auth.currentPrincipal.TenantID {
		return ExecutionPool{}, missing()
	}
	return out, nil
}
