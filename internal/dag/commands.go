package dag

import (
	"context"
	"database/sql"
	"fmt"
)

func (e *Engine) Command(ctx context.Context, tenant, project, runID, action, operationID string, expectedVersion int, payload map[string]any) (Run, error) {
	return e.CommandWithHook(ctx, tenant, project, runID, action, operationID, expectedVersion, payload, nil)
}

// CommandWithHook commits service-owned changes with the command and receipt.
// The hook uses tx exclusively; it must not call Engine or external services.
// Receipt replay returns before invoking the hook.
func (e *Engine) CommandWithHook(ctx context.Context, tenant, project, runID, action, operationID string, expectedVersion int, payload map[string]any, after func(*sql.Tx, Run) error) (Run, error) {
	return e.CommandWithAuthority(ctx, tenant, project, runID, action, operationID, expectedVersion, payload, nil, after)
}

// CommandWithAuthority verifies current service authority before the scheduler
// lock and before recovering a cached receipt. Hooks use the same tx only.
func (e *Engine) CommandWithAuthority(ctx context.Context, tenant, project, runID, action, operationID string, expectedVersion int, payload map[string]any, before func(*sql.Tx) error, after func(*sql.Tx, Run) error) (Run, error) {
	var out Run
	err := e.writeBefore(ctx, before, func(t *transaction) error {
		if err := t.beforeScope(tenant, project); err != nil {
			return err
		}
		if t.e.BoundSubworkflows != nil {
			if err := t.e.BoundSubworkflows.AuthorizeBoundRunCommandTx(ctx, t.tx, tenant, project, runID, action); err != nil {
				return err
			}
		}
		scope := "command:" + runID
		hash := fingerprint(map[string]any{"action": action, "payload": payload})
		previous, err := t.operation(tenant, project, scope, operationID, hash)
		if err != nil {
			return err
		}
		r, err := t.loadFull(tenant, project, runID)
		if err != nil {
			return err
		}
		if previous != "" {
			out = r.public()
			return nil
		}
		if expectedVersion < 1 || expectedVersion != r.Version {
			return dagDiagnostic(fmt.Errorf("%w: expected version %d, current %d", ErrConflict, expectedVersion, r.Version), "dag.run.version", diagnosticParams{"expected": fmt.Sprintf("%d", expectedVersion), "current": r.Version})
		}
		if err = t.checkParentCommand(r, action); err != nil {
			return err
		}
		nodeID, _ := payload["node_id"].(string)
		n := r.find(nodeID)
		reason, _ := payload["reason"].(string)
		switch action {
		case "pause":
			if r.State != "running" {
				return dagDiagnostic(fmt.Errorf("%w: only active runs can pause", ErrConflict), "dag.run.pause_active", nil)
			}
			r.State = "paused"
		case "resume":
			if r.State != "paused" {
				return dagDiagnostic(fmt.Errorf("%w: run is not paused", ErrConflict), "dag.run.not_paused", nil)
			}
			r.State = "running"
			if err = t.resumeRunWakes(r); err != nil {
				return err
			}
		case "cancel":
			if terminal(r.State) {
				return dagDiagnostic(fmt.Errorf("%w: run already terminal", ErrConflict), "dag.run.terminal", nil)
			}
			if err = t.cancelRun(r, reason); err != nil {
				return err
			}
		case "approval", "approve":
			if n == nil {
				return ErrNotFound
			}
			if n.Type != "approval" || n.State != "waiting" || r.State == "cancelling" || terminal(r.State) {
				return dagDiagnostic(fmt.Errorf("%w: approval is not waiting", ErrConflict), "dag.run.approval_waiting", nil)
			}
			decision, _ := payload["decision"].(string)
			state := ""
			switch decision {
			case "approve", "approved", "accepted":
				state = "succeeded"
			case "reject", "rejected":
				state = "failed"
			default:
				return dagDiagnostic(fmt.Errorf("%w: decision approve or reject required", ErrInvalid), "dag.run.approval_decision", nil)
			}
			if err = t.transition(r, n, state, reason); err != nil {
				return err
			}
		case "retry":
			if r.CompensationRunID != "" {
				return dagDiagnostic(fmt.Errorf("%w: compensated source cannot be retried; create a new run", ErrConflict), "dag.run.compensated_retry", nil)
			}
			if n == nil {
				return ErrNotFound
			}
			if WorkBuddyCloudNode(n.spec) && n.Reason != "cloud.not_created" {
				return dagDiagnostic(fmt.Errorf("%w: cloud effects must be reconciled before another attempt", ErrConflict), "dag.cloud.retry_requires_reconciliation", nil)
			}
			if r.State == "cancelling" || r.State == "cancelled" || n.State != "failed" || !n.spec.Idempotent {
				return dagDiagnostic(fmt.Errorf("%w: retry requires failed idempotent node", ErrConflict), "dag.run.retry_node", nil)
			}
			if n.Type == "map" {
				return dagDiagnostic(fmt.Errorf("%w: retry individual failed map children", ErrUnsupported), "dag.run.retry_map_child", nil)
			}
			descendants := map[string]bool{n.ID: true}
			if n.parent != "" {
				descendants[n.parent] = true
			}
			for {
				changed := false
				for _, edge := range r.workflow.Edges {
					if descendants[edge.From] && !descendants[edge.To] {
						descendants[edge.To] = true
						changed = true
					}
				}
				if !changed {
					break
				}
			}
			for _, item := range r.nodes {
				if descendants[item.ID] && item.ID != n.ID && item.ID != n.parent && item.State != "skipped" && item.State != "pending" {
					return dagDiagnostic(fmt.Errorf("%w: descendant %s already executed", ErrConflict, item.ID), "dag.run.descendant_executed", diagnosticParams{"node": item.ID})
				}
			}
			r.State = "running"
			for _, item := range r.nodes {
				if descendants[item.ID] {
					item.CompletedAt = ""
					item.Error = ""
					item.Output = ""
					item.WorkerID = ""
					state := "pending"
					if item.ID == n.parent {
						state = "waiting"
					}
					if err = t.transition(r, item, state, reason); err != nil {
						return err
					}
				}
			}
		case "compensate":
			if err = t.compensate(r, payload); err != nil {
				return err
			}
		case "reconcile":
			if n == nil {
				return ErrNotFound
			}
			if WorkBuddyCloudNode(n.spec) {
				return dagDiagnostic(fmt.Errorf("%w: original cloud operation requires service evidence", ErrConflict), "dag.cloud.manual_result_forbidden", nil)
			}
			if n.State != "reconciling" || reason == "" {
				return dagDiagnostic(fmt.Errorf("%w: reconciliation needs unknown outcome and evidence reason", ErrConflict), "dag.run.reconciliation_evidence", nil)
			}
			outcome, _ := payload["outcome"].(string)
			state := ""
			switch outcome {
			case "not_started":
				state = "pending"
			case "succeeded":
				state = "succeeded"
			case "failed":
				state = "failed"
			default:
				return dagDiagnostic(fmt.Errorf("%w: invalid reconciliation outcome", ErrInvalid), "dag.run.reconciliation_outcome", nil)
			}
			if err = t.resolveAttemptAccounting(r, n, outcome); err != nil {
				return err
			}
			if r.State == "cancelling" && state == "pending" {
				state = "cancelled"
			}
			if err = t.release(r, n); err != nil {
				return err
			}
			if err = t.exec("UPDATE dag_attempts SET state=?,completed_at=?,result=? WHERE run_id=? AND node_id=? AND attempt=?", state, stamp(t.now), encoded(payload), r.ID, n.ID, n.Attempt); err != nil {
				return err
			}
			if err = t.transition(r, n, state, reason); err != nil {
				return err
			}
			if state == "pending" {
				n.WorkerID = ""
				n.CompletedAt = ""
				n.lease = 0
			}
		default:
			return dagDiagnostic(fmt.Errorf("%w: unknown run command %q", ErrInvalid, action), "dag.run.command", diagnosticParams{"action": fmt.Sprintf("%q", action)})
		}
		if action == "pause" || action == "resume" || action == "cancel" {
			if err = t.controlSubruns(r, action); err != nil {
				return err
			}
		}
		if err = t.event(r, nodeID, "command_"+action, payload); err != nil {
			return err
		}
		if err = t.advance(r); err != nil {
			return err
		}
		if err = t.save(r); err != nil {
			return err
		}
		if after != nil {
			if err = after(t.tx, r.public()); err != nil {
				return err
			}
		}
		if err = t.remember(tenant, project, scope, operationID, hash, r.ID); err != nil {
			return err
		}
		out = r.public()
		return nil
	})
	return out, err
}
