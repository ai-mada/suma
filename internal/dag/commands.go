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
		scope := "command:" + runID
		hash := fingerprint(map[string]any{"action": action, "payload": payload})
		previous, err := t.operation(tenant, project, scope, operationID, hash)
		if err != nil {
			return err
		}
		r, err := load(ctx, t.tx, e, tenant, project, runID)
		if err != nil {
			return err
		}
		if previous != "" {
			out = r.public()
			return nil
		}
		if expectedVersion < 1 || expectedVersion != r.Version {
			return fmt.Errorf("%w: expected version %d, current %d", ErrConflict, expectedVersion, r.Version)
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
				return fmt.Errorf("%w: only active runs can pause", ErrConflict)
			}
			r.State = "paused"
		case "resume":
			if r.State != "paused" {
				return fmt.Errorf("%w: run is not paused", ErrConflict)
			}
			r.State = "running"
		case "cancel":
			if terminal(r.State) {
				return fmt.Errorf("%w: run already terminal", ErrConflict)
			}
			if err = t.cancelRun(r, reason); err != nil {
				return err
			}
		case "approval", "approve":
			if n == nil {
				return ErrNotFound
			}
			if n.Type != "approval" || n.State != "waiting" || r.State == "cancelling" || terminal(r.State) {
				return fmt.Errorf("%w: approval is not waiting", ErrConflict)
			}
			decision, _ := payload["decision"].(string)
			state := ""
			switch decision {
			case "approve", "approved", "accepted":
				state = "succeeded"
			case "reject", "rejected":
				state = "failed"
			default:
				return fmt.Errorf("%w: decision approve or reject required", ErrInvalid)
			}
			if err = t.transition(r, n, state, reason); err != nil {
				return err
			}
		case "retry":
			if r.CompensationRunID != "" {
				return fmt.Errorf("%w: compensated source cannot be retried; create a new run", ErrConflict)
			}
			if n == nil {
				return ErrNotFound
			}
			if r.State == "cancelling" || r.State == "cancelled" || n.State != "failed" || !n.spec.Idempotent {
				return fmt.Errorf("%w: retry requires failed idempotent node", ErrConflict)
			}
			if n.Type == "map" {
				return fmt.Errorf("%w: retry individual failed map children", ErrUnsupported)
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
					return fmt.Errorf("%w: descendant %s already executed", ErrConflict, item.ID)
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
			if n.State != "reconciling" || reason == "" {
				return fmt.Errorf("%w: reconciliation needs unknown outcome and evidence reason", ErrConflict)
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
				return fmt.Errorf("%w: invalid reconciliation outcome", ErrInvalid)
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
			return fmt.Errorf("%w: unknown run command %q", ErrInvalid, action)
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
