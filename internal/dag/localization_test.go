package dag

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ai-mada/suma/internal/localization"
	"github.com/ai-mada/suma/internal/toolcontract"
)

// These fixtures exercise the public compiler and its delegated validators.
// Losing an annotation must fail without changing the original machine error.
func TestCompilerDiagnosticsPreserveSourceAndLocalize(t *testing.T) {
	one := func(n Node) Workflow { return Workflow{Nodes: []Node{n}} }
	base := Node{ID: "a", Type: "task"}
	cases := []struct {
		name, key, source string
		workflow          Workflow
	}{
		{"node count", "dag.workflow.node_count", "invalid: workflow requires 1..1000 nodes", Workflow{}},
		{"priority", "dag.node.priority", "invalid: priority must be 0..9", one(Node{ID: "a", Type: "task", Priority: 10})},
		{"identity", "dag.node.identity", "invalid: node ID/type \"a b\"/\"task\"", one(Node{ID: "a b", Type: "task"})},
		{"empty identity", "dag.node.identity", "invalid: node ID/type \"\"/\"task\"", one(Node{Type: "task"})},
		{"quoted identity", "dag.node.identity", "invalid: node ID/type \"a\\\"b\"/\"task\"", one(Node{ID: "a\"b", Type: "task"})},
		{"escaped identity", "dag.node.identity", "invalid: node ID/type \"raw\\nnode\"/\"task\"", one(Node{ID: "raw\nnode", Type: "task"})},
		{"duplicate", "dag.node.duplicate", "invalid: duplicate node a", Workflow{Nodes: []Node{base, base}}},
		{"command", "dag.node.command", "invalid: command/agent/test require one allowlisted command identifier", one(Node{ID: "a", Type: "command"})},
		{"retry", "dag.node.retry_timeout", "invalid: retries require idempotency and 0..2 retries; timeout must be 0..9223372036 whole seconds", one(Node{ID: "a", Type: "task", MaxRetries: 1})},
		{"resource", "dag.node.resource", "invalid: negative resource", one(Node{ID: "a", Type: "task", Resources: map[string]int64{"cpu": -1}})},
		{"join", "dag.node.join", "invalid: unknown join unrecognized", one(Node{ID: "a", Type: "task", Join: "unrecognized"})},
		{"timer", "dag.timer.duration", "invalid: negative timer duration", one(Node{ID: "a", Type: "timer", Config: map[string]any{"duration_seconds": -1}})},
		{"external binding", "dag.external.binding", "invalid: external_workflow requires plugin/workflow config and no shell command", one(Node{ID: "a", Type: "external_workflow"})},
		{"external input", "dag.external.input", "invalid: external workflow input must be an object", one(Node{ID: "a", Type: "external_workflow", Config: map[string]any{"plugin": "plugin", "workflow": "workflow", "input": 1}})},
		{"external endpoint", "dag.external.endpoint", "invalid: external endpoints must be configured by the runner administrator", one(Node{ID: "a", Type: "external_workflow", Config: map[string]any{"plugin": "plugin", "workflow": "workflow", "endpoint": "url"}})},
		{"map items", "dag.map.items_required", "invalid: map config.items required", one(Node{ID: "a", Type: "map"})},
		{"map array", "dag.map.items_array", "invalid: map config.items must be an array", one(Node{ID: "a", Type: "map", Config: map[string]any{"items": 1}})},
		{"map limit", "dag.map.expansion_limit", "invalid: map expansion exceeds 1000", one(Node{ID: "a", Type: "map", Config: map[string]any{"items": make([]any, 1001)}})},
		{"map template", "dag.map.template", "invalid: invalid map template", one(Node{ID: "a", Type: "map", Config: map[string]any{"items": []any{}, "template": 1}})},
		{"dependency", "dag.edge.source", "invalid: missing dependency missing", Workflow{Nodes: []Node{base}, Edges: []Edge{{From: "missing", To: "a"}}}},
		{"target", "dag.edge.target", "invalid: missing target missing", Workflow{Nodes: []Node{base}, Edges: []Edge{{From: "a", To: "missing"}}}},
		{"edge conditions", "dag.edge.conditions", "invalid: contradictory edge conditions", Workflow{Nodes: []Node{base, {ID: "b", Type: "task"}}, Edges: []Edge{{From: "a", To: "b", Condition: "true"}, {From: "a", To: "b", Condition: "false"}}}},
		{"quorum", "dag.node.quorum", "invalid: invalid quorum", one(Node{ID: "a", Type: "join", Join: "quorum"})},
		{"cycle", "dag.workflow.cycle", "invalid: dependency cycle", one(Node{ID: "a", Type: "task", DependsOn: []string{"a"}})},
		{"condition syntax", "dag.condition.syntax", "invalid: condition must be true, false, or node.state/output == value", one(Node{ID: "a", Type: "task", Condition: "bad"})},
		{"condition field", "dag.condition.field", "invalid: unsupported condition field", one(Node{ID: "a", Type: "task", Condition: "a.secret == x"})},
		{"condition ancestry", "dag.condition.ancestor", "invalid: condition references non-ancestor \"missing.state\"", one(Node{ID: "a", Type: "task", Condition: "missing.state == succeeded"})},
		{"subworkflow reference", "dag.subworkflow.reference", "invalid: subworkflow requires workflow_id and positive pinned workflow_version", one(Node{ID: "a", Type: "subworkflow"})},
		{"subworkflow version", "dag.subworkflow.integer_version", "invalid: workflow_version must be an integer", one(Node{ID: "a", Type: "subworkflow", Config: map[string]any{"workflow_id": "sub", "workflow_version": 1.5}})},
		{"tool binding", "dag.tool.binding", "invalid: complete agent tool binding required", one(Node{ID: "a", Type: "task", Config: map[string]any{"tool_profile_id": "tool"}})},
		{"candidate reserved", "dag.candidate.reserved_proof", "invalid: candidate proof fields belong to scheduler assignments", one(Node{ID: "a", Type: "task", Config: map[string]any{"candidate_commit": "commit"}})},
		{"candidate consumer", "dag.candidate.consumer", "invalid: candidate_from requires an immutable build/test consumer and a source node", one(Node{ID: "a", Type: "task", Config: map[string]any{"candidate_from": "source"}})},
		{"candidate source", "dag.candidate.source", "invalid: candidate source must be a static code command or agent", one(Node{ID: "a", Type: "test", Command: []string{"test"}, Config: map[string]any{"candidate_from": "missing"}})},
		{"candidate ancestor", "dag.candidate.ancestor", "invalid: candidate source must be an explicit ancestor", Workflow{Nodes: []Node{{ID: "source", Type: "command", Command: []string{"build"}}, {ID: "a", Type: "test", Command: []string{"test"}, Config: map[string]any{"candidate_from": "source"}}}}},
		{"artifact consumer", "dag.artifact.consumer", "invalid: artifact_from requires a test and an explicit build ancestor", one(Node{ID: "a", Type: "task", Config: map[string]any{"artifact_from": "source"}})},
		{"artifact source", "dag.artifact.source", "invalid: artifact source must be a static build command", one(Node{ID: "a", Type: "test", Command: []string{"test"}, Config: map[string]any{"artifact_from": "missing"}})},
		{"artifact ancestor", "dag.artifact.ancestor", "invalid: artifact source is not an ancestor", Workflow{Nodes: []Node{{ID: "source", Type: "command", Command: []string{"build"}}, {ID: "a", Type: "test", Command: []string{"test"}, Config: map[string]any{"artifact_from": "source"}}}}},
		{"compensation worker", "dag.compensation.worker", "invalid: only worker nodes may declare compensation", one(Node{ID: "a", Type: "approval", Config: map[string]any{"compensation": map[string]any{}}})},
		{"compensation object", "dag.compensation.object", "invalid: compensation must be a node object", one(Node{ID: "a", Type: "task", Config: map[string]any{"compensation": 1}})},
		{"compensation shape", "dag.compensation.unconditional", "invalid: compensation must be an unconditional worker node; inverse ordering is generated", one(Node{ID: "a", Type: "task", Config: map[string]any{"compensation": map[string]any{"type": "approval"}}})},
		{"compensation recursive", "dag.compensation.recursive", "invalid: compensation templates cannot recursively compensate", one(Node{ID: "a", Type: "task", Config: map[string]any{"compensation": map[string]any{"type": "task", "config": map[string]any{"compensation": map[string]any{}}}}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Compile(tc.workflow)
			if err == nil || err.Error() != tc.source || !errors.Is(err, ErrInvalid) {
				t.Fatalf("machine error changed: %v", err)
			}
			for _, locale := range []localization.Locale{localization.ZhCN, localization.EnUS} {
				rendered, ok := localization.FormatError(locale, err)
				if !ok || !rendered.Localized || rendered.Key != tc.key || rendered.Locale != locale || rendered.Text == tc.source || rendered.Text == "" {
					t.Fatalf("%s diagnostic was not projected: %+v, ok=%v", locale, rendered, ok)
				}
			}
			if err.Error() != tc.source {
				t.Fatal("formatting changed original error evidence")
			}
		})
	}
}

func TestCompilerDiagnosticParameterFallbackPreservesEvidence(t *testing.T) {
	for _, workflow := range []Workflow{
		{Nodes: []Node{{ID: strings.Repeat("x", 600), Type: "task"}}},
		{Nodes: []Node{{ID: "a", Type: "task", Join: "unsafe\njoin"}}},
		{Nodes: []Node{{ID: "a", Type: "task", Join: string([]byte{0xff})}}},
	} {
		err := Compile(workflow)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("missing invalid sentinel: %v", err)
		}
		original := err.Error()
		if rendered, ok := localization.FormatError(localization.ZhCN, err); ok || rendered.Localized {
			t.Fatalf("unsafe diagnostic parameter localized: %+v", rendered)
		}
		if err.Error() != original {
			t.Fatal("fallback changed raw evidence")
		}
	}
}

func TestDAGCommandDiagnosticsKeepVersionAndRunState(t *testing.T) {
	e, run := fixture(t, Workflow{Nodes: []Node{{ID: "a", Type: "task"}}})
	for _, expected := range []int{0, -1, run.Version + 1} {
		_, err := e.Command(ctx, "t", "p", run.ID, "pause", "bad-version-"+strconv.Itoa(expected), expected, nil)
		if !errors.Is(err, ErrConflict) {
			t.Fatalf("version conflict lost: %v", err)
		}
		for _, locale := range []localization.Locale{localization.ZhCN, localization.EnUS} {
			rendered, ok := localization.FormatError(locale, err)
			if !ok || rendered.Key != "dag.run.version" || rendered.Params["expected"] != strconv.Itoa(expected) || rendered.Params["current"] != int64(run.Version) {
				t.Fatalf("version parameters changed: %+v, ok=%v", rendered, ok)
			}
		}
	}
	_, err := e.Command(ctx, "t", "p", run.ID, "odd\"action", "unknown-action", run.Version, nil)
	if err == nil || err.Error() != "invalid: unknown run command \"odd\\\"action\"" || !errors.Is(err, ErrInvalid) {
		t.Fatalf("command evidence changed: %v", err)
	}
	if rendered, ok := localization.FormatError(localization.EnUS, err); !ok || rendered.Key != "dag.run.command" || rendered.Params["action"] != "\"odd\\\"action\"" {
		t.Fatalf("quoted command not projected: %+v, ok=%v", rendered, ok)
	}
	after, err := e.GetRun(ctx, "t", "p", run.ID)
	if err != nil || !reflect.DeepEqual(run, after) {
		t.Fatalf("rejected localized commands changed run: before=%+v after=%+v err=%v", run, after, err)
	}
}

func TestDAGExternalErrorsDoNotAcquireDiagnosticTranslations(t *testing.T) {
	for _, err := range []error{errors.New("provider refused arbitrary command"), errors.New("invalid: provider failure"), errCandidatePending} {
		if rendered, ok := localization.FormatError(localization.ZhCN, err); ok || rendered.Localized {
			t.Fatalf("raw error acquired translation: %+v", rendered)
		}
	}
	// This delegated contract diagnostic is evidence from the tool boundary.
	// A DAG sentinel wrapper alone must not grant it a generic translation.
	wrapped := validateToolEvent(Node{}, toolcontract.Event{})
	if wrapped == nil || !errors.Is(wrapped, ErrInvalid) {
		t.Fatalf("delegated contract did not reject fixture: %v", wrapped)
	}
	if rendered, ok := localization.FormatError(localization.ZhCN, wrapped); ok || rendered.Localized {
		t.Fatalf("delegated contract diagnostic was relabeled: %+v", rendered)
	}
}

func TestCompilerUnsupportedNestedMapKeepsSentinel(t *testing.T) {
	err := Compile(Workflow{Nodes: []Node{{ID: "a", Type: "map", Config: map[string]any{"items": []any{}, "template": map[string]any{"type": "map"}}}}})
	if !errors.Is(err, ErrUnsupported) || err.Error() != "unsupported: nested map" {
		t.Fatalf("unsupported sentinel changed: %v", err)
	}
	if rendered, ok := localization.FormatError(localization.ZhCN, err); !ok || rendered.Key != "dag.map.nested" || rendered.Text != "不支持嵌套映射" {
		t.Fatalf("nested map was not projected: %+v, ok=%v", rendered, ok)
	}
}
