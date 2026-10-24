package dag

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

func subReference(n Node) (string, int, error) {
	id, _ := n.Config["workflow_id"].(string)
	version := numeric(n.Config, "workflow_version")
	if !identifier.MatchString(id) || version < 1 || version > 2147483647 {
		return "", 0, dagDiagnostic(fmt.Errorf("%w: subworkflow requires workflow_id and positive pinned workflow_version", ErrInvalid), "dag.subworkflow.reference", nil)
	}
	if value, ok := n.Config["workflow_version"].(float64); ok && value != float64(version) {
		return "", 0, dagDiagnostic(fmt.Errorf("%w: workflow_version must be an integer", ErrInvalid), "dag.subworkflow.integer_version", nil)
	}
	return id, int(version), nil
}
func (t *transaction) workflow(tenant, project, id string, version int) (Workflow, error) {
	var raw string
	var w Workflow
	if err := t.publicWorkflow(tenant, project, id); err != nil {
		return w, err
	}
	err := t.row("SELECT spec FROM dag_workflows WHERE tenant_id=? AND project_id=? AND id=? AND version=?", tenant, project, id, version).Scan(&raw)
	if err == sql.ErrNoRows {
		return w, dagDiagnostic(fmt.Errorf("%w: pinned subworkflow %s version %d is unavailable in this project", ErrInvalid, id, version), "dag.subworkflow.unavailable", diagnosticParams{"workflow": id, "version": version})
	}
	if err != nil {
		return w, err
	}
	err = json.Unmarshal([]byte(raw), &w)
	return w, err
}
func (t *transaction) validateReferences(tenant, project string, w Workflow) error {
	size := 0
	var visit func(Workflow, int, int) error
	visit = func(w Workflow, depth, multiplier int) error {
		if depth > 32 {
			return dagDiagnostic(fmt.Errorf("%w: subworkflow nesting exceeds 32", ErrInvalid), "dag.subworkflow.nesting", nil)
		}
		for _, n := range w.Nodes {
			size += multiplier
			if size > 10000 {
				return dagDiagnostic(fmt.Errorf("%w: materialized workflow graph exceeds 10000 nodes", ErrInvalid), "dag.subworkflow.node_limit", nil)
			}
			copies := multiplier
			if n.Type == "map" {
				items, err := mapItems(n)
				if err != nil {
					return err
				}
				if len(items) > 10000/multiplier {
					return dagDiagnostic(fmt.Errorf("%w: materialized workflow graph exceeds 10000 nodes", ErrInvalid), "dag.subworkflow.node_limit", nil)
				}
				copies = multiplier * len(items)
				size += copies
				if size > 10000 {
					return dagDiagnostic(fmt.Errorf("%w: materialized workflow graph exceeds 10000 nodes", ErrInvalid), "dag.subworkflow.node_limit", nil)
				}
				template, err := mapTemplate(n)
				if err != nil {
					return err
				}
				n = template
			}
			if n.Type == "subworkflow" {
				id, version, err := subReference(n)
				if err != nil {
					return err
				}
				child, err := t.workflow(tenant, project, id, version)
				if err != nil {
					return err
				}
				if copies > 0 {
					if err = visit(child, depth+1, copies); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return visit(w, 0, 1)
}
func (t *transaction) createRun(tenant, project string, w Workflow, bind func(*sql.Tx, string) error) (*runState, error) {
	return t.createRunID(tenant, project, uid(), w, bind)
}
func (t *transaction) createRunID(tenant, project, runID string, w Workflow, bind func(*sql.Tx, string) error) (*runState, error) {
	route, routeErr := t.newRunRoute(tenant, project)
	if routeErr != nil {
		return nil, routeErr
	}
	if err := t.guardProject(tenant, project); err != nil {
		return nil, err
	}
	if _, err := t.ensureDefaultPolicy(tenant, project); err != nil {
		return nil, err
	}
	root := t.familyRoot
	if root == "" {
		root = runID
	}
	if err := t.insertIgnore(`INSERT INTO dag_run_families(run_id,root_run_id)VALUES(?,?)`, "run_id", runID, root); err != nil {
		return nil, err
	}
	if err := t.guardFamily(tenant, project, runID); err != nil {
		return nil, err
	}
	r := &runState{Run: Run{ID: runID, TenantID: tenant, ProjectID: project, WorkflowID: w.ID, WorkflowName: w.Name, WorkflowVersion: w.Version, State: "running", CreatedAt: stamp(t.now), UpdatedAt: stamp(t.now)}, workflow: w}
	applyExecutionRoute(r, route)
	if err := t.exec("INSERT INTO dag_runs (id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?)", r.ID, tenant, project, w.ID, w.Version, w.Name, 0, r.State, r.CreatedAt, r.UpdatedAt); err != nil {
		return nil, err
	}
	if err := t.exec(`INSERT INTO dag_run_execution_routes(run_id,tenant_id,project_id,route_version,pool_id,grant_id,grant_version)VALUES(?,?,?,?,?,?,?)`, runID, tenant, project, route.Version, route.PoolID, route.GrantID, route.GrantVersion); err != nil {
		return nil, err
	}
	for _, spec := range w.Nodes {
		n := &nodeState{NodeRun: NodeRun{ID: spec.ID, Name: spec.Name, Type: spec.Type, State: "pending", Command: spec.Command}, spec: spec}
		r.nodes = append(r.nodes, n)
		if err := t.insertNode(r, n); err != nil {
			return nil, err
		}
	}
	if bind != nil {
		if err := bind(t.tx, r.ID); err != nil {
			return nil, err
		}
	}
	if err := t.initializeGraph(r, false); err != nil {
		return nil, err
	}
	if err := t.event(r, "", "run_created", map[string]any{"workflow_version": w.Version}); err != nil {
		return nil, err
	}
	if err := t.advance(r); err != nil {
		return nil, err
	}
	if err := t.saveQuiet(r); err != nil {
		return nil, err
	}
	return r, nil
}
func (t *transaction) spawnSubrun(r *runState, n *nodeState) error {
	id, version, err := subReference(n.spec)
	if err != nil {
		return err
	}
	w, err := t.workflow(r.TenantID, r.ProjectID, id, version)
	if err != nil {
		return err
	}
	n.Attempt++
	n.Fence++
	n.Error = ""
	n.Output = ""
	n.StartedAt = stamp(t.now)
	n.CompletedAt = ""
	n.wake = 0
	if n.spec.TimeoutSeconds > 0 {
		n.wake = t.now + int64(n.spec.TimeoutSeconds)
	}
	child, err := t.createRun(r.TenantID, r.ProjectID, w, nil)
	if err != nil {
		return err
	}
	n.ChildRunID = child.ID
	if err = t.exec("INSERT INTO dag_subruns (parent_run_id,node_id,attempt,child_run_id,paused_by_parent) VALUES (?,?,?,?,0)", r.ID, n.ID, n.Attempt, child.ID); err != nil {
		return err
	}
	if err = t.exec("INSERT INTO dag_attempts (run_id,node_id,attempt,fence,worker_id,state,started_at,completed_at,result) VALUES (?,?,?,?,?,?,?,?,?)", r.ID, n.ID, n.Attempt, n.Fence, "", "waiting", n.StartedAt, "", encoded(map[string]any{"child_run_id": child.ID})); err != nil {
		return err
	}
	if err = t.event(r, n.ID, "child_run_created", map[string]any{"child_run_id": child.ID, "workflow_id": w.ID, "workflow_version": w.Version, "attempt": n.Attempt}); err != nil {
		return err
	}
	if err = t.transition(r, n, "waiting", "subworkflow"); err != nil {
		return err
	}
	// An entirely internal child can finish before its parent link exists.
	// Queue the newly linked parent so that completion is observed in this tx.
	if terminal(child.State) {
		return t.enqueueNode(r, n.ID)
	}
	return nil
}
func (t *transaction) resolveSubruns(r *runState) (bool, error) {
	changed := false
	for _, n := range r.nodes {
		if n.Type != "subworkflow" || n.State != "waiting" || n.ChildRunID == "" {
			continue
		}
		var state string
		if err := t.row("SELECT state FROM dag_runs WHERE id=? AND tenant_id=? AND project_id=?", n.ChildRunID, r.TenantID, r.ProjectID).Scan(&state); err != nil {
			return false, err
		}
		if !terminal(state) && n.wake > 0 && n.wake <= t.now {
			child, err := t.loadFull(r.TenantID, r.ProjectID, n.ChildRunID)
			if err != nil {
				return false, err
			}
			if err = t.cancelRun(child, "subworkflow deadline exceeded"); err != nil {
				return false, err
			}
			if err = t.controlSubruns(child, "cancel"); err != nil {
				return false, err
			}
			if err = t.advance(child); err != nil {
				return false, err
			}
			if err = t.saveQuiet(child); err != nil {
				return false, err
			}
			state = child.State
			n.wake = 0
			n.Reason = "subworkflow_timeout"
			changed = true
			if err = t.event(r, n.ID, "subworkflow_timeout", map[string]any{"child_run_id": child.ID}); err != nil {
				return false, err
			}
		}
		if !terminal(state) {
			continue
		}
		n.Output = encoded(map[string]any{"child_run_id": n.ChildRunID, "state": state})
		next := "succeeded"
		reason := "child workflow finished " + state
		timedOut := n.Reason == "subworkflow_timeout"
		if state != "succeeded" {
			next = "failed"
			n.Error = "child workflow " + n.ChildRunID + " finished " + state
		}
		if timedOut {
			next = "failed"
			n.Error = "subworkflow deadline exceeded; child stop confirmed"
			reason = n.Error
		}
		if r.State == "cancelling" {
			next = "cancelled"
		}
		if err := t.transition(r, n, next, reason); err != nil {
			return false, err
		}
		if err := t.exec("UPDATE dag_attempts SET state=?,completed_at=?,result=? WHERE run_id=? AND node_id=? AND attempt=?", next, stamp(t.now), n.Output, r.ID, n.ID, n.Attempt); err != nil {
			return false, err
		}
		if r.kernelGeneration == 1 && next == "failed" && r.State != "cancelling" && n.spec.Idempotent && n.Attempt <= n.spec.MaxRetries {
			n.State = "pending"
			n.CompletedAt = ""
			n.wake = t.now + 1
			if err := t.event(r, n.ID, "retry_scheduled", map[string]any{"attempt": n.Attempt + 1, "previous_child_run_id": n.ChildRunID}); err != nil {
				return false, err
			}
		}
		changed = true
	}
	return changed, nil
}
func (t *transaction) propagateParent(child *runState) error {
	if !terminal(child.State) {
		return nil
	}
	var parentID, nodeID string
	var attempt int
	err := t.row("SELECT parent_run_id,node_id,attempt FROM dag_subruns WHERE child_run_id=?", child.ID).Scan(&parentID, &nodeID, &attempt)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	parent, err := t.loadHot(child.TenantID, child.ProjectID, parentID)
	if err != nil {
		return err
	}
	n := parent.find(nodeID)
	if n == nil || n.Attempt != attempt || n.State != "waiting" {
		return nil
	}
	if err = t.event(parent, nodeID, "child_run_completed", map[string]any{"child_run_id": child.ID, "state": child.State, "attempt": attempt}); err != nil {
		return err
	}
	if parent.State == "cancelling" {
		if _, err = t.resolveSubruns(parent); err != nil {
			return err
		}
	} else if err = t.enqueueNode(parent, nodeID); err != nil {
		return err
	}
	if err = t.advance(parent); err != nil {
		return err
	}
	return t.save(parent)
}
func (t *transaction) controlSubruns(r *runState, action string) error {
	for _, n := range r.nodes {
		if n.Type != "subworkflow" || n.ChildRunID == "" {
			continue
		}
		child, err := t.loadFull(r.TenantID, r.ProjectID, n.ChildRunID)
		if err != nil {
			return err
		}
		if terminal(child.State) {
			continue
		}
		var flag int
		if err = t.row("SELECT paused_by_parent FROM dag_subruns WHERE parent_run_id=? AND node_id=? AND attempt=?", r.ID, n.ID, n.Attempt).Scan(&flag); err != nil {
			return err
		}
		switch action {
		case "pause":
			if child.State != "running" {
				continue
			}
			child.State = "paused"
			flag = 1
		case "resume":
			if flag != 1 || child.State != "paused" {
				continue
			}
			child.State = "running"
			flag = 0
			if err = t.resumeRunWakes(child); err != nil {
				return err
			}
		case "cancel":
			if err = t.cancelRun(child, "parent cancellation"); err != nil {
				return err
			}
			flag = 0
		}
		if err = t.exec("UPDATE dag_subruns SET paused_by_parent=? WHERE parent_run_id=? AND node_id=? AND attempt=?", flag, r.ID, n.ID, n.Attempt); err != nil {
			return err
		}
		if err = t.controlSubruns(child, action); err != nil {
			return err
		}
		if err = t.event(child, "", "parent_"+action, map[string]any{"parent_run_id": r.ID, "parent_node_id": n.ID}); err != nil {
			return err
		}
		if err = t.advance(child); err != nil {
			return err
		}
		if err = t.saveQuiet(child); err != nil {
			return err
		}
	}
	return nil
}
func (t *transaction) cancelRun(r *runState, reason string) error {
	r.State = "cancelling"
	for _, n := range r.nodes {
		if terminal(n.State) {
			continue
		}
		if n.State == "running" || n.State == "reconciling" {
			n.Reason = "cancellation_requested"
			if err := t.event(r, n.ID, "stop_requested", map[string]any{"worker_id": n.WorkerID, "fence": n.Fence}); err != nil {
				return err
			}
			continue
		}
		if n.Type == "subworkflow" && n.State == "waiting" {
			continue
		}
		if err := t.transition(r, n, "cancelled", reason); err != nil {
			return err
		}
	}
	return nil
}
func (t *transaction) checkParentCommand(r *runState, action string) error {
	if action != "resume" && action != "retry" {
		return nil
	}
	current := r.ID
	for depth := 0; depth <= 32; depth++ {
		var parentID, nodeID string
		var attempt int
		err := t.row("SELECT parent_run_id,node_id,attempt FROM dag_subruns WHERE child_run_id=?", current).Scan(&parentID, &nodeID, &attempt)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		parent, err := t.loadFull(r.TenantID, r.ProjectID, parentID)
		if err != nil {
			return err
		}
		node := parent.find(nodeID)
		if parent.State != "running" || node == nil || node.Attempt != attempt || node.State != "waiting" {
			return dagDiagnostic(fmt.Errorf("%w: child lifecycle is controlled by parent run %s", ErrConflict, parentID), "dag.subworkflow.parent_control", diagnosticParams{"run": parentID})
		}
		current = parentID
	}
	return dagDiagnostic(fmt.Errorf("%w: subworkflow depth", ErrInvalid), "dag.subworkflow.depth", nil)
}
