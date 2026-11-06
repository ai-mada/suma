// Package dag implements a durable DAG state machine. All transitions and their
// evidence commit together under project and run-family locks, across processes.
package dag

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/ai-mada/suma/internal/store"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

type Engine struct {
	db           *sql.DB
	dialect      string
	LeaseSeconds int64
	// Configure once before starting concurrent scheduler or HTTP work.
	AttemptAccounting     AttemptAccounting
	AttemptReconciliation LiveReadOnlyCompletionReconciler
	BoundSubworkflows     BoundSubworkflowHandler
}
type transaction struct {
	e                        *Engine
	tx                       *sql.Tx
	ctx                      context.Context
	now                      int64
	events                   []pendingEvent
	projects                 map[string]bool
	familyRoot               string
	scopes                   map[string]bool
	scopeCleanups            []func()
	allowBoundCreation       bool
	dedicatedManagedCreation bool
}
type pendingEvent struct {
	runID, tenant, project, node, kind, at string
	data                                   map[string]any
}
type nodeState struct {
	NodeRun
	spec          Node
	lease         int64
	wake          int64
	parent        string
	original      string
	originalState string
}
type runState struct {
	Run
	nodes                                                  []*nodeState
	workflow                                               Workflow
	eventDirty                                             bool
	nodeMap                                                map[string]*nodeState
	tx                                                     *transaction
	partial                                                bool
	indexed                                                bool
	loadErr                                                error
	totalNodes, terminalNodes, failedNodes, cancelledNodes int
	resolved                                               map[string]string
}
type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func New(db *sql.DB, dialect string) (*Engine, error) {
	if dialect != "sqlite" && dialect != "oceanbase" && dialect != "postgres" {
		return nil, dagDiagnostic(fmt.Errorf("%w: database dialect", ErrUnsupported), "dag.storage.dialect", nil)
	}
	if err := ValidateExternalAssignmentsInitialSchema(context.Background(), db, dialect); err != nil {
		return nil, err
	}
	if err := store.InitializeBaseline(context.Background(), db, dialect, "dag", "dag_", store.InitialDataContract); err != nil {
		return nil, err
	}
	e := &Engine{db: db, dialect: dialect, LeaseSeconds: 30}
	for _, ddl := range schema {
		if dialect == "oceanbase" {
			ddl = strings.ReplaceAll(ddl, " TEXT", " LONGTEXT")
		}
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("dag initialization: %w", err)
		}
	}
	if err := initializeEventIndexes(db, dialect); err != nil {
		return nil, fmt.Errorf("dag event index initialization: %w", err)
	}
	insert := "INSERT INTO dag_lock (id,seq) VALUES (1,0) ON CONFLICT (id) DO NOTHING"
	if dialect == "oceanbase" {
		insert = "INSERT IGNORE INTO dag_lock (id,seq) VALUES (1,0)"
	}
	if _, err := db.Exec(insert); err != nil {
		return nil, err
	}
	if err := e.initializeSchedulingSchema(context.Background()); err != nil {
		return nil, fmt.Errorf("scheduling initialization: %w", err)
	}
	if err := e.initializePoolSchema(context.Background()); err != nil {
		return nil, err
	}
	if err := e.initializeExternalAssignmentsSchema(context.Background()); err != nil {
		return nil, err
	}
	return e, nil
}

// GetWorkflow reads a published immutable version; version zero selects latest.
func (e *Engine) GetWorkflow(ctx context.Context, tenant, project, id string, version int) (Workflow, error) {
	return e.getWorkflow(ctx, e.db, tenant, project, id, version)
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
	var options *sql.TxOptions
	if e.dialect != "sqlite" {
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	var tx *sql.Tx
	var err error
	if before != nil {
		// Authority may temporarily tune SQLite's connection-local wait budget.
		// Retain that connection until it is restored after commit or rollback.
		scope, beginErr := store.BeginScopedTx(ctx, e.db, e.dialect, options)
		if beginErr != nil {
			return beginErr
		}
		tx = scope.Tx
		defer func() {
			if closeErr := scope.Close(); closeErr != nil {
				slog.Warn("transaction connection cleanup failed", "driver", e.dialect, "error", closeErr)
			}
		}()
	} else {
		tx, err = e.db.BeginTx(ctx, options)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}
	if before != nil {
		if err = before(tx); err != nil {
			return err
		}
	}
	t := &transaction{e: e, tx: tx, ctx: ctx, projects: map[string]bool{}}
	defer func() { _ = tx.Rollback(); t.closeScopes() }()
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
	// This is the final SQL phase. No domain/project/family locks may be
	// acquired after the global event sequence has been allocated.
	if err = t.flushEvents(); err != nil {
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
		return dagDiagnostic(fmt.Errorf("%w: tenant and project required", ErrInvalid), "dag.scope.required", nil)
	}
	return nil
}
func (t *transaction) operation(tenant, project, scope, key, hash string) (string, error) {
	var routeRun string
	for _, prefix := range []string{"command:", "result:", "progress:"} {
		if strings.HasPrefix(scope, prefix) {
			routeRun = strings.TrimPrefix(scope, prefix)
		}
	}
	if routeRun != "" {
		if err := t.guardExecution(tenant, project, routeRun); err != nil {
			return "", err
		}
	} else if scope == "create_run" || scope == "managed_run" && !t.dedicatedManagedCreation {
		var prior string
		err := t.row(`SELECT result_id FROM dag_operations WHERE tenant_id=? AND project_id=? AND scope=? AND operation_id=?`, tenant, project, scope, key).Scan(&prior)
		if err != nil && err != sql.ErrNoRows {
			return "", err
		}
		if err = t.guardExecution(tenant, project, prior); err != nil {
			return "", err
		}
	}
	if err := t.guardProject(tenant, project); err != nil {
		return "", err
	}
	if key == "" || len(key) > 150 {
		return "", dagDiagnostic(fmt.Errorf("%w: operation_id required (max 150)", ErrInvalid), "dag.operation.id", nil)
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
		return "", dagDiagnostic(fmt.Errorf("%w: idempotency key reused for different request", ErrConflict), "dag.operation.reused", nil)
	}
	return id, nil
}
func (t *transaction) remember(tenant, project, scope, key, hash, id string) error {
	return t.exec("INSERT INTO dag_operations (tenant_id,project_id,scope,operation_id,fingerprint,result_id) VALUES (?,?,?,?,?,?)", tenant, project, scope, key, hash, id)
}
func (t *transaction) event(r *runState, node, kind string, data map[string]any) error {
	// Copy now: callers may mutate their maps later in this transaction.
	var snapshot map[string]any
	if data != nil {
		if err := json.Unmarshal([]byte(encoded(data)), &snapshot); err != nil {
			return err
		}
	}
	t.events = append(t.events, pendingEvent{runID: r.ID, tenant: r.TenantID, project: r.ProjectID, node: node, kind: kind, at: stamp(t.now), data: snapshot})
	r.eventDirty = true
	return nil
}

func (t *transaction) flushEvents() error {
	if len(t.events) == 0 {
		return nil
	}
	if err := t.exec("UPDATE dag_lock SET seq=seq+? WHERE id=1", len(t.events)); err != nil {
		return err
	}
	var end int64
	if err := t.row("SELECT seq FROM dag_lock WHERE id=1").Scan(&end); err != nil {
		return err
	}
	start := end - int64(len(t.events)) + 1
	for i, event := range t.events {
		if err := t.exec("INSERT INTO dag_events(sequence,tenant_id,project_id,run_id,node_id,event_type,at_time,data)VALUES(?,?,?,?,?,?,?,?)", start+int64(i), event.tenant, event.project, event.runID, event.node, event.kind, event.at, encoded(event.data)); err != nil {
			return err
		}
	}
	return nil
}
func (t *transaction) guardProject(tenant, project string) error {
	if err := t.beforeScope(tenant, project); err != nil {
		return err
	}
	key := tenant + "\x00" + project
	if t.projects == nil {
		t.projects = map[string]bool{}
	}
	if t.projects[key] {
		return nil
	}
	if err := t.e.projectGuardTx(t.ctx, t.tx, tenant, project); err != nil {
		return err
	}
	t.projects[key] = true
	return nil
}
func (t *transaction) nextQueueSequence(tenant, project string) (int64, error) {
	if err := t.guardProject(tenant, project); err != nil {
		return 0, err
	}
	if err := t.exec(`UPDATE dag_scheduler_projects SET queue_sequence=queue_sequence+1 WHERE tenant_id=? AND project_id=?`, tenant, project); err != nil {
		return 0, err
	}
	var seq int64
	err := t.row(`SELECT queue_sequence FROM dag_scheduler_projects WHERE tenant_id=? AND project_id=?`, tenant, project).Scan(&seq)
	return seq, err
}

// Publish is an internal kernel entry; interactive callers must supply current
// authorization through PublishWithAuthority.
func (e *Engine) Publish(ctx context.Context, tenant, project string, w Workflow) (Workflow, error) {
	return e.publish(ctx, tenant, project, w, "", false, nil)
}

// PublishWithOperation commits the immutable version and request receipt together.
// It is an internal kernel entry; interactive callers use PublishWithAuthority.
func (e *Engine) PublishWithOperation(ctx context.Context, tenant, project string, w Workflow, operationID string) (Workflow, error) {
	return e.publish(ctx, tenant, project, w, operationID, true, nil)
}

// PublishWithAuthority checks current authority in the publication transaction,
// before acquiring the scheduler lock or reading a receipt. The callback must
// use only the supplied tx and must not call Engine or perform external IO.
func (e *Engine) PublishWithAuthority(ctx context.Context, tenant, project string, w Workflow, operationID string, before func(*sql.Tx) error) (Workflow, error) {
	w, err := e.publish(ctx, tenant, project, w, operationID, true, before)
	if err != nil {
		return Workflow{}, err
	}
	return w, nil
}

func (e *Engine) publish(ctx context.Context, tenant, project string, w Workflow, operationID string, receipt bool, before func(*sql.Tx) error) (Workflow, error) {
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
		return w, dagDiagnostic(fmt.Errorf("%w: workflow ID and name required", ErrInvalid), "dag.workflow.identity", nil)
	}
	err = e.writeBefore(ctx, before, func(t *transaction) error {
		if receipt {
			previous, err := t.operation(tenant, project, "publish_workflow", operationID, hash)
			if err != nil {
				return err
			}
			if previous != "" {
				parts := strings.SplitN(previous, ":", 2)
				if len(parts) != 2 {
					return dagDiagnostic(fmt.Errorf("corrupt workflow receipt"), "dag.workflow.corrupt_receipt", nil)
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
			return dagDiagnostic(fmt.Errorf("%w: published versions are immutable; next version is %d", ErrConflict, version+1), "dag.workflow.immutable_version", diagnosticParams{"version": version + 1})
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
	return e.workflows(ctx, e.db, tenant, project)
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
	if err = e.loadExecutionRoute(ctx, q, r); err != nil {
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
		n.original = nodePersistence(n)
		n.originalState = n.State
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
				n.ToolEvidence = result.ToolEvidence
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
	metadata, err := q.QueryContext(ctx, e.q(`SELECT node_id,winners,ready_at FROM dag_node_dependencies WHERE run_id=?`), r.ID)
	if err != nil {
		return nil, err
	}
	for metadata.Next() {
		var id, winners, ready string
		if err = metadata.Scan(&id, &winners, &ready); err != nil {
			metadata.Close()
			return nil, err
		}
		if n := r.find(id); n != nil {
			if err = json.Unmarshal([]byte(winners), &n.WinnerNodeIDs); err != nil {
				metadata.Close()
				return nil, err
			}
			n.ReadyAt = ready
		}
	}
	err = metadata.Err()
	metadata.Close()
	if err != nil {
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
		public.ToolProfileID, public.ToolProfileSHA256 = "", ""
		if id, sha, bound, err := ToolBinding(n.spec); err == nil && bound {
			public.ToolProfileID, public.ToolProfileSHA256 = id, sha
		}
		// Pending/ready nodes may have a previous failed attempt on the in-memory
		// command path. Its evidence remains in history, never the new attempt.
		if n.State == "pending" || n.State == "ready" || n.State == "waiting" {
			public.ToolEvidence = nil
		}
		_, configured, err := compensationTemplate(n.spec)
		public.Compensatable = configured && err == nil
		out.Nodes = append(out.Nodes, public)
	}
	return out
}
func (e *Engine) GetRun(ctx context.Context, tenant, project, id string) (Run, error) {
	var out Run
	tx, err := e.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return out, err
	}
	defer tx.Rollback()
	out, err = e.GetRunTx(ctx, tx, tenant, project, id)
	if err != nil {
		return out, err
	}
	return out, tx.Commit()
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

// Events returns at most 1000 durable events; callers resume after the last
// returned sequence. HTTP clients use the smaller authorized 128-event window.
func (e *Engine) Events(ctx context.Context, tenant, project, id string, after int64) ([]Event, error) {
	return e.events(ctx, e.db, tenant, project, id, after)
}
func (t *transaction) insertNode(r *runState, n *nodeState) error {
	err := t.exec("INSERT INTO dag_nodes (run_id,id,state,reason,attempt,fence,worker_id,output,error_text,started_at,completed_at,lease_until,wake_at,parent_id,spec) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", r.ID, n.ID, n.State, n.Reason, n.Attempt, n.Fence, n.WorkerID, n.Output, n.Error, n.StartedAt, n.CompletedAt, n.lease, n.wake, n.parent, encoded(n.spec))
	if err == nil {
		n.original = nodePersistence(n)
		n.originalState = n.State
		if r.indexed {
			err = t.insertGraphNode(r, n)
		}
	}
	return err
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
	changed := r.eventDirty
	for _, n := range r.nodes {
		changed = changed || n.original != nodePersistence(n)
	}
	if !changed {
		return nil
	}
	var total, done, failed, cancelled int
	if r.indexed {
		total, done, failed, cancelled = r.counts()
		if err := t.exec(`UPDATE dag_run_counters SET total=?,terminal_count=?,failed_count=?,cancelled_count=? WHERE run_id=?`, total, done, failed, cancelled, r.ID); err != nil {
			return err
		}
	}
	r.Version++
	r.UpdatedAt = stamp(t.now)
	if err := t.exec("UPDATE dag_runs SET version=?,state=?,updated_at=? WHERE id=?", r.Version, r.State, r.UpdatedAt, r.ID); err != nil {
		return err
	}
	for _, n := range r.nodes {
		if n.original == nodePersistence(n) {
			continue
		}
		if err := t.exec("UPDATE dag_nodes SET state=?,reason=?,attempt=?,fence=?,worker_id=?,output=?,error_text=?,started_at=?,completed_at=?,lease_until=?,wake_at=? WHERE run_id=? AND id=?", n.State, n.Reason, n.Attempt, n.Fence, n.WorkerID, n.Output, n.Error, n.StartedAt, n.CompletedAt, n.lease, n.wake, r.ID, n.ID); err != nil {
			return err
		}
		n.original = nodePersistence(n)
		n.originalState = n.State
	}
	r.eventDirty = false
	if r.indexed {
		r.totalNodes, r.terminalNodes, r.failedNodes, r.cancelledNodes = total, done, failed, cancelled
	}
	return t.refreshRunTurn(r)
}
func nodePersistence(n *nodeState) string {
	return encoded([]any{n.State, n.Reason, n.Attempt, n.Fence, n.WorkerID, n.Output, n.Error, n.StartedAt, n.CompletedAt, n.lease, n.wake})
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
	return e.CreateRunBoundAuthorized(ctx, tenant, project, workflowID, version, operationID, bindingKey, nil, bind)
}

// CreateRunBoundAuthorized checks live authority before acquiring the scheduler
// lock or replaying a receipt. Both callbacks must use only the supplied tx.
func (e *Engine) CreateRunBoundAuthorized(ctx context.Context, tenant, project, workflowID string, version int, operationID, bindingKey string, before func(*sql.Tx) error, bind func(*sql.Tx, string) error) (Run, error) {
	var out Run
	if version < 0 || bind != nil && bindingKey == "" {
		return out, dagDiagnostic(fmt.Errorf("%w: version and binding key", ErrInvalid), "dag.run.binding", nil)
	}
	if err := scopeValid(tenant, project); err != nil {
		return out, err
	}
	err := e.writeBefore(ctx, before, func(t *transaction) error {
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
