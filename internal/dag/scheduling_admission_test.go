package dag

import (
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"testing"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func TestClaimOperationPersistsEmptyActiveClosedAndRechecksAuthority(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	if a, err := e.ClaimWithOperation(ctx, "t", "p", "w", "empty-operation", []string{"command:missing"}, nil, nil); err != nil || a != nil {
		t.Fatal(a, err)
	}
	a, err := e.ClaimWithOperation(ctx, "t", "p", "w", "active-operation", []string{"task"}, nil, nil)
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	if nilAgain, err := e.ClaimWithOperation(ctx, "t", "p", "w", "empty-operation", []string{"command:missing"}, nil, nil); err != nil || nilAgain != nil {
		t.Fatal("empty receipt changed", nilAgain, err)
	}
	again, err := e.ClaimWithOperation(ctx, "t", "p", "w", "active-operation", []string{"task"}, nil, nil)
	if err != nil || again == nil || again.DispatchID != a.DispatchID || again.Attempt != 1 || again.DispatchState != "active" {
		t.Fatal(again, err)
	}
	if _, err = e.ClaimWithOperation(ctx, "t", "p", "w", "active-operation", []string{"task"}, nil, func(*sql.Tx, string, string) (bool, error) { return false, nil }); !errors.Is(err, ErrConflict) {
		t.Fatal("active replay ignored current eligibility", err)
	}
	denied := errors.New("current credential revoked")
	if _, err = e.ClaimWithOperation(ctx, "t", "p", "w", "active-operation", []string{"task"}, func(*sql.Tx) error { return denied }, nil); !errors.Is(err, denied) {
		t.Fatal("cached receipt bypassed current authorization", err)
	}
	if _, err = e.ClaimWithOperation(ctx, "t", "p", "w", "active-operation", []string{"task", "other"}, nil, nil); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	done(t, e, a, 0)
	closed, err := e.ClaimWithOperation(ctx, "t", "p", "w", "active-operation", []string{"task"}, nil, nil)
	if err != nil || closed == nil || closed.DispatchState != "closed" || closed.LeaseUntil == "" {
		t.Fatal(closed, err)
	}
	var attempts int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&attempts)
	if attempts != 1 {
		t.Fatal("replayed claim duplicated execution", attempts)
	}
}

func TestBoundedClaimScanPersistsProgressPastBlockedCandidates(t *testing.T) {
	nodes := make([]Node, 130)
	for i := range nodes {
		nodes[i] = Node{ID: fmt.Sprintf("n%03d", i), Type: "task"}
	}
	e, r := fixture(t, Workflow{Nodes: nodes})
	calls := 0
	eligible := func(_ *sql.Tx, _ string, node string) (bool, error) { calls++; return node == "n129", nil }
	for i := 0; i < 3; i++ {
		before := calls
		a, err := e.ClaimWithOperation(ctx, "t", "p", "w", fmt.Sprintf("scan-op-%d", i), []string{"task"}, nil, eligible)
		if err != nil {
			t.Fatal(err)
		}
		if calls-before > 64 {
			t.Fatalf("unbounded eligibility work: %d", calls-before)
		}
		if i < 2 && a != nil {
			t.Fatal("skipped blocked candidates", a)
		}
		if i == 2 && (a == nil || a.NodeID != "n129") {
			t.Fatal("persistent scan never reached eligible tail", a)
		}
	}
	var attempts int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&attempts)
	if attempts != 1 {
		t.Fatal("blocked candidates consumed attempts", attempts)
	}
}

func TestLastBudgetUnitIsAtomicAcrossIndependentConnections(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"execution_slots": 2, "budget": 1}); err != nil {
		t.Fatal(err)
	}
	var path string
	e.db.QueryRow(`SELECT file FROM pragma_database_list WHERE name='main'`).Scan(&path)
	other := openEngine(t, path)
	var wg sync.WaitGroup
	got := make(chan *Assignment, 2)
	errs := make(chan error, 2)
	for i, engine := range []*Engine{e, other} {
		wg.Add(1)
		go func(i int, engine *Engine) {
			defer wg.Done()
			a, err := engine.ClaimWithOperation(ctx, "t", "p", fmt.Sprintf("w%d", i), fmt.Sprintf("last-unit-%d", i), []string{"task"}, nil, nil)
			got <- a
			errs <- err
		}(i, engine)
	}
	wg.Wait()
	close(got)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count := 0
	for a := range got {
		if a != nil {
			count++
			done(t, e, a, 0)
		}
	}
	if count != 1 {
		t.Fatal("last cumulative unit double spent", count)
	}
	var spent, charged int64
	e.db.QueryRow(`SELECT budget_spent FROM dag_scheduling_policies WHERE tenant_id='t' AND project_id='p'`).Scan(&spent)
	e.db.QueryRow(`SELECT COALESCE(SUM(amount),0) FROM dag_resource_ledger WHERE run_id=? AND name='budget'`, r.ID).Scan(&charged)
	if spent != 1 || charged != 1 {
		t.Fatal(spent, charged)
	}
	if a := claim(t, e); a != nil {
		t.Fatal("terminal refunded cumulative budget", a)
	}
}

func TestConfirmedUnknownStopReleasesProcessButRetainsEffectResources(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task", Resources: map[string]int64{"model_concurrency": 1}}, {ID: "b", Type: "task", Resources: map[string]int64{"model_concurrency": 1}}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"model_concurrency": 1}); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "stopped-unknown", Stopped: true, OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	var process, effect int
	e.db.QueryRow(`SELECT used FROM dag_resource_accounts WHERE tenant_id='t' AND project_id='p' AND name='execution_slots'`).Scan(&process)
	e.db.QueryRow(`SELECT used FROM dag_resource_accounts WHERE tenant_id='t' AND project_id='p' AND name='model_concurrency'`).Scan(&effect)
	if process != 0 || effect != 1 {
		t.Fatal(process, effect)
	}
	if next := claim(t, e); next != nil {
		t.Fatal("unknown effect lock was reused", next)
	}
	current, _ := e.GetRun(ctx, "t", "p", r.ID)
	if _, err := e.Command(ctx, "t", "p", r.ID, "reconcile", "effect-confirmed", current.Version, map[string]any{"node_id": a.NodeID, "outcome": "failed", "reason": "independent provider receipt confirms no effect"}); err != nil {
		t.Fatal(err)
	}
	if next := claim(t, e); next == nil {
		t.Fatal("known terminal did not release effect")
	}
}

func TestBudgetAccountMismatchRollsBackEntireAdmission(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	if _, err := e.db.Exec(`UPDATE dag_scheduling_policies SET budget_spent=1 WHERE tenant_id='t' AND project_id='p'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Claim(ctx, "t", "p", "w", []string{"task"}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	var attempts, ledger int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&attempts)
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=?`, r.ID).Scan(&ledger)
	if attempts != 0 || ledger != 0 {
		t.Fatal("inconsistent books partly charged", attempts, ledger)
	}
}

func TestZeroBudgetCannotBeBypassedByLegacyZeroNodeRequest(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	if err := e.SetResourceLimits(ctx, "t", "p", map[string]int64{"budget": 0}); err != nil {
		t.Fatal(err)
	}
	// An old persisted definition may predate compile-time zero rejection.
	if _, err := e.db.Exec(`UPDATE dag_nodes SET spec=? WHERE run_id=? AND id='a'`, encoded(Node{ID: "a", Type: "task", Resources: map[string]int64{"budget": 0}}), r.ID); err != nil {
		t.Fatal(err)
	}
	if a := claim(t, e); a != nil {
		t.Fatal("legacy zero request bypassed zero cumulative limit", a)
	}
	var attempts, ledger int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=?`, r.ID).Scan(&attempts)
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=?`, r.ID).Scan(&ledger)
	if attempts != 0 || ledger != 0 {
		t.Fatal(attempts, ledger)
	}
}

func TestAttemptDeepLinkDistinguishesMissingNodeAndManualReconciliation(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	if _, err := e.AttemptsPage(ctx, "t", "p", r.ID, "missing", 50, ""); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	a := claim(t, e)
	if err := e.Complete(ctx, "t", "p", Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "unknown", OutcomeUnknown: true}); err != nil {
		t.Fatal(err)
	}
	r, _ = e.GetRun(ctx, "t", "p", r.ID)
	if _, err := e.Command(ctx, "t", "p", r.ID, "reconcile", "manual-proof", r.Version, map[string]any{"node_id": a.NodeID, "outcome": "failed", "reason": "verified externally"}); err != nil {
		t.Fatal(err)
	}
	proof, err := e.GetAttempt(ctx, "t", "p", r.ID, a.NodeID, 1)
	if err != nil || proof.Result != nil || proof.ResultSource != "manual_reconciliation" || proof.Reconciliation == nil || proof.Reconciliation.Reason != "verified externally" {
		t.Fatal(proof, err)
	}
	page, err := e.AttemptsPage(ctx, "t", "p", r.ID, a.NodeID, 50, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].Result != nil || page.Items[0].ResultSource != "not_loaded" || page.Items[0].Reconciliation != nil {
		t.Fatal("list must not read or infer full results", page, err)
	}
	if _, err = e.db.Exec(`INSERT INTO dag_managed_resolutions(run_id,node_id,attempt,operation_id,original_result,resolution_result,resolved_at)VALUES(?,?,1,'provider-lookup','{"outcome_unknown":true}',?,'fixture')`, r.ID, a.NodeID, encoded(Result{ReleaseEvidence: &rc.ReleaseResult{}})); err != nil {
		t.Fatal(err)
	}
	proof, err = e.GetAttempt(ctx, "t", "p", r.ID, a.NodeID, 1)
	if err != nil || proof.ResultSource != "managed_observation" {
		t.Fatal("provider observation mislabeled Runner result", proof, err)
	}
	for _, corrupt := range []string{`{broken`, `null`} {
		if _, err = e.db.Exec(`UPDATE dag_managed_resolutions SET resolution_result=? WHERE run_id=? AND node_id=? AND attempt=1`, corrupt, r.ID, a.NodeID); err != nil {
			t.Fatal(err)
		}
		if _, err = e.GetAttempt(ctx, "t", "p", r.ID, a.NodeID, 1); err == nil {
			t.Fatal("corrupt managed result silently fell back to another provenance", corrupt)
		}
	}
}
