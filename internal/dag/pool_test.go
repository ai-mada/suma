package dag

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestPoolPriorityRejectsOutOfRangeInsteadOfSilentlyIgnoring(t *testing.T) {
	var w Workflow
	if err := json.Unmarshal([]byte(`{"nodes":[{"id":"a","type":"task","priority":10}]}`), &w); err != nil {
		t.Fatal(err)
	}
	if err := Compile(w); err == nil {
		t.Fatal("out-of-range explicit priority silently accepted")
	}
}

func configurePoolScope(t *testing.T, e *Engine, pool, tenant, project string, weight int) PoolScope {
	t.Helper()
	scope := PoolScope{PoolID: pool, TenantID: tenant, ProjectID: project, GrantID: "grant-" + tenant + "-" + project, GrantVersion: 2, ProjectWeight: weight, Status: "active"}
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = e.WritePoolScopeTx(ctx, tx, scope); err != nil {
		t.Fatal(err)
	}
	if _, err = e.WriteProjectExecutionRouteTx(ctx, tx, tenant, project, 0, ProjectExecutionRouteInput{PoolID: pool, GrantID: scope.GrantID, GrantVersion: scope.GrantVersion}); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return scope
}
func poolFixture(t *testing.T) (*Engine, []PoolScope) {
	t.Helper()
	e := openEngine(t, filepath.Join(t.TempDir(), "pool.db"))
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WritePoolPolicyTx(ctx, tx, "pool", 0, PoolPolicyInput{ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{}, TenantWeights: map[string]int{"ta": 1, "tb": 3}}); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	scopes := []PoolScope{configurePoolScope(t, e, "pool", "ta", "pa", 1), configurePoolScope(t, e, "pool", "tb", "pb", 1)}
	for _, s := range scopes {
		w := Workflow{ID: "wf", Name: "shared", Nodes: []Node{}}
		for i := 0; i < 80; i++ {
			w.Nodes = append(w.Nodes, Node{ID: fmt.Sprintf("n%03d", i), Type: "task"})
		}
		if _, err = e.Publish(ctx, s.TenantID, s.ProjectID, w); err != nil {
			t.Fatal(err)
		}
		if _, err = e.CreateRun(ctx, s.TenantID, s.ProjectID, "wf", "create"); err != nil {
			t.Fatal(err)
		}
	}
	return e, scopes
}
func poolClaim(t *testing.T, e *Engine, scopes []PoolScope, op string) *Assignment {
	t.Helper()
	in := PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: op}
	for _, s := range scopes {
		in.Scopes = append(in.Scopes, PoolCapabilityScope{TenantID: s.TenantID, ProjectID: s.ProjectID, Capabilities: []string{"task"}})
	}
	a, err := e.ClaimPoolWithOperation(ctx, in, func(*sql.Tx) (PoolClaimAuthority, error) {
		return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 2, PoolActive: true, Scopes: scopes}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestPoolFixedRouteCannotBeClaimedThroughLocalProjectPath(t *testing.T) {
	e, scopes := poolFixture(t)
	s := scopes[0]
	if a, err := e.Claim(ctx, s.TenantID, s.ProjectID, "legacy", []string{"task"}); err != nil || a != nil {
		t.Fatal("local credential bypassed pool fairness", a, err)
	}
	a := poolClaim(t, e, scopes, "claim-route-1")
	if a == nil || a.PoolID != "pool" || a.TenantID == "" || a.ProjectID == "" || a.PoolGrantVersion != 2 {
		t.Fatal("dispatch lacks exact shared scope", a)
	}
	current, err := e.GetRun(ctx, a.TenantID, a.ProjectID, a.RunID)
	if err != nil || current.ExecutionPoolID != "pool" || current.ExecutionPoolGrantID != a.PoolGrantID {
		t.Fatal(current, err)
	}
}

func TestPoolHierarchicalTenantWeightsChargeOnlySuccessfulAdmissions(t *testing.T) {
	e, scopes := poolFixture(t)
	counts := map[string]int{}
	for i := 0; i < 40; i++ {
		a := poolClaim(t, e, scopes, fmt.Sprintf("fair-operation-%03d", i))
		if a == nil {
			t.Fatal("unexpected idle pool", i)
		}
		counts[a.TenantID]++
		if err := e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence, OperationID: fmt.Sprintf("finish-%03d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if counts["ta"] != 10 || counts["tb"] != 30 {
		t.Fatal("pool only sorted labels instead of weighted dispatch", counts)
	}
}

func TestPoolClaimsLockProjectsInStableOrderEvenWhenFairOrderDiffers(t *testing.T) {
	e, scopes := poolFixture(t)
	first := poolClaim(t, e, scopes, "seed-fair-turn")
	if err := e.Complete(ctx, first.TenantID, first.ProjectID, Result{RunID: first.RunID, NodeID: first.NodeID, Attempt: first.Attempt, Fence: first.Fence, WorkerID: first.WorkerID, OperationID: "seed-finish"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`CREATE TABLE pool_lock_trace(id INTEGER PRIMARY KEY,tenant_id TEXT,project_id TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`CREATE TRIGGER pool_project_lock_order AFTER UPDATE ON dag_scheduler_projects WHEN OLD.queue_sequence=NEW.queue_sequence BEGIN INSERT INTO pool_lock_trace(tenant_id,project_id)VALUES(NEW.tenant_id,NEW.project_id); END`); err != nil {
		t.Fatal(err)
	}
	in := PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: "different-fair-order"}
	for _, s := range scopes {
		in.Scopes = append(in.Scopes, PoolCapabilityScope{TenantID: s.TenantID, ProjectID: s.ProjectID, Capabilities: []string{"task"}})
	}
	a, err := e.ClaimPoolWithOperation(ctx, in, func(*sql.Tx) (PoolClaimAuthority, error) {
		return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 2, PoolActive: true, Scopes: scopes}, nil
	}, func(_ *sql.Tx, s PoolScope, _ string, _ string) (bool, error) { return s.TenantID == "ta", nil })
	if err != nil || a == nil {
		t.Fatal(a, err)
	}
	rows, err := e.db.Query(`SELECT tenant_id FROM pool_lock_trace ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := map[string]bool{}
	order := []string{}
	for rows.Next() {
		var tenant string
		rows.Scan(&tenant)
		if !seen[tenant] {
			seen[tenant] = true
			order = append(order, tenant)
		}
	}
	if len(order) != 2 || order[0] != "ta" || order[1] != "tb" {
		t.Fatal("pool fairness reversed project lock order", order)
	}
}

func TestPoolReplayBindingAndUnknownStopRetainsEffects(t *testing.T) {
	e, scopes := poolFixture(t)
	one := int64(1)
	policyTx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.WritePoolPolicyTx(ctx, policyTx, "pool", 1, PoolPolicyInput{ConcurrencyLimit: 1, ResourceLimits: map[string]*int64{"effect_lock": &one}, TenantWeights: map[string]int{"ta": 1, "tb": 3}}); err != nil {
		t.Fatal(err)
	}
	if err = policyTx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = e.db.Exec(`UPDATE dag_nodes SET spec=json_set(spec,'$.resources.effect_lock',1)`); err != nil {
		t.Fatal(err)
	}
	a := poolClaim(t, e, scopes, "replay-operation")
	again := poolClaim(t, e, scopes, "replay-operation")
	if again == nil || again.DispatchID != a.DispatchID || again.Attempt != a.Attempt {
		t.Fatal(a, again)
	}
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = ValidatePoolAssignmentTx(ctx, tx, "sqlite", *a); err != nil {
		t.Fatal(err)
	}
	forged := *a
	forged.ProjectID = "other"
	if err = ValidatePoolAssignmentTx(ctx, tx, "sqlite", forged); !errors.Is(err, ErrStale) {
		t.Fatal("forged project binding accepted", err)
	}
	tx.Rollback()
	if busy := poolClaim(t, e, scopes, "busy-slot-operation"); busy != nil {
		t.Fatal("machine slot double admitted", busy)
	}
	result := Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "unknown-stopped", Stopped: true, OutcomeUnknown: true}
	if err = e.Complete(ctx, a.TenantID, a.ProjectID, result); err != nil {
		t.Fatal(err)
	}
	observed := poolClaim(t, e, scopes, "replay-operation")
	if observed == nil || observed.DispatchState != "reconciling" {
		t.Fatal(observed)
	}
	var slots, process, effect int
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id='pool'`).Scan(&slots)
	e.db.QueryRow(`SELECT used FROM dag_execution_pool_accounts WHERE pool_id='pool' AND name='execution_slots'`).Scan(&process)
	e.db.QueryRow(`SELECT used FROM dag_execution_pool_accounts WHERE pool_id='pool' AND name='effect_lock'`).Scan(&effect)
	if slots != 0 || process != 0 || effect != 1 {
		t.Fatal("stopped process/effect holds conflated", slots, process, effect)
	}
	if next := poolClaim(t, e, scopes, "blocked-effect-operation"); next != nil {
		t.Fatal("unknown shared effect resource released", next)
	}
	current, _ := e.GetRun(ctx, a.TenantID, a.ProjectID, a.RunID)
	if _, err = e.Command(ctx, a.TenantID, a.ProjectID, a.RunID, "reconcile", "confirmed-effect", current.Version, map[string]any{"node_id": a.NodeID, "outcome": "failed", "reason": "independent external lookup confirms terminal effect"}); err != nil {
		t.Fatal(err)
	}
	if next := poolClaim(t, e, scopes, "new-after-reconciliation"); next == nil {
		t.Fatal("confirmed effect did not release shared resource")
	}
	var budget int
	e.db.QueryRow(`SELECT budget_spent FROM dag_execution_pool_policies WHERE pool_id='pool'`).Scan(&budget)
	if budget != 2 {
		t.Fatal("receipt or stop changed cumulative charges", budget)
	}
}

func TestPoolPriorityPersistsAcrossRestart(t *testing.T) {
	e, _ := fixture(t, Workflow{Nodes: []Node{{ID: "low", Type: "task"}, {ID: "urgent", Type: "task", Priority: 9}}})
	reopened, err := New(e.db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	a := claim(t, reopened)
	if a == nil || a.NodeID != "urgent" {
		t.Fatal("durable ready order lost fixed priority/age", a)
	}
}

func TestPoolScopedCapabilitiesCannotCrossProjects(t *testing.T) {
	e, scopes := poolFixture(t)
	in := PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: "scoped-capabilities", Scopes: []PoolCapabilityScope{{TenantID: "ta", ProjectID: "pa", Capabilities: []string{"task"}}, {TenantID: "tb", ProjectID: "pb", Capabilities: []string{"command:unsupported"}}}}
	a, err := e.ClaimPoolWithOperation(ctx, in, func(*sql.Tx) (PoolClaimAuthority, error) {
		return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 2, PoolActive: true, Scopes: scopes}, nil
	}, nil)
	if err != nil || a == nil || a.TenantID != "ta" {
		t.Fatal("global capability leaked into other project", a, err)
	}
	in.OperationID = "forged-capabilities"
	in.Scopes = append(in.Scopes, PoolCapabilityScope{TenantID: "foreign", ProjectID: "secret", Capabilities: []string{"task"}})
	if _, err = e.ClaimPoolWithOperation(ctx, in, func(*sql.Tx) (PoolClaimAuthority, error) {
		return PoolClaimAuthority{CredentialID: "cred", CredentialVersion: 1, MaxSlots: 2, PoolActive: true, Scopes: scopes}, nil
	}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestPoolPriorityUsesWaitingTimeWithinRun(t *testing.T) {
	for _, aged := range []bool{false, true} {
		t.Run(fmt.Sprint(aged), func(t *testing.T) {
			e, r := fixture(t, Workflow{Nodes: []Node{{ID: "low", Type: "task", Priority: 0}, {ID: "urgent", Type: "task", Priority: 9}}})
			if aged {
				if _, err := e.db.Exec(`UPDATE dag_ready_admission_order SET rank_at=rank_at-600 WHERE run_id=? AND node_id='low'`, r.ID); err != nil {
					t.Fatal(err)
				}
			}
			a := claim(t, e)
			want := "urgent"
			if aged {
				want = "low"
			}
			if a == nil || a.NodeID != want {
				t.Fatal("priority ignored or old work starved", a, want)
			}
		})
	}
}
