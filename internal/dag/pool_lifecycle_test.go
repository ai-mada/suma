package dag

import (
	"database/sql"
	"fmt"
	"sync"
	"testing"
)

func publishPoolTasks(t *testing.T, e *Engine, s PoolScope, id string, count int) Run {
	t.Helper()
	w := Workflow{ID: id, Name: id}
	for i := 0; i < count; i++ {
		w.Nodes = append(w.Nodes, Node{ID: fmt.Sprintf("n%03d", i), Type: "task"})
	}
	if _, err := e.Publish(ctx, s.TenantID, s.ProjectID, w); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, s.TenantID, s.ProjectID, id, "create-"+id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestPoolHierarchicalProjectWeightsAreIndependentOfTenantWeights(t *testing.T) {
	e, scopes := poolFixture(t)
	third := configurePoolScope(t, e, "pool", "ta", "pc", 2)
	publishPoolTasks(t, e, third, "third", 40)
	allowed := []PoolScope{scopes[0], third}
	counts := map[string]int{}
	for i := 0; i < 30; i++ {
		a := poolClaim(t, e, allowed, fmt.Sprintf("project-weight-%03d", i))
		if a == nil {
			t.Fatal(i)
		}
		counts[a.ProjectID]++
		if err := e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: fmt.Sprintf("done-project-%03d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if counts["pa"] != 10 || counts["pc"] != 20 {
		t.Fatal("project layer not weighted inside tenant", counts)
	}
}

func TestPoolLastCumulativeUnitIsAtomicAcrossConnections(t *testing.T) {
	e, scopes := poolFixture(t)
	one := int64(1)
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WritePoolPolicyTx(ctx, tx, "pool", 1, PoolPolicyInput{ConcurrencyLimit: 2, BudgetLimit: &one, TenantWeights: map[string]int{"ta": 1, "tb": 3}}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var path string
	e.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path)
	other := openEngine(t, path)
	var wg sync.WaitGroup
	results := make(chan *Assignment, 2)
	errs := make(chan error, 2)
	for i, engine := range []*Engine{e, other} {
		wg.Add(1)
		go func(i int, engine *Engine) {
			defer wg.Done()
			in := testPoolRequest(scopes, fmt.Sprintf("last-pool-unit-%d", i), []string{"task"})
			in.RunnerID = fmt.Sprintf("runner%d", i)
			a, err := engine.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil)
			results <- a
			errs <- err
		}(i, engine)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for a := range results {
		if a != nil {
			count++
			if err = e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "finish-last-pool-unit"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if count != 1 {
		t.Fatal("shared budget admitted more than final unit", count)
	}
	var budget, fair int
	e.db.QueryRow(`SELECT budget_spent FROM dag_execution_pool_policies WHERE pool_id='pool'`).Scan(&budget)
	e.db.QueryRow(`SELECT last_sequence FROM dag_execution_pool_fairness WHERE pool_id='pool' AND tenant_id='' AND project_id=''`).Scan(&fair)
	if budget != 1 || fair != 1 {
		t.Fatal("failed admissions charged or completion refunded", budget, fair)
	}
}

func TestPoolDefaultRouteSwitchPreservesFrozenChildRouting(t *testing.T) {
	e, scopes := poolFixture(t)
	s := scopes[0]
	if _, err := e.Publish(ctx, s.TenantID, s.ProjectID, Workflow{ID: "child", Name: "child", Nodes: []Node{{ID: "work", Type: "task"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Publish(ctx, s.TenantID, s.ProjectID, Workflow{ID: "parent", Name: "parent", Nodes: []Node{{ID: "sub", Type: "subworkflow", Config: map[string]any{"workflow_id": "child", "workflow_version": 1}}}}); err != nil {
		t.Fatal(err)
	}
	parent, err := e.CreateRun(ctx, s.TenantID, s.ProjectID, "parent", "create-parent")
	if err != nil {
		t.Fatal(err)
	}
	childID := parent.Nodes[0].ChildRunID
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WriteProjectExecutionRouteTx(ctx, tx, s.TenantID, s.ProjectID, 1, ProjectExecutionRouteInput{}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{parent.ID, childID} {
		r, err := e.GetRun(ctx, s.TenantID, s.ProjectID, id)
		if err != nil || r.ExecutionPoolID != "pool" || r.ExecutionRouteVersion != 1 || r.ExecutionPoolGrantID != s.GrantID {
			t.Fatal("route switch changed fixed family", r, err)
		}
	}
	local := publishPoolTasks(t, e, s, "local-after-switch", 1)
	if local.ExecutionPoolID != "" {
		t.Fatal("new local route ignored", local)
	}
	a, err := e.Claim(ctx, s.TenantID, s.ProjectID, "local-worker", []string{"task"})
	if err != nil || a == nil || a.RunID != local.ID {
		t.Fatal("old route leaked to local executor", a, err)
	}
}

func TestPoolPausedClaimIsStableAndAlreadyRunningDrains(t *testing.T) {
	e, scopes := poolFixture(t)
	in := testPoolRequest(scopes, "paused-empty-operation", []string{"task"})
	paused := func(tx *sql.Tx) (PoolClaimAuthority, error) {
		a, err := testPoolAuthority(scopes)(tx)
		a.PoolActive = false
		return a, err
	}
	if a, err := e.ClaimPoolWithOperation(ctx, in, paused, nil); err != nil || a != nil {
		t.Fatal(a, err)
	}
	if a, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil); err != nil || a != nil {
		t.Fatal("empty receipt changed after resume", a, err)
	}
	in.OperationID = "running-before-pause"
	a, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil)
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	if _, err = e.ClaimPoolWithOperation(ctx, in, paused, nil); err == nil {
		t.Fatal("paused pool replay started original unacknowledged work")
	}
	if err = e.Heartbeat(ctx, a.TenantID, a.ProjectID, *a); err != nil {
		t.Fatal("running attempt cannot drain", err)
	}
	if err = e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "drained-after-pause"}); err != nil {
		t.Fatal(err)
	}
}

func TestPoolCompensationInheritsOriginalRouteAfterDefaultSwitch(t *testing.T) {
	e, scopes := poolFixture(t)
	s := scopes[0]
	w := Workflow{ID: "inverse-source", Name: "inverse source", Nodes: []Node{{ID: "apply", Type: "command", Command: []string{"apply"}, Config: map[string]any{"compensation": map[string]any{"type": "command", "command": []any{"undo"}, "idempotent": true}}}}}
	if _, err := e.Publish(ctx, s.TenantID, s.ProjectID, w); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, s.TenantID, s.ProjectID, w.ID, "create-inverse-source")
	if err != nil {
		t.Fatal(err)
	}
	in := testPoolRequest(scopes[:1], "execute-inverse-source", []string{"command:apply", "command:undo"})
	a, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil)
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	if err = e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "inverse-source-finished"}); err != nil {
		t.Fatal(err)
	}
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WriteProjectExecutionRouteTx(ctx, tx, s.TenantID, s.ProjectID, 1, ProjectExecutionRouteInput{}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, s.TenantID, s.ProjectID, r.ID)
	r, err = e.Command(ctx, s.TenantID, s.ProjectID, r.ID, "compensate", "explicit-compensation", r.Version, map[string]any{"reason": "operator requested explicit inverse"})
	if err != nil {
		t.Fatal(err)
	}
	comp, err := e.GetRun(ctx, s.TenantID, s.ProjectID, r.CompensationRunID)
	if err != nil || comp.ExecutionPoolID != "pool" || comp.ExecutionPoolGrantID != s.GrantID || comp.ExecutionRouteVersion != 1 {
		t.Fatal("compensation followed changed default", comp, err)
	}
}

func TestPoolLateFenceCannotReleaseNewAttemptResources(t *testing.T) {
	e, scopes := poolFixture(t)
	s := scopes[0]
	if _, err := e.Publish(ctx, s.TenantID, s.ProjectID, Workflow{ID: "recoverable", Name: "recoverable", Nodes: []Node{{ID: "work", Type: "command", Command: []string{"recover"}, Idempotent: true}}}); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, s.TenantID, s.ProjectID, "recoverable", "create-recoverable")
	if err != nil {
		t.Fatal(err)
	}
	in := testPoolRequest(scopes[:1], "first-recoverable", []string{"command:recover"})
	old, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil)
	if err != nil || old == nil {
		t.Fatal(old, err)
	}
	if err = e.Complete(ctx, s.TenantID, s.ProjectID, Result{RunID: old.RunID, NodeID: old.NodeID, Attempt: old.Attempt, Fence: old.Fence, WorkerID: old.WorkerID, OperationID: "uncertain-first", OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, s.TenantID, s.ProjectID, r.ID)
	if _, err = e.Command(ctx, s.TenantID, s.ProjectID, r.ID, "reconcile", "verified-not-started", r.Version, map[string]any{"node_id": "work", "outcome": "not_started", "reason": "independent operation lookup confirms never started"}); err != nil {
		t.Fatal(err)
	}
	in.OperationID = "second-recoverable"
	next, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil)
	if err != nil || next == nil || next.Attempt != 2 || next.Fence <= old.Fence {
		t.Fatal(next, err)
	}
	if err = e.Complete(ctx, s.TenantID, s.ProjectID, Result{RunID: old.RunID, NodeID: old.NodeID, Attempt: old.Attempt, Fence: old.Fence, WorkerID: old.WorkerID, OperationID: "late-first-stop", Stopped: true}); err != ErrStale {
		t.Fatal("old worker affected new attempt", err)
	}
	var used, slots int
	e.db.QueryRow(`SELECT used FROM dag_execution_pool_accounts WHERE pool_id='pool' AND name='execution_slots'`).Scan(&used)
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id='pool'`).Scan(&slots)
	if used != 1 || slots != 1 {
		t.Fatal("late fence released new shared holds", used, slots)
	}
}
