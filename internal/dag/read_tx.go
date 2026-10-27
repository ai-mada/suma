package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// Transaction readers use only the supplied authorization transaction. They do
// not own commit/rollback, advance execution, or acquire scheduler write locks.
func (e *Engine) GetWorkflowTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, version int) (Workflow, error) {
	if tx == nil {
		return Workflow{}, ErrInvalid
	}
	return e.getWorkflow(ctx, tx, tenant, project, id, version)
}

func (e *Engine) WorkflowsTx(ctx context.Context, tx *sql.Tx, tenant, project string) ([]Workflow, error) {
	if tx == nil {
		return nil, ErrInvalid
	}
	return e.workflows(ctx, tx, tenant, project)
}

func (e *Engine) GetRunTx(ctx context.Context, tx *sql.Tx, tenant, project, id string) (Run, error) {
	if tx == nil {
		return Run{}, ErrInvalid
	}
	r, err := load(ctx, tx, e, tenant, project, id)
	if err != nil {
		return Run{}, err
	}
	return r.public(), nil
}

func (e *Engine) ListRunsPageTx(ctx context.Context, tx *sql.Tx, tenant, project string, limit int, cursor string) ([]Run, int, string, error) {
	if tx == nil {
		return nil, 0, "", ErrInvalid
	}
	return e.listRunsPage(ctx, tx, tenant, project, limit, cursor)
}

func (e *Engine) EventsTx(ctx context.Context, tx *sql.Tx, tenant, project, id string, after int64) ([]Event, error) {
	if tx == nil {
		return nil, ErrInvalid
	}
	return e.events(ctx, tx, tenant, project, id, after)
}

func (e *Engine) getWorkflow(ctx context.Context, reader queryer, tenant, project, id string, version int) (Workflow, error) {
	var w Workflow
	var raw string
	q := "SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=?"
	args := []any{tenant, project, id}
	if version > 0 {
		q += " AND version=?"
		args = append(args, version)
	}
	q += " ORDER BY version DESC LIMIT 1"
	err := reader.QueryRowContext(ctx, e.q(q), args...).Scan(&raw)
	if err == sql.ErrNoRows {
		return w, ErrNotFound
	}
	if err != nil {
		return w, err
	}
	err = json.Unmarshal([]byte(raw), &w)
	return w, err
}

var schema = []string{
	`CREATE TABLE IF NOT EXISTS dag_lock (id INTEGER PRIMARY KEY, seq BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_workflows (tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,id VARCHAR(100) NOT NULL,version INTEGER NOT NULL,name VARCHAR(255) NOT NULL,spec TEXT NOT NULL,PRIMARY KEY(tenant_id,project_id,id,version))`,
	`CREATE TABLE IF NOT EXISTS dag_managed_workflows (tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,workflow_id VARCHAR(100) NOT NULL,run_id VARCHAR(100) NOT NULL UNIQUE,PRIMARY KEY(tenant_id,project_id,workflow_id))`,
	`CREATE TABLE IF NOT EXISTS dag_managed_resolutions (run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,operation_id VARCHAR(150) NOT NULL,original_result TEXT NOT NULL,resolution_result TEXT NOT NULL,resolved_at VARCHAR(40) NOT NULL,PRIMARY KEY(run_id,node_id,attempt))`,
	`CREATE TABLE IF NOT EXISTS dag_runs (id VARCHAR(100) PRIMARY KEY,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,workflow_id VARCHAR(100) NOT NULL,workflow_version INTEGER NOT NULL,workflow_name VARCHAR(255) NOT NULL,version INTEGER NOT NULL,state VARCHAR(32) NOT NULL,created_at VARCHAR(40) NOT NULL,updated_at VARCHAR(40) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_nodes (run_id VARCHAR(100) NOT NULL,id VARCHAR(150) NOT NULL,state VARCHAR(32) NOT NULL,reason TEXT NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,output TEXT NOT NULL,error_text TEXT NOT NULL,started_at VARCHAR(40) NOT NULL,completed_at VARCHAR(40) NOT NULL,lease_until BIGINT NOT NULL,wake_at BIGINT NOT NULL,parent_id VARCHAR(150) NOT NULL,spec TEXT NOT NULL,PRIMARY KEY(run_id,id))`,
	`CREATE TABLE IF NOT EXISTS dag_attempts (run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,worker_id VARCHAR(100) NOT NULL,state VARCHAR(32) NOT NULL,started_at VARCHAR(40) NOT NULL,completed_at VARCHAR(40) NOT NULL,result TEXT NOT NULL,PRIMARY KEY(run_id,node_id,attempt))`,
	`CREATE TABLE IF NOT EXISTS dag_candidate_revisions (run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,commit_id VARCHAR(64) NOT NULL,source_node VARCHAR(150) NOT NULL,source_attempt INTEGER NOT NULL,source_fence BIGINT NOT NULL,evidence_sha256 VARCHAR(64) NOT NULL,PRIMARY KEY(run_id,node_id,attempt))`,
	`CREATE TABLE IF NOT EXISTS dag_events (sequence BIGINT PRIMARY KEY,tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,event_type VARCHAR(64) NOT NULL,at_time VARCHAR(40) NOT NULL,data TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_run_fairness (run_id VARCHAR(100) PRIMARY KEY,last_claim_sequence BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_operations (tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,scope VARCHAR(150) NOT NULL,operation_id VARCHAR(150) NOT NULL,fingerprint VARCHAR(64) NOT NULL,result_id VARCHAR(150) NOT NULL,PRIMARY KEY(tenant_id,project_id,scope,operation_id))`,
	`CREATE TABLE IF NOT EXISTS dag_subruns (parent_run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,child_run_id VARCHAR(100) NOT NULL UNIQUE,paused_by_parent INTEGER NOT NULL,PRIMARY KEY(parent_run_id,node_id,attempt))`,
	`CREATE TABLE IF NOT EXISTS dag_compensations (original_run_id VARCHAR(100) PRIMARY KEY,compensation_run_id VARCHAR(100) NOT NULL UNIQUE,original_version INTEGER NOT NULL,original_state VARCHAR(32) NOT NULL,reason TEXT NOT NULL,last_state VARCHAR(32) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_compensation_effects (original_run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,compensation_node_id VARCHAR(100) NOT NULL,source_ref TEXT NOT NULL,PRIMARY KEY(original_run_id,node_id,attempt))`,
}

func (e *Engine) workflows(ctx context.Context, reader queryer, tenant, project string) ([]Workflow, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, err
	}
	rows, err := reader.QueryContext(ctx, e.q("SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND NOT EXISTS (SELECT 1 FROM dag_managed_workflows m WHERE m.tenant_id=w.tenant_id AND m.project_id=w.project_id AND m.workflow_id=w.id) AND NOT EXISTS (SELECT 1 FROM dag_compensations c JOIN dag_runs r ON r.id=c.compensation_run_id WHERE r.tenant_id=w.tenant_id AND r.project_id=w.project_id AND r.workflow_id=w.id) ORDER BY w.id,w.version"), tenant, project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workflow{}
	for rows.Next() {
		var raw string
		var w Workflow
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
func (e *Engine) events(ctx context.Context, reader queryer, tenant, project, id string, after int64) ([]Event, error) {
	if after < 0 {
		return nil, dagDiagnostic(fmt.Errorf("%w: negative event cursor", ErrInvalid), "dag.events.cursor", nil)
	}
	var exists int
	if err := reader.QueryRowContext(ctx, e.q(`SELECT 1 FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`), tenant, project, id).Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	rows, err := reader.QueryContext(ctx, e.q("SELECT sequence,run_id,node_id,event_type,at_time,data FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND sequence>? ORDER BY sequence LIMIT 1000"), tenant, project, id, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var event Event
		var raw string
		if err = rows.Scan(&event.Sequence, &event.RunID, &event.NodeID, &event.Type, &event.At, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &event.Data); err != nil {
			return nil, err
		}
		if event.Data == nil {
			event.Data = map[string]any{}
		}
		out = append(out, event)
	}
	return out, rows.Err()
}
