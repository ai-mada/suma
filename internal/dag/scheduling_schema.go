package dag

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ai-mada/suma/internal/store"
)

const maxSchedulingInteger int64 = 9007199254740991

var schedulingSchema = []string{
	`CREATE TABLE IF NOT EXISTS dag_kernel_migrations(version INTEGER PRIMARY KEY,state VARCHAR(30) NOT NULL,completed_at VARCHAR(40) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_kernel_migration_lock(id INTEGER PRIMARY KEY,version BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_scheduler_projects(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,queue_sequence BIGINT NOT NULL,PRIMARY KEY(tenant_id,project_id))`,
	`CREATE TABLE IF NOT EXISTS dag_run_families(run_id VARCHAR(100) PRIMARY KEY,root_run_id VARCHAR(100) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_family_locks(root_run_id VARCHAR(100) PRIMARY KEY,version BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_scheduling_policies(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,version INTEGER NOT NULL,concurrency_limit BIGINT NOT NULL,resource_limits TEXT NOT NULL,budget_limit BIGINT,budget_spent BIGINT NOT NULL,budget_history VARCHAR(20) NOT NULL,updated_at VARCHAR(40) NOT NULL,PRIMARY KEY(tenant_id,project_id))`,
	`CREATE TABLE IF NOT EXISTS dag_resource_accounts(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,name VARCHAR(100) NOT NULL,capacity BIGINT,used BIGINT NOT NULL,consumed BIGINT NOT NULL,version BIGINT NOT NULL,release_class VARCHAR(20) NOT NULL,PRIMARY KEY(tenant_id,project_id,name))`,
	`CREATE TABLE IF NOT EXISTS dag_resource_ledger(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,name VARCHAR(100) NOT NULL,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,amount BIGINT NOT NULL,release_class VARCHAR(20) NOT NULL,state VARCHAR(20) NOT NULL,PRIMARY KEY(run_id,node_id,attempt,fence,name))`,
	`CREATE TABLE IF NOT EXISTS dag_graph_edges(run_id VARCHAR(100) NOT NULL,source_id VARCHAR(150) NOT NULL,target_id VARCHAR(150) NOT NULL,condition_value TEXT NOT NULL,PRIMARY KEY(run_id,source_id,target_id))`,
	`CREATE TABLE IF NOT EXISTS dag_condition_watchers(run_id VARCHAR(100) NOT NULL,source_id VARCHAR(150) NOT NULL,target_id VARCHAR(150) NOT NULL,PRIMARY KEY(run_id,source_id,target_id))`,
	`CREATE TABLE IF NOT EXISTS dag_node_dependencies(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,generation INTEGER NOT NULL,total INTEGER NOT NULL,resolved INTEGER NOT NULL,selected INTEGER NOT NULL,success INTEGER NOT NULL,failed INTEGER NOT NULL,winners TEXT NOT NULL,decision_at VARCHAR(40) NOT NULL,ready_at VARCHAR(40) NOT NULL,version BIGINT NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_edge_resolutions(run_id VARCHAR(100) NOT NULL,target_id VARCHAR(150) NOT NULL,generation INTEGER NOT NULL,source_id VARCHAR(150) NOT NULL,source_attempt INTEGER NOT NULL,source_fence BIGINT NOT NULL,resolved INTEGER NOT NULL,selected INTEGER NOT NULL,success INTEGER NOT NULL,failed INTEGER NOT NULL,resolution_order BIGINT NOT NULL,PRIMARY KEY(run_id,target_id,generation,source_id))`,
	`CREATE TABLE IF NOT EXISTS dag_run_counters(run_id VARCHAR(100) PRIMARY KEY,total INTEGER NOT NULL,terminal_count INTEGER NOT NULL,failed_count INTEGER NOT NULL,cancelled_count INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_run_generations(run_id VARCHAR(100) PRIMARY KEY,kernel_generation INTEGER NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_node_work(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,sequence BIGINT NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_join_decisions(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,generation INTEGER NOT NULL,winners TEXT NOT NULL,decided_at VARCHAR(40) NOT NULL,PRIMARY KEY(run_id,node_id,generation))`,
	`CREATE TABLE IF NOT EXISTS dag_retry_windows(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,generation INTEGER NOT NULL,base_attempt INTEGER NOT NULL,started_at BIGINT NOT NULL,deadline_at BIGINT NOT NULL,PRIMARY KEY(run_id,node_id,generation))`,
	`CREATE TABLE IF NOT EXISTS dag_map_progress(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,total INTEGER NOT NULL,terminal_count INTEGER NOT NULL,failed_count INTEGER NOT NULL,closed INTEGER NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_map_child_states(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,parent_id VARCHAR(150) NOT NULL,terminal_flag INTEGER NOT NULL,failed_flag INTEGER NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_ready_v2(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,generation INTEGER NOT NULL,sequence BIGINT NOT NULL,ready_at VARCHAR(40) NOT NULL,node_type VARCHAR(40) NOT NULL,command_id VARCHAR(100) NOT NULL,required_capability VARCHAR(200) NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_wakeups(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,kind VARCHAR(30) NOT NULL,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,generation INTEGER NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,due_at BIGINT NOT NULL,state VARCHAR(20) NOT NULL,PRIMARY KEY(run_id,node_id,kind))`,
	`CREATE TABLE IF NOT EXISTS dag_map_expansions(run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,generation INTEGER NOT NULL,input_sha256 VARCHAR(64) NOT NULL,template_sha256 VARCHAR(64) NOT NULL,items TEXT NOT NULL,template TEXT NOT NULL,total INTEGER NOT NULL,next_index INTEGER NOT NULL,state VARCHAR(20) NOT NULL,PRIMARY KEY(run_id,node_id,generation))`,
	`CREATE TABLE IF NOT EXISTS dag_map_children(run_id VARCHAR(100) NOT NULL,map_node_id VARCHAR(150) NOT NULL,child_node_id VARCHAR(150) NOT NULL,PRIMARY KEY(run_id,map_node_id,child_node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_claim_scans(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,worker_id VARCHAR(100) NOT NULL,capability_sha256 VARCHAR(64) NOT NULL,last_fair_sequence BIGINT NOT NULL,last_ready_sequence BIGINT NOT NULL,last_run_id VARCHAR(100) NOT NULL,last_node_id VARCHAR(150) NOT NULL,PRIMARY KEY(tenant_id,project_id,worker_id,capability_sha256))`,
	`CREATE TABLE IF NOT EXISTS dag_scheduler_run_turns(run_id VARCHAR(100) PRIMARY KEY,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,enabled INTEGER NOT NULL,last_sequence BIGINT NOT NULL,first_ready_sequence BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_claim_run_scans(tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,worker_id VARCHAR(100) NOT NULL,capability_sha256 VARCHAR(64) NOT NULL,last_turn BIGINT NOT NULL,last_head BIGINT NOT NULL,last_run_id VARCHAR(100) NOT NULL,last_node_sequence BIGINT NOT NULL,last_node_id VARCHAR(150) NOT NULL,PRIMARY KEY(tenant_id,project_id,worker_id,capability_sha256))`,
	`CREATE TABLE IF NOT EXISTS dag_dispatches(id VARCHAR(100) PRIMARY KEY,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,operation_id VARCHAR(150) NOT NULL,request_sha256 VARCHAR(64) NOT NULL,worker_id VARCHAR(100) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,policy_version INTEGER NOT NULL,assignment TEXT NOT NULL,created_at VARCHAR(40) NOT NULL,UNIQUE(tenant_id,project_id,operation_id))`,
}

func (e *Engine) migrateSchedulingSchema(ctx context.Context) error {
	for _, ddl := range schedulingSchema {
		if e.dialect == "oceanbase" {
			ddl = strings.ReplaceAll(ddl, " TEXT", " LONGTEXT")
		}
		if _, err := e.db.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("scheduling schema: %w", err)
		}
	}
	for _, x := range []struct {
		name, table string
		columns     []string
	}{
		{"dag_runs_scope_state", "dag_runs", []string{"tenant_id", "project_id", "state", "id"}},
		{"dag_graph_target", "dag_graph_edges", []string{"run_id", "target_id", "source_id"}},
		{"dag_ready_scope_order", "dag_ready_v2", []string{"tenant_id", "project_id", "sequence", "run_id", "node_id"}},
		{"dag_ready_command_order", "dag_ready_v2", []string{"tenant_id", "project_id", "command_id", "sequence", "run_id", "node_id"}},
		{"dag_wake_due_complete", "dag_wakeups", []string{"state", "due_at", "run_id", "node_id", "kind"}},
		{"dag_wake_scope", "dag_wakeups", []string{"tenant_id", "project_id", "run_id", "state", "due_at"}},
		{"dag_resource_active", "dag_resource_ledger", []string{"tenant_id", "project_id", "name", "state"}},
		{"dag_dispatch_run_attempt", "dag_dispatches", []string{"run_id", "node_id", "attempt", "fence"}},
		{"dag_node_work_order", "dag_node_work", []string{"run_id", "sequence", "node_id"}},
		{"dag_ready_run_order", "dag_ready_v2", []string{"run_id", "sequence", "node_id"}},
		{"dag_turn_scope_order", "dag_scheduler_run_turns", []string{"tenant_id", "project_id", "enabled", "last_sequence", "first_ready_sequence", "run_id"}},
	} {
		if err := store.EnsureIndex(ctx, e.db, e.dialect, x.name, x.table, x.columns...); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) clockTx(ctx context.Context, tx *sql.Tx) (int64, error) {
	query := "SELECT unixepoch('now')"
	if e.dialect == "postgres" {
		query = "SELECT CAST(EXTRACT(EPOCH FROM clock_timestamp()) AS BIGINT)"
	}
	if e.dialect == "oceanbase" {
		query = "SELECT UNIX_TIMESTAMP()"
	}
	var now int64
	err := tx.QueryRowContext(ctx, query).Scan(&now)
	return now, err
}

func (e *Engine) projectGuardTx(ctx context.Context, tx *sql.Tx, tenant, project string) error {
	if err := scopeValid(tenant, project); err != nil {
		return err
	}
	query := `INSERT INTO dag_scheduler_projects(tenant_id,project_id,queue_sequence)VALUES(?,?,0) ON CONFLICT(tenant_id,project_id) DO NOTHING`
	if e.dialect == "oceanbase" {
		query = `INSERT IGNORE INTO dag_scheduler_projects(tenant_id,project_id,queue_sequence)VALUES(?,?,0)`
	}
	if _, err := tx.ExecContext(ctx, e.q(query), tenant, project); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, e.q(`UPDATE dag_scheduler_projects SET queue_sequence=queue_sequence WHERE tenant_id=? AND project_id=?`), tenant, project)
	return err
}

// GuardProjectTx joins the scheduler's project mutation lock in an existing
// transaction. Call it after current organization/credential authorization and
// before reading execution facts or taking a run-family lock. It never creates
// an Engine, migrates schema, commits, or acquires the global event-tail lock.
func GuardProjectTx(ctx context.Context, tx *sql.Tx, dialect, tenant, project string) error {
	if dialect != "sqlite" && dialect != "oceanbase" && dialect != "postgres" {
		return dagDiagnostic(fmt.Errorf("%w: database dialect", ErrUnsupported), "dag.storage.dialect", nil)
	}
	return (&Engine{dialect: dialect}).projectGuardTx(ctx, tx, tenant, project)
}
