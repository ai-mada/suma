package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// LiveReadOnlyCompletion identifies one still-leased attempt and its immutable
// specification. This is a completion proof, never authorization to rerun it.
type LiveReadOnlyCompletion struct {
	TenantID, ProjectID, RunID, NodeID, WorkerID string
	Attempt                                      int
	Fence                                        int64
	Spec                                         Node
}

// LiveReadOnlyCompletionReconciler is configured at startup. Its proof runs in
// the current completion transaction after tool evidence validation, uses tx
// only, and must not call Engine or perform network I/O. False retains ErrStale.
type LiveReadOnlyCompletionReconciler interface {
	CheckLiveReadOnlyCompletionTx(context.Context, *sql.Tx, LiveReadOnlyCompletion, Result, Result) (bool, error)
}

func noCompletionEffects(result Result) bool {
	return result.WorkspaceEvidence == nil && result.ArtifactEvidence == nil && result.ReleaseEvidence == nil && result.IntegrationEvidence == nil && result.TestReport == nil
}

func (t *transaction) liveReadOnlyCompletionCandidate(r *runState, n *nodeState, final Result, serviceBound bool) bool {
	return serviceBound && t.e.AttemptReconciliation != nil && n.State == "reconciling" && n.lease > t.now && r.State != "cancelling" && n.spec.Type == "agent" &&
		!final.Stopped && !final.OutcomeUnknown && final.ExitCode == 0 && final.Error == "" && noCompletionEffects(final) && final.ToolEvidence != nil && final.ToolEvidence.Profile.ReadOnly && final.ToolEvidence.Terminal == "completed"
}

// Called only after ordinary tool-result validation. The scheduler proves the
// same live tuple and retained slot; the service proves its managed purpose and
// current authority. Both original and formal facts survive atomically.
func (t *transaction) reconcileLiveReadOnlyCompletion(r *runState, n *nodeState, final Result) error {
	var original string
	err := t.row(`SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`, r.ID, n.ID, n.Attempt, n.Fence, n.WorkerID).Scan(&original)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrStale
	}
	if err != nil {
		return err
	}
	var previous Result
	if json.Unmarshal([]byte(original), &previous) != nil || !previous.OutcomeUnknown || previous.Stopped || !noCompletionEffects(previous) || previous.RunID != r.ID || previous.NodeID != n.ID || previous.Attempt != n.Attempt || previous.Fence != n.Fence || previous.WorkerID != n.WorkerID {
		return ErrStale
	}
	var held int
	if err = t.row(`SELECT COUNT(*) FROM dag_resource_ledger WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND name='execution_slots' AND state='active'`, r.ID, n.ID, n.Attempt, n.Fence).Scan(&held); err != nil {
		return err
	}
	if held != 1 {
		return ErrStale
	}
	if r.ExecutionPoolID != "" {
		if err = t.row(`SELECT COUNT(*) FROM dag_execution_pool_slots WHERE pool_id=? AND run_id=? AND node_id=? AND attempt=? AND fence=?`, r.ExecutionPoolID, r.ID, n.ID, n.Attempt, n.Fence).Scan(&held); err != nil {
			return err
		}
		if held != 1 {
			return ErrStale
		}
	}
	// A hook receives independent snapshots, so even a mistaken map/pointer
	// mutation cannot rewrite the workflow, caller result, or accounting input.
	var spec Node
	var proof Result
	if err = json.Unmarshal([]byte(encoded(n.spec)), &spec); err != nil {
		return err
	}
	if err = json.Unmarshal([]byte(encoded(final)), &proof); err != nil {
		return err
	}
	scope := LiveReadOnlyCompletion{TenantID: r.TenantID, ProjectID: r.ProjectID, RunID: r.ID, NodeID: n.ID, WorkerID: n.WorkerID, Attempt: n.Attempt, Fence: n.Fence, Spec: spec}
	allowed, err := t.e.AttemptReconciliation.CheckLiveReadOnlyCompletionTx(t.ctx, t.tx, scope, previous, proof)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrStale
	}
	if err = t.exec(`INSERT INTO dag_managed_resolutions(run_id,node_id,attempt,operation_id,original_result,resolution_result,resolved_at)VALUES(?,?,?,?,?,?,?)`, r.ID, n.ID, n.Attempt, final.OperationID, original, encoded(final), stamp(t.now)); err != nil {
		return err
	}
	return t.event(r, n.ID, "live_read_only_completion_reconciled", map[string]any{"attempt": n.Attempt, "previous_result_sha256": hashText(original), "final_result_sha256": hashText(encoded(final)), "resolution_operation_id": final.OperationID})
}
