package platform

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/ai-mada/suma/internal/store"
	"github.com/ai-mada/suma/internal/strictjson"
	"strings"
)

const poolPairingDDL = `CREATE TABLE IF NOT EXISTS suma_pool_runner_pairings(id VARCHAR(32) PRIMARY KEY,owner_tenant_id VARCHAR(64) NOT NULL,pool_id VARCHAR(64) NOT NULL,version INTEGER NOT NULL,state VARCHAR(20) NOT NULL,created_at VARCHAR(40) NOT NULL,expires_at VARCHAR(40) NOT NULL,proposal_operation_id VARCHAR(128) NOT NULL,proposal_sha256 VARCHAR(64) NOT NULL,payload TEXT NOT NULL)`
const poolPairingColumns = `id,owner_tenant_id,pool_id,version,state,created_at,expires_at,proposal_operation_id,proposal_sha256,payload`

type poolPairingRecord struct {
	PoolRunnerPairing
	proposalOperation, proposalSHA string
}

// Existing tables must be this initialization schema. No migrations or silent
// repairs run while authenticating machines or accepting human approvals.
func (s *Service) validatePoolRunnerPairingsInitialSchema(ctx context.Context) error {
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table','view') AND name='suma_pool_runner_pairings'`
	switch s.Dialect {
	case "sqlite":
	case "postgres":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=current_schema() AND table_name='suma_pool_runner_pairings'`
	case "oceanbase":
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name='suma_pool_runner_pairings'`
	default:
		return fmt.Errorf("%w: Pool pairing dialect", store.ErrInitialSchema)
	}
	var n int
	if e := s.DB.QueryRowContext(ctx, query).Scan(&n); e != nil {
		return e
	}
	if n == 0 {
		return nil
	}
	var baseline string
	if e := s.DB.QueryRowContext(ctx, s.q(`SELECT version FROM suma_schema_baselines WHERE component='platform'`)).Scan(&baseline); e != nil || baseline != store.InitialDataContract {
		return fmt.Errorf("%w: registered Pool pairing schema required", store.ErrInitialSchema)
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT * FROM suma_pool_runner_pairings WHERE 1=0`)
	if e != nil {
		return fmt.Errorf("%w: Pool pairing columns", store.ErrInitialSchema)
	}
	columns, e := rows.Columns()
	rows.Close()
	if e != nil {
		return e
	}
	want := strings.Split(poolPairingColumns, ",")
	seen := map[string]bool{}
	for _, c := range columns {
		seen[c] = true
	}
	if len(columns) != len(want) {
		return fmt.Errorf("%w: exact Pool pairing columns required", store.ErrInitialSchema)
	}
	for _, c := range want {
		if !seen[c] {
			return fmt.Errorf("%w: Pool pairing column required", store.ErrInitialSchema)
		}
	}
	pk, _, e := s.notificationInboxSchemaKeys(ctx, "suma_pool_runner_pairings")
	if e != nil {
		return e
	}
	if pk != "id" {
		return fmt.Errorf("%w: Pool pairing primary key required", store.ErrInitialSchema)
	}
	return nil
}
func (s *Service) InitializePoolRunnerPairings(ctx context.Context) error {
	if e := s.validatePoolRunnerPairingsInitialSchema(ctx); e != nil {
		return e
	}
	if e := store.InitializeBaseline(ctx, s.DB, s.Dialect, "platform", "suma_", store.InitialDataContract); e != nil {
		return e
	}
	if _, e := s.DB.ExecContext(ctx, s.ddl(poolPairingDDL)); e != nil {
		return e
	}
	if e := store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_pool_pairing_state", "suma_pool_runner_pairings", "owner_tenant_id", "pool_id", "state", "expires_at"); e != nil {
		return e
	}
	return store.EnsureIndex(ctx, s.DB, s.Dialect, "suma_pool_pairing_history", "suma_pool_runner_pairings", "owner_tenant_id", "pool_id", "created_at", "id")
}
func scanPoolPairing(row automationScanner) (out poolPairingRecord, err error) {
	var id, owner, pool, state, created, expires, raw string
	var version int
	err = row.Scan(&id, &owner, &pool, &version, &state, &created, &expires, &out.proposalOperation, &out.proposalSHA, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, missing()
	}
	if err != nil {
		return out, err
	}
	if strictjson.Decode([]byte(raw), &out.PoolRunnerPairing, 8192) != nil || !validPoolPairing(out.PoolRunnerPairing) || out.ID != id || out.OwnerTenantID != owner || out.PoolID != pool || out.Version != version || out.State != state || out.CreatedAt != created || out.ExpiresAt != expires || out.CanApprove || out.ReasonCode != "" {
		return poolPairingRecord{}, poolPairingError(409, "changed")
	}
	if out.CredentialSHA256 == "" {
		if out.proposalOperation != "" || out.proposalSHA != "" {
			return poolPairingRecord{}, poolPairingError(409, "changed")
		}
	} else if validatePoolOperation(out.proposalOperation) != nil || !desktopWorkspaceHex(out.proposalSHA, 64) {
		return poolPairingRecord{}, poolPairingError(409, "changed")
	}
	return out, nil
}
func (s *Service) poolPairingTx(ctx context.Context, q authorityQuery, id, pool, owner string, current bool) (poolPairingRecord, error) {
	query := `SELECT ` + poolPairingColumns + ` FROM suma_pool_runner_pairings WHERE id=? AND pool_id=?`
	args := []any{id, pool}
	if owner != "" {
		query += ` AND owner_tenant_id=?`
		args = append(args, owner)
	}
	if current {
		query = s.currentSQL(query)
	}
	return scanPoolPairing(q.QueryRowContext(ctx, s.q(query), args...))
}
func (s *Service) savePoolPairingTx(ctx context.Context, tx *sql.Tx, row poolPairingRecord, before int) error {
	p := row.PoolRunnerPairing
	p.CanApprove = false
	p.ReasonCode = ""
	if !validPoolPairing(p) {
		return poolPairingError(409, "changed")
	}
	result, e := tx.ExecContext(ctx, s.q(`UPDATE suma_pool_runner_pairings SET version=?,state=?,proposal_operation_id=?,proposal_sha256=?,payload=? WHERE id=? AND owner_tenant_id=? AND pool_id=? AND version=?`), p.Version, p.State, row.proposalOperation, row.proposalSHA, stringify(p), p.ID, p.OwnerTenantID, p.PoolID, before)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e == nil && n != 1 {
		return poolPairingError(409, "changed")
	}
	return e
}
