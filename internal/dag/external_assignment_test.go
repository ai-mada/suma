package dag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/store"
	wc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

func TestExternalAssignmentProtectedClaimSnapshotAndTamper(t *testing.T) {
	e, r := fixture(t, Workflow{Nodes: []Node{{ID: "cloud", Type: wc.NodeType, Config: map[string]any{"plugin": wc.Plugin, "workflow": wc.Protocol}}}})
	caps := []string{"external_workflow:workbuddy-cloud"}
	if _, err := e.ClaimWithOperation(ctx, "t", "p", "worker", "unprotected", caps, nil, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("unprotected cloud claim: %v", err)
	}
	b := wc.CloudAssignmentBinding{DelegationID: "delegation", DelegationVersion: 1, DelegationSHA256: strings.Repeat("a", 64)}
	hook := &RunnableHook{BindAssignmentTx: func(_ context.Context, _ *sql.Tx, a *Assignment) error { a.WorkBuddyCloud = &b; return nil }, ValidateReplayTx: func(context.Context, *sql.Tx, Assignment) error { return nil }}
	a, err := e.ClaimWithOperation(ctx, "t", "p", "worker", "protected", caps, nil, nil, hook)
	if err != nil || a == nil {
		t.Fatalf("protected claim: %v", err)
	}
	tx, _ := e.db.BeginTx(ctx, nil)
	snapshot, err := ReadExternalAssignmentTx(ctx, tx, "sqlite", "t", "p", r.ID, "cloud", a.Attempt)
	if err != nil || snapshot.Binding != b || snapshot.DispatchID != a.DispatchID || snapshot.Assignment.ProjectID != "" {
		t.Fatalf("snapshot: %#v %v", snapshot, err)
	}
	tx.Rollback()
	if _, err = e.db.Exec(`UPDATE dag_external_assignments SET assignment_sha256=?`, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	tx, _ = e.db.BeginTx(ctx, nil)
	defer tx.Rollback()
	if _, err = ReadExternalAssignmentTx(ctx, tx, "sqlite", "t", "p", r.ID, "cloud", a.Attempt); !errors.Is(err, ErrConflict) {
		t.Fatalf("tamper accepted: %v", err)
	}
}

func TestExternalAssignmentInitialSchemaRejectsBeforeWrites(t *testing.T) {
	for _, kind := range []string{"missing", "columns", "keys"} {
		t.Run(kind, func(t *testing.T) {
			e := openEngine(t, filepath.Join(t.TempDir(), "schema.db"))
			if _, err := e.db.Exec(`DROP TABLE dag_external_assignments`); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "columns":
				if _, err := e.db.Exec(`CREATE TABLE dag_external_assignments(id TEXT PRIMARY KEY,original_note TEXT)`); err != nil {
					t.Fatal(err)
				}
			case "keys":
				ddl := strings.ReplaceAll(externalAssignmentDDL, " NOT NULL UNIQUE", " NOT NULL")
				if _, err := e.db.Exec(ddl); err != nil {
					t.Fatal(err)
				}
			}
			var before int
			e.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&before)
			if _, err := New(e.db, "sqlite"); !errors.Is(err, store.ErrInitialSchema) {
				t.Fatal("incomplete current database accepted", err)
			}
			var after int
			e.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&after)
			if before != after {
				t.Fatal("rejected initialization wrote schema")
			}
		})
	}
}

func TestClaimDiscoveryUsesReadOnlyFairWindow(t *testing.T) {
	t.Run("private_cursor", func(t *testing.T) {
		w := Workflow{Nodes: []Node{}}
		for i := 0; i < 80; i++ {
			w.Nodes = append(w.Nodes, Node{ID: fmt.Sprintf("n%03d", i), Type: "task"})
		}
		e, _ := fixture(t, w)
		caps := []string{"task"}
		if _, err := e.ClaimWithOperation(ctx, "t", "p", "worker", "advance-cursor", caps, nil, func(*sql.Tx, string, string) (bool, error) { return false, nil }); err != nil {
			t.Fatal(err)
		}
		tx, _ := e.db.BeginTx(ctx, nil)
		var before, after int
		tx.QueryRow(`SELECT total_changes()`).Scan(&before)
		items, err := DiscoverClaimCandidatesTx(ctx, tx, "sqlite", "t", "p", "worker", "next-operation", caps)
		if err != nil {
			t.Fatal(err)
		}
		tx.QueryRow(`SELECT total_changes()`).Scan(&after)
		tx.Rollback()
		if len(items) != 16 || before != after {
			t.Fatal("discovery mutated cursor or ignored bounded resume", len(items), before, after)
		}
		a, err := e.ClaimWithOperation(ctx, "t", "p", "worker", "next-operation", caps, nil, nil)
		if err != nil || a == nil || a.NodeID != items[0].NodeID {
			t.Fatal("actual claim diverged from peek", a, err)
		}
	})
	t.Run("pool_fair_cursor", func(t *testing.T) {
		e, scopes := poolFixture(t)
		in := PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: "peek-pool-op"}
		for _, s := range scopes {
			in.Scopes = append(in.Scopes, PoolCapabilityScope{TenantID: s.TenantID, ProjectID: s.ProjectID, Capabilities: []string{"task"}})
		}
		// Exercise the same epoch normalization as the actual mutating fair rank.
		for _, row := range []struct {
			tenant, project string
			pass, floor     int64
		}{{"", "", 0, poolFairRebase}, {"ta", "", poolFairRebase + 100, 0}, {"tb", "", poolFairRebase, poolFairRebase}, {"tb", "pb", poolFairRebase + 10, 0}} {
			if _, err := e.db.Exec(`INSERT INTO dag_execution_pool_fairness(pool_id,tenant_id,project_id,pass,remainder,weight,floor,epoch,last_sequence)VALUES('pool',?,?,?,0,1,?,0,0)`, row.tenant, row.project, row.pass, row.floor); err != nil {
				t.Fatal(err)
			}
		}
		tx, _ := e.db.BeginTx(ctx, nil)
		var before, after int
		tx.QueryRow(`SELECT total_changes()`).Scan(&before)
		items, err := DiscoverPoolClaimCandidatesTx(ctx, tx, "sqlite", in)
		if err != nil {
			t.Fatal(err)
		}
		tx.QueryRow(`SELECT total_changes()`).Scan(&after)
		tx.Rollback()
		if len(items) == 0 || len(items) > 64 || before != after {
			t.Fatal("pool discovery wrote fairness/cursor", len(items), before, after)
		}
		a := poolClaim(t, e, scopes, in.OperationID)
		if a == nil || a.TenantID != items[0].TenantID || a.ProjectID != items[0].ProjectID || a.NodeID != items[0].NodeID {
			t.Fatal("pool rank diverged from discovery", a, items[0])
		}
	})
}
