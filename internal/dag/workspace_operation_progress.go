package dag

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	wo "github.com/ai-mada/suma/internal/workspaceoperationcontract"
)

func WorkspaceOperationIdentity(tenant, project string, a Assignment) wo.Identity {
	return wo.Identity{TenantID: tenant, ProjectID: project, RunID: a.RunID, NodeID: a.NodeID, WorkerID: a.WorkerID, Attempt: a.Attempt, Fence: a.Fence}
}

// HostOperationProgressWithAuthority records immutable Go-host metadata only.
// A known old attempt may drain its outbox after completion/retry; this never
// renews a lease, replaces a result, changes a node, or performs a tool action.
// The authority callback always precedes receipt recovery and scheduler reads.
func (e *Engine) HostOperationProgressWithAuthority(ctx context.Context, tenant, project string, a Assignment, operationID string, event wo.Event, before func(*sql.Tx) error) error {
	identity := WorkspaceOperationIdentity(tenant, project, a)
	if a.TenantID != tenant || a.ProjectID != project || operationID != event.EventID || wo.ValidateBoundEvent(identity, event) != nil {
		return fmt.Errorf("%w: invalid Go workspace operation identity", ErrInvalid)
	}
	return e.writeBefore(ctx, before, func(t *transaction) error {
		r, err := t.loadHot(tenant, project, a.RunID)
		if err != nil {
			return err
		}
		n := r.find(a.NodeID)
		if n == nil {
			return ErrNotFound
		}
		_, profileSHA, bound, err := ToolBinding(n.spec)
		if err != nil || !bound || profileSHA != event.ProfileSHA256 {
			return fmt.Errorf("%w: host workspace profile differs from fixed node", ErrInvalid)
		}
		var known int
		if err = t.row(`SELECT COUNT(*) FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=? AND fence=? AND worker_id=?`, a.RunID, a.NodeID, a.Attempt, a.Fence, a.WorkerID).Scan(&known); err != nil {
			return err
		}
		if known != 1 {
			return ErrStale
		}
		// Fixed-size scope keeps dynamic node identifiers within SQL key bounds.
		scope := "workspace-host:" + fingerprint(identity)
		phaseKey := hashText(event.OperationID + "/" + strconv.Itoa(event.Revision))
		hash := fingerprint(struct {
			Identity wo.Identity
			Event    wo.Event
		}{identity, event})
		previous, err := t.operation(tenant, project, scope, phaseKey, hash)
		if err != nil || previous != "" {
			return err
		}
		// Sequence and invocation headers prevent a different transport ID from
		// redefining an existing transition or joining unrelated started/terminal
		// facts. Terminal-before-started delivery remains safe and supported.
		headerHash := fingerprint([]any{identity, event.OperationID, event.Action, event.StartedAt, event.GenerationBefore, event.RequestSHA256, event.PolicySHA256, event.ProfileSHA256})
		headerKey := "call:" + hashText(event.OperationID)
		policyHash := fingerprint([]any{identity, event.PolicySHA256, event.ProfileSHA256})
		for _, slot := range []struct{ key, hash string }{{"policy", policyHash}, {headerKey, headerHash}, {"sequence:" + strconv.Itoa(event.Sequence), hash}} {
			found, err := t.operation(tenant, project, scope, slot.key, slot.hash)
			if err != nil {
				return err
			}
			if found == "" && slot.key == headerKey {
				var calls int
				if err = t.row(`SELECT COUNT(*) FROM dag_operations WHERE tenant_id=? AND project_id=? AND scope=? AND operation_id LIKE 'call:%'`, tenant, project, scope).Scan(&calls); err != nil {
					return err
				}
				if calls >= wo.MaxCalls {
					return fmt.Errorf("%w: host workspace invocation capacity reached", ErrInvalid)
				}
			}
			if found == "" {
				if err = t.remember(tenant, project, scope, slot.key, slot.hash, r.ID); err != nil {
					return err
				}
			}
		}
		// An identifier/locator may become known only after validation or seal.
		// Preserve monotonic nonempty facts without making arrival order matter.
		for _, fact := range []struct{ name, value string }{{"effect", event.EffectOperationID}, {"path", event.Path}, {"check", event.CheckID}} {
			if fact.value == "" {
				continue
			}
			key := "known:" + hashText(event.OperationID+"/"+fact.name)
			factHash := fingerprint(fact.value)
			found, err := t.operation(tenant, project, scope, key, factHash)
			if err != nil {
				return err
			}
			if found == "" {
				if err = t.remember(tenant, project, scope, key, factHash, r.ID); err != nil {
					return err
				}
			}
		}
		if err = t.event(r, a.NodeID, "tool_progress", map[string]any{"operation_id": operationID, "attempt": a.Attempt, "fence": a.Fence, "worker_id": a.WorkerID, "host_operation": wo.CloneEvent(event)}); err != nil {
			return err
		}
		return t.remember(tenant, project, scope, phaseKey, hash, r.ID)
	})
}
