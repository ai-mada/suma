package dag

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (t *transaction) insertIgnore(query, keys string, args ...any) error {
	if t.e.dialect == "oceanbase" {
		query = strings.Replace(query, "INSERT INTO ", "INSERT IGNORE INTO ", 1)
	} else {
		query += " ON CONFLICT(" + keys + ") DO NOTHING"
	}
	return t.exec(query, args...)
}
func (t *transaction) enqueueNode(r *runState, id string) error {
	seq, err := t.nextQueueSequence(r.TenantID, r.ProjectID)
	if err != nil {
		return err
	}
	return t.insertIgnore(`INSERT INTO dag_node_work(run_id,node_id,sequence)VALUES(?,?,?)`, "run_id,node_id", r.ID, id, seq)
}

func (t *transaction) initializeGraph(r *runState, legacy bool) error {
	r.tx = t
	r.indexed = true
	r.nodeMap = map[string]*nodeState{}
	r.resolved = map[string]string{}
	r.kernelGeneration = 2
	if legacy {
		r.kernelGeneration = 1
	}
	r.totalNodes = len(r.nodes)
	r.terminalNodes, r.failedNodes, r.cancelledNodes = 0, 0, 0
	for _, n := range r.nodes {
		r.nodeMap[n.ID] = n
		n.originalState = n.State
		if terminal(n.State) {
			r.terminalNodes++
		}
		if n.State == "failed" {
			r.failedNodes++
		}
		if n.State == "cancelled" {
			r.cancelledNodes++
		}
	}
	if err := t.insertIgnore(`INSERT INTO dag_run_generations(run_id,kernel_generation)VALUES(?,?)`, "run_id", r.ID, r.kernelGeneration); err != nil {
		return err
	}
	if err := t.insertIgnore(`INSERT INTO dag_run_counters(run_id,total,terminal_count,failed_count,cancelled_count)VALUES(?,?,?,?,?)`, "run_id", r.ID, r.totalNodes, r.terminalNodes, r.failedNodes, r.cancelledNodes); err != nil {
		return err
	}
	incoming := map[string]int{}
	for _, edge := range r.workflow.Edges {
		if err := t.insertIgnore(`INSERT INTO dag_graph_edges(run_id,source_id,target_id,condition_value)VALUES(?,?,?,?)`, "run_id,source_id,target_id", r.ID, edge.From, edge.To, edge.Condition); err != nil {
			return err
		}
		incoming[edge.To]++
	}
	for _, n := range r.nodes {
		decision := ""
		if legacy && n.State != "pending" {
			decision = "legacy_unrecorded"
		}
		if err := t.insertIgnore(`INSERT INTO dag_node_dependencies(run_id,node_id,generation,total,resolved,selected,success,failed,winners,decision_at,ready_at,version)VALUES(?,?,1,?,0,0,0,0,'[]',?,'',1)`, "run_id,node_id", r.ID, n.ID, incoming[n.ID], decision); err != nil {
			return err
		}
		for _, expression := range predicates(n.spec) {
			parts := strings.SplitN(expression, "==", 2)
			if len(parts) != 2 {
				continue
			}
			lhs := strings.TrimSpace(parts[0])
			pos := strings.LastIndex(lhs, ".")
			if pos < 1 {
				continue
			}
			if err := t.insertIgnore(`INSERT INTO dag_condition_watchers(run_id,source_id,target_id)VALUES(?,?,?)`, "run_id,source_id,target_id", r.ID, lhs[:pos], n.ID); err != nil {
				return err
			}
		}
		if n.parent != "" {
			if err := t.insertIgnore(`INSERT INTO dag_map_children(run_id,map_node_id,child_node_id)VALUES(?,?,?)`, "run_id,map_node_id,child_node_id", r.ID, n.parent, n.ID); err != nil {
				return err
			}
		}
		if n.Type == "map" {
			count := 0
			for _, child := range r.nodes {
				if child.parent == n.ID {
					count++
				}
			}
			closed := 0
			if legacy && n.State != "pending" {
				closed = 1
			}
			if err := t.insertIgnore(`INSERT INTO dag_map_progress(run_id,node_id,total,terminal_count,failed_count,closed)VALUES(?,?,?,0,0,?)`, "run_id,node_id", r.ID, n.ID, count, closed); err != nil {
				return err
			}
		}
	}
	for _, n := range r.nodes {
		if err := t.resolveOutgoing(r, n, false); err != nil {
			return err
		}
	}
	if !legacy {
		for _, n := range r.nodes {
			if err := t.enqueueNode(r, n.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *transaction) loadHot(tenant, project, id string) (*runState, error) {
	if err := t.guardProject(tenant, project); err != nil {
		return nil, err
	}
	if err := t.guardFamily(tenant, project, id); err != nil {
		return nil, err
	}
	r := &runState{tx: t, partial: true, indexed: true, nodeMap: map[string]*nodeState{}, resolved: map[string]string{}}
	err := t.row(`SELECT id,tenant_id,project_id,workflow_id,workflow_version,workflow_name,version,state,created_at,updated_at FROM dag_runs WHERE tenant_id=? AND project_id=? AND id=?`, tenant, project, id).Scan(&r.ID, &r.TenantID, &r.ProjectID, &r.WorkflowID, &r.WorkflowVersion, &r.WorkflowName, &r.Version, &r.State, &r.CreatedAt, &r.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	err = t.row(`SELECT c.total,c.terminal_count,c.failed_count,c.cancelled_count,g.kernel_generation FROM dag_run_counters c JOIN dag_run_generations g ON g.run_id=c.run_id WHERE c.run_id=?`, id).Scan(&r.totalNodes, &r.terminalNodes, &r.failedNodes, &r.cancelledNodes, &r.kernelGeneration)
	if err == sql.ErrNoRows {
		full, err := load(t.ctx, t.tx, t.e, tenant, project, id)
		if err != nil {
			return nil, err
		}
		if err = t.initializeGraph(full, true); err != nil {
			return nil, err
		}
		full.partial = false
		return full, nil
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (t *transaction) guardFamily(tenant, project, id string) error {
	root := id
	seen := map[string]bool{}
	for depth := 0; depth < 64; depth++ {
		if seen[root] {
			return fmt.Errorf("%w: execution family cycle", ErrInvalid)
		}
		seen[root] = true
		var previous string
		err := t.row(`SELECT root_run_id FROM dag_run_families WHERE run_id=?`, root).Scan(&previous)
		if err == nil {
			root = previous
			break
		}
		if err != sql.ErrNoRows {
			return err
		}
		err = t.row(`SELECT parent_run_id FROM dag_subruns WHERE child_run_id=?`, root).Scan(&previous)
		if err == sql.ErrNoRows {
			err = t.row(`SELECT original_run_id FROM dag_compensations WHERE compensation_run_id=?`, root).Scan(&previous)
		}
		if err == sql.ErrNoRows {
			break
		}
		if err != nil {
			return err
		}
		root = previous
	}
	if err := t.insertIgnore(`INSERT INTO dag_run_families(run_id,root_run_id)VALUES(?,?)`, "run_id", id, root); err != nil {
		return err
	}
	if err := t.insertIgnore(`INSERT INTO dag_family_locks(root_run_id,version)VALUES(?,0)`, "root_run_id", root); err != nil {
		return err
	}
	t.familyRoot = root
	return t.exec(`UPDATE dag_family_locks SET version=version WHERE root_run_id=?`, root)
}

func (t *transaction) insertGraphNode(r *runState, n *nodeState) error {
	if r.nodeMap == nil {
		r.nodeMap = map[string]*nodeState{}
	}
	r.nodeMap[n.ID] = n
	r.totalNodes++
	if err := t.exec(`INSERT INTO dag_node_dependencies(run_id,node_id,generation,total,resolved,selected,success,failed,winners,decision_at,ready_at,version)VALUES(?,?,1,0,0,0,0,0,'[]','','',1)`, r.ID, n.ID); err != nil {
		return err
	}
	if n.parent != "" {
		if err := t.insertIgnore(`INSERT INTO dag_map_children(run_id,map_node_id,child_node_id)VALUES(?,?,?)`, "run_id,map_node_id,child_node_id", r.ID, n.parent, n.ID); err != nil {
			return err
		}
	}
	return t.enqueueNode(r, n.ID)
}

func (t *transaction) loadFull(tenant, project, id string) (*runState, error) {
	if err := t.guardProject(tenant, project); err != nil {
		return nil, err
	}
	if err := t.guardFamily(tenant, project, id); err != nil {
		return nil, err
	}
	r, err := load(t.ctx, t.tx, t.e, tenant, project, id)
	if err != nil {
		return nil, err
	}
	r.tx = t
	r.nodeMap = map[string]*nodeState{}
	r.resolved = map[string]string{}
	for _, n := range r.nodes {
		r.nodeMap[n.ID] = n
	}
	err = t.row(`SELECT c.total,c.terminal_count,c.failed_count,c.cancelled_count,g.kernel_generation FROM dag_run_counters c JOIN dag_run_generations g ON g.run_id=c.run_id WHERE c.run_id=?`, id).Scan(&r.totalNodes, &r.terminalNodes, &r.failedNodes, &r.cancelledNodes, &r.kernelGeneration)
	if err == sql.ErrNoRows {
		if err = t.initializeGraph(r, true); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	r.indexed = true
	return r, nil
}

func (r *runState) lazyNode(id string) *nodeState {
	if r.tx == nil || !r.partial {
		return nil
	}
	n := &nodeState{}
	var raw, result string
	err := r.tx.row(`SELECT n.id,n.state,n.reason,n.attempt,n.fence,n.worker_id,n.output,n.error_text,n.started_at,n.completed_at,n.lease_until,n.wake_at,n.parent_id,n.spec,COALESCE(a.result,'') FROM dag_nodes n LEFT JOIN dag_attempts a ON a.run_id=n.run_id AND a.node_id=n.id AND a.attempt=n.attempt WHERE n.run_id=? AND n.id=?`, r.ID, id).Scan(&n.ID, &n.State, &n.Reason, &n.Attempt, &n.Fence, &n.WorkerID, &n.Output, &n.Error, &n.StartedAt, &n.CompletedAt, &n.lease, &n.wake, &n.parent, &raw, &result)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		r.loadErr = err
		return nil
	}
	if err = json.Unmarshal([]byte(raw), &n.spec); err != nil {
		r.loadErr = err
		return nil
	}
	n.Name, n.Type, n.Command = n.spec.Name, n.spec.Type, n.spec.Command
	n.original = nodePersistence(n)
	n.originalState = n.State
	if result != "" && n.State != "pending" && n.State != "ready" && n.State != "waiting" {
		var proof Result
		if json.Unmarshal([]byte(result), &proof) == nil {
			n.WorkspaceEvidence, n.ArtifactEvidence, n.ToolEvidence = proof.WorkspaceEvidence, proof.ArtifactEvidence, proof.ToolEvidence
		}
	}
	if n.Type == "subworkflow" {
		err = r.tx.row(`SELECT child_run_id FROM dag_subruns WHERE parent_run_id=? AND node_id=? AND attempt=?`, r.ID, id, n.Attempt).Scan(&n.ChildRunID)
		if err != nil && err != sql.ErrNoRows {
			r.loadErr = err
			return nil
		}
	}
	r.nodes = append(r.nodes, n)
	r.nodeMap[id] = n
	return n
}

func skippedUnselected(n *nodeState) bool {
	return n.State == "skipped" && (n.Reason == "condition false" || strings.HasPrefix(n.Reason, "unselected"))
}
func (t *transaction) resolveOutgoing(r *runState, n *nodeState, enqueue bool) error {
	signature := fingerprint([]any{n.State, n.Attempt, n.Fence, n.Output, n.Reason})
	if r.resolved == nil {
		r.resolved = map[string]string{}
	}
	if r.resolved[n.ID] == signature {
		return nil
	}
	r.resolved[n.ID] = signature
	rows, err := t.rows(`SELECT target_id,condition_value FROM dag_graph_edges WHERE run_id=? AND source_id=? ORDER BY target_id`, r.ID, n.ID)
	if err != nil {
		return err
	}
	edges := []Edge{}
	for rows.Next() {
		var edge Edge
		edge.From = n.ID
		if err = rows.Scan(&edge.To, &edge.Condition); err != nil {
			rows.Close()
			return err
		}
		edges = append(edges, edge)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, edge := range edges {
		if err = t.resolveEdge(r, n, edge, enqueue); err != nil {
			return err
		}
	}
	if enqueue && terminal(n.State) {
		watch, err := t.rows(`SELECT target_id FROM dag_condition_watchers WHERE run_id=? AND source_id=? ORDER BY target_id`, r.ID, n.ID)
		if err != nil {
			return err
		}
		targets := []string{}
		for watch.Next() {
			var id string
			if err = watch.Scan(&id); err != nil {
				watch.Close()
				return err
			}
			targets = append(targets, id)
		}
		err = watch.Err()
		watch.Close()
		if err != nil {
			return err
		}
		for _, id := range targets {
			if err = t.enqueueNode(r, id); err != nil {
				return err
			}
		}
	}
	if n.parent != "" {
		return t.updateMapProgress(r, n, enqueue)
	}
	return nil
}

func (t *transaction) resolveEdge(r *runState, n *nodeState, edge Edge, enqueue bool) error {
	var generation int
	if err := t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, edge.To).Scan(&generation); err != nil {
		return err
	}
	resolved, selected, success, failed := 0, 0, 0, 0
	if terminal(n.State) {
		resolved = 1
		if edgeActive(edge, n) && !(r.kernelGeneration >= 2 && edge.Condition == "" && skippedUnselected(n)) {
			selected = 1
			if n.State == "succeeded" {
				success = 1
			} else {
				failed = 1
			}
		}
	}
	var oldResolved, oldSelected, oldSuccess, oldFailed, oldAttempt int
	var oldFence, order int64
	err := t.row(`SELECT resolved,selected,success,failed,source_attempt,source_fence,resolution_order FROM dag_edge_resolutions WHERE run_id=? AND target_id=? AND generation=? AND source_id=?`, r.ID, edge.To, generation, n.ID).Scan(&oldResolved, &oldSelected, &oldSuccess, &oldFailed, &oldAttempt, &oldFence, &order)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	exists := err == nil
	if exists && oldResolved == resolved && oldSelected == selected && oldSuccess == success && oldFailed == failed && oldAttempt == n.Attempt && oldFence == n.Fence {
		return nil
	}
	if resolved == 1 && oldResolved == 0 {
		order, err = t.nextQueueSequence(r.TenantID, r.ProjectID)
		if err != nil {
			return err
		}
	}
	if resolved == 0 {
		order = 0
	}
	if exists {
		err = t.exec(`UPDATE dag_edge_resolutions SET source_attempt=?,source_fence=?,resolved=?,selected=?,success=?,failed=?,resolution_order=? WHERE run_id=? AND target_id=? AND generation=? AND source_id=?`, n.Attempt, n.Fence, resolved, selected, success, failed, order, r.ID, edge.To, generation, n.ID)
	} else {
		err = t.exec(`INSERT INTO dag_edge_resolutions(run_id,target_id,generation,source_id,source_attempt,source_fence,resolved,selected,success,failed,resolution_order)VALUES(?,?,?,?,?,?,?,?,?,?,?)`, r.ID, edge.To, generation, n.ID, n.Attempt, n.Fence, resolved, selected, success, failed, order)
	}
	if err != nil {
		return err
	}
	if err = t.exec(`UPDATE dag_node_dependencies SET resolved=resolved+?,selected=selected+?,success=success+?,failed=failed+?,version=version+1 WHERE run_id=? AND node_id=? AND generation=?`, resolved-oldResolved, selected-oldSelected, success-oldSuccess, failed-oldFailed, r.ID, edge.To, generation); err != nil {
		return err
	}
	if enqueue {
		return t.enqueueNode(r, edge.To)
	}
	return nil
}

func (t *transaction) incrementalEligible(r *runState, n *nodeState) (bool, bool, error) {
	var generation, total, resolved, selected, success int
	var winners, decision string
	err := t.row(`SELECT generation,total,resolved,selected,success,winners,decision_at,ready_at FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&generation, &total, &resolved, &selected, &success, &winners, &decision, &n.ReadyAt)
	if err != nil {
		return false, false, err
	}
	if json.Unmarshal([]byte(winners), &n.WinnerNodeIDs) != nil {
		return false, false, fmt.Errorf("%w: invalid frozen join decision", ErrInvalid)
	}
	if total == 0 {
		return true, false, nil
	}
	all := resolved == total
	ready, impossible := false, false
	switch n.spec.Join {
	case "all_done":
		ready = all && selected > 0
		impossible = all && selected == 0
	case "any_success":
		ready = success > 0
		impossible = all && success == 0
	case "quorum":
		ready = success >= n.spec.Quorum
		impossible = success+total-resolved < n.spec.Quorum
	default:
		required := selected
		if r.kernelGeneration == 1 {
			required = total
		}
		ready = all && selected > 0 && success == required
		impossible = all && !ready
	}
	if ready && decision == "" && (n.spec.Join == "any_success" || n.spec.Join == "quorum") {
		count := 1
		if n.spec.Join == "quorum" {
			count = n.spec.Quorum
		}
		rows, err := t.rows(`SELECT source_id FROM dag_edge_resolutions WHERE run_id=? AND target_id=? AND generation=? AND success=1 ORDER BY resolution_order,source_id LIMIT ?`, r.ID, n.ID, generation, count)
		if err != nil {
			return false, false, err
		}
		n.WinnerNodeIDs = []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return false, false, err
			}
			n.WinnerNodeIDs = append(n.WinnerNodeIDs, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return false, false, err
		}
		if len(n.WinnerNodeIDs) != count {
			return false, false, fmt.Errorf("%w: insufficient winner evidence", ErrConflict)
		}
		if err = t.exec(`INSERT INTO dag_join_decisions(run_id,node_id,generation,winners,decided_at)VALUES(?,?,?,?,?)`, r.ID, n.ID, generation, encoded(n.WinnerNodeIDs), stamp(t.now)); err != nil {
			return false, false, err
		}
		if err = t.exec(`UPDATE dag_node_dependencies SET winners=?,decision_at=?,version=version+1 WHERE run_id=? AND node_id=? AND generation=? AND decision_at=''`, encoded(n.WinnerNodeIDs), stamp(t.now), r.ID, n.ID, generation); err != nil {
			return false, false, err
		}
	}
	return ready, impossible, nil
}

func (t *transaction) resetDependencies(r *runState, n *nodeState) error {
	if err := t.exec(`UPDATE dag_node_dependencies SET generation=generation+1,resolved=0,selected=0,success=0,failed=0,winners='[]',decision_at='',ready_at='',version=version+1 WHERE run_id=? AND node_id=? AND generation<2147483647`, r.ID, n.ID); err != nil {
		return err
	}
	n.WinnerNodeIDs = nil
	rows, err := t.rows(`SELECT source_id,condition_value FROM dag_graph_edges WHERE run_id=? AND target_id=?`, r.ID, n.ID)
	if err != nil {
		return err
	}
	edges := []Edge{}
	for rows.Next() {
		edge := Edge{To: n.ID}
		if err = rows.Scan(&edge.From, &edge.Condition); err != nil {
			rows.Close()
			return err
		}
		edges = append(edges, edge)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, edge := range edges {
		source := r.find(edge.From)
		if source == nil {
			return fmt.Errorf("%w: missing dependency source", ErrInvalid)
		}
		if err = t.resolveEdge(r, source, edge, false); err != nil {
			return err
		}
	}
	return nil
}

func (t *transaction) updateMapProgress(r *runState, n *nodeState, enqueue bool) error {
	var oldTerminal, oldFailed int
	err := t.row(`SELECT terminal_flag,failed_flag FROM dag_map_child_states WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&oldTerminal, &oldFailed)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	exists := err == nil
	term, fail := 0, 0
	if terminal(n.State) {
		term = 1
		if n.State != "succeeded" {
			fail = 1
		}
	}
	if exists && term == oldTerminal && fail == oldFailed {
		return nil
	}
	if exists {
		err = t.exec(`UPDATE dag_map_child_states SET terminal_flag=?,failed_flag=? WHERE run_id=? AND node_id=?`, term, fail, r.ID, n.ID)
	} else {
		err = t.exec(`INSERT INTO dag_map_child_states(run_id,node_id,parent_id,terminal_flag,failed_flag)VALUES(?,?,?,?,?)`, r.ID, n.ID, n.parent, term, fail)
	}
	if err != nil {
		return err
	}
	if err = t.exec(`UPDATE dag_map_progress SET terminal_count=terminal_count+?,failed_count=failed_count+? WHERE run_id=? AND node_id=?`, term-oldTerminal, fail-oldFailed, r.ID, n.parent); err != nil {
		return err
	}
	if enqueue {
		return t.enqueueNode(r, n.parent)
	}
	return nil
}

func (r *runState) counts() (total, done, failed, cancelled int) {
	total, done, failed, cancelled = r.totalNodes, r.terminalNodes, r.failedNodes, r.cancelledNodes
	for _, n := range r.nodes {
		if terminal(n.originalState) {
			done--
		}
		if terminal(n.State) {
			done++
		}
		if n.originalState == "failed" {
			failed--
		}
		if n.State == "failed" {
			failed++
		}
		if n.originalState == "cancelled" {
			cancelled--
		}
		if n.State == "cancelled" {
			cancelled++
		}
	}
	return
}
