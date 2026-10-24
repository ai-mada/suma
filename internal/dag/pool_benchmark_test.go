package dag

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"
)

// These metrics count database/sql calls and returned rows, not SQLite VM work
// or cluster throughput. The query-plan regression separately checks index use.
func BenchmarkPoolAdmission(b *testing.B) {
	for _, ready := range []int{100, 1000, 10000} {
		runs := (ready + 999) / 1000
		b.Run(fmt.Sprintf("one_of_%d_ready_in_%d_runs", ready, runs), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				e, stats := schedulingMeasuredEngine(b)
				tx, err := e.db.Begin()
				if err != nil {
					b.Fatal(err)
				}
				if _, err = e.WritePoolPolicyTx(context.Background(), tx, "pool", 0, PoolPolicyInput{ConcurrencyLimit: 1}); err != nil {
					tx.Rollback()
					b.Fatal(err)
				}
				scope := PoolScope{PoolID: "pool", TenantID: "tenant", ProjectID: "project", GrantID: "grant", GrantVersion: 2, ProjectWeight: 1, Status: "active"}
				if err = e.WritePoolScopeTx(context.Background(), tx, scope); err != nil {
					tx.Rollback()
					b.Fatal(err)
				}
				if _, err = e.WriteProjectExecutionRouteTx(context.Background(), tx, "tenant", "project", 0, ProjectExecutionRouteInput{PoolID: "pool", GrantID: "grant", GrantVersion: 2}); err != nil {
					tx.Rollback()
					b.Fatal(err)
				}
				if err = tx.Commit(); err != nil {
					b.Fatal(err)
				}
				w := Workflow{ID: "ready", Name: "Pool admission measurement"}
				perRun := ready / runs
				for j := 0; j < perRun; j++ {
					w.Nodes = append(w.Nodes, Node{ID: fmt.Sprintf("n%05d", j), Type: "task", Priority: j % 10})
				}
				if _, err = e.Publish(context.Background(), "tenant", "project", w); err != nil {
					b.Fatal(err)
				}
				deadline, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				for run := 0; run < runs; run++ {
					if _, err = e.CreateRun(deadline, "tenant", "project", w.ID, fmt.Sprintf("create-measurement-%d", run)); err != nil {
						cancel()
						b.Fatal(err)
					}
				}
				cancel()
				stats.reset()
				deadline, cancel = context.WithTimeout(context.Background(), 30*time.Second)
				b.StartTimer()
				a, err := e.ClaimPoolWithOperation(deadline, PoolClaimRequest{PoolID: "pool", RunnerID: "runner", WorkerID: "worker", SlotID: "0", OperationID: "measure-one-admission", Scopes: []PoolCapabilityScope{{TenantID: "tenant", ProjectID: "project", Capabilities: []string{"task"}}}}, func(*sql.Tx) (PoolClaimAuthority, error) {
					return PoolClaimAuthority{CredentialID: "credential", CredentialVersion: 1, MaxSlots: 1, PoolActive: true, Scopes: []PoolScope{scope}}, nil
				}, nil)
				b.StopTimer()
				cancel()
				schedulingReportBench(b, stats, err)
				if err != nil || a == nil || a.NodeID != "n00009" {
					b.Fatal("priority-aware actual admission failed", a, err)
				}
			}
		})
	}
}
