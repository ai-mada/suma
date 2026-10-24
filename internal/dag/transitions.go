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
	for _, n := range r.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}
func (t *transaction) transition(r *runState, n *nodeState, state, reason string) error {
	before := n.State
	n.State = state
	n.Reason = reason
	if terminal(state) {
		n.CompletedAt = stamp(t.now)
		n.lease = 0
		if err := t.exec("DELETE FROM dag_ready WHERE run_id=? AND node_id=?", r.ID, n.ID); err != nil {
			return err
		}
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
	var seq int64
	if err := t.row("SELECT seq FROM dag_lock WHERE id=1").Scan(&seq); err != nil {
		return err
	}
	return t.exec("INSERT INTO dag_ready (run_id,node_id,sequence) VALUES (?,?,?)", r.ID, n.ID, seq)
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
	raw, err := mapItems(n.spec)
	if err != nil {
		return err
	}
	if len(raw) > 1000 {
		return fmt.Errorf("%w: map expansion exceeds 1000", ErrInvalid)
	}
	template, err := mapTemplate(n.spec)
	if err != nil {
		return err
	}
	if template.Type == "map" {
		return fmt.Errorf("%w: nested map unsupported", ErrUnsupported)
	}
	for i, item := range raw {
		spec := template
		spec.ID = fmt.Sprintf("%s[%d]", n.ID, i)
		spec.Name = spec.ID
		spec.DependsOn = nil
		spec.Command = append([]string{}, template.Command...)
		value, ok := item.(string)
		if !ok {
			value = encoded(item)
		}
		for j := range spec.Command {
			spec.Command[j] = strings.ReplaceAll(spec.Command[j], "{{item}}", value)
			spec.Command[j] = strings.ReplaceAll(spec.Command[j], "{{index}}", fmt.Sprint(i))
		}
		spec.Config = map[string]any{}
		for key, value := range template.Config {
			spec.Config[key] = value
		}
		spec.Config["item"] = item
		spec.Config["index"] = i
		child := &nodeState{NodeRun: NodeRun{ID: spec.ID, Name: spec.Name, Type: spec.Type, State: "pending", Command: spec.Command}, spec: spec, parent: n.ID}
		r.nodes = append(r.nodes, child)
		if err := t.insertNode(r, child); err != nil {
			return err
		}
	}
	if len(raw) == 0 {
		return t.transition(r, n, "succeeded", "empty map")
	}
	return t.transition(r, n, "waiting", "map_children")
}

// advance reaches a fixed point before committing: joining, branch skips,
// approval waits, timer waits and expansion never hold a worker lease.
func (t *transaction) advance(r *runState) error {
	if _, err := t.resolveSubruns(r); err != nil {
		return err
	}
	if r.State == "paused" || r.State == "cancelling" || terminal(r.State) {
		return t.finishRun(r)
	}
	for {
		changed, resolveErr := t.resolveSubruns(r)
		if resolveErr != nil {
			return resolveErr
		}
		for _, n := range r.nodes {
			if n.State == "waiting" && n.Type == "map" {
				all, failed := true, false
				for _, child := range r.nodes {
					if child.parent == n.ID {
						all = all && terminal(child.State)
						failed = failed || child.State != "succeeded"
					}
				}
				if all {
					state := "succeeded"
					if failed {
						state = "failed"
					}
					if err := t.transition(r, n, state, "map children finished"); err != nil {
						return err
					}
					changed = true
				}
				continue
			}
			if n.State == "waiting" && n.Type == "timer" && n.wake <= t.now {
				if err := t.transition(r, n, "succeeded", "timer elapsed"); err != nil {
					return err
				}
				changed = true
				continue
			}
			if n.State != "pending" {
				continue
			}
			if n.wake > t.now {
				continue
			}
			ok, impossible := r.eligible(n)
			if impossible {
				if err := t.transition(r, n, "skipped", "join cannot be satisfied"); err != nil {
					return err
				}
				changed = true
				continue
			}
			if !ok {
				continue
			}
			if !predicatesReady(n, r) {
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
				changed = true
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
			changed = true
		}
		if !changed {
			break
		}
	}
	return t.finishRun(r)
}
func (t *transaction) finishRun(r *runState) error {
	all, failed, cancelled := true, false, false
	for _, n := range r.nodes {
		all = all && terminal(n.State)
		failed = failed || n.State == "failed"
		cancelled = cancelled || n.State == "cancelled"
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
