package dag

import (
	"strings"
	"testing"
)

func removeSchedulingDerivedSchema(t *testing.T, e *Engine) {
	t.Helper()
	for i := len(schedulingSchema) - 1; i >= 0; i-- {
		name := strings.Fields(schedulingSchema[i])[5]
		name = strings.Split(name, "(")[0]
		if _, err := e.db.Exec(`DROP TABLE ` + name); err != nil {
			t.Fatal(name, err)
		}
	}
}

func TestLegacySchedulingMigrationPreservesFactsUnknownSlotsAndKnownBudget(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}, {ID: "b", Type: "task"}}})
	a := claim(t, e)
	if a == nil {
		t.Fatal("missing initial attempt")
	}
	if _, err := e.db.Exec(`UPDATE dag_nodes SET state='reconciling' WHERE run_id=? AND id=?`, r.ID, a.NodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`UPDATE dag_attempts SET state='reconciling' WHERE run_id=?`, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`DELETE FROM dag_reservations WHERE name='execution_slots'`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.db.Exec(`INSERT INTO dag_resource_limits(tenant_id,project_id,name,capacity,consumed)VALUES('t','p','budget',2,2)`); err != nil {
		t.Fatal(err)
	}
	var beforeVersion, beforeEvents int
	e.db.QueryRow(`SELECT version FROM dag_runs WHERE id=?`, r.ID).Scan(&beforeVersion)
	e.db.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=?`, r.ID).Scan(&beforeEvents)
	// Reconstruct the actual pre-stage schema on this disposable database.
	removeSchedulingDerivedSchema(t, e)
	reopened, err := New(e.db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		tx, err := e.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		policy, err := reopened.ReadSchedulingPolicyTx(ctx, tx, "t", "p")
		tx.Rollback()
		if err != nil {
			t.Fatal(err)
		}
		if policy.ConcurrencyLimit != 1 || policy.BudgetSpent != 2 || policy.BudgetHistory != "partial" {
			t.Fatalf("legacy policy changed facts: %+v", policy)
		}
		var slot int64
		if err = e.db.QueryRow(`SELECT used FROM dag_resource_accounts WHERE tenant_id='t' AND project_id='p' AND name='execution_slots'`).Scan(&slot); err != nil || slot != 1 {
			t.Fatalf("unknown old attempt lost slot: %d %v", slot, err)
		}
		if next, err := reopened.Claim(ctx, "t", "p", "second", testCapabilities(t, reopened)); err != nil || next != nil {
			t.Fatal("old unknown attempt permitted over-admission", next, err)
		}
		var version, events int
		e.db.QueryRow(`SELECT version FROM dag_runs WHERE id=?`, r.ID).Scan(&version)
		e.db.QueryRow(`SELECT COUNT(*) FROM dag_events WHERE run_id=?`, r.ID).Scan(&events)
		if version != beforeVersion || events != beforeEvents {
			t.Fatalf("migration rewrote run history: %d/%d -> %d/%d", beforeVersion, beforeEvents, version, events)
		}
		reopened, err = New(e.db, "sqlite")
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyActiveRunKeepsFixedJoinGeneration(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "route", Type: "condition", Branches: map[string]string{"east": "true", "west": "false"}}, {ID: "east", Type: "task"}, {ID: "west", Type: "task"}, {ID: "merge", Type: "noop", DependsOn: []string{"east", "west"}}}, Edges: []Edge{{From: "route", To: "east", Condition: "east"}, {From: "route", To: "west", Condition: "west"}}})
	removeSchedulingDerivedSchema(t, e)
	legacy, err := New(e.db, "sqlite")
	if err != nil {
		t.Fatal(err)
	}
	var generation int
	e.db.QueryRow(`SELECT kernel_generation FROM dag_run_generations WHERE run_id=?`, r.ID).Scan(&generation)
	if generation != 1 {
		t.Fatal(generation)
	}
	done(t, legacy, claim(t, legacy), 0)
	current, err := legacy.GetRun(ctx, "t", "p", r.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range current.Nodes {
		if n.ID == "merge" && n.State != "skipped" {
			t.Fatal("migration silently changed already fixed join semantics", n)
		}
	}
}
