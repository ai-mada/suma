package dag

import (
	"database/sql"
	"errors"
	"testing"

	rc "github.com/ai-mada/suma/internal/releasecontract"
)

func TestManagedWorkflowAtomicBindingAndNoPublicReuse(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "initial", Type: "noop"}}})
	w := Workflow{Name: "protected release", Nodes: []Node{{ID: "deploy", Type: "command", Command: []string{"test-command"}}}}
	beforeCalls, bindCalls := 0, 0
	before := func(tx *sql.Tx) error { beforeCalls++; return nil }
	fail := errors.New("binding refused")
	bind := func(tx *sql.Tx, id string) error {
		bindCalls++
		if id != "reserved-release" {
			t.Fatal(id)
		}
		return fail
	}
	if _, err := e.CreateManagedRunBound(ctx, "t", "p", "reserved-release", "managed-create", w, before, bind); !errors.Is(err, fail) {
		t.Fatal(err)
	}
	var count int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_managed_workflows`).Scan(&count); err != nil || count != 0 {
		t.Fatal("failed binding left managed workflow", err)
	}
	r, err := e.CreateManagedRunBound(ctx, "t", "p", "reserved-release", "managed-create", w, before, func(tx *sql.Tx, id string) error { bindCalls++; return nil })
	if err != nil || r.ID != "reserved-release" {
		t.Fatal(r, err)
	}
	_, err = e.CreateManagedRunBound(ctx, "t", "p", "reserved-release", "managed-create", w, before, func(*sql.Tx, string) error { t.Fatal("replay executed binding twice"); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if beforeCalls != 3 || bindCalls != 2 {
		t.Fatal(beforeCalls, bindCalls)
	}
	if _, err = e.CreateRun(ctx, "t", "p", r.WorkflowID, "ordinary-reuse"); !errors.Is(err, ErrInvalid) {
		t.Fatal("ordinary managed reuse", err)
	}
	if _, err = e.Publish(ctx, "t", "p", Workflow{ID: r.WorkflowID, Name: "replace", Nodes: w.Nodes}); !errors.Is(err, ErrInvalid) {
		t.Fatal("managed definition changed", err)
	}
	if _, err = e.Publish(ctx, "t", "p", Workflow{Name: "embed", Nodes: []Node{{ID: "child", Type: "subworkflow", Config: map[string]any{"workflow_id": r.WorkflowID, "workflow_version": 1}}}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("managed workflow embedded", err)
	}
	a, err := e.ClaimEligible(ctx, "t", "p", "worker", testCapabilities(t, e), nil, func(*sql.Tx, string, string) (bool, error) { return false, nil })
	if err != nil || a != nil {
		t.Fatal("ineligible worker acquired attempt", a, err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Nodes[0].Attempt != 0 {
		t.Fatal("denial consumed attempt", err)
	}
	if a, err = e.ClaimEligible(ctx, "t", "p", "worker", testCapabilities(t, e), nil, func(*sql.Tx, string, string) (bool, error) { return true, nil }); err != nil || a == nil || a.RunID != r.ID {
		t.Fatal("eligible worker lost assignment", a, err)
	}
}

func managedObservationFixture(t *testing.T) (*Engine, Run, *Assignment, Result) {
	t.Helper()
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "initial", Type: "noop"}}})
	e.LeaseSeconds = 3600
	run, err := e.CreateManagedRunBound(ctx, "t", "p", "observed-managed-run", "observed-managed-create", Workflow{Name: "observed managed fixture", Nodes: []Node{{ID: "deploy", Type: "command", Command: []string{"test-command"}}, {ID: "after", Type: "command", Command: []string{"test-command"}, DependsOn: []string{"deploy"}}}}, func(*sql.Tx) error { return nil }, func(*sql.Tx, string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	if a == nil || a.RunID != run.ID || a.NodeID != "deploy" {
		t.Fatal("managed fixture missing current assignment", a)
	}
	// This is a scheduler boundary fixture, not a real provider attestation.
	// The service callback is responsible for authenticating these references.
	result := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "observed-terminal-result", ReleaseEvidence: &rc.ReleaseResult{OperationID: "deploy-managed-fixture", RequestSHA256: hashText("managed request"), EvidenceSHA256: hashText("terminal provider reference"), Version: 1}}
	return e, run, a, result
}

func TestManagedObservationResolutionRejectsOrdinaryReceiptReplay(t *testing.T) {
	e, run := fixture(t, Workflow{Nodes: []Node{{ID: "ordinary", Type: "command", Command: []string{"test-command"}}}})
	a := claim(t, e)
	if a == nil {
		t.Fatal("ordinary assignment missing")
	}
	result := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "ordinary-final-result", ReleaseEvidence: &rc.ReleaseResult{OperationID: "not-a-managed-operation", RequestSHA256: hashText("ordinary reference"), EvidenceSHA256: hashText("ordinary evidence"), Version: 1}}
	// Produce the real operation receipt through the ordinary engine API. The
	// managed entry point must check its domain before accepting this same replay.
	if err := e.Complete(ctx, "t", "p", result); err != nil {
		t.Fatal(err)
	}
	beforeCalls := 0
	err := e.CompleteManagedObservedBound(ctx, "t", "p", result, func(*sql.Tx) error { beforeCalls++; return nil }, func(*sql.Tx, string) error {
		t.Fatal("ordinary receipt invoked managed proof hook")
		return nil
	})
	if !errors.Is(err, ErrInvalid) || beforeCalls != 1 {
		t.Fatalf("ordinary receipt escaped the managed guard: err=%v authority_calls=%d", err, beforeCalls)
	}
	var resolutions int
	if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_managed_resolutions WHERE run_id=?`, run.ID).Scan(&resolutions); err != nil || resolutions != 0 {
		t.Fatal("ordinary receipt gained a managed resolution", resolutions, err)
	}
	current, err := e.GetRun(ctx, "t", "p", run.ID)
	if err != nil || current.State != "succeeded" {
		t.Fatal("denial changed the original ordinary result", current.State, err)
	}
}

func TestManagedObservationResolutionRejectsCurrentWorkerLease(t *testing.T) {
	e, run, _, result := managedObservationFixture(t)
	beforeCalls := 0
	err := e.CompleteManagedObservedBound(ctx, "t", "p", result, func(*sql.Tx) error { beforeCalls++; return nil }, func(*sql.Tx, string) error {
		t.Fatal("current worker lease invoked observation commit hook")
		return nil
	})
	if !errors.Is(err, ErrConflict) || beforeCalls != 1 {
		t.Fatalf("current lease was overwritten: err=%v authority_calls=%d", err, beforeCalls)
	}
	var resolutions, receipts int
	if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_managed_resolutions WHERE run_id=?`, run.ID).Scan(&resolutions); err != nil {
		t.Fatal(err)
	}
	if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_operations WHERE scope=? AND operation_id=?`, "result:"+run.ID, result.OperationID).Scan(&receipts); err != nil || resolutions != 0 || receipts != 0 {
		t.Fatal("denied current-lease resolution left a durable result", resolutions, receipts, err)
	}
	current, err := e.GetRun(ctx, "t", "p", run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range current.Nodes {
		if node.ID == "deploy" && node.State != "running" || node.ID == "after" && node.Attempt != 0 {
			t.Fatal("denied observation changed a running attempt or its downstream", node)
		}
	}
	if next := claim(t, e); next != nil {
		t.Fatal("denied observation activated downstream", next)
	}
}

func TestManagedObservationResolutionReplayRechecksAuthority(t *testing.T) {
	e, run, a, result := managedObservationFixture(t)
	original := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "worker-unknown-result", ExitCode: -1, Error: "final provider outcome was not received", OutcomeUnknown: true}
	if err := e.Complete(ctx, "t", "p", original); err != nil {
		t.Fatal(err)
	}
	// Explicit database-time fault injection avoids sleeping for a lease period.
	if _, err := e.db.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, run.ID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	beforeCalls, commitCalls := 0, 0
	denied := errors.New("current service authority denied")
	allow := true
	before := func(*sql.Tx) error {
		beforeCalls++
		if !allow {
			return denied
		}
		return nil
	}
	commit := func(*sql.Tx, string) error { commitCalls++; return nil }
	for range 2 {
		if err := e.CompleteManagedObservedBound(ctx, "t", "p", result, before, commit); err != nil {
			t.Fatal("current authorized resolution/replay failed", err)
		}
	}
	allow = false
	if err := e.CompleteManagedObservedBound(ctx, "t", "p", result, before, commit); !errors.Is(err, denied) {
		t.Fatal("persisted receipt bypassed current service authority", err)
	}
	allow = true
	changed := result
	changed.Output = "different payload under the same operation"
	if err := e.CompleteManagedObservedBound(ctx, "t", "p", changed, before, commit); !errors.Is(err, ErrConflict) {
		t.Fatal("managed replay accepted changed content", err)
	}
	if beforeCalls != 4 || commitCalls != 1 {
		t.Fatal("managed replay did not preserve authority/proof call counts", beforeCalls, commitCalls)
	}
	var originalRaw, resolutionRaw string
	if err := e.db.QueryRow(`SELECT original_result,resolution_result FROM dag_managed_resolutions WHERE run_id=? AND node_id=? AND attempt=?`, run.ID, a.NodeID, a.Attempt).Scan(&originalRaw, &resolutionRaw); err != nil || originalRaw != encoded(original) || resolutionRaw != encoded(result) {
		t.Fatal("managed resolution erased or changed original evidence", err)
	}
	var events int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=? AND node_id=? AND event_type='managed_observation_reconciled'`, run.ID, a.NodeID).Scan(&events); err != nil || events != 1 {
		t.Fatal("managed replay duplicated its reconciliation event", events, err)
	}
	if next := claim(t, e); next == nil || next.NodeID != "after" || next.Attempt != 1 {
		t.Fatal("verified terminal resolution failed to release one downstream", next)
	}
	if duplicate := claim(t, e); duplicate != nil {
		t.Fatal("managed resolution activated downstream more than once", duplicate)
	}
}

func TestCompleteBoundProofFailureRollsBackResultAndDownstream(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "build", Type: "command", Command: []string{"test-command"}}, {ID: "test", Type: "test", Command: []string{"test-command"}, DependsOn: []string{"build"}}}})
	if _, err := e.db.Exec(`CREATE TABLE test_bound_proofs(id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	a := claim(t, e)
	result := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "bound-complete"}
	refused := errors.New("proof failed")
	hook := func(tx *sql.Tx, state string) error {
		if state != "succeeded" {
			t.Fatal(state)
		}
		if _, err := tx.Exec(`INSERT INTO test_bound_proofs(id)VALUES(1)`); err != nil {
			return err
		}
		return refused
	}
	if err := e.CompleteBound(ctx, "t", "p", result, nil, hook); !errors.Is(err, refused) {
		t.Fatal(err)
	}
	var count int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM test_bound_proofs`).Scan(&count); err != nil || count != 0 {
		t.Fatal("proof half committed", err)
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == "build" && n.State != "running" || n.ID == "test" && n.Attempt != 0 {
			t.Fatal("failed proof advanced DAG", n)
		}
	}
	calls := 0
	accept := func(tx *sql.Tx, state string) error {
		calls++
		_, err := tx.Exec(`INSERT INTO test_bound_proofs(id)VALUES(1)`)
		return err
	}
	for range 2 {
		if err = e.CompleteBound(ctx, "t", "p", result, nil, accept); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatal("result replay regenerated proof")
	}
	if a = claim(t, e); a == nil || a.NodeID != "test" {
		t.Fatal("downstream not ready", a)
	}
}
