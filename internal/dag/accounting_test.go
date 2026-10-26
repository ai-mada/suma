package dag

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

type accountingTestHook struct {
	check      func(context.Context, *sql.Tx, AccountingAttempt) (bool, error)
	reserve    func(context.Context, *sql.Tx, AccountingAttempt) error
	result     func(context.Context, *sql.Tx, AccountingAttempt, Result) error
	resolution func(context.Context, *sql.Tx, AccountingAttempt, string) error
}

func (h *accountingTestHook) CheckAttemptTx(c context.Context, tx *sql.Tx, r AccountingAttempt) (bool, error) {
	if h.check != nil {
		return h.check(c, tx, r)
	}
	return true, nil
}
func (h *accountingTestHook) ReserveAttemptTx(c context.Context, tx *sql.Tx, r AccountingAttempt) error {
	if h.reserve != nil {
		return h.reserve(c, tx, r)
	}
	return nil
}
func (h *accountingTestHook) RecordResultTx(c context.Context, tx *sql.Tx, r AccountingAttempt, v Result) error {
	if h.result != nil {
		return h.result(c, tx, r, v)
	}
	return nil
}
func (h *accountingTestHook) RecordResolutionTx(c context.Context, tx *sql.Tx, r AccountingAttempt, v string) error {
	if h.resolution != nil {
		return h.resolution(c, tx, r, v)
	}
	return nil
}

func TestAccountingDenyIsStableAndFree(t *testing.T) {
	for _, mode := range []string{"private", "pool"} {
		t.Run(mode, func(t *testing.T) {
			var e *Engine
			var r Run
			var claim func(string) (*Assignment, error)
			if mode == "pool" {
				e, r, claim = accountingPoolFixture(t)
			} else {
				e, r = fixture(t, Workflow{Nodes: []Node{toolNode()}})
				claim = func(op string) (*Assignment, error) {
					return e.ClaimWithOperation(ctx, "t", "p", "worker", op, []string{"command:native-agent"}, nil, nil)
				}
			}
			checked, reserved := 0, 0
			e.AttemptAccounting = &accountingTestHook{check: func(_ context.Context, tx *sql.Tx, ref AccountingAttempt) (bool, error) {
				checked++
				if ref.TenantID != "t" || ref.ProjectID != "p" || ref.RunID != r.ID || ref.NodeID != "agent" || ref.WorkerID != "worker" || ref.Attempt != 1 || ref.Fence != 1 {
					t.Fatalf("wrong next identity: %+v", ref)
				}
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger`).Scan(&count); err != nil {
					return false, err
				}
				if count != 0 {
					t.Fatal("accounting check followed resource charge")
				}
				return false, nil
			}, reserve: func(context.Context, *sql.Tx, AccountingAttempt) error { reserved++; return nil }}
			for _, op := range []string{"deny-claim-one", "deny-claim-two"} {
				a, err := claim(op)
				if err != nil || a != nil {
					t.Fatal(a, err)
				}
			}
			current, err := e.GetRun(ctx, "t", "p", r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if checked != 2 || reserved != 0 || current.Nodes[0].Attempt != 0 || current.Nodes[0].State != "ready" || current.Nodes[0].Reason != "Model token or cost allowance is unavailable" {
				t.Fatalf("denied claim changed execution: checked=%d reserved=%d run=%+v", checked, reserved, current)
			}
			var events int
			if err = e.db.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE event_type='accounting_blocked'`).Scan(&events); err != nil || events != 1 {
				t.Fatalf("deny reason repeated events: %d %v", events, err)
			}
			var ledger int
			e.db.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger`).Scan(&ledger)
			if ledger != 0 {
				t.Fatal("denied claim charged resources")
			}
			var poolLedger, slots int
			e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_ledger`).Scan(&poolLedger)
			e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_slots`).Scan(&slots)
			if poolLedger != 0 || slots != 0 {
				t.Fatal("denied pool claim consumed resources", poolLedger, slots)
			}
		})
	}
}

var errAccountingFixture = errors.New("accounting fixture failure")

func accountingTrace(t *testing.T, e *Engine) {
	t.Helper()
	if _, err := e.db.Exec(`CREATE TABLE accounting_fixture(id INTEGER PRIMARY KEY,kind TEXT NOT NULL,run_id TEXT NOT NULL,attempt INTEGER NOT NULL,fence BIGINT NOT NULL,held BIGINT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
}

func accountingPoolFixture(t *testing.T) (*Engine, Run, func(string) (*Assignment, error)) {
	t.Helper()
	e := openEngine(t, filepath.Join(t.TempDir(), "accounting-pool.db"))
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WritePoolPolicyTx(ctx, tx, "pool", 0, PoolPolicyInput{ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{"t": 1}}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	scope := configurePoolScope(t, e, "pool", "t", "p", 1)
	w, err := e.Publish(ctx, "t", "p", Workflow{ID: "wf", Name: "accounting-pool", Nodes: []Node{toolNode()}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := e.CreateRun(ctx, "t", "p", w.ID, "create-accounting-pool")
	if err != nil {
		t.Fatal(err)
	}
	claim := func(operation string) (*Assignment, error) {
		return e.ClaimPoolWithOperation(ctx, PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: operation, Scopes: []PoolCapabilityScope{{TenantID: "t", ProjectID: "p", Capabilities: []string{"command:native-agent"}}}}, func(*sql.Tx) (PoolClaimAuthority, error) {
			return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 1, PoolActive: true, Scopes: []PoolScope{scope}}, nil
		}, nil)
	}
	return e, r, claim
}

func TestAccountingClaimReservationsRollbackAndReplay(t *testing.T) {
	for _, mode := range []string{"private", "pool"} {
		t.Run(mode, func(t *testing.T) {
			var e *Engine
			var r Run
			var claim func(string) (*Assignment, error)
			if mode == "pool" {
				e, r, claim = accountingPoolFixture(t)
			} else {
				e, r = fixture(t, Workflow{Nodes: []Node{toolNode()}})
				claim = func(op string) (*Assignment, error) {
					return e.ClaimWithOperation(ctx, "t", "p", "worker", op, []string{"command:native-agent"}, nil, nil)
				}
			}
			accountingTrace(t, e)
			fail := true
			checked, reserved := 0, 0
			e.AttemptAccounting = &accountingTestHook{check: func(_ context.Context, tx *sql.Tx, ref AccountingAttempt) (bool, error) {
				checked++
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger`).Scan(&count); err != nil {
					return false, err
				}
				if count != 0 {
					t.Fatal("Check charged after resource allocation")
				}
				ref.Spec.Config["goal"] = "hook-private-copy"
				return true, nil
			}, reserve: func(_ context.Context, tx *sql.Tx, ref AccountingAttempt) error {
				reserved++
				if ref.Spec.Config["goal"] != "inspect" {
					t.Fatal("Check mutated later immutable spec")
				}
				var count int
				if err := tx.QueryRow(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=? AND attempt=? AND fence=?`, ref.RunID, ref.Attempt, ref.Fence).Scan(&count); err != nil {
					return err
				}
				if count == 0 {
					t.Fatal("Reserve preceded execution resources")
				}
				if mode == "pool" {
					if err := tx.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_ledger WHERE run_id=? AND attempt=? AND fence=?`, ref.RunID, ref.Attempt, ref.Fence).Scan(&count); err != nil {
						return err
					}
					if count == 0 {
						t.Fatal("Reserve preceded pool resources")
					}
				}
				var attempt int
				if err := tx.QueryRow(`SELECT attempt FROM dag_nodes WHERE run_id=? AND id=?`, ref.RunID, ref.NodeID).Scan(&attempt); err != nil {
					return err
				}
				if attempt != 0 || ref.Attempt != 1 || ref.Fence != 1 {
					t.Fatal("wrong pre-dispatch identity")
				}
				if _, err := tx.Exec(`INSERT INTO accounting_fixture(kind,run_id,attempt,fence,held)VALUES('reserve',?,?,?,9)`, ref.RunID, ref.Attempt, ref.Fence); err != nil {
					return err
				}
				if fail {
					return errAccountingFixture
				}
				return nil
			}}
			if _, err := claim("accounting-reserve-op"); !errors.Is(err, errAccountingFixture) {
				t.Fatal("reserve failure did not abort claim", err)
			}
			for _, table := range []string{"accounting_fixture", "dag_resource_ledger", "dag_execution_pool_ledger", "dag_attempts", "dag_dispatches", "dag_execution_pool_dispatches", "dag_execution_pool_slots"} {
				var count int
				if err := e.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("partial claim leaked %s=%d: %v", table, count, err)
				}
			}
			current, err := e.GetRun(ctx, "t", "p", r.ID)
			if err != nil || current.Nodes[0].Attempt != 0 || current.Nodes[0].State != "ready" {
				t.Fatal("rollback consumed attempt", current, err)
			}
			fail = false
			a, err := claim("accounting-reserve-op")
			if err != nil || a == nil {
				t.Fatal(a, err)
			}
			beforeChecked, beforeReserved := checked, reserved
			replay, err := claim("accounting-reserve-op")
			if err != nil || replay == nil || replay.Attempt != a.Attempt || replay.Fence != a.Fence || checked != beforeChecked || reserved != beforeReserved {
				t.Fatal("replay reran accounting", replay, err, checked, reserved)
			}
			var count int
			e.db.QueryRow(`SELECT COUNT(*) FROM accounting_fixture WHERE kind='reserve'`).Scan(&count)
			if count != 1 {
				t.Fatal("reservation duplicated", count)
			}
			current, err = e.GetRun(ctx, "t", "p", r.ID)
			if err != nil || current.Nodes[0].Attempt != 1 {
				t.Fatal(current, err)
			}
		})
	}
}

func TestAccountingCompletionFollowsProofAndRollsBackWithReceipt(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{toolNode()}})
	accountingTrace(t, e)
	fail := true
	recorded := 0
	e.AttemptAccounting = &accountingTestHook{result: func(_ context.Context, tx *sql.Tx, ref AccountingAttempt, result Result) error {
		recorded++
		var state string
		if err := tx.QueryRow(`SELECT state FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, ref.RunID, ref.NodeID, ref.Attempt).Scan(&state); err != nil {
			return err
		}
		if state != "failed" || ref.WorkerID != "worker" || ref.Fence != 1 || result.Error != "fixture known failure" {
			t.Fatal("RecordResult before exact formal result")
		}
		var count int
		tx.QueryRow(`SELECT COUNT(*) FROM accounting_fixture WHERE kind='platform'`).Scan(&count)
		if count != 1 {
			t.Fatal("RecordResult preceded platform proof")
		}
		if _, err := tx.Exec(`INSERT INTO accounting_fixture(kind,run_id,attempt,fence,held)VALUES('result',?,?,?,0)`, ref.RunID, ref.Attempt, ref.Fence); err != nil {
			return err
		}
		if fail {
			return errAccountingFixture
		}
		return nil
	}}
	a, err := e.Claim(ctx, "t", "p", "worker", []string{"command:native-agent"})
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	result := Result{RunID: r.ID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, ExitCode: 1, Error: "fixture known failure", OperationID: "accounting-result-op"}
	platform := func(tx *sql.Tx, state string) error {
		if state != "failed" {
			t.Fatal(state)
		}
		_, err := tx.Exec(`INSERT INTO accounting_fixture(kind,run_id,attempt,fence,held)VALUES('platform',?,?,?,0)`, r.ID, a.Attempt, a.Fence)
		return err
	}
	if err = e.CompleteBound(ctx, "t", "p", result, nil, func(*sql.Tx, string) error { return errAccountingFixture }); !errors.Is(err, errAccountingFixture) || recorded != 0 {
		t.Fatal("rejected platform proof reached accounting", err, recorded)
	}
	if err = e.CompleteBound(ctx, "t", "p", result, nil, platform); !errors.Is(err, errAccountingFixture) {
		t.Fatal(err)
	}
	var count int
	e.db.QueryRow(`SELECT COUNT(*) FROM accounting_fixture`).Scan(&count)
	if count != 0 {
		t.Fatal("completion hook leaked after rollback")
	}
	current, err := e.GetRun(ctx, "t", "p", r.ID)
	if err != nil || current.Nodes[0].State != "running" {
		t.Fatal("formal result committed without ledger", current, err)
	}
	fail = false
	if err = e.CompleteBound(ctx, "t", "p", result, nil, platform); err != nil {
		t.Fatal(err)
	}
	if err = e.CompleteBound(ctx, "t", "p", result, nil, platform); err != nil {
		t.Fatal("result replay failed", err)
	}
	if recorded != 2 {
		t.Fatal("result replay reran ledger", recorded)
	}
	e.db.QueryRow(`SELECT COUNT(*) FROM accounting_fixture`).Scan(&count)
	if count != 2 {
		t.Fatal("proof or accounting receipt duplicated", count)
	}
	stale := result
	stale.Fence++
	stale.OperationID = "stale-accounting-result"
	if err = e.CompleteBound(ctx, "t", "p", stale, nil, platform); !errors.Is(err, ErrStale) {
		t.Fatal("stale result accepted", err)
	}
	if recorded != 2 {
		t.Fatal("stale result reached ledger")
	}
	plain, _ := fixture(t, Workflow{Nodes: []Node{{ID: "plain", Type: "task"}}})
	calls := 0
	plain.AttemptAccounting = &accountingTestHook{check: func(context.Context, *sql.Tx, AccountingAttempt) (bool, error) { calls++; return false, nil }, result: func(context.Context, *sql.Tx, AccountingAttempt, Result) error { calls++; return nil }}
	done(t, plain, claim(t, plain), 0)
	if calls != 0 {
		t.Fatal("unbound work entered tool accounting")
	}
}

func TestAccountingReconcileAndExpiredLeaseKeepUnknown(t *testing.T) {
	for _, mode := range []string{"manual", "lease"} {
		t.Run(mode, func(t *testing.T) {
			e, r := fixture(t, Workflow{Nodes: []Node{toolNode()}})
			accountingTrace(t, e)
			resolutions := 0
			fail := true
			e.AttemptAccounting = &accountingTestHook{reserve: func(_ context.Context, tx *sql.Tx, ref AccountingAttempt) error {
				_, err := tx.Exec(`INSERT INTO accounting_fixture(kind,run_id,attempt,fence,held)VALUES('reserve',?,?,?,9)`, ref.RunID, ref.Attempt, ref.Fence)
				return err
			}, resolution: func(_ context.Context, tx *sql.Tx, ref AccountingAttempt, decision string) error {
				resolutions++
				if ref.Attempt != 1 || ref.Fence != 1 || ref.WorkerID != "worker" || decision != "not_started" {
					t.Fatal("resolution lost frozen identity", ref, decision)
				}
				var original string
				if err := tx.QueryRow(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?`, ref.RunID, ref.NodeID, ref.Attempt).Scan(&original); err != nil {
					return err
				}
				if !strings.Contains(original, `"outcome_unknown":true`) {
					t.Fatal("resolution original was already overwritten")
				}
				_, err := tx.Exec(`INSERT INTO accounting_fixture(kind,run_id,attempt,fence,held)VALUES('resolution',?,?,?,0)`, ref.RunID, ref.Attempt, ref.Fence)
				if err != nil {
					return err
				}
				if fail {
					return errAccountingFixture
				}
				return nil
			}}
			a, err := e.Claim(ctx, "t", "p", "worker", []string{"command:native-agent"})
			if err != nil || a == nil {
				t.Fatal(a, err)
			}
			if mode == "lease" {
				if _, err = e.db.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=?`, r.ID); err != nil {
					t.Fatal(err)
				}
				if _, err = e.db.Exec(`UPDATE dag_wakeups SET due_at=0 WHERE run_id=? AND kind='lease'`, r.ID); err != nil {
					t.Fatal(err)
				}
				if _, err = e.TickBatch(ctx, 10); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = e.Complete(ctx, "t", "p", Result{RunID: r.ID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, ExitCode: -1, OutcomeUnknown: true, OperationID: "unknown-accounting-result"}); err != nil {
					t.Fatal(err)
				}
				current, err := e.GetRun(ctx, "t", "p", r.ID)
				if err != nil {
					t.Fatal(err)
				}
				payload := map[string]any{"node_id": a.NodeID, "reason": "Business reconciliation does not prove measured zero usage", "outcome": "not_started"}
				if _, err = e.Command(ctx, "t", "p", r.ID, "reconcile", "resolve-accounting-op", current.Version, payload); !errors.Is(err, errAccountingFixture) {
					t.Fatal(err)
				}
				var count int
				e.db.QueryRow(`SELECT COUNT(*) FROM accounting_fixture WHERE kind='resolution'`).Scan(&count)
				if count != 0 {
					t.Fatal("resolution leaked on rollback")
				}
				fail = false
				if _, err = e.Command(ctx, "t", "p", r.ID, "reconcile", "resolve-accounting-op", current.Version, payload); err != nil {
					t.Fatal(err)
				}
				if _, err = e.Command(ctx, "t", "p", r.ID, "reconcile", "resolve-accounting-op", current.Version, payload); err != nil {
					t.Fatal(err)
				}
				if resolutions != 2 {
					t.Fatal("resolution replay reran accounting", resolutions)
				}
			}
			var held int64
			e.db.QueryRow(`SELECT SUM(held) FROM accounting_fixture`).Scan(&held)
			if held != 9 {
				t.Fatal("business resolution or expiry refunded unknown usage", held)
			}
			current, err := e.GetRun(ctx, "t", "p", r.ID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "lease" && (current.Nodes[0].State != "reconciling" || resolutions != 0) {
				t.Fatal("lease expiry settled accounting", current, resolutions)
			}
		})
	}
}
