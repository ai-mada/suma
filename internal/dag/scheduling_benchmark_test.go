package dag

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ai-mada/suma/internal/store"
)

// Count actual database/sql driver calls and rows, not a timing-only proxy.
// Transaction boundaries and SQL executed internally by SQLite are excluded.
type schedulingSQLStats struct {
	mu                                                           sync.Mutex
	statements, rows, nodeUpdates, runUpdates, globalLockUpdates int64
}

func (s *schedulingSQLStats) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.statements, s.rows, s.nodeUpdates, s.runUpdates, s.globalLockUpdates = 0, 0, 0, 0, 0
}
func (s *schedulingSQLStats) statement() { s.mu.Lock(); s.statements++; s.mu.Unlock() }
func (s *schedulingSQLStats) row()       { s.mu.Lock(); s.rows++; s.mu.Unlock() }
func (s *schedulingSQLStats) affected(query string, result driver.Result) {
	if result == nil {
		return
	}
	n, err := result.RowsAffected()
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	q := strings.ToUpper(strings.TrimSpace(query))
	switch {
	case strings.HasPrefix(q, "UPDATE DAG_NODES "):
		s.nodeUpdates += n
	case strings.HasPrefix(q, "UPDATE DAG_RUNS "):
		s.runUpdates += n
	case strings.HasPrefix(q, "UPDATE DAG_LOCK "):
		s.globalLockUpdates += n
	}
}
func (s *schedulingSQLStats) snapshot() map[string]int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]int64{"sql_calls": s.statements, "rows_read": s.rows, "node_update_rows": s.nodeUpdates, "run_update_rows": s.runUpdates, "global_lock_update_rows": s.globalLockUpdates}
}

type schedulingConnector struct {
	base  driver.Driver
	dsn   string
	stats *schedulingSQLStats
}

func (c schedulingConnector) Connect(context.Context) (driver.Conn, error) {
	v, e := c.base.Open(c.dsn)
	if e != nil {
		return nil, e
	}
	return schedulingConn{Conn: v, stats: c.stats}, nil
}
func (c schedulingConnector) Driver() driver.Driver { return c.base }

type schedulingConn struct {
	driver.Conn
	stats *schedulingSQLStats
}

func (c schedulingConn) Prepare(q string) (driver.Stmt, error) {
	s, e := c.Conn.Prepare(q)
	if e != nil {
		return nil, e
	}
	return schedulingStmt{Stmt: s, query: q, stats: c.stats}, nil
}
func (c schedulingConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	if v, ok := c.Conn.(driver.ConnBeginTx); ok {
		return v.BeginTx(ctx, o)
	}
	return c.Conn.Begin()
}
func (c schedulingConn) CheckNamedValue(v *driver.NamedValue) error {
	if n, ok := c.Conn.(driver.NamedValueChecker); ok {
		return n.CheckNamedValue(v)
	}
	return driver.ErrSkip
}
func (c schedulingConn) ExecContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Result, error) {
	v, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	c.stats.statement()
	r, e := v.ExecContext(ctx, q, a)
	if e == nil {
		c.stats.affected(q, r)
	}
	return r, e
}
func (c schedulingConn) QueryContext(ctx context.Context, q string, a []driver.NamedValue) (driver.Rows, error) {
	v, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	c.stats.statement()
	r, e := v.QueryContext(ctx, q, a)
	if e != nil {
		return nil, e
	}
	return schedulingRows{Rows: r, stats: c.stats}, nil
}

type schedulingStmt struct {
	driver.Stmt
	query string
	stats *schedulingSQLStats
}

func (s schedulingStmt) Exec(v []driver.Value) (driver.Result, error) {
	s.stats.statement()
	r, e := s.Stmt.Exec(v)
	if e == nil {
		s.stats.affected(s.query, r)
	}
	return r, e
}
func (s schedulingStmt) Query(v []driver.Value) (driver.Rows, error) {
	s.stats.statement()
	r, e := s.Stmt.Query(v)
	if e != nil {
		return nil, e
	}
	return schedulingRows{Rows: r, stats: s.stats}, nil
}

type schedulingRows struct {
	driver.Rows
	stats *schedulingSQLStats
}

func (r schedulingRows) Next(v []driver.Value) error {
	e := r.Rows.Next(v)
	if e == nil {
		r.stats.row()
	}
	return e
}

func schedulingMeasuredEngine(t testing.TB) (*Engine, *schedulingSQLStats) {
	t.Helper()
	base, err := store.Open("sqlite", filepath.Join(t.TempDir(), "driver.db"))
	if err != nil {
		t.Fatal(err)
	}
	stats := &schedulingSQLStats{}
	db := sql.OpenDB(schedulingConnector{base: base.Driver(), dsn: filepath.Join(t.TempDir(), "measured.db") + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)", stats: stats})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close(); base.Close() })
	e, err := New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return e, stats
}

func schedulingBenchWorkflow(size int, reverse bool, kind string) Workflow {
	w := Workflow{ID: "baseline", Name: "Scheduler measurement"}
	for i := 0; i < size; i++ {
		n := Node{ID: fmt.Sprintf("n%04d", i), Type: kind}
		if kind == "command" {
			n.Command = []string{"measure"}
		}
		if reverse && i > 0 {
			n.DependsOn = []string{fmt.Sprintf("n%04d", i-1)}
		}
		if kind == "timer" {
			n.Config = map[string]any{"duration_seconds": 86400}
		}
		w.Nodes = append(w.Nodes, n)
	}
	if reverse {
		for i, j := 0, len(w.Nodes)-1; i < j; i, j = i+1, j-1 {
			w.Nodes[i], w.Nodes[j] = w.Nodes[j], w.Nodes[i]
		}
	}
	return w
}

func BenchmarkSchedulingKernel(b *testing.B) {
	for _, size := range []int{100, 300} {
		b.Run(fmt.Sprintf("reverse_internal_chain_%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				e, s := schedulingMeasuredEngine(b)
				w, err := e.Publish(context.Background(), "t", "p", schedulingBenchWorkflow(size, true, "noop"))
				if err != nil {
					b.Fatal(err)
				}
				s.reset()
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				b.StartTimer()
				r, err := e.CreateRun(ctx, "t", "p", w.ID, "create")
				b.StopTimer()
				cancel()
				schedulingReportBench(b, s, err)
				if err == nil && r.State != "succeeded" {
					b.Fatal("internal chain did not finish", r.State)
				}
			}
		})
	}
	b.Run("complete_one_of_1000", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			e, s := schedulingMeasuredEngine(b)
			w, err := e.Publish(context.Background(), "t", "p", schedulingBenchWorkflow(1000, false, "command"))
			if err != nil {
				b.Fatal(err)
			}
			_, err = e.CreateRun(context.Background(), "t", "p", w.ID, "create")
			if err != nil {
				b.Fatal(err)
			}
			a, err := e.Claim(context.Background(), "t", "p", "w", []string{"command:measure"})
			if err != nil || a == nil {
				b.Fatal(a, err)
			}
			s.reset()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			b.StartTimer()
			err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "complete"})
			b.StopTimer()
			cancel()
			schedulingReportBench(b, s, err)
		}
	})
	b.Run("future_timer_1000", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			e, s := schedulingMeasuredEngine(b)
			w, err := e.Publish(context.Background(), "t", "p", schedulingBenchWorkflow(1000, false, "timer"))
			if err != nil {
				b.Fatal(err)
			}
			_, err = e.CreateRun(context.Background(), "t", "p", w.ID, "create")
			if err != nil {
				b.Fatal(err)
			}
			s.reset()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			b.StartTimer()
			err = e.Tick(ctx)
			b.StopTimer()
			cancel()
			schedulingReportBench(b, s, err)
		}
	})
	b.Run("claim_300_unmatched_before_match", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			e, s := schedulingMeasuredEngine(b)
			w := schedulingBenchWorkflow(300, false, "command")
			w.Nodes[299].Command = []string{"matching"}
			published, err := e.Publish(context.Background(), "t", "p", w)
			if err != nil {
				b.Fatal(err)
			}
			_, err = e.CreateRun(context.Background(), "t", "p", published.ID, "create")
			if err != nil {
				b.Fatal(err)
			}
			s.reset()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			b.StartTimer()
			a, err := e.Claim(ctx, "t", "p", "w", []string{"command:matching"})
			b.StopTimer()
			cancel()
			schedulingReportBench(b, s, err)
			if err == nil && (a == nil || a.NodeID != "n0299") {
				b.Fatal("matching candidate not reached", a)
			}
		}
	})
}

func BenchmarkSchedulingDenseGraph1000Nodes2994Edges(b *testing.B) {
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		e, s := schedulingMeasuredEngine(b)
		w := schedulingBenchWorkflow(1000, true, "noop")
		w.Edges = nil
		for j := 0; j < 1000; j++ {
			w.Nodes[j].DependsOn = nil
		}
		for from := 0; from < 1000; from++ {
			for offset := 1; offset <= 3 && from+offset < 1000; offset++ {
				w.Edges = append(w.Edges, Edge{From: fmt.Sprintf("n%04d", from), To: fmt.Sprintf("n%04d", from+offset)})
			}
		}
		published, err := e.Publish(context.Background(), "t", "p", w)
		if err != nil {
			b.Fatal(err)
		}
		s.reset()
		deadline, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		b.StartTimer()
		r, err := e.CreateRun(deadline, "t", "p", published.ID, "create-dense")
		for ticks := 0; err == nil && r.State == "running" && ticks < 10; ticks++ {
			_, err = e.TickBatch(deadline, 100)
			if err == nil {
				r, err = e.GetRun(deadline, "t", "p", r.ID)
			}
		}
		b.StopTimer()
		cancel()
		schedulingReportBench(b, s, err)
		if err == nil && r.State != "succeeded" {
			b.Fatal("bounded propagation did not finish dense graph", r.State)
		}
	}
}

func TestSchedulingIndexQueryPlans(t *testing.T) {
	e, _ := schedulingMeasuredEngine(t)
	queries := []struct {
		name, sql string
		args      []any
	}{
		{"due_wake", `SELECT run_id,node_id FROM dag_wakeups WHERE state='pending' AND due_at<=? ORDER BY due_at,run_id,node_id,kind LIMIT ?`, []any{int64(1), 100}},
		{"ready_scope", `SELECT run_id,node_id FROM dag_ready WHERE tenant_id=? AND project_id=? AND sequence>? ORDER BY sequence,run_id,node_id LIMIT ?`, []any{"t", "p", int64(0), 50}},
		{"run_turns", `SELECT run_id,last_sequence,first_ready_sequence FROM dag_scheduler_run_turns WHERE tenant_id=? AND project_id=? AND enabled=1 ORDER BY last_sequence,first_ready_sequence,run_id LIMIT 1`, []any{"t", "p"}},
		{"node_work", `SELECT node_id FROM dag_node_work WHERE run_id=? ORDER BY sequence,node_id LIMIT 1`, []any{"r"}},
		{"outbound_edges", `SELECT target_id,condition_value FROM dag_graph_edges WHERE run_id=? AND source_id=? ORDER BY target_id`, []any{"r", "n"}},
	}
	for _, q := range queries {
		rows, err := e.db.Query("EXPLAIN QUERY PLAN "+q.sql, q.args...)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var a, b, c int
			var detail string
			if err = rows.Scan(&a, &b, &c, &detail); err != nil {
				t.Fatal(err)
			}
			t.Logf("%s: %s", q.name, detail)
		}
		if err = rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
}

func schedulingReportBench(b *testing.B, s *schedulingSQLStats, err error) {
	b.Helper()
	metrics := s.snapshot()
	raw, _ := json.Marshal(metrics)
	b.Logf("actual_driver_metrics=%s", raw)
	for key, value := range metrics {
		b.ReportMetric(float64(value), key+"/op")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		b.Log("case timeout: elapsed lower bound only, not a successful product result")
		b.SkipNow()
	}
	if err != nil {
		b.Fatal(err)
	}
}
