package dag

import "database/sql"

func (t *transaction) putWake(r *runState, n *nodeState, kind string, due int64) error {
	var generation int
	if err := t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&generation); err != nil {
		return err
	}
	if err := t.deleteWakes(r.ID, n.ID, kind); err != nil {
		return err
	}
	state := "pending"
	if r.State == "paused" && kind != "lease" {
		state = "paused"
	}
	return t.exec(`INSERT INTO dag_wakeups(run_id,node_id,kind,tenant_id,project_id,generation,attempt,fence,due_at,state)VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, n.ID, kind, r.TenantID, r.ProjectID, generation, n.Attempt, n.Fence, due, state)
}
func (t *transaction) syncWake(r *runState, n *nodeState) error {
	if !r.indexed {
		return nil
	}
	if terminal(n.State) {
		return t.deleteWakes(r.ID, n.ID, "")
	}
	if n.State == "running" && n.lease > 0 {
		return t.putWake(r, n, "lease", n.lease)
	}
	if n.State == "waiting" && n.Type == "timer" {
		return t.putWake(r, n, "timer", n.wake)
	}
	if n.State == "waiting" && n.Type == "subworkflow" && n.wake > 0 {
		return t.putWake(r, n, "child_deadline", n.wake)
	}
	if n.State == "pending" && n.wake > t.now {
		return t.putWake(r, n, "retry", n.wake)
	}
	return nil
}
func (t *transaction) putPropagationWake(r *runState) error {
	if err := t.deleteWakes(r.ID, "", "propagation"); err != nil {
		return err
	}
	state := "pending"
	if r.State == "paused" {
		state = "paused"
	}
	return t.exec(`INSERT INTO dag_wakeups(run_id,node_id,kind,tenant_id,project_id,generation,attempt,fence,due_at,state)VALUES(?,'','propagation',?,?,0,0,0,?,?)`, r.ID, r.TenantID, r.ProjectID, t.now, state)
}
func (t *transaction) nextNodeWork(r *runState) (string, error) {
	var id string
	err := t.row(`SELECT node_id FROM dag_node_work WHERE run_id=? ORDER BY sequence,node_id LIMIT 1`, r.ID).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err = t.exec(`DELETE FROM dag_node_work WHERE run_id=? AND node_id=?`, r.ID, id); err != nil {
		return "", err
	}
	return id, nil
}
