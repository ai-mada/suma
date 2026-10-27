package dag

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

// Pool routes and resources use the same current scheduling contract as private work.
var poolSchema = []string{
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_guards(pool_id VARCHAR(100) PRIMARY KEY,version BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_policies(pool_id VARCHAR(100) PRIMARY KEY,version INTEGER NOT NULL,concurrency_limit BIGINT NOT NULL,resource_limits TEXT NOT NULL,budget_limit BIGINT,budget_spent BIGINT NOT NULL,tenant_weights TEXT NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_scopes(pool_id VARCHAR(100) NOT NULL,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,grant_id VARCHAR(100) NOT NULL,grant_version INTEGER NOT NULL,project_weight INTEGER NOT NULL,status VARCHAR(20) NOT NULL,PRIMARY KEY(pool_id,tenant_id,project_id))`,
	`CREATE TABLE IF NOT EXISTS dag_project_execution_routes(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,version INTEGER NOT NULL,pool_id VARCHAR(100) NOT NULL,grant_id VARCHAR(100) NOT NULL,grant_version INTEGER NOT NULL,PRIMARY KEY(tenant_id,project_id))`,
	`CREATE TABLE IF NOT EXISTS dag_run_execution_routes(run_id VARCHAR(100) PRIMARY KEY,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,route_version INTEGER NOT NULL,pool_id VARCHAR(100) NOT NULL,grant_id VARCHAR(100) NOT NULL,grant_version INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_accounts(pool_id VARCHAR(100) NOT NULL,name VARCHAR(100) NOT NULL,capacity BIGINT,used BIGINT NOT NULL,consumed BIGINT NOT NULL,version BIGINT NOT NULL,release_class VARCHAR(20) NOT NULL,PRIMARY KEY(pool_id,name))`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_ledger(pool_id VARCHAR(100) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,name VARCHAR(100) NOT NULL,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,grant_id VARCHAR(100) NOT NULL,grant_version INTEGER NOT NULL,amount BIGINT NOT NULL,release_class VARCHAR(20) NOT NULL,state VARCHAR(20) NOT NULL,PRIMARY KEY(run_id,node_id,attempt,fence,name))`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_fairness(pool_id VARCHAR(100) NOT NULL,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,pass BIGINT NOT NULL,remainder BIGINT NOT NULL,weight INTEGER NOT NULL,floor BIGINT NOT NULL,epoch BIGINT NOT NULL,last_sequence BIGINT NOT NULL,PRIMARY KEY(pool_id,tenant_id,project_id))`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_dispatches(pool_id VARCHAR(100) NOT NULL,runner_id VARCHAR(100) NOT NULL,operation_id VARCHAR(128) NOT NULL,dispatch_id VARCHAR(100) NOT NULL UNIQUE,request_sha256 VARCHAR(64) NOT NULL,credential_id VARCHAR(100) NOT NULL,credential_version INTEGER NOT NULL,assignment TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,PRIMARY KEY(pool_id,runner_id,operation_id))`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_slots(pool_id VARCHAR(100) NOT NULL,runner_id VARCHAR(100) NOT NULL,slot_id VARCHAR(100) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,PRIMARY KEY(pool_id,runner_id,slot_id))`,
	`CREATE TABLE IF NOT EXISTS dag_execution_pool_scans(pool_id VARCHAR(100) NOT NULL,runner_id VARCHAR(100) NOT NULL,request_scope_sha256 VARCHAR(64) NOT NULL,last_scope VARCHAR(250) NOT NULL,PRIMARY KEY(pool_id,runner_id,request_scope_sha256))`,
	`CREATE TABLE IF NOT EXISTS dag_ready_admission_order(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,rank_at BIGINT NOT NULL,ready_sequence BIGINT NOT NULL,priority INTEGER NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_claim_ready_scans(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,worker_id VARCHAR(100) NOT NULL,capability_sha256 VARCHAR(64) NOT NULL,last_turn BIGINT NOT NULL,last_head BIGINT NOT NULL,last_run_id VARCHAR(100) NOT NULL,last_node_rank BIGINT NOT NULL,last_node_sequence BIGINT NOT NULL,last_node_id VARCHAR(150) NOT NULL,PRIMARY KEY(tenant_id,project_id,worker_id,capability_sha256))`,
}

func (e *Engine) initializePoolSchema(ctx context.Context) error {
	for _, ddl := range poolSchema {
		if e.dialect == "oceanbase" {
			ddl = strings.ReplaceAll(ddl, " TEXT", " LONGTEXT")
		}
		if _, err := e.db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("pool schema: %w", err)
		}
	}
	for _, x := range []struct {
		name, table string
		columns     []string
	}{
		{"dag_pool_route_scope", "dag_run_execution_routes", []string{"pool_id", "tenant_id", "project_id", "run_id"}},
		{"dag_pool_ledger_active", "dag_execution_pool_ledger", []string{"pool_id", "state", "name"}},
		{"dag_ready_admission_order_index", "dag_ready_admission_order", []string{"run_id", "rank_at", "ready_sequence", "node_id"}},
	} {
		if err := store.EnsureIndex(ctx, e.db, e.dialect, x.name, x.table, x.columns...); err != nil {
			return err
		}
	}
	return nil
}

// guardPoolTx precedes every project/family lock that may touch pool resources.
// It is deliberately separate from the final global event-tail lock.
func (e *Engine) guardPoolTx(ctx context.Context, tx *sql.Tx, pool string) error {
	if !identifier.MatchString(pool) {
		return dagDiagnostic(fmt.Errorf("%w: pool identifier", ErrInvalid), "dag.pool.identity", nil)
	}
	q := `INSERT INTO dag_execution_pool_guards(pool_id,version)VALUES(?,0) ON CONFLICT(pool_id) DO NOTHING`
	if e.dialect == "oceanbase" {
		q = `INSERT IGNORE INTO dag_execution_pool_guards(pool_id,version)VALUES(?,0)`
	}
	if _, err := tx.ExecContext(ctx, e.q(q), pool); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, e.q(`UPDATE dag_execution_pool_guards SET version=version WHERE pool_id=?`), pool)
	return err
}
