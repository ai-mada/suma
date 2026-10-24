package dag

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

func workerNode(kind string) bool {
	switch kind {
	case "task", "command", "script", "test", "agent", "external_workflow":
		return true
	}
	return false
}

// Generated inverse definitions are evidence, never public dispatch templates.
// The durable compensation relationship marks the whole workflow identity,
// including any attempted new versions, without relying on a name prefix.
func (t *transaction) publicWorkflow(tenant, project, id string) error {
	if err := t.guardProject(tenant, project); err != nil {
		return err
	}
	var count int
	if err := t.row("SELECT COUNT(*) FROM dag_managed_workflows WHERE tenant_id=? AND project_id=? AND workflow_id=?", tenant, project, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: service-managed workflow cannot be published, reused or embedded", ErrInvalid)
	}
	if err := t.row("SELECT COUNT(*) FROM dag_compensations c JOIN dag_runs r ON r.id=c.compensation_run_id WHERE r.tenant_id=? AND r.project_id=? AND r.workflow_id=?", tenant, project, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: generated compensation workflow is read-only and cannot be reused", ErrInvalid)
	}
	return nil
}
func compensationTemplate(n Node) (Node, bool, error) {
	raw, ok := n.Config["compensation"]
	if !ok {
		return Node{}, false, nil
	}
	var template Node
	if !workerNode(n.Type) {
		return template, false, fmt.Errorf("%w: only worker nodes may declare compensation", ErrInvalid)
	}
	if err := decode(raw, &template); err != nil {
		return template, false, fmt.Errorf("%w: compensation must be a node object", ErrInvalid)
	}
	if !workerNode(template.Type) || len(template.DependsOn) > 0 || template.Condition != "" || len(template.Branches) > 0 || template.Join != "" && template.Join != "all_success" || template.Quorum != 0 {
		return template, false, fmt.Errorf("%w: compensation must be an unconditional worker node; inverse ordering is generated", ErrInvalid)
	}
	if _, nested := template.Config["compensation"]; nested {
		return template, false, fmt.Errorf("%w: compensation templates cannot recursively compensate", ErrInvalid)
	}
	template.ID = "compensation"
	if _, err := normalize(Workflow{Nodes: []Node{template}}); err != nil {
		return template, false, err
	}
	return template, true, nil
}
func bindSource(v any, replacements map[string]string) any {
	switch value := v.(type) {
	case string:
		pairs := []string{}
		for _, key := range []string{"run_id", "node_id", "attempt", "fence", "output"} {
			pairs = append(pairs, "{{source."+key+"}}", replacements[key])
		}
		return strings.NewReplacer(pairs...).Replace(value)
	case map[string]any:
		out := map[string]any{}
		for key, item := range value {
			out[key] = bindSource(item, replacements)
		}
		return out
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = bindSource(item, replacements)
		}
		return out
	}
	return v
}
func (t *transaction) compensate(r *runState, payload map[string]any) error {
	if !terminal(r.State) {
		return fmt.Errorf("%w: compensation requires a terminal run with no active or unknown work", ErrConflict)
	}
	if r.CompensationRunID != "" {
		return fmt.Errorf("%w: compensation already requested", ErrConflict)
	}
	reason, _ := payload["reason"].(string)
	if strings.TrimSpace(reason) == "" || len(reason) > 5000 {
		return fmt.Errorf("%w: compensation reason required", ErrInvalid)
	}
	var parent string
	err := t.row("SELECT original_run_id FROM dag_compensations WHERE compensation_run_id=?", r.ID).Scan(&parent)
	if err == nil {
		return fmt.Errorf("%w: a compensation run cannot itself be compensated", ErrInvalid)
	}
	if err != sql.ErrNoRows {
		return err
	}
	selected := map[string]bool{}
	explicit := false
	if raw, ok := payload["node_ids"]; ok {
		explicit = true
		var ids []string
		if err := decode(raw, &ids); err != nil {
			return fmt.Errorf("%w: node_ids must be strings", ErrInvalid)
		}
		for _, id := range ids {
			selected[id] = true
		}
		if len(selected) == 0 {
			return fmt.Errorf("%w: select at least one configured successful worker node", ErrInvalid)
		}
	}
	source := map[string]*nodeState{}
	templates := map[string]Node{}
	for _, n := range r.nodes {
		if explicit && !selected[n.ID] {
			continue
		}
		template, configured, err := compensationTemplate(n.spec)
		if err != nil {
			return err
		}
		if !configured || !workerNode(n.Type) || n.State != "succeeded" || n.Attempt < 1 {
			continue
		}
		var state string
		var fence int64
		if err = t.row("SELECT state,fence FROM dag_attempts WHERE run_id=? AND node_id=? AND attempt=?", r.ID, n.ID, n.Attempt).Scan(&state, &fence); err != nil {
			return err
		}
		if state != "succeeded" || fence != n.Fence {
			return fmt.Errorf("%w: source attempt evidence differs from node state", ErrConflict)
		}
		source[n.ID] = n
		templates[n.ID] = template
	}
	if len(source) == 0 {
		return fmt.Errorf("%w: no successful worker nodes have explicit compensation", ErrInvalid)
	}
	if explicit {
		for id := range selected {
			if source[id] == nil {
				return fmt.Errorf("%w: node %s has no successful configured compensation", ErrInvalid, id)
			}
		}
	}
	w := Workflow{ID: "comp-" + uid(), Name: "Compensation " + r.WorkflowName, Version: 1, Nodes: []Node{}, Edges: []Edge{}}
	for len(w.Name) > 240 {
		runes := []rune(w.Name)
		w.Name = string(runes[:len(runes)-1])
	}
	refs := map[string]map[string]any{}
	compIDs := map[string]string{}
	for _, n := range r.nodes {
		if source[n.ID] == nil {
			continue
		}
		id := "undo-" + fingerprint(n.ID)[:24]
		compIDs[n.ID] = id
		template := templates[n.ID]
		template.ID = id
		if template.Name == "" {
			template.Name = "Compensate " + n.Name
		}
		template.Join = "all_success"
		replacements := map[string]string{"run_id": r.ID, "node_id": n.ID, "attempt": strconv.Itoa(n.Attempt), "fence": strconv.FormatInt(n.Fence, 10), "output": n.Output}
		for i, arg := range template.Command {
			template.Command[i] = bindSource(arg, replacements).(string)
		}
		if template.Config == nil {
			template.Config = map[string]any{}
		}
		template.Config = bindSource(template.Config, replacements).(map[string]any)
		ref := map[string]any{"run_id": r.ID, "node_id": n.ID, "attempt": n.Attempt, "fence": n.Fence, "worker_id": n.WorkerID, "output_sha256": hashText(n.Output)}
		refs[n.ID] = ref
		template.Config["source_execution"] = ref
		if template.Type == "external_workflow" {
			input, ok := template.Config["input"].(map[string]any)
			if !ok {
				input = map[string]any{}
			}
			input["source_execution"] = ref
			template.Config["input"] = input
		}
		w.Nodes = append(w.Nodes, template)
	}
	// Reachability preserves ordering through nodes that have no inverse and
	// through expanded map children. Inverses of parallel nodes stay parallel.
	next := map[string][]string{}
	for _, edge := range r.workflow.Edges {
		next[edge.From] = append(next[edge.From], edge.To)
	}
	for _, n := range r.nodes {
		if n.parent != "" {
			next[n.ID] = append(next[n.ID], n.parent)
			if p := r.find(n.parent); p != nil {
				for _, dependency := range p.spec.DependsOn {
					next[dependency] = append(next[dependency], n.ID)
				}
			}
		}
	}
	for id := range source {
		seen := map[string]bool{}
		var visit func(string)
		visit = func(current string) {
			for _, child := range next[current] {
				if seen[child] {
					continue
				}
				seen[child] = true
				if source[child] != nil {
					w.Edges = append(w.Edges, Edge{From: compIDs[child], To: compIDs[id]})
				}
				visit(child)
			}
		}
		visit(id)
	}
	if len(encoded(w)) > 2<<20 {
		return fmt.Errorf("%w: generated compensation definition exceeds 2 MiB", ErrInvalid)
	}
	w, err = normalize(w)
	if err != nil {
		return err
	}
	if err = t.exec("INSERT INTO dag_workflows (tenant_id,project_id,id,version,name,spec) VALUES (?,?,?,?,?,?)", r.TenantID, r.ProjectID, w.ID, w.Version, w.Name, encoded(w)); err != nil {
		return err
	}
	child, err := t.createRun(r.TenantID, r.ProjectID, w, nil)
	if err != nil {
		return err
	}
	if err = t.exec("INSERT INTO dag_compensations (original_run_id,compensation_run_id,original_version,original_state,reason,last_state) VALUES (?,?,?,?,?,?)", r.ID, child.ID, r.Version, r.State, reason, child.State); err != nil {
		return err
	}
	for id, n := range source {
		if err = t.exec("INSERT INTO dag_compensation_effects (original_run_id,node_id,attempt,fence,compensation_node_id,source_ref) VALUES (?,?,?,?,?,?)", r.ID, n.ID, n.Attempt, n.Fence, compIDs[id], encoded(refs[id])); err != nil {
			return err
		}
	}
	r.CompensationRunID = child.ID
	r.CompensationState = child.State
	return t.event(r, "", "compensation_requested", map[string]any{"compensation_run_id": child.ID, "source_version": r.Version, "reason": reason, "effects": refs})
}
func (t *transaction) propagateCompensation(child *runState) error {
	var originalID, previous string
	err := t.row("SELECT original_run_id,last_state FROM dag_compensations WHERE compensation_run_id=?", child.ID).Scan(&originalID, &previous)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}
	if previous == child.State {
		return nil
	}
	original, err := t.loadHot(child.TenantID, child.ProjectID, originalID)
	if err != nil {
		return err
	}
	if err = t.exec("UPDATE dag_compensations SET last_state=? WHERE original_run_id=?", child.State, originalID); err != nil {
		return err
	}
	if err = t.event(original, "", "compensation_state_changed", map[string]any{"compensation_run_id": child.ID, "previous_state": previous, "state": child.State}); err != nil {
		return err
	}
	return t.saveQuiet(original)
}
