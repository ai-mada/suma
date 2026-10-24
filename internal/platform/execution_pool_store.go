package platform

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

// MigrateExecutionPools stores ownership and delegation separately from the
// scheduler's resource ledger. No machine credential is a human session.
func (s *Service) MigrateExecutionPools(ctx context.Context) error {
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

// poolHumanWriteTx rechecks current organization membership and exact browser
// credential before even recovering an operation receipt. It must run before
// taking any pool/project scheduler locks.
func (s *Service) poolHumanWriteTx(ctx context.Context, tx *sql.Tx, p Principal, token string) error {
	if err := s.lockOrganization(ctx, tx, p.TenantID); err != nil {
		return err
	}
	live, err := s.liveOrganization(ctx, tx, p)
	if err != nil {
		return err
	}
	if !orgAdmin(live) {
		return forbidden()
	}
	if err = s.currentSchedulingSessionTx(ctx, tx, live, token); err != nil {
		return err
	}
	var label string
	if err = tx.QueryRowContext(ctx, s.q(`SELECT label FROM suma_sessions WHERE token_hash=?`), digest(token)).Scan(&label); err != nil || label != "browser" {
		return forbidden()
	}
	return nil
}

func (s *Service) poolAdminTx(ctx context.Context, tx *sql.Tx, p Principal, token, poolID string) (ExecutionPool, error) {
	var out ExecutionPool
	if err := s.poolHumanWriteTx(ctx, tx, p, token); err != nil {
		return out, err
	}
	out, err := s.executionPoolMetadataTx(ctx, tx, poolID)
	if err != nil {
		return out, err
	}
	if out.OwnerTenantID != p.TenantID {
		return ExecutionPool{}, missing()
	}
	return out, nil
}

// A read transaction verifies its own credential and role snapshot; it never
// acquires the writer locks used by admission or authorizes a later mutation.
func (s *Service) poolReadTx(ctx context.Context, p Principal, token string) (*sql.Tx, error) {
	return s.poolBrowserReadTx(ctx, p, token, true)
}

func (s *Service) poolBrowserReadTx(ctx context.Context, p Principal, token string, organizationAdmin bool) (*sql.Tx, error) {
	options := &sql.TxOptions{ReadOnly: true}
	if s.Dialect != "sqlite" {
		options.Isolation = sql.LevelRepeatableRead
	}
	tx, err := s.DB.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	valid := false
	defer func() {
		if !valid {
			_ = tx.Rollback()
		}
	}()
	live, err := s.liveOrganization(ctx, tx, p)
	if err != nil {
		return nil, err
	}
	if organizationAdmin && !orgAdmin(live) {
		return nil, forbidden()
	}
	var user, tenant, label, expires string
	var scoped int
	if err = tx.QueryRowContext(ctx, s.q(`SELECT user_id,tenant_id,label,expires_at FROM suma_sessions WHERE token_hash=?`), digest(token)).Scan(&user, &tenant, &label, &expires); err != nil {
		return nil, forbidden()
	}
	if err = tx.QueryRowContext(ctx, s.q(`SELECT COUNT(*) FROM suma_token_scopes WHERE token_hash=?`), digest(token)).Scan(&scoped); err != nil {
		return nil, err
	}
	clock, err := s.automationClock(ctx, tx)
	if err != nil {
		return nil, err
	}
	expiry, parseErr := time.Parse(time.RFC3339Nano, expires)
	if user != p.ID || tenant != p.TenantID || label != "browser" || scoped != 0 || parseErr != nil || expiry.Unix() <= clock.Unix() {
		return nil, forbidden()
	}
	valid = true
	return tx, nil
}
