package dag

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

// BoundSubworkflowRef is created by the kernel from the original parent
// attempt. A binding string in an arbitrary published workflow is not a grant.
type BoundSubworkflowRef struct {
	TenantID, ProjectID, ParentRunID, NodeID, BindingID string
	Attempt                                             int
	Fence                                               int64
}
type BoundSubworkflowPlan struct {
	RunID         string
	Workflow      Workflow
	BlockedReason string
}
type BoundSubworkflowCheck struct {
	PauseReason   string
	StopConfirmed bool
}

// BoundSubworkflowHandler runs inside the original scheduler transaction. It
// may use tx only; nested Engine calls and external I/O are forbidden. The
// scope lifecycle runs before project/pool/family locks and its cleanup runs
// after commit/rollback. Configure once, before concurrent scheduler work.
type BoundSubworkflowHandler interface {
	BeforeBoundSubworkflowScope(context.Context, *sql.Tx, string, string) (func(), error)
	ResolveBoundSubworkflowTx(context.Context, *sql.Tx, BoundSubworkflowRef) (BoundSubworkflowPlan, error)
	BindBoundSubworkflowTx(context.Context, *sql.Tx, BoundSubworkflowRef, string) error
	CheckBoundSubworkflowTx(context.Context, *sql.Tx, BoundSubworkflowRef, string, string) (BoundSubworkflowCheck, error)
	AuthorizeBoundRunCommandTx(context.Context, *sql.Tx, string, string, string, string) error
}

func boundSubworkflowBinding(n Node) (string, bool, error) {
	raw, present := n.Config["service_binding_id"]
	if !present {
		return "", false, nil
	}
	id, ok := raw.(string)
	if n.Type != "subworkflow" || !ok || !identifier.MatchString(id) || len(n.Config) != 1 || len(n.Command) != 0 || n.MaxRetries != 0 || n.Idempotent || n.TimeoutSeconds != 0 {
		return "", true, dagDiagnostic(fmt.Errorf("%w: invalid service-bound subworkflow", ErrInvalid), "dag.subworkflow.service_binding_invalid", nil)
	}
	return id, true, nil
}
func (t *transaction) boundRef(r *runState, n *nodeState, next bool) (BoundSubworkflowRef, bool, error) {
	id, bound, err := boundSubworkflowBinding(n.spec)
	ref := BoundSubworkflowRef{TenantID: r.TenantID, ProjectID: r.ProjectID, ParentRunID: r.ID, NodeID: n.ID, BindingID: id, Attempt: n.Attempt, Fence: n.Fence}
	if next {
		ref.Attempt++
		ref.Fence++
	}
	return ref, bound, err
}
func (t *transaction) beforeScope(tenant, project string) error {
	if t.e.BoundSubworkflows == nil {
		return nil
	}
	key := tenant + "\x00" + project
	if t.scopes == nil {
		t.scopes = map[string]bool{}
	}
	if t.scopes[key] {
		return nil
	}
	cleanup, err := t.e.BoundSubworkflows.BeforeBoundSubworkflowScope(t.ctx, t.tx, tenant, project)
	if cleanup != nil {
		t.scopeCleanups = append(t.scopeCleanups, cleanup)
	}
	if err != nil {
		return err
	}
	t.scopes[key] = true
	return nil
}
func (t *transaction) closeScopes() {
	for i := len(t.scopeCleanups) - 1; i >= 0; i-- {
		t.scopeCleanups[i]()
	}
}
func (t *transaction) pauseBoundParent(r *runState, n *nodeState, reason string) error {
	if reason == "" {
		return nil
	}
	if r.State == "cancelling" || terminal(r.State) {
		return nil
	}
	r.State = "paused"
	n.Reason = reason
	if err := t.enqueueNode(r, n.ID); err != nil {
		return err
	}
	return t.event(r, n.ID, "bound_execution_paused", map[string]any{"reason": reason, "child_run_id": n.ChildRunID})
}
func (t *transaction) spawnBoundSubrun(r *runState, n *nodeState, ref BoundSubworkflowRef) error {
	if t.e.BoundSubworkflows == nil {
		return dagDiagnostic(fmt.Errorf("%w: service-bound subworkflow handler unavailable", ErrInvalid), "dag.subworkflow.service_binding_unavailable", nil)
	}
	plan, err := t.e.BoundSubworkflows.ResolveBoundSubworkflowTx(t.ctx, t.tx, ref)
	if err != nil {
		return err
	}
	if plan.BlockedReason != "" {
		return t.pauseBoundParent(r, n, plan.BlockedReason)
	}
	if !identifier.MatchString(plan.RunID) || len(plan.RunID) > 92 || plan.RunID == r.ID {
		return ErrInvalid
	}
	w, err := cloneBoundWorkflow(plan.Workflow)
	if err != nil {
		return err
	}
	w.ID, w.Version = "managed-"+plan.RunID, 1
	w, err = normalize(w)
	if err != nil {
		return err
	}
	// A bound child may use the ordinary execution node types. It cannot
	// recursively claim another service binding through a copied definition.
	for _, spec := range w.Nodes {
		if _, bound, _ := boundSubworkflowBinding(spec); bound {
			return ErrInvalid
		}
	}
	if err = t.validateReferences(r.TenantID, r.ProjectID, w); err != nil {
		return err
	}
	if err = t.exec(`INSERT INTO dag_workflows(tenant_id,project_id,id,version,name,spec)VALUES(?,?,?,?,?,?)`, r.TenantID, r.ProjectID, w.ID, 1, w.Name, encoded(w)); err != nil {
		return err
	}
	if err = t.exec(`INSERT INTO dag_managed_workflows(tenant_id,project_id,workflow_id,run_id)VALUES(?,?,?,?)`, r.TenantID, r.ProjectID, w.ID, plan.RunID); err != nil {
		return err
	}
	n.Attempt, n.Fence = ref.Attempt, ref.Fence
	n.StartedAt, n.CompletedAt = stamp(t.now), ""
	n.Error, n.Output, n.Reason = "", "", ""
	n.ChildRunID = plan.RunID
	child, err := t.createRunID(r.TenantID, r.ProjectID, plan.RunID, w, func(tx *sql.Tx, run string) error {
		return t.e.BoundSubworkflows.BindBoundSubworkflowTx(t.ctx, tx, ref, run)
	})
	if err != nil {
		return err
	}
	if err = t.exec(`INSERT INTO dag_subruns(parent_run_id,node_id,attempt,child_run_id,paused_by_parent)VALUES(?,?,?,?,0)`, r.ID, n.ID, n.Attempt, child.ID); err != nil {
		return err
	}
	if err = t.exec(`INSERT INTO dag_attempts(run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result)VALUES(?,?,?,?,?,?,?,?,?)`, r.ID, n.ID, n.Attempt, n.Fence, "", "waiting", n.StartedAt, "", encoded(map[string]any{"child_run_id": child.ID})); err != nil {
		return err
	}
	if err = t.transition(r, n, "waiting", "bound_subworkflow"); err != nil {
		return err
	}
	if err = t.event(r, n.ID, "child_run_created", map[string]any{"child_run_id": child.ID, "binding_id": ref.BindingID, "attempt": n.Attempt}); err != nil {
		return err
	}
	if terminal(child.State) {
		return t.enqueueNode(r, n.ID)
	}
	return nil
}

func (t *transaction) checkBoundChild(r *runState, n *nodeState, child, state string) (bool, error) {
	ref, bound, err := t.boundRef(r, n, false)
	if err != nil || !bound {
		return bound, err
	}
	if t.e.BoundSubworkflows == nil {
		return true, ErrInvalid
	}
	check, err := t.e.BoundSubworkflows.CheckBoundSubworkflowTx(t.ctx, t.tx, ref, child, state)
	if err != nil {
		return true, err
	}
	if check.PauseReason != "" {
		if r.State == "cancelling" && check.StopConfirmed && terminal(state) {
			return false, nil
		}
		return true, t.pauseBoundParent(r, n, check.PauseReason)
	}
	return false, nil
}

// Clone the handler output before mutation; service-owned maps must never
// mutate the kernel's stored immutable workflow through a retained pointer.
func cloneBoundWorkflow(w Workflow) (Workflow, error) {
	var out Workflow
	err := json.Unmarshal([]byte(encoded(w)), &out)
	return out, err
}
