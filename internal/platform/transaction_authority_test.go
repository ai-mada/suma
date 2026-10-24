package platform

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/dag"
	"modernc.org/sqlite"
)

// The barrier pauses an actual SQLite authorization read or the completed short
// receipt transaction. A second connection commits a real owner-authorized role
// change before Review continues; no stored users or grants are forged here.
type reviewAuthorityGate struct {
	stage       string
	ctx         context.Context
	armed       atomic.Bool
	observed    chan struct{}
	resume      chan struct{}
	releaseOnce sync.Once
}

func (g *reviewAuthorityGate) release() { g.releaseOnce.Do(func() { close(g.resume) }) }
func (g *reviewAuthorityGate) wait() error {
	close(g.observed)
	select {
	case <-g.resume:
		return nil
	case <-g.ctx.Done():
		return g.ctx.Err()
	}
}

type reviewAuthorityDriver struct{ gate *reviewAuthorityGate }

func (d *reviewAuthorityDriver) Open(name string) (driver.Conn, error) {
	conn, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &reviewAuthorityConn{Conn: conn, gate: d.gate}, nil
}

type reviewAuthorityConn struct {
	driver.Conn
	gate *reviewAuthorityGate
}

func (c *reviewAuthorityConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}
func (c *reviewAuthorityConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
	if err != nil {
		return nil, err
	}
	if c.gate.stage == "authorization_read" && strings.HasPrefix(query, `SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`) && c.gate.armed.CompareAndSwap(true, false) {
		return &reviewAuthorityRows{Rows: rows, gate: c.gate}, nil
	}
	return rows, nil
}
func (c *reviewAuthorityConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &reviewAuthorityTx{Tx: tx, gate: c.gate}, nil
}

type reviewAuthorityRows struct {
	driver.Rows
	gate   *reviewAuthorityGate
	paused bool
}

func (r *reviewAuthorityRows) Next(values []driver.Value) error {
	err := r.Rows.Next(values)
	if err == nil && !r.paused {
		r.paused = true
		return r.gate.wait()
	}
	return err
}

type reviewAuthorityTx struct {
	driver.Tx
	gate *reviewAuthorityGate
}

func (t *reviewAuthorityTx) Rollback() error {
	// Release the SQLite/org write lock before the second connection changes the
	// role. Pausing while holding it would test lock contention instead of TOCTOU.
	err := t.Tx.Rollback()
	if err == nil && t.gate.stage == "receipt_released" && t.gate.armed.CompareAndSwap(true, false) {
		return t.gate.wait()
	}
	return err
}

func reviewAuthorityConnection(t *testing.T, f *fixture, ctx context.Context, stage string) (*Service, *reviewAuthorityGate) {
	t.Helper()
	var sequence int
	var schema, path string
	if err := f.s.DB.QueryRow(`PRAGMA database_list`).Scan(&sequence, &schema, &path); err != nil {
		t.Fatal(err)
	}
	gate := &reviewAuthorityGate{stage: stage, ctx: ctx, observed: make(chan struct{}), resume: make(chan struct{})}
	gate.armed.Store(true)
	name := "review-authority-gate-" + newID()
	sql.Register(name, &reviewAuthorityDriver{gate: gate})
	db, err := sql.Open(name, path+"?_pragma=busy_timeout(10000)&_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	t.Cleanup(gate.release)
	return &Service{DB: db, Dialect: "sqlite", DataDir: f.s.DataDir}, gate
}

func invitedReviewer(t *testing.T, f *fixture) Principal {
	t.Helper()
	ownerCookie := f.cookie
	inv := governanceInvite(f, "authority-reviewer@example.test", "member", []any{map[string]any{"project_id": f.project, "role": "reviewer"}})
	session := governanceAccept(f, inv, "authority-reviewer@example.test")
	f.cookie = ownerCookie
	raw, err := json.Marshal(session["user"])
	if err != nil {
		t.Fatal(err)
	}
	var p Principal
	if err = json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func unreviewedTestTask(t *testing.T, f *fixture) (map[string]any, map[string]any) {
	t.Helper()
	// This is an explicit non-code test fixture. It makes no workspace/merge
	// claim and isolates the authority guard from candidate execution semantics.
	workflow := f.call("POST", "/workflows", f.body(map[string]any{"name": "权限回归的非代码测试", "nodes": []any{map[string]any{"id": "test", "type": "test"}}, "edges": []any{}}), 201)
	task := f.call("POST", "/tasks", f.body(map[string]any{"title": "审核权限事务回归"}), 201)
	run := f.call("POST", "/runs", f.body(map[string]any{"workflow_id": workflow["id"], "task_id": task["id"]}), 201)
	a, err := f.e.Claim(context.Background(), f.user.TenantID, f.project, "authority-evidence-worker", nil)
	if err != nil || a == nil {
		t.Fatalf("claim: %v %v", a, err)
	}
	if err = f.e.Complete(context.Background(), f.user.TenantID, f.project, dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID()}); err != nil {
		t.Fatal(err)
	}
	task = f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
	request := f.body(map[string]any{"expected_version": task["version"], "run_id": run["id"], "decision": "accepted", "reason": "核对当前审核权限"})
	return task, request
}

func expectRevokedReview(t *testing.T, f *fixture, p Principal, task map[string]any, request map[string]any, stage string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service, gate := reviewAuthorityConnection(t, f, ctx, stage)
	finished := make(chan error, 1)
	go func() {
		_, err := service.Review(ctx, p, f.project, task["id"].(string), request, f.e)
		finished <- err
	}()
	select {
	case <-gate.observed:
	case <-ctx.Done():
		t.Fatal("review never reached the role-change barrier")
	}
	_, err := f.s.SaveProjectAccess(ctx, f.user, map[string]any{"project_id": f.project, "user_id": p.ID, "role": "viewer", "expected_version": float64(1), "idempotency_key": newID()})
	gate.release()
	if err != nil {
		t.Fatal("concurrent owner role update failed:", err)
	}
	select {
	case err = <-finished:
		var api *APIError
		if !errors.As(err, &api) || api.Status != 403 {
			t.Fatalf("revoked reviewer was not denied: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("review hung after its organization authority changed")
	}
	var live string
	if err = f.s.DB.QueryRow(`SELECT role FROM suma_project_members WHERE project_id=? AND user_id=?`, f.project, p.ID).Scan(&live); err != nil || live != "viewer" {
		t.Fatalf("owner role change was not committed: %s %v", live, err)
	}
}

func TestReviewRechecksAuthorityAfterConcurrentProjectRoleRevocation(t *testing.T) {
	for _, stage := range []string{"authorization_read", "receipt_released"} {
		t.Run(stage, func(t *testing.T) {
			f := newFixture(t)
			p := invitedReviewer(t, f)
			task, request := unreviewedTestTask(t, f)
			expectRevokedReview(t, f, p, task, request, stage)
			current := f.call("GET", "/tasks/"+task["id"].(string)+"?project_id="+f.project, nil, 200)
			if current["status"] != "in_progress" || current["version"] != task["version"] {
				t.Fatalf("denied review changed task: %v", current)
			}
			var count int
			if err := f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=?`, f.user.TenantID, f.project, task["id"]).Scan(&count); err != nil || count != 0 {
				t.Fatalf("denied review left evidence: count=%d %v", count, err)
			}
		})
	}
}

func TestReviewSuccessReceiptCannotReplayAfterConcurrentRoleRevocation(t *testing.T) {
	f := newFixture(t)
	p := invitedReviewer(t, f)
	task, request := unreviewedTestTask(t, f)
	accepted, err := f.s.Review(context.Background(), p, f.project, task["id"].(string), request, f.e)
	if err != nil || accepted.Status != "accepted" {
		t.Fatalf("authorized initial review: %+v %v", accepted, err)
	}
	again, err := f.s.Review(context.Background(), p, f.project, task["id"].(string), request, f.e)
	if err != nil || again.Version != accepted.Version {
		t.Fatalf("authorized receipt did not replay: %+v %v", again, err)
	}
	expectRevokedReview(t, f, p, task, request, "authorization_read")
	var count int
	if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM suma_reviews WHERE tenant_id=? AND project_id=? AND task_id=?`, f.user.TenantID, f.project, task["id"]).Scan(&count); err != nil || count != 1 {
		t.Fatalf("receipt replay duplicated/removed review evidence: count=%d %v", count, err)
	}
}
