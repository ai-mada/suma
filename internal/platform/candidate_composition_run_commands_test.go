package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/ai-mada/suma/internal/dag"
)

func TestCandidateCompositionManagedRetryPreservesAttemptAndQualityGuards(t *testing.T) {
	for _, mode := range []string{"known_failed_retry", "non_idempotent", "unknown", "source_drift"} {
		t.Run(mode, func(t *testing.T) {
			h, original := compositionConflictFixture(t, true)
			f, ctx := h.f, context.Background()
			req, profile, token := compositionRepairRequestFixture(t, h, original)
			w, err := f.e.Publish(ctx, f.user.TenantID, f.project, dag.Workflow{Name: "Explicit fixed retry policy", Nodes: []dag.Node{{ID: "compose", Type: "agent", Command: []string{"original-agent"}, Idempotent: mode != "non_idempotent", MaxRetries: 0, Config: map[string]any{"goal": "Resolve the frozen sources and retain independent quality gates"}}, {ID: "verify", Type: "test", Command: []string{"check"}, DependsOn: []string{"compose"}, Config: map[string]any{"candidate_from": "compose"}}}})
			if err != nil {
				t.Fatal(err)
			}
			req.WorkflowID = w.ID
			req.WorkflowVersion = w.Version
			p, err := f.s.CreateCandidateCompositionRepair(ctx, f.e, f.actor, f.project, original.ID, req)
			if err != nil {
				t.Fatal(err)
			}
			p = h.start(t, p)
			reply := f.bearer("POST", "/runner/claim", token, map[string]any{"project_id": f.project, "worker_id": profile.WorkerID, "capabilities": []string{"command:" + profile.Profile.CommandID}, "idempotency_key": newID()}, 200)
			var a dag.Assignment
			if err = json.Unmarshal([]byte(stringify(reply["assignment"])), &a); err != nil || a.RunID != p.RunID || a.Attempt != 1 || a.CandidateConflictRepair == nil {
				t.Fatal(reply, err)
			}
			failure := dag.Result{RunID: a.RunID, NodeID: a.NodeID, Attempt: a.Attempt, Fence: a.Fence, WorkerID: a.WorkerID, ExitCode: -1, Error: "unsupported agent purpose", OperationID: newID(), OutcomeUnknown: mode == "unknown"}
			f.bearer("POST", "/runner/results", token, map[string]any{"project_id": f.project, "result": failure}, 200)
			run, err := f.e.GetRun(ctx, f.user.TenantID, f.project, p.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "source_drift" {
				_, err = f.s.Update(ctx, f.actor, f.project, "tasks", p.Sources[0].TaskID, f.body(map[string]any{"expected_version": p.Sources[0].TaskVersion, "description": "Changed source cannot be resumed through managed commands"}))
				if err != nil {
					t.Fatal(err)
				}
			}
			command := f.body(map[string]any{"action": "retry", "expected_version": run.Version, "payload": map[string]any{"node_id": a.NodeID, "reason": "Explicit retry of known pre-workspace failure under unchanged fixed policy"}})
			want := 409
			if mode == "known_failed_retry" {
				want = 200
			}
			f.call("POST", "/runs/"+p.RunID+"/commands", command, want)
			first, err := f.e.GetAttempt(ctx, f.user.TenantID, f.project, p.RunID, a.NodeID, 1)
			if err != nil || first.Result == nil || first.Result.Error != failure.Error || first.Result.Fence != a.Fence || first.Result.OutcomeUnknown != failure.OutcomeUnknown {
				t.Fatal("retry rewrote the original attempt", first, err)
			}
			if mode != "known_failed_retry" {
				var count int
				if err = f.s.DB.QueryRow(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=? AND node_id=?`, p.RunID, a.NodeID).Scan(&count); err != nil || count != 1 {
					t.Fatal(count, err)
				}
				if mode == "non_idempotent" {
					normal, err := f.e.GetRun(ctx, f.user.TenantID, f.project, original.RunID)
					if err != nil {
						t.Fatal(err)
					}
					f.call("POST", "/runs/"+original.RunID+"/commands", f.body(map[string]any{"action": "retry", "expected_version": normal.Version, "payload": map[string]any{"node_id": original.ProducerNodeID, "reason": "Frozen normal composition is non-idempotent"}}), 409)
				}
				return
			}
			f.call("POST", "/runs/"+p.RunID+"/commands", command, 200)
			reset, err := f.e.GetRun(ctx, f.user.TenantID, f.project, p.RunID)
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range reset.Nodes {
				if node.ID == "verify" && (node.State != "pending" || node.Attempt != 0) {
					t.Fatal("skipped quality was not reset without executing", node)
				}
			}
			p, err = f.s.GetCandidateComposition(ctx, f.e, f.actor, f.project, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			p, _ = compositionRepairCompleteFixture(t, h, p, profile, token, nil)
			second, err := f.e.GetAttempt(ctx, f.user.TenantID, f.project, p.RunID, a.NodeID, 2)
			if err != nil || second.Result == nil || second.Result.ExitCode != 0 || second.Fence <= a.Fence {
				t.Fatal(second, err)
			}
			finished, err := f.e.GetRun(ctx, f.user.TenantID, f.project, p.RunID)
			if err != nil {
				t.Fatal(err)
			}
			for _, node := range finished.Nodes {
				if node.ID == "verify" && (node.State != "succeeded" || node.Attempt != 1 || number(node.WorkspaceEvidence, "candidate_attempt") != 2) {
					t.Fatal("formal quality did not test exactly retry attempt2 once", node)
				}
			}
			if p.State != "awaiting_review" {
				t.Fatal("retry/test success performed implicit Review", p)
			}
		})
	}
	t.Run("unrelated_managed_run_stays_denied", func(t *testing.T) {
		f, ctx := newFixture(t), context.Background()
		run, err := f.e.CreateManagedRunBound(ctx, f.user.TenantID, f.project, newID(), newID(), dag.Workflow{Name: "Protected unrelated managed fixture", Nodes: []dag.Node{{ID: "code", Type: "command", Command: []string{"develop"}}}}, func(*sql.Tx) error { return nil }, func(*sql.Tx, string) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		f.call("POST", "/runs/"+run.ID+"/commands", f.body(map[string]any{"action": "pause", "expected_version": run.Version}), 400)
	})
}
