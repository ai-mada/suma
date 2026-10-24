package dag

import (
	"database/sql"
	"errors"
	"testing"
)

func testPoolAuthority(scopes []PoolScope) func(*sql.Tx) (PoolClaimAuthority, error) {
	return func(*sql.Tx) (PoolClaimAuthority, error) {
		return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 2, PoolActive: true, Scopes: scopes}, nil
	}
}
func testPoolRequest(scopes []PoolScope, op string, caps []string) PoolClaimRequest {
	in := PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: op}
	for _, s := range scopes {
		in.Scopes = append(in.Scopes, PoolCapabilityScope{TenantID: s.TenantID, ProjectID: s.ProjectID, Capabilities: caps})
	}
	return in
}

func TestPoolGrantVersionChangeCannotRewriteProjectWeight(t *testing.T) {
	e, scopes := poolFixture(t)
	in := scopes[0]
	in.GrantVersion++
	in.ProjectWeight = 9
	in.Status = "inactive"
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = e.WritePoolScopeTx(ctx, tx, in); !errors.Is(err, ErrConflict) {
		t.Fatal("authorization generation silently edited immutable project weight", err)
	}
	in.ProjectWeight = scopes[0].ProjectWeight
	if err = e.WritePoolScopeTx(ctx, tx, in); err != nil {
		t.Fatal("legitimate revoke must preserve fixed weight", err)
	}
}

func TestPoolFairnessWriteFailureRollsBackAdmission(t *testing.T) {
	e, scopes := poolFixture(t)
	if _, err := e.db.Exec(`CREATE TRIGGER reject_fair_charge BEFORE UPDATE ON dag_execution_pool_fairness WHEN NEW.project_id<>'' BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	a, err := e.ClaimPoolWithOperation(ctx, testPoolRequest(scopes, "fair-charge-fault", []string{"task"}), testPoolAuthority(scopes), nil)
	var attempts, holds, receipts int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_attempts`).Scan(&attempts)
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_ledger`).Scan(&holds)
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_dispatches`).Scan(&receipts)
	if !errors.Is(err, ErrConflict) || a != nil || attempts != 0 || holds != 0 || receipts != 0 {
		t.Fatalf("missing fair CAS committed partial admission: assignment=%v err=%v attempts=%d holds=%d receipts=%d", a != nil, err, attempts, holds, receipts)
	}
}

func TestPoolInvalidCandidateEvidenceFailsWithoutExecutionCharge(t *testing.T) {
	e, scopes := poolFixture(t)
	scope := scopes[0]
	w := Workflow{ID: "candidate", Name: "candidate", Nodes: []Node{{ID: "develop", Type: "command", Command: []string{"build"}}, {ID: "verify", Type: "test", Command: []string{"test"}, DependsOn: []string{"develop"}, Config: map[string]any{"candidate_from": "develop"}}}}
	if _, err := e.Publish(ctx, scope.TenantID, scope.ProjectID, w); err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, scope.TenantID, scope.ProjectID, w.ID, "create-candidate")
	if err != nil {
		t.Fatal(err)
	}
	in := testPoolRequest(scopes[:1], "candidate-develop", []string{"command:build", "command:test"})
	a, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil)
	if err != nil || a == nil || a.NodeID != "develop" {
		t.Fatal(a, err)
	}
	if err = e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "develop-no-proof"}); err != nil {
		t.Fatal(err)
	}
	in.OperationID = "candidate-verification"
	if a, err = e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), nil); err != nil || a != nil {
		t.Fatal(a, err)
	}
	r, err = e.GetRun(ctx, scope.TenantID, scope.ProjectID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range r.Nodes {
		if n.ID == "verify" && (n.State != "failed" || n.Attempt != 0) {
			t.Fatal("known invalid candidate stayed indefinitely ready or executed", n.State, n.Attempt)
		}
	}
	var budget int
	e.db.QueryRow(`SELECT budget_spent FROM dag_execution_pool_policies WHERE pool_id='pool'`).Scan(&budget)
	if budget != 1 {
		t.Fatal("invalid candidate consumed execution budget", budget)
	}
}

func TestPoolFairEpochRebasePreservesSuccessfulDispatch(t *testing.T) {
	e, scopes := poolFixture(t)
	seed := poolClaim(t, e, scopes, "seed-before-epoch")
	if err := e.Complete(ctx, seed.TenantID, seed.ProjectID, Result{RunID: seed.RunID, NodeID: seed.NodeID, Attempt: seed.Attempt, Fence: seed.Fence, WorkerID: seed.WorkerID, OperationID: "seed-epoch-finished"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE dag_execution_pool_fairness SET floor=? WHERE pool_id='pool' AND project_id=''`, poolFairRebase); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE dag_execution_pool_fairness SET pass=? WHERE pool_id='pool' AND tenant_id<>''`, poolFairRebase); err != nil {
		t.Fatal(err)
	}
	a, err := e.ClaimPoolWithOperation(ctx, testPoolRequest(scopes, "claim-after-epoch", []string{"task"}), testPoolAuthority(scopes), nil)
	if err != nil || a == nil {
		t.Fatal("rebase made valid admission fail or lose its fair CAS", a, err)
	}
	var maxPass int64
	e.db.QueryRow(`SELECT COALESCE(MAX(pass),0) FROM dag_execution_pool_fairness WHERE pool_id='pool'`).Scan(&maxPass)
	if maxPass >= poolFairRebase {
		t.Fatal("fair arithmetic was not bounded", maxPass)
	}
}
