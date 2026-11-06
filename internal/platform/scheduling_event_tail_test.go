package platform

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"modernc.org/sqlite"
)

// Observe real driver calls, not source text. Arguments (including credentials)
// are deliberately never retained. The original SQLite store remains real.
type schedulingSQLTrace struct {
	mu           sync.Mutex
	transactions []*schedulingSQLTransaction
}
type schedulingSQLTransaction struct {
	Statements []string
	Committed  bool
}
type schedulingTraceDriver struct{ trace *schedulingSQLTrace }
type schedulingTraceConn struct {
	driver.Conn
	trace  *schedulingSQLTrace
	active *schedulingSQLTransaction
}
type schedulingTraceTx struct {
	driver.Tx
	conn   *schedulingTraceConn
	record *schedulingSQLTransaction
}

func (d *schedulingTraceDriver) Open(name string) (driver.Conn, error) {
	c, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return &schedulingTraceConn{Conn: c, trace: d.trace}, nil
}
func (c *schedulingTraceConn) record(query string) {
	c.trace.mu.Lock()
	defer c.trace.mu.Unlock()
	if c.active != nil {
		c.active.Statements = append(c.active.Statements, query)
	}
}
func (c *schedulingTraceConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.record(query)
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}
func (c *schedulingTraceConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.record(query)
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
func (c *schedulingTraceConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	tx, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	record := &schedulingSQLTransaction{Statements: []string{}}
	c.trace.mu.Lock()
	c.trace.transactions = append(c.trace.transactions, record)
	c.active = record
	c.trace.mu.Unlock()
	return &schedulingTraceTx{Tx: tx, conn: c, record: record}, nil
}
func (t *schedulingTraceTx) Commit() error {
	err := t.Tx.Commit()
	t.conn.trace.mu.Lock()
	t.record.Committed = err == nil
	t.conn.active = nil
	t.conn.trace.mu.Unlock()
	return err
}
func (t *schedulingTraceTx) Rollback() error {
	err := t.Tx.Rollback()
	t.conn.trace.mu.Lock()
	t.conn.active = nil
	t.conn.trace.mu.Unlock()
	return err
}

func TestReleaseAndArtifactGateNeverAcquireEventSequenceBeforeDomainWork(t *testing.T) {
	f, candidate, _, plan, _, _ := verifiedReleaseDeploymentPlan(t)
	ctx := context.Background()
	var path string
	if err := f.s.DB.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	trace := &schedulingSQLTrace{}
	name := "scheduling_tail_" + newID()
	sql.Register(name, &schedulingTraceDriver{trace: trace})
	db, err := sql.Open(name, path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err = db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		t.Fatal(err)
	}
	engine, err := dag.New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	f.s.DB, f.e = db, engine
	f.handler = (&Server{Service: f.s, Engine: engine, WebDir: t.TempDir()}).Handler()
	var beforeSequence, beforeEvents int64
	if err = db.QueryRow(`SELECT seq FROM dag_lock WHERE id=1`).Scan(&beforeSequence); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM dag_events`).Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	trace.mu.Lock()
	trace.transactions = nil
	trace.mu.Unlock()
	// Display-only candidate reads must not take a global execution write lock.
	if _, err = f.s.ReleaseCandidate(ctx, f.actor, f.project, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = f.s.ReleaseCandidates(ctx, f.actor, f.project, 50, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.ExecuteReleasePlan(ctx, f.actor, f.project, plan.ID, releaseExecuteBody(f, plan), engine); err != nil {
		t.Fatal(err)
	}
	worker := f.call("POST", "/release-workers", f.body(map[string]any{"worker_id": "tail-deployer", "profiles": []any{}, "deployment": true}), 201)
	claim := f.bearer("POST", "/runner/claim", worker["token"].(string), f.body(map[string]any{"worker_id": "tail-deployer", "capabilities": releaseTestCapabilities("local-static-test")}), 200)
	if claim["assignment"] == nil {
		t.Fatal("actual protected attempt was not dispatched")
	}
	var afterSequence, afterEvents int64
	if err = db.QueryRow(`SELECT seq FROM dag_lock WHERE id=1`).Scan(&afterSequence); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT COUNT(*) FROM dag_events`).Scan(&afterEvents); err != nil {
		t.Fatal(err)
	}
	if afterSequence-beforeSequence != afterEvents-beforeEvents {
		t.Fatalf("domain barriers consumed event cursor numbers: sequence delta=%d events delta=%d", afterSequence-beforeSequence, afterEvents-beforeEvents)
	}
	trace.mu.Lock()
	defer trace.mu.Unlock()
	eventTransactions := 0
	for _, tx := range trace.transactions {
		locked, locks, inserted := false, 0, 0
		for _, query := range tx.Statements {
			statement := strings.ToUpper(strings.TrimSpace(query))
			if strings.HasPrefix(statement, "UPDATE DAG_LOCK ") {
				locked = true
				locks++
				continue
			}
			if strings.HasPrefix(statement, "INSERT INTO DAG_EVENTS ") || strings.HasPrefix(statement, "INSERT INTO DAG_EVENTS(") {
				inserted++
				continue
			}
			if locked && !strings.HasPrefix(statement, "SELECT SEQ FROM DAG_LOCK ") {
				t.Fatalf("domain query/write occurred after global event allocation: %s", query)
			}
		}
		if locks > 0 && (locks != 1 || inserted == 0) {
			t.Fatalf("early/global barrier persisted without final event batch: locks=%d events=%d", locks, inserted)
		}
		if inserted > 0 && tx.Committed {
			eventTransactions++
			if locks != 1 {
				t.Fatal("event batch lacked exactly one ordered allocation")
			}
		}
	}
	if eventTransactions < 2 {
		t.Fatalf("trace did not observe managed run and claim commit: %d", eventTransactions)
	}
}
