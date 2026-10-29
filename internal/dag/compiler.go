package dag

import (
	"fmt"
	"regexp"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var types = map[string]bool{"task": true, "command": true, "agent": true, "script": true, "test": true, "approval": true, "timer": true, "condition": true, "map": true, "join": true, "noop": true, "subworkflow": true, "external_workflow": true, "terminal_session": true}

const maxExecutionTimeoutSeconds = 9223372036

func Compile(w Workflow) error { _, err := normalize(w); return err }
func predicates(n Node) []string {
	out := []string{n.Condition}
	if n.Type == "condition" {
		if expression, ok := n.Config["expression"].(string); ok {
			out = append(out, expression)
		}
		for _, expression := range n.Branches {
			out = append(out, expression)
		}
	}
	return out
}
func normalize(w Workflow) (Workflow, error) {
	w.Nodes = append([]Node(nil), w.Nodes...)
	if len(w.Nodes) == 0 || len(w.Nodes) > 1000 {
		return w, dagDiagnostic(fmt.Errorf("%w: workflow requires 1..1000 nodes", ErrInvalid), "dag.workflow.node_count", nil)
	}
	seen := map[string]int{}
	for i, n := range w.Nodes {
		if n.Priority < 0 || n.Priority > 9 {
			return w, dagDiagnostic(fmt.Errorf("%w: priority must be 0..9", ErrInvalid), "dag.node.priority", nil)
		}
		if !identifier.MatchString(n.ID) || !types[n.Type] {
			return w, dagDiagnostic(fmt.Errorf("%w: node ID/type %q/%q", ErrInvalid, n.ID, n.Type), "dag.node.identity", diagnosticParams{"node": fmt.Sprintf("%q", n.ID), "type": fmt.Sprintf("%q", n.Type)})
		}
		if _, ok := seen[n.ID]; ok {
			return w, dagDiagnostic(fmt.Errorf("%w: duplicate node %s", ErrInvalid, n.ID), "dag.node.duplicate", diagnosticParams{"node": n.ID})
		}
		seen[n.ID] = i
		if n.Type == "terminal_session" && (n.MaxRetries != 0 || n.Idempotent || n.TimeoutSeconds < 60 || n.TimeoutSeconds > 3600) {
			return w, fmt.Errorf("%w: terminal sessions require an explicit 60..3600 second limit and cannot retry", ErrInvalid)
		}
		if n.Type == "command" || n.Type == "agent" || n.Type == "test" || n.Type == "terminal_session" {
			if len(n.Command) != 1 || !identifier.MatchString(n.Command[0]) {
				return w, dagDiagnostic(fmt.Errorf("%w: command/agent/test require one allowlisted command identifier", ErrInvalid), "dag.node.command", nil)
			}
		}
		if _, _, _, err := ToolBinding(n); err != nil {
			return w, err
		}
		if _, err := ModelBinding(n); err != nil {
			return w, err
		}
		if _, err := CodexBinding(n); err != nil {
			return w, err
		}
		if n.MaxRetries < 0 || n.MaxRetries > 2 || n.TimeoutSeconds < 0 || n.TimeoutSeconds > maxExecutionTimeoutSeconds || n.MaxRetries > 0 && !n.Idempotent {
			return w, dagDiagnostic(fmt.Errorf("%w: retries require idempotency and 0..2 retries; timeout must be 0..9223372036 whole seconds", ErrInvalid), "dag.node.retry_timeout", nil)
		}
		for resource, v := range n.Resources {
			if !safeSchedulingInteger(v) || !identifier.MatchString(resource) || (resource == "budget" || resource == "execution_slots") && v == 0 {
				return w, dagDiagnostic(fmt.Errorf("%w: negative resource", ErrInvalid), "dag.node.resource", nil)
			}
		}
		if n.Join == "" {
			n.Join = "all_success"
		}
		switch n.Join {
		case "all_success", "all_done", "any_success", "quorum":
		default:
			return w, dagDiagnostic(fmt.Errorf("%w: unknown join %s", ErrInvalid, n.Join), "dag.node.join", diagnosticParams{"join": n.Join})
		}
		if n.Name == "" {
			n.Name = n.ID
		}
		if n.Condition != "" {
			if err := validateCondition(n.Condition); err != nil {
				return w, err
			}
		}
		for _, expression := range n.Branches {
			if expression != "default" {
				if err := validateCondition(expression); err != nil {
					return w, err
				}
			}
		}
		if n.Type == "timer" {
			if numeric(n.Config, "duration_seconds") < 0 {
				return w, dagDiagnostic(fmt.Errorf("%w: negative timer duration", ErrInvalid), "dag.timer.duration", nil)
			}
		}
		if n.Type == "subworkflow" {
			if _, bound, err := boundSubworkflowBinding(n); err != nil {
				return w, err
			} else if !bound {
				if _, _, err := subReference(n); err != nil {
					return w, err
				}
			}
		}
		if n.Type == "external_workflow" {
			plugin, _ := n.Config["plugin"].(string)
			workflow, _ := n.Config["workflow"].(string)
			if !identifier.MatchString(plugin) || workflow == "" || len(workflow) > 500 || len(n.Command) > 0 {
				return w, dagDiagnostic(fmt.Errorf("%w: external_workflow requires plugin/workflow config and no shell command", ErrInvalid), "dag.external.binding", nil)
			}
			if input, ok := n.Config["input"]; ok {
				if _, valid := input.(map[string]any); !valid {
					return w, dagDiagnostic(fmt.Errorf("%w: external workflow input must be an object", ErrInvalid), "dag.external.input", nil)
				}
			}
			if _, ok := n.Config["endpoint"]; ok {
				return w, dagDiagnostic(fmt.Errorf("%w: external endpoints must be configured by the runner administrator", ErrInvalid), "dag.external.endpoint", nil)
			}
		}
		if _, _, err := compensationTemplate(n); err != nil {
			return w, err
		}
		if n.Type == "map" {
			items, err := mapItems(n)
			if err != nil {
				return w, err
			}
			if len(items) > 1000 {
				return w, dagDiagnostic(fmt.Errorf("%w: map expansion exceeds 1000", ErrInvalid), "dag.map.expansion_limit", nil)
			}
			template, err := mapTemplate(n)
			if err != nil {
				return w, err
			}
			template.ID = "template"
			if template.Type == "map" {
				return w, dagDiagnostic(fmt.Errorf("%w: nested map", ErrUnsupported), "dag.map.nested", nil)
			}
			if _, err = normalize(Workflow{Nodes: []Node{template}}); err != nil {
				return w, err
			}
		}
		w.Nodes[i] = n
	}
	edges := []Edge{}
	pairs := map[string]bool{}
	add := func(e Edge) error {
		if _, ok := seen[e.From]; !ok {
			return dagDiagnostic(fmt.Errorf("%w: missing dependency %s", ErrInvalid, e.From), "dag.edge.source", diagnosticParams{"node": e.From})
		}
		if _, ok := seen[e.To]; !ok {
			return dagDiagnostic(fmt.Errorf("%w: missing target %s", ErrInvalid, e.To), "dag.edge.target", diagnosticParams{"node": e.To})
		}
		key := e.From + "\x00" + e.To
		if pairs[key] {
			for _, existing := range edges {
				if existing.From == e.From && existing.To == e.To && existing.Condition != e.Condition && e.Condition != "" && existing.Condition != "" {
					return dagDiagnostic(fmt.Errorf("%w: contradictory edge conditions", ErrInvalid), "dag.edge.conditions", nil)
				}
			}
			return nil
		}
		pairs[key] = true
		edges = append(edges, e)
		return nil
	}
	// Explicit edge conditions take priority over a duplicate DependsOn entry.
	for _, e := range w.Edges {
		if err := add(e); err != nil {
			return w, err
		}
	}
	for _, n := range w.Nodes {
		for _, d := range n.DependsOn {
			if err := add(Edge{From: d, To: n.ID}); err != nil {
				return w, err
			}
		}
	}
	for i := range w.Nodes {
		w.Nodes[i].DependsOn = nil
	}
	indegree := map[string]int{}
	next := map[string][]string{}
	for _, e := range edges {
		indegree[e.To]++
		next[e.From] = append(next[e.From], e.To)
		i := seen[e.To]
		w.Nodes[i].DependsOn = append(w.Nodes[i].DependsOn, e.From)
	}
	queue := []string{}
	for _, n := range w.Nodes {
		if n.Join == "quorum" && (n.Quorum < 1 || n.Quorum > len(n.DependsOn)) {
			return w, dagDiagnostic(fmt.Errorf("%w: invalid quorum", ErrInvalid), "dag.node.quorum", nil)
		}
		if indegree[n.ID] == 0 {
			queue = append(queue, n.ID)
		}
	}
	count := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		count++
		for _, id2 := range next[id] {
			indegree[id2]--
			if indegree[id2] == 0 {
				queue = append(queue, id2)
			}
		}
	}
	if count != len(w.Nodes) {
		return w, dagDiagnostic(fmt.Errorf("%w: dependency cycle", ErrInvalid), "dag.workflow.cycle", nil)
	}
	w.Edges = edges
	if err := validateCandidateReferences(w); err != nil {
		return w, err
	}
	if err := validateArtifactReferences(w); err != nil {
		return w, err
	}
	// Predicates only read ancestors. This prevents hidden data dependencies and
	// avoids evaluating a condition while its referenced task is still running.
	parents := map[string][]string{}
	for _, edge := range edges {
		parents[edge.To] = append(parents[edge.To], edge.From)
	}
	for _, n := range w.Nodes {
		expressions := predicates(n)
		hasReference := false
		for _, expression := range expressions {
			if expression != "" && expression != "default" {
				if err := validateCondition(expression); err != nil {
					return w, err
				}
				hasReference = hasReference || strings.Contains(expression, "==")
			}
		}
		if !hasReference {
			continue
		}
		ancestors := map[string]bool{}
		var visit func(string)
		visit = func(id string) {
			for _, parent := range parents[id] {
				if !ancestors[parent] {
					ancestors[parent] = true
					visit(parent)
				}
			}
		}
		visit(n.ID)
		for _, expression := range expressions {
			parts := strings.SplitN(expression, "==", 2)
			if len(parts) == 2 {
				lhs := strings.TrimSpace(parts[0])
				pos := strings.LastIndex(lhs, ".")
				if pos < 1 || !ancestors[lhs[:pos]] {
					return w, dagDiagnostic(fmt.Errorf("%w: condition references non-ancestor %q", ErrInvalid, lhs), "dag.condition.ancestor", diagnosticParams{"reference": fmt.Sprintf("%q", lhs)})
				}
			}
		}
	}
	return w, nil
}
func validateCondition(s string) error {
	s = strings.TrimSpace(s)
	if s == "true" || s == "false" {
		return nil
	}
	parts := strings.SplitN(s, "==", 2)
	if len(parts) != 2 {
		return dagDiagnostic(fmt.Errorf("%w: condition must be true, false, or node.state/output == value", ErrInvalid), "dag.condition.syntax", nil)
	}
	lhs := strings.TrimSpace(parts[0])
	if !strings.HasSuffix(lhs, ".state") && !strings.HasSuffix(lhs, ".output") {
		return dagDiagnostic(fmt.Errorf("%w: unsupported condition field", ErrInvalid), "dag.condition.field", nil)
	}
	return nil
}
