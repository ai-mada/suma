package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	wbc "github.com/ai-mada/suma/internal/workbuddycloudcontract"
)

// ExternalResultResolution is produced only by a trusted control-plane resolver
// while the machine and original run are locked. No provider IO belongs here.
// NoExternalEffect requires proof that no central prepare/consumption occurred.
type ExternalResultResolution struct {
	NoExternalEffect bool
	Reason           string
}

type ExternalResultResolver func(context.Context, *sql.Tx, ExternalAssignment, Result) (ExternalResultResolution, error)

func (t *transaction) cloudSnapshot(r *runState, n *nodeState) (ExternalAssignment, error) {
	snapshot, err := ReadExternalAssignmentTx(t.ctx, t.tx, t.e.dialect, r.TenantID, r.ProjectID, r.ID, n.ID, n.Attempt)
	if err != nil {
		return snapshot, err
	}
	if snapshot.Protocol != wbc.Protocol || snapshot.Fence != n.Fence || snapshot.WorkerID != n.WorkerID || snapshot.Binding.Validate() != nil {
		return snapshot, ErrConflict
	}
	return snapshot, nil
}

func (t *transaction) completeCloudWait(r *runState, n *nodeState, result Result, hash, scope string, serviceBound bool, resolve ExternalResultResolver, commit func(*sql.Tx, string) error) error {
	e := result.WorkBuddyCloudWait
	if !serviceBound || resolve == nil || e == nil || e.Validate() != nil || result.Stopped || result.ExitCode == 0 ||
		result.WorkspaceEvidence != nil || result.ArtifactEvidence != nil || result.ReleaseEvidence != nil || result.IntegrationEvidence != nil ||
		result.ToolEvidence != nil || result.TestReport != nil || result.StartupFailure != nil {
		return dagDiagnostic(fmt.Errorf("%w: protected cloud result requires original assignment reconciliation", ErrInvalid), "dag.cloud.original_result_required", nil)
	}
	if n.State != "running" && n.State != "reconciling" && n.State != "waiting" {
		return ErrStale
	}
	snapshot, err := t.cloudSnapshot(r, n)
	if err != nil {
		return err
	}
	if snapshot.DispatchID != e.ClaimID || snapshot.Binding != e.Binding || !wbc.SameAssignmentIdentity(snapshot.Assignment, e.Assignment) {
		return ErrStale
	}
	// A timer may have parked the attempt before the original worker returned.
	// Only that first machine result can refine the timer's explicit unknown fact.
	var original string
	if err = t.row("SELECT result FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=? AND fence=?", r.ID, n.ID, n.Attempt, n.Fence).Scan(&original); err != nil {
		return err
	}
	if original != "" {
		var previous Result
		if json.Unmarshal([]byte(original), &previous) != nil || previous.WorkBuddyCloudWait != nil || previous.OperationID != "" {
			return ErrConflict
		}
	}
	decision, err := resolve(t.ctx, t.tx, snapshot, result)
	if err != nil {
		return err
	}
	state, reason := "waiting", decision.Reason
	if decision.NoExternalEffect {
		state, reason = "failed", "cloud.not_created"
		if r.State == "cancelling" {
			state = "cancelled"
		}
	} else {
		if !validCloudWaitReason(reason) {
			return ErrInvalid
		}
		if r.State == "cancelling" {
			reason = "cloud.stop_unconfirmed"
		}
	}
	n.Output, n.Error = result.Output, result.Error
	n.lease, n.wake, n.CompletedAt = 0, 0, ""
	if err = t.transition(r, n, state, reason); err != nil {
		return err
	}
	if err = t.releasePolicyResources(r, n, !decision.NoExternalEffect); err != nil {
		return err
	}
	completed := ""
	if terminal(state) {
		completed = stamp(t.now)
	}
	if err = t.exec("UPDATE dag_attempts SET state=?,completed_at=?,result=? WHERE run_id=? AND node_id=? AND attempt=? AND fence=?", state, completed, encoded(result), r.ID, n.ID, n.Attempt, n.Fence); err != nil {
		return err
	}
	if err = t.event(r, n.ID, "cloud_execution_reconciled", map[string]any{"attempt": n.Attempt, "fence": n.Fence, "dispatch_id": snapshot.DispatchID, "result_sha256": hash, "previous_result_sha256": hashText(original), "no_external_effect": decision.NoExternalEffect, "reason": reason}); err != nil {
		return err
	}
	if err = commit(t.tx, state); err != nil {
		return err
	}
	effective := result
	if decision.NoExternalEffect {
		effective.OutcomeUnknown = false
	}
	if err = t.recordAttemptAccounting(r, n, effective); err != nil {
		return err
	}
	if state == "failed" && r.State != "cancelling" {
		if _, err = t.scheduleTransientRetry(r, n, effective); err != nil {
			return err
		}
	}
	if err = t.advance(r); err != nil {
		return err
	}
	if err = t.save(r); err != nil {
		return err
	}
	return t.remember(r.TenantID, r.ProjectID, scope, result.OperationID, hash, r.ID)
}

func validCloudWaitReason(reason string) bool {
	switch reason {
	case "cloud.creation_unconfirmed", "cloud.remote_unknown", "cloud.waiting_input", "cloud.delivery_required", "cloud.interrupted", "cloud.authorization_changed", "cloud.observation_unconfirmed", "cloud.runner_disconnected", "cloud.stop_unconfirmed":
		return true
	default:
		return false
	}
}

// parkCloudLease never asserts the remote task stopped. Process reservations
// and the Pool slot are releasable; external-effect holds remain active.
func (t *transaction) parkCloudLease(r *runState, n *nodeState) error {
	if _, err := t.cloudSnapshot(r, n); err != nil {
		return err
	}
	reason := "cloud.runner_disconnected"
	if r.State == "cancelling" {
		reason = "cloud.stop_unconfirmed"
	}
	n.lease, n.wake, n.CompletedAt = 0, 0, ""
	if err := t.transition(r, n, "waiting", reason); err != nil {
		return err
	}
	if err := t.releasePolicyResources(r, n, true); err != nil {
		return err
	}
	return t.exec("UPDATE dag_attempts SET state='waiting',completed_at='' WHERE run_id=? AND node_id=? AND attempt=? AND fence=?", r.ID, n.ID, n.Attempt, n.Fence)
}
