package platform

import (
	"testing"

	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/dag"
)

func TestCandidateCompositionWorkflowSupportsRootCommandAndAgent(t *testing.T) {
	for _, tc := range []struct {
		name, kind, condition, capability string
		depends                           []string
		allowed                           bool
	}{
		{name: "command", kind: "command", allowed: true},
		{name: "agent", kind: "agent", allowed: true},
		{name: "command_fixed_capability", kind: "command", capability: "command:" + ccmp.CompositionCommand, allowed: true},
		{name: "test_is_not_a_producer", kind: "test"},
		{name: "nonroot", kind: "command", depends: []string{"precondition"}},
		{name: "conditional", kind: "agent", condition: "true"},
		{name: "extra_capability", kind: "command", capability: "environment:production"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := map[string]any{"goal": "Original administrator-fixed instructions"}
			if tc.capability != "" {
				config["capability"] = tc.capability
			}
			source := dag.Workflow{ID: "composition-template", Name: "Fixed code composition", Version: 1, Nodes: []dag.Node{
				{ID: "produce", Type: tc.kind, Command: []string{"original-command"}, DependsOn: tc.depends, Condition: tc.condition, Config: config, MaxRetries: 2, Idempotent: true},
				{ID: "quality", Type: "test", Command: []string{"administrator-fixed-test"}, DependsOn: []string{"produce"}, Config: map[string]any{"candidate_from": "produce", "fixed": "quality-contract"}},
			}}
			original := stringify(source)
			derived, err := buildCandidateCompositionWorkflow(source, "produce")
			if !tc.allowed {
				if err == nil {
					t.Fatal("unsupported producer was accepted", derived)
				}
				return
			}
			if err != nil {
				t.Fatal("valid code-producing root was rejected", err)
			}
			if stringify(source) != original {
				t.Fatal("published workflow was mutated")
			}
			producer := derived.Nodes[0]
			if producer.Type != "command" || len(producer.Command) != 1 || producer.Command[0] != ccmp.CompositionCommand || producer.Config["purpose"] != "candidate_composition" || producer.MaxRetries != 0 || producer.Idempotent {
				t.Fatal("producer was not replaced by the bounded trusted command", producer)
			}
			if stringify(derived.Nodes[1]) != stringify(source.Nodes[1]) {
				t.Fatal("independent quality contract changed")
			}
			if err = dag.Compile(derived); err != nil {
				t.Fatal(err)
			}
		})
	}
}
