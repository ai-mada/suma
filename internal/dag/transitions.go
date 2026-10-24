package dag

import (
	"fmt"
	"sort"
	"strings"
)

func terminal(s string) bool {
	switch s {
	case "succeeded", "failed", "skipped", "cancelled":
		return true
	}
	return false
}
func (r *runState) find(id string) *nodeState {
	if r.nodeMap == nil {
		r.nodeMap = map[string]*nodeState{}
		for _, n := range r.nodes {
			r.nodeMap[n.ID] = n
		}
	}
	if n := r.nodeMap[id]; n != nil {
		return n
	}
	for _, n := range r.nodes {
		if n.ID == id {
			r.nodeMap[id] = n
			return n
		}
	}
	return r.lazyNode(id)
}
func (t *transaction) transition(r *runState, n *nodeState, state, reason string) error {
	before := n.State
	n.State = state
	n.Reason = reason
	if r.indexed && terminal(before) && (state == "pending" || state == "waiting") {
		if err := t.resetDependencies(r, n); err != nil {
			return err
		}
		// Retracting an old terminal outcome must reach counters before any
		// reopened parent/descendant can inspect them in this transaction.
		if err := t.resolveOutgoing(r, n, true); err != nil {
			return err
		}
	}
	if terminal(state) {
		n.CompletedAt = stamp(t.now)
		n.lease = 0
		if err := t.exec("DELETE FROM dag_ready WHERE run_id=? AND node_id=?", r.ID, n.ID); err != nil {
			return err
		}
		if err := t.exec(`DELETE FROM dag_ready_v2 WHERE run_id=? AND node_id=?`, r.ID, n.ID); err != nil {
			return err
		}
	}
	if r.indexed && (terminal(state) || state == "pending") {
		if err := t.enqueueNode(r, n.ID); err != nil {
			return err
		}
	}
	if err := t.syncWake(r, n); err != nil {
		return err
	}
	return t.event(r, n.ID, "node_"+state, map[string]any{"previous_state": before, "reason": reason, "attempt": n.Attempt, "fence": n.Fence})
}
func condition(expression string, r *runState) (bool, error) {
	s := strings.TrimSpace(expression)
	if s == "" || s == "true" {
		return true, nil
	}
	if s == "false" {
		return false, nil
	}
	if err := validateCondition(s); err != nil {
		return false, err
	}
	parts := strings.SplitN(s, "==", 2)
	lhs := strings.TrimSpace(parts[0])
	value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
	pos := strings.LastIndex(lhs, ".")
	n := r.find(lhs[:pos])
	if n == nil {
		return false, fmt.Errorf("%w: condition references missing node", ErrInvalid)
	}
	if !terminal(n.State) {
		return false, fmt.Errorf("%w: condition dependency is not terminal", ErrInvalid)
	}
	actual := n.State
	if lhs[pos+1:] == "output" {
		actual = n.Output
	}
	return actual == value, nil
}
func predicatesReady(n *nodeState, r *runState) bool {
	for _, expression := range predicates(n.spec) {
		parts := strings.SplitN(expression, "==", 2)
		if len(parts) != 2 {
			continue
		}
		lhs := strings.TrimSpace(parts[0])
		pos := strings.LastIndex(lhs, ".")
		if pos < 1 {
			return false
		}
		ancestor := r.find(lhs[:pos])
		if ancestor == nil || !terminal(ancestor.State) {
			return false
		}
	}
	return true
}
func edgeActive(e Edge, parent *nodeState) bool {
	switch e.Condition {
	case "", "always", "all_done":
		return true
	case "success", "succeeded":
		return parent.State == "succeeded"
	case "failure", "failed":
		return parent.State == "failed"
	case "skipped":
		return parent.State == "skipped"
	case "cancelled":
		return parent.State == "cancelled"
	default:
		return parent.Output == e.Condition
	}
}
func (r *runState) eligible(n *nodeState) (ready, impossible bool) {
	if n.parent != "" {
		return true, false
	}
	parents := []*nodeState{}
	for _, e := range r.workflow.Edges {
		if e.To == n.ID {
			parents = append(parents, r.find(e.From))
		}
	}
	if len(parents) == 0 {
		return true, false
	}
	done, success, active := 0, 0, 0
	for _, e := range r.workflow.Edges {
		if e.To != n.ID {
			continue
		}
		p := r.find(e.From)
		if !terminal(p.State) {
			continue
		}
		done++
		if edgeActive(e, p) {
			active++
			if p.State == "succeeded" {
				success++
			}
		}
	}
	allDone := done == len(parents)
	switch n.spec.Join {
	case "all_done":
		return allDone && active > 0, allDone && active == 0
	case "any_success":
		return success > 0, allDone && success == 0
	case "quorum":
		return success >= n.spec.Quorum, success+(len(parents)-done) < n.spec.Quorum
	default:
		return allDone && success == len(parents), allDone && success != len(parents)
	}
}
func (t *transaction) ready(r *runState, n *nodeState) error {
	if err := t.transition(r, n, "ready", ""); err != nil {
		return err
	}
	seq, err := t.nextQueueSequence(r.TenantID, r.ProjectID)
	if err != nil {
		return err
	}
	if err = t.exec("INSERT INTO dag_ready (run_id,node_id,sequence) VALUES (?,?,?)", r.ID, n.ID, seq); err != nil {
		return err
	}
	command := ""
	if len(n.spec.Command) == 1 {
		command = n.spec.Command[0]
	}
	required, _ := n.spec.Config["capability"].(string)
	n.ReadyAt = stamp(t.now)
	generation := 1
	if r.indexed {
		if err = t.row(`SELECT generation FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&generation); err != nil {
			return err
		}
		if err = t.exec(`UPDATE dag_node_dependencies SET ready_at=?,version=version+1 WHERE run_id=? AND node_id=?`, n.ReadyAt, r.ID, n.ID); err != nil {
			return err
		}
	}
	if n.Type == "external_workflow" {
		plugin, _ := n.spec.Config["plugin"].(string)
		required = "external_workflow:" + plugin
	}
	if err = t.exec(`INSERT INTO dag_ready_v2(run_id,node_id,tenant_id,project_id,generation,sequence,ready_at,node_type,command_id,required_capability)VALUES(?,?,?,?,?,?,?,?,?,?)`, r.ID, n.ID, r.TenantID, r.ProjectID, generation, seq, n.ReadyAt, n.Type, command, required); err != nil {
		return err
	}
	return t.refreshRunTurn(r)
}
func numeric(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	}
	return 0
}
func (t *transaction) expand(r *runState, n *nodeState) error {
	return t.beginMapExpansion(r, n)
}

// advance reaches a fixed point before committing: joining, branch skips,
// approval waits, timer waits and expansion never hold a worker lease.
func (t *transaction) advance(r *runState) error {
	if !r.indexed {
		if err := t.initializeGraph(r, true); err != nil {
			return err
		}
	}
	if r.loadErr != nil {
		return r.loadErr
	}
	if r.State == "cancelling" {
		if _, err := t.resolveSubruns(r); err != nil {
			return err
		}
		return t.finishRun(r)
	}
	if r.State == "paused" || terminal(r.State) {
		return t.finishRun(r)
	}
	for processed := 0; processed < 2048; processed++ {
		id, err := t.nextNodeWork(r)
		if err != nil {
			return err
		}
		if id == "" {
			return t.finishRun(r)
		}
		n := r.find(id)
		if n == nil {
			if r.loadErr != nil {
				return r.loadErr
			}
			return fmt.Errorf("%w: queued node missing", ErrInvalid)
		}
		if err = t.resolveOutgoing(r, n, true); err != nil {
			return err
		}
		if n.State == "waiting" && n.Type == "map" {
			var total, done, failed, closed int
			if err = t.row(`SELECT total,terminal_count,failed_count,closed FROM dag_map_progress WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&total, &done, &failed, &closed); err != nil {
				return err
			}
			if closed == 1 && done == total {
				state := "succeeded"
				if failed > 0 {
					state = "failed"
				}
				if err = t.transition(r, n, state, "map children finished"); err != nil {
					return err
				}
			}
			continue
		}
		if n.State == "waiting" && n.Type == "timer" && n.wake <= t.now {
			if err = t.transition(r, n, "succeeded", "timer elapsed"); err != nil {
				return err
			}
			continue
		}
		if n.State == "waiting" && n.Type == "subworkflow" {
			if _, err = t.resolveSubruns(r); err != nil {
				return err
			}
			continue
		}
		if n.State != "pending" {
			continue
		}
		if n.wake > t.now {
			if err = t.syncWake(r, n); err != nil {
				return err
			}
			continue
		}
		ok, impossible, err := t.incrementalEligible(r, n)
		if err != nil {
			return err
		}
		if impossible {
			reason := "join cannot be satisfied"
			var selected int
			if err = t.row(`SELECT selected FROM dag_node_dependencies WHERE run_id=? AND node_id=?`, r.ID, n.ID).Scan(&selected); err != nil {
				return err
			}
			if selected == 0 && r.kernelGeneration >= 2 {
				reason = "unselected branch"
			}
			if err = t.transition(r, n, "skipped", reason); err != nil {
				return err
			}
			continue
		}
		if !ok || !predicatesReady(n, r) {
			continue
		}
		enabled, err := condition(n.spec.Condition, r)
		if err != nil {
			return err
		}
		if !enabled {
			if err = t.transition(r, n, "skipped", "condition false"); err != nil {
				return err
			}
			continue
		}
		switch n.Type {
		case "approval":
			err = t.transition(r, n, "waiting", "approval_required")
		case "timer":
			duration := numeric(n.spec.Config, "duration_seconds")
			n.wake = t.now + duration
			err = t.transition(r, n, "waiting", "timer")
		case "condition":
			var value bool
			expr, _ := n.spec.Config["expression"].(string)
			value, err = condition(expr, r)
			if err == nil {
				n.Output = fmt.Sprint(value)
				if len(n.spec.Branches) > 0 {
					keys := []string{}
					for key := range n.spec.Branches {
						keys = append(keys, key)
					}
					sort.Strings(keys)
					n.Output = ""
					fallback := ""
					for _, key := range keys {
						expr := n.spec.Branches[key]
						if expr == "default" {
							fallback = key
							continue
						}
						matched, e := condition(expr, r)
						if e != nil {
							return e
						}
						if matched {
							n.Output = key
							break
						}
					}
					if n.Output == "" {
						n.Output = fallback
					}
				}
				err = t.transition(r, n, "succeeded", "condition evaluated")
			}
		case "join", "noop":
			err = t.transition(r, n, "succeeded", "")
		case "map":
			err = t.expand(r, n)
		case "subworkflow":
			err = t.spawnSubrun(r, n)
		default:
			err = t.ready(r, n)
		}
		if err != nil {
			return err
		}
	}
	if err := t.putPropagationWake(r); err != nil {
		return err
	}
	return t.finishRun(r)
}
func (t *transaction) finishRun(r *runState) error {
	all, failed, cancelled := true, false, false
	if r.indexed {
		total, done, bad, stopped := r.counts()
		all = done == total
		failed = bad > 0
		cancelled = stopped > 0
	} else {
		for _, n := range r.nodes {
			all = all && terminal(n.State)
			failed = failed || n.State == "failed"
			cancelled = cancelled || n.State == "cancelled"
		}
	}
	if !all {
		return nil
	}
	state := "succeeded"
	if failed {
		state = "failed"
	}
	if cancelled || r.State == "cancelling" {
		state = "cancelled"
	}
	if r.State != state {
		r.State = state
		return t.event(r, "", "run_"+state, nil)
	}
	return nil
}
