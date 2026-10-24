package dag

import (
	"strings"
	"testing"
)

// LIMIT bounds returned candidates, but must not hide a full-run temporary
// sort. Inspect the exact SQL used by Claim, including its resume predicate.
func TestPoolReadyPriorityUsesOrderedIndex(t *testing.T) {
	e, _ := poolFixture(t)
	filter, params := capabilityFilter([]string{"task"})
	for _, resume := range []bool{false, true} {
		query, args := readyCandidateStatement("run", filter, params, claimScan{nodeRank: 10, nodeSequence: 1, node: "a"}, resume, 4)
		rows, err := e.db.Query(`EXPLAIN QUERY PLAN `+query, args...)
		if err != nil {
			t.Fatal(err)
		}
		plan := []string{}
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			plan = append(plan, detail)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("resume=%t plan=%v", resume, plan)
		seek := !resume
		for _, detail := range plan {
			if strings.Contains(detail, "rank_at") && strings.Contains(detail, ">") {
				seek = true
			}
			if strings.Contains(strings.ToUpper(detail), "TEMP B-TREE") {
				t.Fatalf("ready selection sorts the complete run before LIMIT: %v", plan)
			}
		}
		if !seek {
			t.Fatalf("resume scans old ready rows instead of seeking rank key: %v", plan)
		}
	}
}

func TestPoolAdmissionOrderDoesNotAccumulateCompletedOrRunningRows(t *testing.T) {
	e, scopes := poolFixture(t)
	a := poolClaim(t, e, scopes, "rank-projection-active")
	if a == nil {
		t.Fatal("missing real admission")
	}
	var ranks int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_ready_admission_order WHERE run_id=? AND node_id=?`, a.RunID, a.NodeID).Scan(&ranks); err != nil {
		t.Fatal(err)
	}
	if ranks != 0 {
		t.Fatal("active attempt retained stale rank, extending future scans", ranks)
	}
	if err := e.Complete(ctx, a.TenantID, a.ProjectID, Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: "rank-projection-finish"}); err != nil {
		t.Fatal(err)
	}
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM dag_ready_admission_order k LEFT JOIN dag_ready_v2 q ON q.run_id=k.run_id AND q.node_id=k.node_id WHERE q.node_id IS NULL`).Scan(&ranks); err != nil {
		t.Fatal(err)
	}
	if ranks != 0 {
		t.Fatal("rank projection retains nonready history", ranks)
	}
}
