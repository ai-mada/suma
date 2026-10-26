package dag

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func managedBatchRequests(count int) []ManagedRunRequest {
	items := make([]ManagedRunRequest, count)
	for i := range items {
		items[i] = ManagedRunRequest{RunID: fmt.Sprintf("batch-run-%02d", i), Workflow: Workflow{Name: fmt.Sprintf("Task %d", i), Nodes: []Node{{ID: "work", Type: "command", Command: []string{"test-command"}, Config: map[string]any{"goal": "frozen"}}}}}
	}
	return items
}

func TestManagedBatchAtomicRootsRoutesAndReplay(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(map[bool]string{false: "private", true: "pool"}[pool], func(t *testing.T) {
			e := openEngine(t, filepath.Join(t.TempDir(), "batch.db"))
			if pool {
				tx, _ := e.db.Begin()
				if _, err := e.WritePoolPolicyTx(ctx, tx, "pool", 0, PoolPolicyInput{ConcurrencyLimit: 2, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{}}); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
				configurePoolScope(t, e, "pool", "t", "p", 1)
			}
			items := managedBatchRequests(2)
			beforeCalls, binds, commits := 0, 0, 0
			allow := true
			denied := errors.New("current authority revoked")
			var transaction *sql.Tx
			before := func(tx *sql.Tx) error {
				beforeCalls++
				transaction = tx
				if !allow {
					return denied
				}
				return nil
			}
			bind := func(tx *sql.Tx, id string) error {
				binds++
				if tx != transaction || id != items[binds-1].RunID {
					t.Fatal("bind left its original transaction/order", id)
				}
				// The before/bind callbacks cannot alter later prepared workflows.
				items[1].Workflow.Nodes[0].Config["goal"] = "caller mutation"
				return nil
			}
			commit := func(tx *sql.Tx, runs []Run) error {
				commits++
				if tx != transaction || len(runs) != 2 || runs[0].ID != items[0].RunID || runs[1].ID != items[1].RunID {
					t.Fatal("commit did not receive the ordered original batch")
				}
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count); err != nil || count != 2 {
					t.Fatal("final callback preceded complete batch creation", count, err)
				}
				runs[0].ID = "commit-local mutation"
				return nil
			}
			runs, err := e.CreateManagedRunsBound(ctx, "t", "p", "start-batch-original", items, before, bind, commit)
			if err != nil || len(runs) != 2 || runs[0].ID != items[0].RunID {
				t.Fatal("batch failed or commit mutated return facts", runs, err)
			}
			for _, run := range runs {
				var root string
				if err = e.db.QueryRow(`SELECT root_run_id FROM dag_run_families WHERE run_id=?`, run.ID).Scan(&root); err != nil || root != run.ID {
					t.Fatal("independent task inherited a prior family root", run.ID, root, err)
				}
				w, readErr := e.GetWorkflow(ctx, "t", "p", run.WorkflowID, 1)
				if readErr != nil || w.Nodes[0].Config["goal"] != "frozen" || run.Nodes[0].State != "ready" {
					t.Fatal("workflow/queue input drifted", readErr)
				}
				if pool != (run.ExecutionPoolID == "pool") {
					t.Fatal("batch route changed or inherited incorrectly", run.ExecutionPoolID)
				}
				if managed, readErr := e.ManagedRun(ctx, "t", "p", run.ID); readErr != nil || !managed {
					t.Fatal("batch exposed an ordinary reusable workflow", readErr)
				}
				if _, readErr := e.CreateRun(ctx, "t", "p", run.WorkflowID, "forbidden-public-reuse"); !errors.Is(readErr, ErrInvalid) {
					t.Fatal("managed workflow reused publicly", readErr)
				}
			}
			var count int
			if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger`).Scan(&count); err != nil || count != 0 {
				t.Fatal("pending batch consumed execution resources", count, err)
			}
			items[1].Workflow.Nodes[0].Config["goal"] = "frozen"
			if pool {
				tx, _ := e.db.Begin()
				if _, err := e.WriteProjectExecutionRouteTx(ctx, tx, "t", "p", 1, ProjectExecutionRouteInput{}); err != nil {
					t.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
					t.Fatal(err)
				}
			}
			again, err := e.CreateManagedRunsBound(ctx, "t", "p", "start-batch-original", items, before, bind, commit)
			if err != nil || len(again) != 2 || again[0].ID != runs[0].ID || again[1].ID != runs[1].ID || binds != 2 || commits != 1 || beforeCalls != 2 {
				t.Fatal("receipt replay recreated batch effects", again, err, binds, commits, beforeCalls)
			}
			if pool && (again[0].ExecutionPoolID != "pool" || again[1].ExecutionPoolID != "pool") {
				t.Fatal("replay replaced immutable routes with the new project route")
			}
			allow = false
			if _, err = e.CreateManagedRunsBound(ctx, "t", "p", "start-batch-original", items, before, bind, commit); !errors.Is(err, denied) {
				t.Fatal("receipt bypassed current authority", err)
			}
			allow = true
			items[0], items[1] = items[1], items[0]
			if _, err = e.CreateManagedRunsBound(ctx, "t", "p", "start-batch-original", items, before, bind, commit); !errors.Is(err, ErrConflict) {
				t.Fatal("same operation accepted a reordered request", err)
			}
		})
	}
}

func managedBatchCounts(t *testing.T, e *Engine) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, table := range []string{"dag_workflows", "dag_managed_workflows", "dag_runs", "dag_nodes", "dag_run_families", "dag_run_execution_routes", "dag_ready_v2", "dag_operations", "dag_events", "dag_resource_ledger", "batch_domain_fixture"} {
		var count int
		if err := e.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		out[table] = count
	}
	return out
}

func TestManagedBatchFailuresRollBackEveryRunAndDomainEffect(t *testing.T) {
	for _, point := range []string{"before", "bind", "commit", "references", "occupied_id"} {
		t.Run(point, func(t *testing.T) {
			e := openEngine(t, filepath.Join(t.TempDir(), "rollback.db"))
			if _, err := e.db.Exec(`CREATE TABLE batch_domain_fixture(id TEXT PRIMARY KEY)`); err != nil {
				t.Fatal(err)
			}
			items := managedBatchRequests(2)
			if point == "references" {
				items[1].Workflow.Nodes = []Node{{ID: "child", Type: "subworkflow", Config: map[string]any{"workflow_id": "missing", "workflow_version": 1}}}
			}
			if point == "occupied_id" {
				if _, err := e.CreateManagedRunBound(ctx, "t", "p", items[1].RunID, "existing-reserved-run", items[1].Workflow, func(*sql.Tx) error { return nil }, func(*sql.Tx, string) error { return nil }); err != nil {
					t.Fatal(err)
				}
			}
			baseline := managedBatchCounts(t, e)
			beforeCalls, binds, commits := 0, 0, 0
			refused := errors.New("batch domain refused")
			before := func(tx *sql.Tx) error {
				beforeCalls++
				if _, err := tx.Exec(`INSERT INTO batch_domain_fixture(id)VALUES('before')`); err != nil {
					return err
				}
				if point == "before" {
					return refused
				}
				return nil
			}
			bind := func(tx *sql.Tx, id string) error {
				binds++
				if _, err := tx.Exec(`INSERT INTO batch_domain_fixture(id)VALUES(?)`, id); err != nil {
					return err
				}
				if point == "bind" && binds == 2 {
					return refused
				}
				return nil
			}
			commit := func(tx *sql.Tx, runs []Run) error {
				commits++
				if _, err := tx.Exec(`INSERT INTO batch_domain_fixture(id)VALUES('commit')`); err != nil {
					return err
				}
				return refused
			}
			out, err := e.CreateManagedRunsBound(ctx, "t", "p", "rollback-whole-batch", items, before, bind, commit)
			if err == nil || out != nil || beforeCalls != 1 {
				t.Fatal("failed batch exposed partial results or repeated authority", out, err, beforeCalls)
			}
			if point == "bind" && (binds != 2 || commits != 0) || point == "commit" && (binds != 2 || commits != 1) {
				t.Fatal("failure occurred outside requested boundary", binds, commits)
			}
			for table, count := range managedBatchCounts(t, e) {
				if count != baseline[table] {
					t.Fatal("batch rollback left durable state", table, count, baseline[table])
				}
			}
		})
	}
}

func TestManagedBatchLimitsAndReceiptIntegrity(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "limits.db"))
	beforeCalls := 0
	before := func(*sql.Tx) error { beforeCalls++; return nil }
	bind := func(*sql.Tx, string) error { return nil }
	commit := func(*sql.Tx, []Run) error { return nil }
	for _, name := range []string{"empty", "seventeen", "duplicate", "long_id", "bad_workflow", "unserializable", "total_nodes", "nil_before", "nil_bind", "nil_commit"} {
		t.Run(name, func(t *testing.T) {
			items, currentBefore, currentBind, currentCommit := managedBatchRequests(2), before, bind, commit
			switch name {
			case "empty":
				items = nil
			case "seventeen":
				items = managedBatchRequests(17)
			case "duplicate":
				items[1].RunID = items[0].RunID
			case "long_id":
				items[1].RunID = fmt.Sprintf("%093d", 1)
			case "bad_workflow":
				items[1].Workflow.Nodes[0].Type = "invented"
			case "unserializable":
				items[1].Workflow.Nodes[0].Config["invalid"] = make(chan int)
			case "nil_before":
				currentBefore = nil
			case "nil_bind":
				currentBind = nil
			case "nil_commit":
				currentCommit = nil
			case "total_nodes":
				items = managedBatchRequests(11)
				for i := range items {
					count := 1000
					if i == 10 {
						count = 1
					}
					items[i].Workflow.Nodes = make([]Node, count)
					for n := range items[i].Workflow.Nodes {
						items[i].Workflow.Nodes[n] = Node{ID: fmt.Sprintf("n%04d", n), Type: "command", Command: []string{"test-command"}}
					}
				}
			}
			if out, err := e.CreateManagedRunsBound(ctx, "t", "p", "invalid-batch", items, currentBefore, currentBind, currentCommit); !errors.Is(err, ErrInvalid) || out != nil || beforeCalls != 0 {
				t.Fatal("invalid batch reached authority/DB", name, out, err, beforeCalls)
			}
		})
	}
	items := managedBatchRequests(16)
	runs, err := e.CreateManagedRunsBound(ctx, "t", "p", "maximum-valid-batch", items, before, bind, commit)
	if err != nil || len(runs) != 16 || beforeCalls != 1 {
		t.Fatal("maximum bounded batch failed", len(runs), err, beforeCalls)
	}
	if _, err = e.db.Exec(`DELETE FROM dag_operations WHERE scope='managed_runs:1' AND operation_id='maximum-valid-batch'`); err != nil {
		t.Fatal(err)
	}
	if _, err = e.CreateManagedRunsBound(ctx, "t", "p", "maximum-valid-batch", items, before, func(*sql.Tx, string) error { t.Fatal("incomplete receipt rebound a run"); return nil }, func(*sql.Tx, []Run) error { t.Fatal("incomplete receipt committed another batch"); return nil }); !errors.Is(err, ErrConflict) {
		t.Fatal("incomplete receipt silently created missing work", err)
	}
	var count int
	if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count); err != nil || count != 16 {
		t.Fatal("receipt recovery created replacement runs", count, err)
	}
}

func TestManagedBatchConcurrentConnectionsRecoverOriginalRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent.db")
	e, second := openEngine(t, path), openEngine(t, path)
	if _, err := e.db.Exec(`CREATE TABLE batch_domain_fixture(id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`CREATE TABLE batch_authority_fixture(id INTEGER PRIMARY KEY,sequence INTEGER NOT NULL); INSERT INTO batch_authority_fixture(id,sequence)VALUES(1,0)`); err != nil {
		t.Fatal(err)
	}
	var beforeCalls, bindCalls, commitCalls atomic.Int32
	before := func(tx *sql.Tx) error {
		// Match the platform authorityWriter boundary: acquire current authority
		// coordination before reading scheduler state, not a counter-only stub.
		beforeCalls.Add(1)
		_, err := tx.Exec(`UPDATE batch_authority_fixture SET sequence=sequence+1 WHERE id=1`)
		return err
	}
	bind := func(tx *sql.Tx, id string) error {
		bindCalls.Add(1)
		_, err := tx.Exec(`INSERT INTO batch_domain_fixture(id)VALUES(?)`, id)
		return err
	}
	commit := func(tx *sql.Tx, runs []Run) error {
		commitCalls.Add(1)
		_, err := tx.Exec(`INSERT INTO batch_domain_fixture(id)VALUES('commit')`)
		return err
	}
	items := managedBatchRequests(2)
	type result struct {
		runs []Run
		err  error
	}
	results := make(chan result, 2)
	for _, engine := range []*Engine{e, second} {
		go func(engine *Engine) {
			runs, err := engine.CreateManagedRunsBound(ctx, "t", "p", "concurrent-whole-batch", items, before, bind, commit)
			results <- result{runs, err}
		}(engine)
	}
	for range 2 {
		value := <-results
		if value.err != nil || len(value.runs) != 2 || value.runs[0].ID != items[0].RunID || value.runs[1].ID != items[1].RunID {
			t.Fatal("concurrent retry lost original ordered runs", value.err)
		}
	}
	if beforeCalls.Load() != 2 || bindCalls.Load() != 2 || commitCalls.Load() != 1 {
		t.Fatal("concurrent operation repeated domain side effects", beforeCalls.Load(), bindCalls.Load(), commitCalls.Load())
	}
	var count int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_runs`).Scan(&count); err != nil || count != 2 {
		t.Fatal("concurrent batch created duplicates", count, err)
	}
}
