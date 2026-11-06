package platform

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
	"github.com/ai-mada/suma/internal/executioncontract"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func TestRunnerResultClosedRequiresExactReconciledOrdinaryAttempt(t *testing.T) {
	for _, pooled := range []bool{false, true} {
		name := "project"
		if pooled {
			name = "pool"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var f *fixture
			var a dag.Assignment
			var token string
			var claim func() map[string]any
			if pooled {
				p := newPoolMachineFixture(t)
				f, token = p.f, p.credential.Token
				claim = func() map[string]any {
					return f.bearer("POST", "/pool-runner/claim", token, dag.PoolClaimRequest{PoolID: p.pool.ID, RunnerID: p.credential.Runner.ID, WorkerID: p.credential.Runner.WorkerID, SlotID: "0", OperationID: newID(), Scopes: p.scopes[:1]}, 200)
				}
			} else {
				f = newFixture(t)
				w := f.call("POST", "/workflows", f.body(map[string]any{"name": "Ordinary result recovery", "nodes": []dag.Node{{ID: "inspect", Type: "command", Command: []string{"inspect"}}}}), 201)
				f.call("POST", "/runs", f.body(map[string]any{"workflow_id": w["id"]}), 201)
				token = f.call("POST", "/auth/tokens", f.body(map[string]any{"scope": "runner"}), 201)["token"].(string)
				claim = func() map[string]any {
					return f.bearer("POST", "/runner/claim", token, f.body(map[string]any{"worker_id": "result-worker", "capabilities": []string{"command:inspect"}}), 200)
				}
			}
			if err := json.Unmarshal([]byte(stringify(claim()["assignment"])), &a); err != nil || a.RunID == "" || a.Attempt != 1 {
				t.Fatalf("original attempt was not claimed: %+v, %v", a, err)
			}
			post := func(assignment dag.Assignment, result dag.Result, status int) map[string]any {
				t.Helper()
				if pooled {
					body := poolAssignmentBody(assignment)
					body["result"] = result
					return f.bearer("POST", "/pool-runner/results", token, body, status)
				}
				return f.bearer("POST", "/runner/results", token, f.body(map[string]any{"result": result}), status)
			}
			checkCode := func(reply map[string]any, want string) {
				t.Helper()
				errorBody, ok := reply["error"].(map[string]any)
				if !ok || errorBody["code"] != want {
					t.Fatalf("expected %q, got %+v", want, reply)
				}
			}
			late := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, OperationID: newID(), Output: "Finished while disconnected"}
			if _, err := f.s.DB.Exec(`UPDATE dag_nodes SET lease_until=0 WHERE run_id=? AND id=?`, a.RunID, a.NodeID); err != nil {
				t.Fatal(err)
			}
			checkCode(post(a, late, 409), "version_conflict")
			if _, err := f.s.DB.Exec(`UPDATE dag_wakeups SET due_at=1 WHERE run_id=? AND node_id=? AND kind='lease'`, a.RunID, a.NodeID); err != nil {
				t.Fatal(err)
			}
			if err := f.e.Tick(ctx); err != nil {
				t.Fatal(err)
			}
			checkCode(post(a, late, 409), "version_conflict")
			run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, a.RunID)
			if err != nil || len(run.Nodes) != 1 || run.Nodes[0].ID != a.NodeID || run.Nodes[0].State != "reconciling" {
				t.Fatalf("original attempt was not left in reconciliation: %+v, %v", run, err)
			}
			outcome := "failed"
			if pooled {
				outcome = "not_started"
			}
			if _, err = f.e.Command(ctx, f.user.TenantID, f.project, a.RunID, "reconcile", newID(), run.Version, map[string]any{"node_id": a.NodeID, "outcome": outcome, "reason": "Operator verified original execution independently"}); err != nil {
				t.Fatal(err)
			}
			var retry dag.Assignment
			if pooled {
				if err = json.Unmarshal([]byte(stringify(claim()["assignment"])), &retry); err != nil || retry.RunID != a.RunID || retry.Attempt != 2 || retry.Fence <= a.Fence {
					t.Fatalf("reconciliation did not allow a new claim: %+v, %v", retry, err)
				}
			}
			checkCode(post(a, late, 409), executioncontract.RunnerResultClosedCode)
			checkCode(post(a, late, 409), executioncontract.RunnerResultClosedCode)
			forgedAssignment, forged := a, late
			forgedAssignment.Fence, forged.Fence = a.Fence+100, a.Fence+100
			checkCode(post(forgedAssignment, forged, 409), "version_conflict")
			protected := late
			protected.ToolEvidence = &toolcontract.Evidence{Profile: toolTestProfile(), Terminal: "completed", EventCount: 1, UsageState: toolcontract.UsageUnreported}
			checkCode(post(a, protected, 409), "version_conflict")
			protected = late
			protected.Stopped = true
			checkCode(post(a, protected, 409), "version_conflict")
			var receiptCount int
			if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_operations WHERE tenant_id=? AND project_id=? AND scope=? AND operation_id=?`, f.user.TenantID, f.project, "result:"+a.RunID, late.OperationID).Scan(&receiptCount); err != nil || receiptCount != 0 {
				t.Fatalf("closed result was falsely acknowledged: %d, %v", receiptCount, err)
			}
			if pooled {
				current := late
				current.Attempt, current.Fence, current.WorkerID, current.OperationID = retry.Attempt, retry.Fence, retry.WorkerID, newID()
				for range 2 {
					if post(retry, current, 200)["acknowledged"] != true {
						t.Fatal("committed original result did not recover its acknowledgement")
					}
				}
				current.Output = "Changed body with an existing operation key"
				checkCode(post(retry, current, 409), "version_conflict")
			}
		})
	}
}
