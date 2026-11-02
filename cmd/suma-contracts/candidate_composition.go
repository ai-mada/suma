package main

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/platform"
)

func candidateCompositionModels() []any {
	return []any{platform.CandidateCompositionConflictBasis{}, platform.CandidateCompositionConflictContext{}, platform.CandidateCompositionRepairDraft{}, platform.CandidateCompositionRepairCreateRequest{}, ccmp.CandidateConflictStage{}, ccmp.CandidateConflictRepairExecution{}, ccmp.CandidateConflictRepairStep{}, ccmp.CandidateConflictRepairEvidence{}, platform.CandidateCompositionSourceRequest{}, platform.CandidateCompositionCreateRequest{}, platform.CandidateCompositionStartRequest{}, platform.CandidateCompositionCommandRequest{}, platform.CandidateCompositionPlan{}, platform.CandidateCompositionPage{}, platform.CandidateCompositionSourceChoice{}, platform.CandidateCompositionOptions{}, platform.TaskCodeBaselineBindRequest{}, platform.CandidateCompositionBaseline{}, ccmp.CandidateCompositionExecution{}, ccmp.CandidateCompositionEvidence{}, ccmp.CodeBaselineBinding{}, ccmp.CodeInputRequest{}}
}

func candidateCompositionContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/candidate-compositions/options", "get", "", "CandidateCompositionOptions", "200"},
		{"/candidate-compositions", "get", "", "CandidateCompositionPage", "200"},
		{"/candidate-compositions", "post", "CandidateCompositionCreateRequest", "CandidateCompositionPlan", "201"},
		{"/candidate-compositions/{id}", "get", "", "CandidateCompositionPlan", "200"},
		{"/candidate-compositions/{id}/conflict", "get", "", "CandidateCompositionConflictContext", "200"},
		{"/candidate-compositions/{id}/repairs", "post", "CandidateCompositionRepairCreateRequest", "CandidateCompositionPlan", "201"},
		{"/candidate-compositions/{id}/start", "post", "CandidateCompositionStartRequest", "CandidateCompositionPlan", "200"},
		{"/candidate-compositions/{id}/commands", "post", "CandidateCompositionCommandRequest", "CandidateCompositionPlan", "200"},
		{"/tasks/{id}/code-baseline", "get", "", "CandidateCompositionBaseline", "200"},
		{"/tasks/{id}/code-baseline", "post", "TaskCodeBaselineBindRequest", "Record", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
		}
		op["description"] = "Current-scoped fixed accepted sources -> explicit managed composition task -> independent tests of the exact composed candidate -> original human task review. Protected development baseline binding is explicit and versioned. No automatic acceptance, target mutation, latest-source selection or unknown execution retry."
		if route.method == "get" && (route.path == "/candidate-compositions/options" || route.path == "/candidate-compositions") {
			query := queryParameter("task_id", "Integration task filter; options requires an explicit current task.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
			query["required"] = route.path == "/candidate-compositions/options"
			appendQuery(op, query)
			if route.path == "/candidate-compositions" {
				appendQuery(op, queryParameter("limit", "Bounded canonical decimal page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
				appendQuery(op, queryParameter("before", "Opaque project/task-bound history cursor.", map[string]any{"type": "string", "maxLength": 4096}))
			}
		}
	}
	for _, path := range []string{"/runner/code-inputs/bundle", "/pool-runner/code-inputs/bundle"} {
		op := add(path, "post", "", false, true)
		request(op, "CodeInputRequest")
		successStatus(op, "200")
		op["description"] = "Authenticated current full consumer assignment only. Read exact frozen composition/baseline input membership; reauthorize around immutable blob IO. Pool identity is retained in assignment, never converted into human authority."
		toolContractMap(op, "responses", "200")["content"] = map[string]any{"application/octet-stream": map[string]any{"schema": map[string]any{"type": "string", "format": "binary", "maxLength": 64 << 20}}}
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(api, "components", "schemas", model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, name := range []string{"CandidateCompositionSourceRequest", "CandidateCompositionCreateRequest", "CandidateCompositionStartRequest", "CandidateCompositionCommandRequest", "TaskCodeBaselineBindRequest", "CodeInputRequest", "CandidateCompositionRepairCreateRequest", "CandidateCompositionRepairDraft"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	field("CandidateCompositionCreateRequest", "sources", map[string]any{"minItems": 2, "maxItems": 16})
	field("CandidateCompositionPlan", "strategy", map[string]any{"enum": []string{"merge_commit", "conflict_repair"}})
	// A repair is a separate plan with its own frozen conflict and tool identity.
	// Keep this discriminant identical to the domain and public UI wire guard.
	toolContractMap(api, "components", "schemas", "CandidateCompositionPlan")["allOf"] = []any{map[string]any{
		"if":   map[string]any{"required": []string{"conflict_basis"}, "properties": map[string]any{"conflict_basis": map[string]any{"type": "object"}}},
		"then": map[string]any{"required": []string{"tool_profile_id", "tool_profile_version", "tool_profile_sha256"}, "properties": map[string]any{"strategy": map[string]any{"const": "conflict_repair"}}},
		"else": map[string]any{"properties": map[string]any{"strategy": map[string]any{"const": "merge_commit"}}},
	}}
	field("CandidateCompositionExecution", "protocol", map[string]any{"const": ccmp.CandidateCompositionProtocol})
	field("CandidateCompositionEvidence", "protocol", map[string]any{"const": ccmp.CandidateCompositionProtocol})
	field("CodeBaselineBinding", "protocol", map[string]any{"const": ccmp.CodeBaselineProtocol})
	field("CodeInputRequest", "input_kind", map[string]any{"enum": []string{"composition", "code_baseline", "conflict_repair", "development_continuation"}})
	field("CandidateConflictRepairExecution", "protocol", map[string]any{"const": ccmp.CandidateConflictRepairProtocol})
	field("CandidateConflictRepairEvidence", "protocol", map[string]any{"const": ccmp.CandidateConflictRepairProtocol})
	field("CandidateCompositionRepairCreateRequest", "expected_conflict_sha256", map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"})
	field("CandidateCompositionRepairDraft", "acceptance_criteria", map[string]any{"minItems": 1, "maxItems": 16, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000}})
	field("CandidateCompositionRepairDraft", "title", map[string]any{"minLength": 1, "maxLength": 200})
	field("CandidateCompositionRepairDraft", "description", map[string]any{"minLength": 1, "maxLength": 16000})
	field("CandidateCompositionRepairDraft", "priority", map[string]any{"enum": []string{"low", "medium", "high"}, "default": "medium"})
	field("CandidateCompositionCommandRequest", "action", map[string]any{"enum": []string{"dismiss", "refresh"}})
	for _, name := range []string{"CandidateCompositionStartRequest", "TaskCodeBaselineBindRequest"} {
		field(name, "acknowledge", map[string]any{"const": true})
	}
	for _, name := range []string{"CandidateCompositionCreateRequest", "CandidateCompositionStartRequest", "CandidateCompositionCommandRequest", "TaskCodeBaselineBindRequest", "CandidateCompositionRepairCreateRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, name := range []string{"expected_composition_version", "workflow_version", "expected_tool_version"} {
		field("CandidateCompositionRepairCreateRequest", name, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("TaskCodeBaselineBindRequest", "expected_binding_version", map[string]any{"minimum": 0, "maximum": 2147483646})
	field("TaskCodeBaselineBindRequest", "expected_terminal_candidate_plan_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	for _, model := range []string{"CodeBaselineBinding", "TaskCodeBaselineBindRequest"} {
		compositionVersion, terminalVersion, sourceVersion := "composition_version", "terminal_candidate_plan_version", "source_task_version"
		if model == "TaskCodeBaselineBindRequest" {
			compositionVersion, terminalVersion, sourceVersion = "expected_composition_version", "expected_terminal_candidate_plan_version", "expected_source_task_version"
		}
		groups := [][]string{{"composition_id", compositionVersion}, {"terminal_candidate_plan_id", terminalVersion}, {"source_task_id", sourceVersion}}
		if model == "TaskCodeBaselineBindRequest" {
			groups[2] = append(groups[2], "source_node_id")
		}
		variants := []any{}
		for index, required := range groups {
			forbidden := []any{}
			for other, fields := range groups {
				if other != index {
					for _, name := range fields {
						forbidden = append(forbidden, map[string]any{"required": []string{name}})
					}
				}
			}
			variants = append(variants, map[string]any{"required": required, "not": map[string]any{"anyOf": forbidden}})
		}
		toolContractMap(api, "components", "schemas", model)["oneOf"] = variants
		field(model, sourceVersion, map[string]any{"minimum": 1, "maximum": 2147483646})
	}

	for _, pair := range [][2]string{{"CandidateCompositionSourceRequest", "expected_task_version"}, {"CandidateCompositionCreateRequest", "expected_task_version"}, {"CandidateCompositionCreateRequest", "workflow_version"}, {"CandidateCompositionStartRequest", "expected_version"}, {"CandidateCompositionCommandRequest", "expected_version"}, {"TaskCodeBaselineBindRequest", "expected_task_version"}, {"TaskCodeBaselineBindRequest", "expected_composition_version"}} {
		field(pair[0], pair[1], map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("Record", "code_baseline", map[string]any{"readOnly": true, "description": "Protected task-domain input; generic record/data writes cannot manufacture or replace this binding."})
}
