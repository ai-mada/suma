// Package dag implements a durable DAG state machine. All transitions and their
// evidence commit together under a database scheduler lock, across processes.
package dag

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Engine struct {
	db           *sql.DB
	dialect      string
	LeaseSeconds int64
}
type transaction struct {
	e   *Engine
	tx  *sql.Tx
	ctx context.Context
	now int64
}
type nodeState struct {
	NodeRun
	spec   Node
	lease  int64
	wake   int64
	parent string
}
type runState struct {
	Run
	nodes    []*nodeState
	workflow Workflow
}
type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func New(db *sql.DB, dialect string) (*Engine, error) {
	if dialect != "sqlite" && dialect != "oceanbase" && dialect != "postgres" {
		return nil, fmt.Errorf("%w: database dialect", ErrUnsupported)
	}
	e := &Engine{db: db, dialect: dialect, LeaseSeconds: 30}
	for _, ddl := range schema {
		if dialect == "oceanbase" {
			ddl = strings.ReplaceAll(ddl, " TEXT", " LONGTEXT")
		}
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("dag migration: %w", err)
		}
	}
	insert := "INSERT INTO dag_lock (id,seq) VALUES (1,0) ON CONFLICT (id) DO NOTHING"
	if dialect == "oceanbase" {
		insert = "INSERT IGNORE INTO dag_lock (id,seq) VALUES (1,0)"
	}
	if _, err := db.Exec(insert); err != nil {
		return nil, err
	}
	return e, nil
}

// GetWorkflow reads a published immutable version; version zero selects latest.
func (e *Engine) GetWorkflow(ctx context.Context, tenant, project, id string, version int) (Workflow, error) {
	var w Workflow
	var raw string
	q := "SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=?"
	args := []any{tenant, project, id}
	if version > 0 {
		q += " AND version=?"
		args = append(args, version)
	}
	q += " ORDER BY version DESC LIMIT 1"
	err := e.db.QueryRowContext(ctx, e.q(q), args...).Scan(&raw)
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
	`CREATE TABLE IF NOT EXISTS dag_ready (run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,sequence BIGINT NOT NULL,PRIMARY KEY(run_id,node_id))`,
	`CREATE TABLE IF NOT EXISTS dag_run_fairness (run_id VARCHAR(100) PRIMARY KEY,last_claim_sequence BIGINT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_operations (tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,scope VARCHAR(150) NOT NULL,operation_id VARCHAR(150) NOT NULL,fingerprint VARCHAR(64) NOT NULL,result_id VARCHAR(150) NOT NULL,PRIMARY KEY(tenant_id,project_id,scope,operation_id))`,
	`CREATE TABLE IF NOT EXISTS dag_resource_limits (tenant_id VARCHAR(100) NOT NULL,project_id VARCHAR(100) NOT NULL,name VARCHAR(100) NOT NULL,capacity BIGINT NOT NULL,consumed BIGINT NOT NULL,PRIMARY KEY(tenant_id,project_id,name))`,
	`CREATE TABLE IF NOT EXISTS dag_reservations (run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,name VARCHAR(100) NOT NULL,amount BIGINT NOT NULL,PRIMARY KEY(run_id,node_id,name))`,
	`CREATE TABLE IF NOT EXISTS dag_subruns (parent_run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,child_run_id VARCHAR(100) NOT NULL UNIQUE,paused_by_parent INTEGER NOT NULL,PRIMARY KEY(parent_run_id,node_id,attempt))`,
	`CREATE TABLE IF NOT EXISTS dag_compensations (original_run_id VARCHAR(100) PRIMARY KEY,compensation_run_id VARCHAR(100) NOT NULL UNIQUE,original_version INTEGER NOT NULL,original_state VARCHAR(32) NOT NULL,reason TEXT NOT NULL,last_state VARCHAR(32) NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS dag_compensation_effects (original_run_id VARCHAR(100) NOT NULL,node_id VARCHAR(150) NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,compensation_node_id VARCHAR(100) NOT NULL,source_ref TEXT NOT NULL,PRIMARY KEY(original_run_id,node_id,attempt))`,
}

func (e *Engine) q(q string) string {
	if e.dialect != "postgres" {
		return q
	}
	var b strings.Builder
	n := 1
	for _, r := range q {
		if r == '?' {
			fmt.Fprintf(&b, "$%d", n)
			n++
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func (t *transaction) exec(q string, args ...any) error {
	_, err := t.tx.ExecContext(t.ctx, t.e.q(q), args...)
	return err
}
func (t *transaction) row(q string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(t.ctx, t.e.q(q), args...)
}
func (t *transaction) rows(q string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(t.ctx, t.e.q(q), args...)
}
func (e *Engine) write(ctx context.Context, fn func(*transaction) error) error {
	return e.writeBefore(ctx, nil, fn)
}
func (e *Engine) writeBefore(ctx context.Context, before func(*sql.Tx) error, fn func(*transaction) error) error {
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if before != nil {
		if err = before(tx); err != nil {
			return err
		}
	}
	t := &transaction{e: e, tx: tx, ctx: ctx}
	if err = t.exec("UPDATE dag_lock SET seq=seq WHERE id=1"); err != nil {
		return err
	}
	clock := "SELECT unixepoch('now')"
	if e.dialect == "postgres" {
		clock = "SELECT CAST(EXTRACT(EPOCH FROM clock_timestamp()) AS BIGINT)"
	}
	if e.dialect == "oceanbase" {
		clock = "SELECT UNIX_TIMESTAMP()"
	}
	if err = t.row(clock).Scan(&t.now); err != nil {
		return err
	}
	if err = fn(t); err != nil {
		return err
	}
	return tx.Commit()
}
func stamp(now int64) string { return time.Unix(now, 0).UTC().Format(time.RFC3339) }
func uid() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func encoded(v any) string { b, _ := json.Marshal(v); return string(b) }
func fingerprint(v any) string {
	return hashText(encoded(v))
}
func hashText(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
func scopeValid(t, p string) error {
	if t == "" || p == "" || len(t) > 100 || len(p) > 100 {
		return fmt.Errorf("%w: tenant and project required", ErrInvalid)
	}
	return nil
}
func (t *transaction) operation(tenant, project, scope, key, hash string) (string, error) {
	if key == "" || len(key) > 150 {
		return "", fmt.Errorf("%w: operation_id required (max 150)", ErrInvalid)
	}
	var previous, id string
	err := t.row("SELECT fingerprint,result_id FROM dag_operations WHERE tenant_id=? AND project_id=? AND scope=? AND operation_id=?", tenant, project, scope, key).Scan(&previous, &id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if previous != hash {
		return "", fmt.Errorf("%w: idempotency key reused for different request", ErrConflict)
	}
	return id, nil
}
func (t *transaction) remember(tenant, project, scope, key, hash, id string) error {
	return t.exec("INSERT INTO dag_operations (tenant_id,project_id,scope,operation_id,fingerprint,result_id) VALUES (?,?,?,?,?,?)", tenant, project, scope, key, hash, id)
}
func (t *transaction) event(r *runState, node, kind string, data map[string]any) error {
	if err := t.exec("UPDATE dag_lock SET seq=seq+1 WHERE id=1"); err != nil {
		return err
	}
	var seq int64
	if err := t.row("SELECT seq FROM dag_lock WHERE id=1").Scan(&seq); err != nil {
		return err
	}
	return t.exec("INSERT INTO dag_events (sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data) VALUES (?,?,?,?,?,?,?,?)", seq, r.TenantID, r.ProjectID, r.ID, node, kind, stamp(t.now), encoded(data))
}

func (e *Engine) Publish(ctx context.Context, tenant, project string, w Workflow) (Workflow, error) {
	return e.publish(ctx, tenant, project, w, "", false)
}

// PublishWithOperation commits the immutable version and request receipt together.
func (e *Engine) PublishWithOperation(ctx context.Context, tenant, project string, w Workflow, operationID string) (Workflow, error) {
	return e.publish(ctx, tenant, project, w, operationID, true)
}
func (e *Engine) publish(ctx context.Context, tenant, project string, w Workflow, operationID string, receipt bool) (Workflow, error) {
	if err := scopeValid(tenant, project); err != nil {
		return w, err
	}
	var err error
	w, err = normalize(w)
	if err != nil {
		return w, err
	}
	hash := fingerprint(w)
	if w.ID == "" {
		w.ID = uid()
	}
	if !identifier.MatchString(w.ID) || w.Name == "" || len(w.Name) > 255 {
		return w, fmt.Errorf("%w: workflow ID and name required", ErrInvalid)
	}
	err = e.write(ctx, func(t *transaction) error {
		if receipt {
			previous, err := t.operation(tenant, project, "publish_workflow", operationID, hash)
			if err != nil {
				return err
			}
			if previous != "" {
				parts := strings.SplitN(previous, ":", 2)
				if len(parts) != 2 {
					return fmt.Errorf("corrupt workflow receipt")
				}
				version, err := strconv.Atoi(parts[1])
				if err != nil {
					return err
				}
				var raw string
				if err = t.row("SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?", tenant, project, parts[0], version).Scan(&raw); err != nil {
					return err
				}
				return json.Unmarshal([]byte(raw), &w)
			}
		}
		var version int
		if err := t.publicWorkflow(tenant, project, w.ID); err != nil {
			return err
		}
		if err := t.row("SELECT COALESCE(MAX(version),0) FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=?", tenant, project, w.ID).Scan(&version); err != nil {
			return err
		}
		if w.Version != 0 && w.Version != version+1 {
			return fmt.Errorf("%w: published versions are immutable; next version is %d", ErrConflict, version+1)
		}
		w.Version = version + 1
		if err := t.validateReferences(tenant, project, w); err != nil {
			return err
		}
		if err := t.exec("INSERT INTO dag_workflows (tenant_id,project_id,id,version,name,spec) VALUES (?,?,?,?,?,?)", tenant, project, w.ID, w.Version, w.Name, encoded(w)); err != nil {
			return err
		}
		if receipt {
			return t.remember(tenant, project, "publish_workflow", operationID, hash, w.ID+":"+strconv.Itoa(w.Version))
		}
		return nil
	})
	return w, err
}
func (e *Engine) Workflows(ctx context.Context, tenant, project string) ([]Workflow, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, err
	}
	rows, err := e.db.QueryContext(ctx, e.q("SELECT w.spec FROM dag_workflows w WHERE w.tenant_id=? AND w.project_id=? AND NOT EXISTS (SELECT 1 FROM dag_compensations c JOIN dag_runs r ON r.id=c.compensation_run_id WHERE r.tenant_id=w.tenant_id AND r.project_id=w.project_id AND r.workflow_id=w.id) ORDER BY w.id,w.version"), tenant, project)
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
func load(ctx context.Context, q queryer, e *Engine, tenant, project, id string) (*runState, error) {
	r := &runState{}
	err := q.QueryRowContext(ctx, e.q("SELECT id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?"), tenant, project, id).Scan(&r.ID, &r.TenantID, &r.ProjectID, &r.WorkflowID, &r.WorkflowVersion, &r.WorkflowName, &r.Version, &r.State, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var raw string
	if err = q.QueryRowContext(ctx, e.q("SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?"), tenant, project, r.WorkflowID, r.WorkflowVersion).Scan(&raw); err != nil {
		return nil, err
	}
	if err = json.Unmarshal([]byte(raw), &r.workflow); err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, e.q("SELECT id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec FROM dag_nodes WHERE run_id=? ORDER BY id"), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		n := &nodeState{}
		if err = rows.Scan(&n.ID, &n.State, &n.Reason, &n.Attempt, &n.Fence, &n.WorkerID, &n.Output, &n.Error, &n.StartedAt, &n.CompletedAt, &n.lease, &n.wake, &n.parent, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &n.spec); err != nil {
			return nil, err
		}
		n.Name = n.spec.Name
		n.Type = n.spec.Type
		n.Command = n.spec.Command
		r.nodes = append(r.nodes, n)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	links, err := q.QueryContext(ctx, e.q("SELECT node_id,attempt,child_run_id FROM dag_subruns WHERE parent_run_id=?"), r.ID)
	if err != nil {
		return nil, err
	}
	defer links.Close()
	for links.Next() {
		var nodeID, childID string
		var attempt int
		if err = links.Scan(&nodeID, &attempt, &childID); err != nil {
			return nil, err
		}
		n := r.find(nodeID)
		if n != nil && n.Attempt == attempt {
			n.ChildRunID = childID
		}
	}
	if err = links.Err(); err != nil {
		return nil, err
	}
	links.Close()
	results, er := q.QueryContext(ctx, e.q(`SELECT a.node_id,a.result FROM dag_attempts a JOIN dag_nodes n ON n.run_id=a.run_id AND n.id=a.node_id AND n.attempt=a.attempt WHERE a.run_id=?`), r.ID)
	if er != nil {
		return nil, er
	}
	for results.Next() {
		var nodeID, payload string
		if er = results.Scan(&nodeID, &payload); er != nil {
			results.Close()
			return nil, er
		}
		var result Result
		if json.Unmarshal([]byte(payload), &result) == nil {
			if n := r.find(nodeID); n != nil && n.State != "pending" && n.State != "ready" && n.State != "waiting" {
				n.WorkspaceEvidence = result.WorkspaceEvidence
				n.ArtifactEvidence = result.ArtifactEvidence
			}
		}
	}
	er = results.Err()
	results.Close()
	if er != nil {
		return nil, er
	}
	err = q.QueryRowContext(ctx, e.q("SELECT c.compensation_run_id,x.state FROM dag_compensations c JOIN dag_runs x ON x.id=c.compensation_run_id WHERE c.original_run_id=?"), r.ID).Scan(&r.CompensationRunID, &r.CompensationState)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	return r, nil
}
func (r *runState) public() Run {
	out := r.Run
	out.Nodes = []NodeRun{}
	for _, n := range r.nodes {
		public := n.NodeRun
		public.Idempotent = n.spec.Idempotent
		_, configured, err := compensationTemplate(n.spec)
		public.Compensatable = configured && err == nil
		out.Nodes = append(out.Nodes, public)
	}
	return out
}
func (e *Engine) GetRun(ctx context.Context, tenant, project, id string) (Run, error) {
	var out Run
	err := e.write(ctx, func(t *transaction) error {
		r, err := load(ctx, t.tx, e, tenant, project, id)
		if err != nil {
			return err
		}
		out = r.public()
		return nil
	})
	return out, err
}
func (e *Engine) Runs(ctx context.Context, tenant, project string) ([]Run, error) {
	if err := scopeValid(tenant, project); err != nil {
		return nil, err
	}
	out := []Run{}
	err := e.write(ctx, func(t *transaction) error {
		rows, err := t.rows("SELECT id FROM dag_runs WHERE tenant_id=? AND project_id=? ORDER BY created_at DESC,id", tenant, project)
		if err != nil {
			return err
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, id := range ids {
			r, err := load(ctx, t.tx, e, tenant, project, id)
			if err != nil {
				return err
			}
			out = append(out, r.public())
		}
		return nil
	})
	return out, err
}
func (e *Engine) Events(ctx context.Context, tenant, project, id string, after int64) ([]Event, error) {
	if _, err := e.GetRun(ctx, tenant, project, id); err != nil {
		return nil, err
	}
	rows, err := e.db.QueryContext(ctx, e.q("SELECT sequence,run_id,node_id,event_type,at_time,data FROM dag_events WHERE tenant_id=? AND project_id=? AND run_id=? AND sequence>? ORDER BY sequence"), tenant, project, id, after)
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
		out = append(out, event)
	}
	return out, rows.Err()
}
func (t *transaction) insertNode(r *runState, n *nodeState) error {
	return t.exec("INSERT INTO dag_nodes (run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", r.ID, n.ID, n.State, n.Reason, n.Attempt, n.Fence, n.WorkerID, n.Output, n.Error, n.StartedAt, n.CompletedAt, n.lease, n.wake, n.parent, encoded(n.spec))
}
func (t *transaction) save(r *runState) error {
	if err := t.saveQuiet(r); err != nil {
		return err
	}
	if err := t.propagateParent(r); err != nil {
		return err
	}
	return t.propagateCompensation(r)
}
func (t *transaction) saveQuiet(r *runState) error {
	r.Version++
	r.UpdatedAt = stamp(t.now)
	if err := t.exec("UPDATE dag_runs SET version=?,state=?,updated_at=? WHERE id=?", r.Version, r.State, r.UpdatedAt, r.ID); err != nil {
		return err
	}
	for _, n := range r.nodes {
		if err := t.exec("UPDATE dag_nodes SET state=?,reason=?,attempt=?,fence=?,worker_id=?,output=?,error_text=?,started_at=?,completed_at=?,lease_until=?,wake_at=? WHERE run_id=? AND id=?", n.State, n.Reason, n.Attempt, n.Fence, n.WorkerID, n.Output, n.Error, n.StartedAt, n.CompletedAt, n.lease, n.wake, r.ID, n.ID); err != nil {
			return err
		}
	}
	return nil
}
func (e *Engine) CreateRun(ctx context.Context, tenant, project, workflowID, operationID string) (Run, error) {
	return e.CreateRunVersion(ctx, tenant, project, workflowID, 0, operationID)
}
func (e *Engine) CreateRunVersion(ctx context.Context, tenant, project, workflowID string, version int, operationID string) (Run, error) {
	return e.CreateRunBound(ctx, tenant, project, workflowID, version, operationID, "", nil)
}

// CreateRunBound admits service-owned bindings inside the scheduler transaction.
// The callback must use tx exclusively, never call Engine or perform external IO.
func (e *Engine) CreateRunBound(ctx context.Context, tenant, project, workflowID string, version int, operationID, bindingKey string, bind func(*sql.Tx, string) error) (Run, error) {
	var out Run
	if version < 0 || bind != nil && bindingKey == "" {
		return out, fmt.Errorf("%w: version and binding key", ErrInvalid)
	}
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	err := e.write(ctx, func(t *transaction) error {
		hash := fingerprint(map[string]any{"workflow_id": workflowID, "version": version, "binding_key": bindingKey})
		id, err := t.operation(tenant, project, "create_run", operationID, hash)
		if err != nil {
			return err
		}
		if id != "" {
			r, err := load(ctx, t.tx, e, tenant, project, id)
			if err == nil {
				out = r.public()
			}
			return err
		}
		var raw string
		query := "SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=?"
		if err = t.publicWorkflow(tenant, project, workflowID); err != nil {
			return err
		}
		args := []any{tenant, project, workflowID}
		if version > 0 {
			query += " AND version=?"
			args = append(args, version)
		}
		query += " ORDER BY version DESC LIMIT 1"
		err = t.row(query, args...).Scan(&raw)
		if err == sql.ErrNoRows {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var w Workflow
		if err = json.Unmarshal([]byte(raw), &w); err != nil {
			return err
		}
		r, err := t.createRun(tenant, project, w, bind)
		if err != nil {
			return err
		}
		if err = t.remember(tenant, project, "create_run", operationID, hash, r.ID); err != nil {
			return err
		}
		out = r.public()
		return nil
	})
	return out, err
}
