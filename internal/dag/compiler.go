package dag

import (
	"fmt"
	"regexp"
	"strings"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
var types = map[string]bool{"task": true, "command": true, "agent": true, "script": true, "test": true, "approval": true, "timer": true, "condition": true, "map": true, "join": true, "noop": true, "subworkflow": true, "external_workflow": true}

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
		return w, fmt.Errorf("%w: workflow requires 1..1000 nodes", ErrInvalid)
	}
	seen := map[string]int{}
	for i, n := range w.Nodes {
		if !identifier.MatchString(n.ID) || !types[n.Type] {
			return w, fmt.Errorf("%w: node ID/type %q/%q", ErrInvalid, n.ID, n.Type)
		}
		if _, ok := seen[n.ID]; ok {
			return w, fmt.Errorf("%w: duplicate node %s", ErrInvalid, n.ID)
		}
		seen[n.ID] = i
		if n.MaxRetries < 0 || n.TimeoutSeconds < 0 || n.MaxRetries > 0 && !n.Idempotent {
			return w, fmt.Errorf("%w: retries require idempotency", ErrInvalid)
		}
		for resource, v := range n.Resources {
			if v < 0 || !identifier.MatchString(resource) {
				return w, fmt.Errorf("%w: negative resource", ErrInvalid)
			}
		}
		if n.Join == "" {
			n.Join = "all_success"
		}
		switch n.Join {
		case "all_success", "all_done", "any_success", "quorum":
		default:
			return w, fmt.Errorf("%w: unknown join %s", ErrInvalid, n.Join)
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
				return w, fmt.Errorf("%w: negative timer duration", ErrInvalid)
			}
		}
		if n.Type == "subworkflow" {
			if _, _, err := subReference(n); err != nil {
				return w, err
			}
		}
		if n.Type == "external_workflow" {
			plugin, _ := n.Config["plugin"].(string)
			workflow, _ := n.Config["workflow"].(string)
			if !identifier.MatchString(plugin) || workflow == "" || len(workflow) > 500 || len(n.Command) > 0 {
				return w, fmt.Errorf("%w: external_workflow requires plugin/workflow config and no shell command", ErrInvalid)
			}
			if input, ok := n.Config["input"]; ok {
				if _, valid := input.(map[string]any); !valid {
					return w, fmt.Errorf("%w: external workflow input must be an object", ErrInvalid)
				}
			}
			if _, ok := n.Config["endpoint"]; ok {
				return w, fmt.Errorf("%w: external endpoints must be configured by the runner administrator", ErrInvalid)
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
				return w, fmt.Errorf("%w: map expansion exceeds 1000", ErrInvalid)
			}
			template, err := mapTemplate(n)
			if err != nil {
				return w, err
			}
			template.ID = "template"
			if template.Type == "map" {
				return w, fmt.Errorf("%w: nested map", ErrUnsupported)
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
			return fmt.Errorf("%w: missing dependency %s", ErrInvalid, e.From)
		}
		if _, ok := seen[e.To]; !ok {
			return fmt.Errorf("%w: missing target %s", ErrInvalid, e.To)
		}
		key := e.From + "\x00" + e.To
		if pairs[key] {
			for _, existing := range edges {
				if existing.From == e.From && existing.To == e.To && existing.Condition != e.Condition && e.Condition != "" && existing.Condition != "" {
					return fmt.Errorf("%w: contradictory edge conditions", ErrInvalid)
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
			return w, fmt.Errorf("%w: invalid quorum", ErrInvalid)
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
		return w, fmt.Errorf("%w: dependency cycle", ErrInvalid)
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
					return w, fmt.Errorf("%w: condition references non-ancestor %q", ErrInvalid, lhs)
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
		return fmt.Errorf("%w: condition must be true, false, or node.state/output == value", ErrInvalid)
	}
	lhs := strings.TrimSpace(parts[0])
	if !strings.HasSuffix(lhs, ".state") && !strings.HasSuffix(lhs, ".output") {
		return fmt.Errorf("%w: unsupported condition field", ErrInvalid)
	}
	return nil
}
