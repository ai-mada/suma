package dag

import (
	"context"
	"database/sql"
	"errors"
	"github.com/ai-mada/suma/internal/store"
	"path/filepath"
	"sync"
	"testing"
)

var ctx = context.Background()

func openEngine(t *testing.T, path string) *Engine {
	t.Helper()
	db, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e, err := New(db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func fixture(t *testing.T, w Workflow) (*Engine, Run) {
	t.Helper()
	e := openEngine(t, filepath.Join(t.TempDir(), "dag.db"))
	w.ID = "wf"
	w.Name = "workflow"
	if _, err := e.Publish(ctx, "t", "p", w); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil {
		t.Fatal(err)
	}
	return e, r
}
func done(t *testing.T, e *Engine, a *Assignment, code int) {
	t.Helper()
	if a == nil {
		t.Fatal("missing assignment")
	}
	err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: code, OperationID: a.NodeID + "-done"})
	if err != nil {
		t.Fatal(err)
	}
}
func claim(t *testing.T, e *Engine) *Assignment {
	t.Helper()
	a, err := e.Claim(ctx, "t", "p", "worker", testCapabilities(t, e))
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func TestCompilerRejectsCycleAndUnsafeRetry(t *testing.T) {
	for _, w := range []Workflow{{Nodes: []Node{{ID: "a", Type: "task", DependsOn: []string{"b"}}, {ID: "b", Type: "task", DependsOn: []string{"a"}}}}, {Nodes: []Node{{ID: "a", Type: "task", MaxRetries: 1}}}} {
		if Compile(w) == nil {
			t.Fatal("invalid workflow accepted")
		}
	}
}
func TestDiamondRunsBranchesInParallel(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task", DependsOn: []string{"a"}}, {ID: "c", Type: "task", DependsOn: []string{"a"}}, {ID: "d", Type: "task", DependsOn: []string{"b", "c"}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	done(t, e, claim(t, e), 0)
	b, c := claim(t, e), claim(t, e)
	if b == nil || c == nil || b.NodeID == c.NodeID {
		t.Fatal("branches not independently claimable")
	}
	done(t, e, b, 0)
	if claim(t, e) != nil {
		t.Fatal("join ran early")
	}
	done(t, e, c, 0)
	done(t, e, claim(t, e), 0)
	r, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || r.State != "succeeded" {
		t.Fatalf("run=%+v err=%v", r, err)
	}
}
func TestClaimOnceAndTenantIsolation(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	var wg sync.WaitGroup
	var mu sync.Mutex
	count := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := e.Claim(ctx, "t", "p", "worker", testCapabilities(t, e))
			if err != nil {
				t.Error(err)
			}
			if a != nil {
				mu.Lock()
				count++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if count != 1 {
		t.Fatalf("claims %d", count)
	}
	if _, err := e.GetRun(ctx, "other", "p", r.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("isolation err %v", err)
	}
}
func TestDuplicateResultAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dag.db")
	e := openEngine(t, path)
	_, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "wf", Nodes: []Node{{ID: "a", Type: "task"}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	done(t, e, a, 0)
	done(t, e, a, 0)
	other := openEngine(t, path)
	loaded, err := other.GetRun(ctx, "t", "p", r.ID)
	if err != nil || loaded.State != "succeeded" {
		t.Fatalf("persisted run %+v %v", loaded, err)
	}
	r2, err := other.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil || r2.ID != r.ID {
		t.Fatal("creation not idempotent")
	}
	a.Fence++
	if err := other.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Fence: a.Fence, Attempt: a.Attempt, WorkerID: a.WorkerID, OperationID: "late"}); !errors.Is(err, ErrStale) {
		t.Fatalf("late result %v", err)
	}
}
func TestConditionSkipAndApprovalVersion(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Condition: "false"}, {ID: "approval", Type: "approval", DependsOn: []string{"a"}, Join: "all_done"}, {ID: "end", Type: "task", DependsOn: []string{"approval"}}}, Edges: []Edge{{From: "a", To: "approval", Condition: "all_done"}}})
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.Nodes[0].State != "skipped" {
		t.Fatal(r.Nodes)
	}
	if _, err := e.Command(ctx, "t", "p", r.ID, "approval", "wrong-version", r.Version+1, map[string]any{"node_id": "approval", "decision": "approve"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if _, err := e.Command(ctx, "t", "p", r.ID, "approval", "approve", r.Version, map[string]any{"node_id": "approval", "decision": "approve"}); err != nil {
		t.Fatal(err)
	}
	done(t, e, claim(t, e), 0)
}

func TestDynamicMapPersistsExpansionAndJoin(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "map", Type: "map", Command: []string{"echo", "{{item}}"}, Config: map[string]any{"items": []any{"first", "second"}}}, {ID: "end", Type: "task", DependsOn: []string{"map"}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	a, b := claim(t, e), claim(t, e)
	if a == nil || b == nil || a.NodeID == b.NodeID || a.Command[1] == b.Command[1] {
		t.Fatalf("bad map children %+v %+v", a, b)
	}
	if claim(t, e) != nil {
		t.Fatal("map join ran early")
	}
	done(t, e, a, 0)
	done(t, e, b, 0)
	last := claim(t, e)
	if last == nil || last.NodeID != "end" {
		t.Fatal(last)
	}
	done(t, e, last, 0)
	run, _ := e.GetRun(ctx, "t", "p", r.ID)
	if run.State != "succeeded" || len(run.Nodes) != 4 {
		t.Fatalf("run %+v", run)
	}
}
func TestLeaseLossRetainsResourceUntilReconciled(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Resources: map[string]int64{"cpu": 1}}, {ID: "b", Type: "task", Resources: map[string]int64{"cpu": 1}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"cpu": 1}); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if claim(t, e) != nil {
		t.Fatal("resource oversubscribed")
	}
	if _, err := e.db.Exec("UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?", r.ID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec("UPDATE dag_wakeups SET due_at=0 WHERE run_id=? AND node_id=? AND kind='lease'", r.ID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if claim(t, e) != nil {
		t.Fatal("unknown work released resource")
	}
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "late"}); !errors.Is(err, ErrStale) {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "reconcile", "reconcile", r.Version, map[string]any{"node_id": a.NodeID, "outcome": "not_started", "reason": "executor inspection confirms no process started"})
	if err != nil {
		t.Fatal(err)
	}
	next := claim(t, e)
	if next == nil {
		t.Fatal("resource not released after proof")
	}
}
func TestCancellationWaitsForStopProof(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}}})
	a := claim(t, e)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err := e.Command(ctx, "t", "p", r.ID, "cancel", "cancel", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.State != "cancelling" {
		t.Fatalf("false stop claim %s", r.State)
	}
	if err = e.Heartbeat(ctx, "t", "p", *a); !errors.Is(err, ErrCancelled) {
		t.Fatal(err)
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, Stopped: true, OperationID: "stop"}); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "cancelled" {
		t.Fatal(r.State)
	}
}
func TestBudgetAndCapabilitiesAdmission(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Resources: map[string]int64{"budget": 2}, Config: map[string]any{"capability": "shell"}}, {ID: "b", Type: "task", Resources: map[string]int64{"budget": 2}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"budget": 2}); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if a == nil || a.NodeID != "b" {
		t.Fatal(a)
	}
	done(t, e, a, 0)
	a, err := e.Claim(ctx, "t", "p", "worker", []string{"shell"})
	if err != nil || a != nil {
		t.Fatalf("budget admission %+v %v", a, err)
	}
}
func TestPublishedVersionIsPinnedAndImmutable(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Command: []string{"old"}}}})
	_, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "wf", Version: 1, Nodes: []Node{{ID: "a", Type: "task"}}})
	if !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	_, err = e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "wf", Nodes: []Node{{ID: "a", Type: "task", Command: []string{"new"}}}})
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if a.Command[0] != "old" {
		t.Fatal("run definition mutated")
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.WorkflowVersion != 1 {
		t.Fatal(r.WorkflowVersion)
	}
}

func TestCompilerRejectsInvalidConditionAndMapTemplate(t *testing.T) {
	for _, w := range []Workflow{{Nodes: []Node{{ID: "a", Type: "task", Condition: "missing.state == succeeded"}}}, {Nodes: []Node{{ID: "a", Type: "map", Config: map[string]any{"items": []any{1}, "template": map[string]any{"type": "task", "max_retries": 1}}}}}, {Nodes: []Node{{ID: "a", Type: "timer", Config: map[string]any{"duration_seconds": -1}}}}} {
		if Compile(w) == nil {
			t.Fatalf("invalid workflow accepted %+v", w)
		}
	}
}
func TestBranchSelectionSkipsUnselectedPath(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "route", Type: "condition", Branches: map[string]string{"east": "true", "west": "false"}}, {ID: "east", Type: "task"}, {ID: "west", Type: "task"}, {ID: "end", Type: "join", DependsOn: []string{"east", "west"}, Join: "all_done"}}, Edges: []Edge{{From: "route", To: "east", Condition: "east"}, {From: "route", To: "west", Condition: "west"}}})
	a := claim(t, e)
	if a == nil || a.NodeID != "east" {
		t.Fatalf("wrong branch %+v", a)
	}
	done(t, e, a, 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "succeeded" {
		t.Fatalf("branch join failed %+v", r)
	}
}

func TestManualRetryOfMapChildReopensParentAndSkippedDescendants(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "map", Type: "map", Idempotent: true, Config: map[string]any{"items": []any{"item"}}}, {ID: "end", Type: "task", DependsOn: []string{"map"}}}})
	a := claim(t, e)
	done(t, e, a, 1)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "failed" {
		t.Fatal(r.State)
	}
	r, err := e.Command(ctx, "t", "p", r.ID, "retry", "retry-child", r.Version, map[string]any{"node_id": a.NodeID})
	if err != nil {
		t.Fatal(err)
	}
	a = claim(t, e)
	if a == nil {
		t.Fatal("no retry")
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "retry-done"}); err != nil {
		t.Fatal(err)
	}
	last := claim(t, e)
	if last == nil || last.NodeID != "end" {
		t.Fatalf("map parent not reopened %+v", last)
	}
	done(t, e, last, 0)
}
func TestAutomaticRetryAndAnySuccessJoin(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "retry", Type: "task", Idempotent: true, MaxRetries: 1}, {ID: "fail", Type: "task"}, {ID: "join", Type: "task", DependsOn: []string{"retry", "fail"}, Join: "any_success"}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	a, b := claim(t, e), claim(t, e)
	var retry, fail *Assignment
	if a.NodeID == "retry" {
		retry, fail = a, b
	} else {
		retry, fail = b, a
	}
	done(t, e, fail, 1)
	if err := e.Complete(ctx, "t", "p", Result{RunID: retry.RunID, NodeID: retry.NodeID, Attempt: retry.Attempt, Fence: retry.Fence, WorkerID: retry.WorkerID, ExitCode: 1, FailureClass: "transient", OperationID: "transient-fail"}); err != nil {
		t.Fatal(err)
	}
	if claim(t, e) != nil {
		t.Fatal("retry bypassed persisted backoff")
	}
	if _, err := e.db.Exec("UPDATE dag_nodes SET wake_at=1 WHERE run_id=? AND id=?", retry.RunID, retry.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec("UPDATE dag_wakeups SET due_at=1 WHERE run_id=? AND node_id=? AND kind='retry'", retry.RunID, retry.NodeID); err != nil {
		t.Fatal(err)
	}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	next := claim(t, e)
	if next == nil || next.NodeID != "retry" || next.Attempt != 2 || next.Fence <= retry.Fence {
		t.Fatalf("retry fence %+v", next)
	}
	if err := e.Complete(ctx, "t", "p", Result{RunID: next.RunID, NodeID: next.NodeID, Attempt: next.Attempt, Fence: next.Fence, WorkerID: next.WorkerID, OperationID: "retry-success"}); err != nil {
		t.Fatal(err)
	}
	join := claim(t, e)
	if join == nil || join.NodeID != "join" {
		t.Fatal("any success join did not run")
	}
	done(t, e, join, 0)
}
func TestPauseDoesNotConsumeReadyAndTimerDoesNotClaim(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "timer", Type: "timer", Config: map[string]any{"duration_seconds": 10}}, {ID: "task", Type: "task", DependsOn: []string{"timer"}}}})
	if claim(t, e) != nil {
		t.Fatal("timer claimed worker")
	}
	r, err := e.Command(ctx, "t", "p", r.ID, "pause", "pause", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec("UPDATE dag_nodes SET wake_at=0 WHERE run_id=? AND id='timer'", r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec("UPDATE dag_wakeups SET due_at=0 WHERE run_id=? AND node_id='timer' AND kind='timer'", r.ID); err != nil {
		t.Fatal(err)
	}
	if err = e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if claim(t, e) != nil {
		t.Fatal("paused claim")
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	r, err = e.Command(ctx, "t", "p", r.ID, "resume", "resume", r.Version, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if a == nil || a.NodeID != "task" {
		t.Fatal(a)
	}
}

func TestIndependentEnginesClaimSameDatabaseOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shared.db")
	e := openEngine(t, path)
	other := openEngine(t, path)
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "shared", Nodes: []Node{{ID: "a", Type: "task"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CreateRun(ctx, "t", "p", "wf", "create"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	claims := make(chan *Assignment, 2)
	errs := make(chan error, 2)
	for _, engine := range []*Engine{e, other} {
		wg.Add(1)
		go func(engine *Engine) {
			defer wg.Done()
			a, err := engine.Claim(ctx, "t", "p", "worker", []string{"task"})
			claims <- a
			errs <- err
		}(engine)
	}
	wg.Wait()
	close(claims)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for a := range claims {
		if a != nil {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("cross engine claims=%d", count)
	}
}
func TestQuorumWaitsForThreshold(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}, {ID: "c", Type: "task"}, {ID: "join", Type: "task", Join: "quorum", Quorum: 2, DependsOn: []string{"a", "b", "c"}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 3}); err != nil {
		t.Fatal(err)
	}
	a, b, c := claim(t, e), claim(t, e), claim(t, e)
	done(t, e, a, 0)
	if claim(t, e) != nil {
		t.Fatal("quorum fired early")
	}
	done(t, e, b, 0)
	j := claim(t, e)
	if j == nil || j.NodeID != "join" {
		t.Fatal("threshold did not fire")
	}
	done(t, e, j, 0)
	done(t, e, c, 1)
}
func TestUnknownResultRetainsReservationUntilStopped(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Resources: map[string]int64{"cpu": 1}}, {ID: "b", Type: "task", Resources: map[string]int64{"cpu": 1}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"cpu": 1}); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	result := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OutcomeUnknown: true, ExitCode: -1, OperationID: "unknown"}
	if err := e.Complete(ctx, "t", "p", result); err != nil {
		t.Fatal(err)
	}
	if claim(t, e) != nil {
		t.Fatal("unknown outcome released resources")
	}
	result.OutcomeUnknown = false
	result.Stopped = true
	result.OperationID = "stopped"
	if err := e.Complete(ctx, "t", "p", result); err != nil {
		t.Fatal(err)
	}
	if claim(t, e) == nil {
		t.Fatal("stop proof did not release resource")
	}
	events, err := e.Events(ctx, "t", "p", r.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var previous int64
	for _, event := range events {
		if event.Sequence <= previous {
			t.Fatal("event order")
		}
		previous = event.Sequence
	}
}

func TestSchedulingRotatesAcrossEligibleRuns(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}, {ID: "c", Type: "task"}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	r2, err := e.CreateRun(ctx, "t", "p", "wf", "second")
	if err != nil {
		t.Fatal(err)
	}
	a, b := claim(t, e), claim(t, e)
	if a == nil || b == nil || a.RunID != r.ID || b.RunID != r2.ID {
		t.Fatalf("run rotation %+v %+v", a, b)
	}
}
func TestMapExpansionSurvivesEngineRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "map.db")
	e := openEngine(t, path)
	_, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "map", Nodes: []Node{{ID: "map", Type: "map", Command: []string{"echo", "{{item}}"}, Config: map[string]any{"items": []any{"a", "b"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", "wf", "create")
	if err != nil {
		t.Fatal(err)
	}
	restarted := openEngine(t, path)
	done(t, restarted, claim(t, restarted), 0)
	done(t, restarted, claim(t, restarted), 0)
	loaded, err := restarted.GetRun(ctx, "t", "p", r.ID)
	if err != nil || loaded.State != "succeeded" || len(loaded.Nodes) != 3 {
		t.Fatalf("expanded state %+v %v", loaded, err)
	}
}
func TestAllDoneJoinsAfterFailureWithoutErasingFailure(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "fail", Type: "task"}, {ID: "cleanup", Type: "task", Join: "all_done", DependsOn: []string{"fail"}}}})
	done(t, e, claim(t, e), 1)
	cleanup := claim(t, e)
	if cleanup == nil || cleanup.NodeID != "cleanup" {
		t.Fatal("cleanup did not become ready")
	}
	done(t, e, cleanup, 0)
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.State != "failed" {
		t.Fatal("failure erased")
	}
}

func TestStoppedUnknownEffectStillRequiresReconciliation(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Idempotent: true, MaxRetries: 2}}})
	a := claim(t, e)
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "unknown-stopped", Stopped: true, OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if r.Nodes[0].State != "reconciling" || claim(t, e) != nil {
		t.Fatalf("unknown effect fabricated %+v", r)
	}
}

func TestCancellingStoppedUnknownEffectRemainsUnresolved(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "effect", Type: "command", Command: []string{"test-command"}}}})
	a := claim(t, e)
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Command(ctx, "t", "p", r.ID, "cancel", "cancel", current.Version, map[string]any{"reason": "stop fixture"}); err != nil {
		t.Fatal(err)
	}
	if err = e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "stopped-unknown", Stopped: true, OutcomeUnknown: true, ExitCode: -1}); err != nil {
		t.Fatal(err)
	}
	current, err = e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.State != "cancelling" || current.Nodes[0].State != "reconciling" {
		t.Fatalf("unknown effect became a confirmed cancellation: %+v %v", current, err)
	}
}
func TestMapTemplatePreservesCapabilityConfiguration(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "map", Type: "map", Config: map[string]any{"items": []any{1}, "template": map[string]any{"type": "agent", "command": []string{"test-command"}, "config": map[string]any{"capability": "model", "goal": "inspect"}}}}}})
	if a := claim(t, e); a != nil {
		t.Fatalf("map child lost capability %+v", a)
	}
	a, err := e.Claim(ctx, "t", "p", "worker", []string{"command:test-command", "model"})
	if err != nil || a == nil || a.Config["goal"] != "inspect" {
		t.Fatalf("map template config %+v %v", a, err)
	}
}

func TestPublishReceiptDoesNotCreateExtraVersion(t *testing.T) {
	e := openEngine(t, filepath.Join(t.TempDir(), "publish.db"))
	w := Workflow{Name: "published", Nodes: []Node{{ID: "a", Type: "task"}}}
	first, err := e.PublishWithOperation(ctx, "t", "p", w, "publish")
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.PublishWithOperation(ctx, "t", "p", w, "publish")
	if err != nil || first.ID != second.ID || second.Version != 1 {
		t.Fatalf("receipt changed %+v %+v %v", first, second, err)
	}
	items, err := e.Workflows(ctx, "t", "p")
	if err != nil || len(items) != 1 {
		t.Fatalf("published twice %+v %v", items, err)
	}
	w.Name = "different"
	if _, err = e.PublishWithOperation(ctx, "t", "p", w, "publish"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
func TestCreateRunUsesExplicitPublishedVersion(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Command: []string{"v1"}}}})
	if _, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "wf", Nodes: []Node{{ID: "a", Type: "task", Command: []string{"v2"}}}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRunVersion(ctx, "t", "p", "wf", 1, "pinned")
	if err != nil || r.WorkflowVersion != 1 || r.Nodes[0].Command[0] != "v1" {
		t.Fatalf("explicit pin ignored %+v %v", r, err)
	}
	if _, err = e.CreateRunVersion(ctx, "t", "p", "wf", 2, "pinned"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestRunBindingRollsBackAdmissionAndReplaysOnce(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	if _, err := e.db.Exec("CREATE TABLE binding_test (run_id VARCHAR(100) PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	reject := errors.New("task prerequisite incomplete")
	bind := func(tx *sql.Tx, id string) error {
		calls++
		if _, err := tx.ExecContext(ctx, "INSERT INTO binding_test (run_id) VALUES (?)", id); err != nil {
			return err
		}
		return reject
	}
	if _, err := e.CreateRunBound(ctx, "t", "p", "wf", 1, "bound", "task:1:v2", bind); !errors.Is(err, reject) {
		t.Fatal(err)
	}
	var count int
	if err := e.db.QueryRow("SELECT COUNT(*) FROM binding_test").Scan(&count); err != nil || count != 0 {
		t.Fatalf("binding leaked %d %v", count, err)
	}
	runs, err := e.Runs(ctx, "t", "p")
	if err != nil || len(runs) != 1 {
		t.Fatalf("rejected admission leaked %+v %v", runs, err)
	}
	reject = nil
	first, err := e.CreateRunBound(ctx, "t", "p", "wf", 1, "bound", "task:1:v2", bind)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.CreateRunBound(ctx, "t", "p", "wf", 1, "bound", "task:1:v2", bind)
	if err != nil || second.ID != first.ID || calls != 2 {
		t.Fatalf("binding repeated calls=%d %+v %v", calls, second, err)
	}
	if _, err = e.CreateRunBound(ctx, "t", "p", "wf", 1, "bound", "task:2:v2", bind); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestEarlyJoinWaitsForReferencedConditionAncestor(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}, {ID: "conditional", Type: "task", Join: "any_success", DependsOn: []string{"a", "b"}, Condition: "b.state == succeeded"}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2}); err != nil {
		t.Fatal(err)
	}
	a, b := claim(t, e), claim(t, e)
	if a.NodeID != "a" {
		a, b = b, a
	}
	done(t, e, a, 0)
	if claim(t, e) != nil {
		t.Fatal("predicate evaluated before ancestor outcome")
	}
	done(t, e, b, 0)
	n := claim(t, e)
	if n == nil || n.NodeID != "conditional" {
		t.Fatal(n)
	}
}

func TestMapTemplateConditionCannotDispatchDisabledChild(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "map", Type: "map", Config: map[string]any{"items": []any{1}, "template": map[string]any{"type": "task", "condition": "false"}}}}})
	if a := claim(t, e); a != nil {
		t.Fatalf("disabled map child dispatched %+v", a)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	for _, n := range r.Nodes {
		if n.ID == "map[0]" && n.State != "skipped" {
			t.Fatal(n)
		}
	}
}
