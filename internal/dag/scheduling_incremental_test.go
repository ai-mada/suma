package dag

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestIncrementalCompletionDoesNotLoadOrRewriteUnrelatedThousandNodes(t *testing.T) {
	e, stats := schedulingMeasuredEngine(t)
	w, err := e.Publish(ctx, "t", "p", schedulingBenchWorkflow(1000, false, "command"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CreateRun(ctx, "t", "p", w.ID, "create"); err != nil {
		t.Fatal(err)
	}
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:measure"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	stats.reset()
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "complete"}); err != nil {
		t.Fatal(err)
	}
	actual := stats.snapshot()
	if actual["node_update_rows"] != 1 || actual["rows_read"] >= 100 {
		t.Fatalf("single completion touched unrelated graph: %v", actual)
	}
}

func TestGetRunIsReadOnlyAndDoesNotTakeGlobalSchedulerWriteLock(t *testing.T) {
	e, stats := schedulingMeasuredEngine(t)
	w, err := e.Publish(ctx, "t", "p", schedulingBenchWorkflow(3, false, "command"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", w.ID, "create")
	if err != nil {
		t.Fatal(err)
	}
	stats.reset()
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Version != r.Version {
		t.Fatal(current, err)
	}
	if actual := stats.snapshot(); actual["global_lock_update_rows"] != 0 || actual["node_update_rows"] != 0 || actual["run_update_rows"] != 0 {
		t.Fatal("read acquired mutation lock or changed state", actual)
	}
}

func TestMapManifestPersistsBoundedChunksAcrossRestart(t *testing.T) {
	items := make([]any, 300)
	for i := range items {
		items[i] = i
	}
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "map", Type: "map", Config: map[string]any{"items": items}, Command: []string{"echo", "{{item}}"}}}})
	var count, next, total int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND parent_id='map'`, r.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 128 {
		t.Fatalf("initial materialization must be bounded: %d", count)
	}
	if err := e.db.QueryRow(`SELECT next_index,total FROM dag_map_expansions WHERE run_id=? AND node_id='map'`, r.ID).Scan(&next, &total); err != nil {
		t.Fatal(err)
	}
	if next != 128 || total != 300 {
		t.Fatalf("manifest %d/%d", next, total)
	}
	reopened, err := New(e.db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err = reopened.TickBatch(ctx, 100); err != nil {
			t.Fatal(err)
		}
	}
	if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_nodes WHERE run_id=? AND parent_id='map'`, r.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 300 {
		t.Fatalf("restart lost or duplicated map entries: %d", count)
	}
	var closed int
	if err = e.db.QueryRow(`SELECT closed FROM dag_map_progress WHERE run_id=? AND node_id='map'`, r.ID).Scan(&closed); err != nil || closed != 1 {
		t.Fatal(closed, err)
	}
}

func TestDefaultJoinAcceptsOnlySelectedSuccessfulBranches(t *testing.T) {
	w := Workflow{Nodes: []Node{{ID: "route", Type: "condition", Branches: map[string]string{"east": "true", "west": "false"}}, {ID: "east", Type: "command", Command: []string{"east"}}, {ID: "west", Type: "command", Command: []string{"west"}}, {ID: "merge", Type: "noop", DependsOn: []string{"east", "west"}}}, Edges: []Edge{{From: "route", To: "east", Condition: "east"}, {From: "route", To: "west", Condition: "west"}}}
	e, r := fixture(t, w)
	a, err := e.Claim(ctx, "t", "p", "w", []string{"command:east"})
	if err != nil || a == nil || a.NodeID != "east" {
		t.Fatal(a, err)
	}
	done(t, e, a, 0)
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range current.Nodes {
		if node.ID == "merge" && node.State != "succeeded" {
			t.Fatal("unselected branch prevented default convergence", node)
		}
	}
}

func TestFutureWakeTickDoesNotLoadUnrelatedThousandNodes(t *testing.T) {
	e, stats := schedulingMeasuredEngine(t)
	w, err := e.Publish(ctx, "t", "p", schedulingBenchWorkflow(1000, false, "timer"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.CreateRun(ctx, "t", "p", w.ID, "create"); err != nil {
		t.Fatal(err)
	}
	stats.reset()
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if actual := stats.snapshot(); actual["rows_read"] >= 100 || actual["node_update_rows"] != 0 || actual["run_update_rows"] != 0 {
		t.Fatal("future wake scan touched unrelated graph", actual)
	}
}

func TestPausedExpiredTimerDoesNotChurnRunVersion(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "timer", Type: "timer", Config: map[string]any{"duration_seconds": 1}}}})
	paused, err := e.Command(ctx, "t", "p", r.ID, "pause", "pause", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Version != paused.Version || current.State != "paused" {
		t.Fatal("paused timer repeatedly mutated run", paused.Version, current.Version, current.State, err)
	}
}

func TestTickFailureDoesNotRollBackAnotherDueRun(t *testing.T) {
	e, _ := schedulingMeasuredEngine(t)
	w, err := e.Publish(ctx, "t", "p", Workflow{ID: "timers", Name: "due timers", Nodes: []Node{{ID: "timer", Type: "timer", Config: map[string]any{"duration_seconds": 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	bad, err := e.CreateRun(ctx, "t", "p", w.ID, "bad")
	if err != nil {
		t.Fatal(err)
	}
	good, err := e.CreateRun(ctx, "t", "p", w.ID, "good")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec("UPDATE dag_nodes SET spec='{' WHERE run_id=?", bad.ID); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	_ = e.Tick(context.Background())
	current, err := e.GetRun(ctx, "t", "p", good.ID)
	if err != nil || current.State != "succeeded" {
		t.Fatal("one bad wake rolled back healthy run", current.State, err)
	}
}

func TestAnySuccessWinnerIsPersistedAndDoesNotChangeAfterLateSibling(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "command", Command: []string{"work"}}, {ID: "b", Type: "command", Command: []string{"work"}}, {ID: "join", Type: "command", Command: []string{"join"}, Join: "any_success", DependsOn: []string{"a", "b"}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	b := claim(t, e)
	if a == nil || b == nil {
		t.Fatal("parallel source fixture requires capacity", a, b)
	}
	done(t, e, a, 0)
	assertWinner := func() {
		t.Helper()
		current, err := e.GetRun(ctx, "t", "p", r.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(current)
		var out map[string]any
		if json.Unmarshal(raw, &out) != nil {
			t.Fatal("invalid public run")
		}
		for _, item := range out["nodes"].([]any) {
			n := item.(map[string]any)
			if n["id"] == "join" {
				w, ok := n["winner_node_ids"].([]any)
				if !ok || len(w) != 1 || w[0] != a.NodeID {
					t.Fatal("winner missing or changed", n)
				}
			}
		}
	}
	assertWinner()
	done(t, e, b, 0)
	assertWinner()
}
