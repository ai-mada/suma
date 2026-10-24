package dag

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"
)

func finishPoolAdmission(t *testing.T, e *Engine, a *Assignment, operation string) {
	t.Helper()
	if a == nil {
		t.Fatal("missing admission", operation)
	}
	if err := e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: operation}); err != nil {
		t.Fatal(err)
	}
}

func TestPoolBlockedTenantDoesNotBankHistoricalCatchUpCredit(t *testing.T) {
	e, scopes := poolFixture(t)
	if err := e.SetResourceLimits(ctx, "ta", "pa", map[string]int64{"execution_slots": 0}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 24; i++ {
		a := poolClaim(t, e, scopes, fmt.Sprintf("only-eligible-%03d", i))
		if a == nil || a.TenantID != "tb" {
			t.Fatal("resource-blocked project consumed admission", a)
		}
		finishPoolAdmission(t, e, a, fmt.Sprintf("done-only-eligible-%03d", i))
	}
	var heldService int64
	if err := e.db.QueryRow(`SELECT pass FROM dag_execution_pool_fairness WHERE pool_id='pool' AND tenant_id='ta' AND project_id=''`).Scan(&heldService); err != nil {
		t.Fatal(err)
	}
	if heldService != 0 {
		t.Fatal("blocked work was charged successful service", heldService)
	}
	if err := e.SetResourceLimits(ctx, "ta", "pa", map[string]int64{"execution_slots": 1}); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for i := 0; i < 8; i++ {
		a := poolClaim(t, e, scopes, fmt.Sprintf("reactivated-%03d", i))
		if a == nil {
			t.Fatal(i)
		}
		counts[a.TenantID]++
		finishPoolAdmission(t, e, a, fmt.Sprintf("done-reactivated-%03d", i))
	}
	if counts["ta"] > 3 || counts["tb"] < 5 {
		t.Fatal("reactivated tenant monopolized old virtual credit", counts)
	}
}

func TestPoolWeightChangePreservesServiceAndConsumedBudget(t *testing.T) {
	e, scopes := poolFixture(t)
	for i := 0; i < 40; i++ {
		a := poolClaim(t, e, scopes, fmt.Sprintf("before-weight-%03d", i))
		finishPoolAdmission(t, e, a, fmt.Sprintf("done-before-weight-%03d", i))
	}
	var beforePass, beforeSpent int64
	e.db.QueryRow(`SELECT pass FROM dag_execution_pool_fairness WHERE pool_id='pool' AND tenant_id='ta' AND project_id=''`).Scan(&beforePass)
	e.db.QueryRow(`SELECT budget_spent FROM dag_execution_pool_policies WHERE pool_id='pool'`).Scan(&beforeSpent)
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WritePoolPolicyTx(ctx, tx, "pool", 1, PoolPolicyInput{ConcurrencyLimit: 1, TenantWeights: map[string]int{"ta": 3, "tb": 1}}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var afterPass, afterSpent int64
	e.db.QueryRow(`SELECT pass FROM dag_execution_pool_fairness WHERE pool_id='pool' AND tenant_id='ta' AND project_id=''`).Scan(&afterPass)
	e.db.QueryRow(`SELECT budget_spent FROM dag_execution_pool_policies WHERE pool_id='pool'`).Scan(&afterSpent)
	if beforePass != afterPass || beforeSpent != 40 || afterSpent != 40 {
		t.Fatal("policy change reset previous service or budget", beforePass, afterPass, beforeSpent, afterSpent)
	}
	counts := map[string]int{}
	for i := 0; i < 40; i++ {
		a := poolClaim(t, e, scopes, fmt.Sprintf("after-weight-%03d", i))
		if a == nil {
			t.Fatal(i)
		}
		counts[a.TenantID]++
		finishPoolAdmission(t, e, a, fmt.Sprintf("done-after-weight-%03d", i))
	}
	if counts["ta"] != 30 || counts["tb"] != 10 {
		t.Fatal("new weight did not govern future admissions", counts)
	}
}

func TestPoolMaximumRegisteredSlot128IsNotSilentlyTruncated(t *testing.T) {
	e, scopes := poolFixture(t)
	in := testPoolRequest(scopes, "maximum-slot-operation", []string{"task"})
	in.SlotID = "127"
	before := func(*sql.Tx) (PoolClaimAuthority, error) {
		return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 128, PoolActive: true, Scopes: scopes}, nil
	}
	a, err := e.ClaimPoolWithOperation(ctx, in, before, nil)
	if err != nil || a == nil || a.PoolSlotID != "127" {
		t.Fatal(a, err)
	}
	in.OperationID = "outside-slot-operation"
	in.SlotID = "128"
	if _, err = e.ClaimPoolWithOperation(ctx, in, before, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("slot outside registered capacity accepted")
	}
	var receipts int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_dispatches`).Scan(&receipts)
	if receipts != 1 {
		t.Fatal("invalid slot persisted side effects", receipts)
	}
}

func TestPoolBoundedScopeScanMakesDurableProgressWithoutChargingMisses(t *testing.T) {
	e, scopes := poolFixture(t)
	for i := 0; i < 18; i++ {
		s := configurePoolScope(t, e, "pool", fmt.Sprintf("z%02d", i), "project", 1)
		publishPoolTasks(t, e, s, "single", 1)
		scopes = append(scopes, s)
	}
	in := testPoolRequest(scopes, "bounded-first-operation", []string{"task"})
	checks := 0
	eligible := func(_ *sql.Tx, s PoolScope, _ string, _ string) (bool, error) {
		checks++
		return s.TenantID == "z17", nil
	}
	a, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), eligible)
	if err != nil || a != nil {
		t.Fatal("first bounded page did not stop before twentieth scope", a, err)
	}
	if checks > 64 {
		t.Fatal("candidate check bound exceeded", checks)
	}
	var fair int
	e.db.QueryRow(`SELECT last_sequence FROM dag_execution_pool_fairness WHERE pool_id='pool' AND tenant_id='' AND project_id=''`).Scan(&fair)
	if fair != 0 {
		t.Fatal("ineligible scan consumed fair share", fair)
	}
	in.OperationID = "bounded-next-operation"
	a, err = e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), eligible)
	if err != nil || a == nil || a.TenantID != "z17" {
		t.Fatal("persistent cursor failed to reach later supported scope", a, err)
	}
	in.OperationID = "bounded-first-operation"
	if again, err := e.ClaimPoolWithOperation(ctx, in, testPoolAuthority(scopes), eligible); err != nil || again != nil {
		t.Fatal("null receipt was not immutable", again, err)
	}
}
