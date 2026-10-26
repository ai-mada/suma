package main

import (
	ccmp "github.com/ai-mada/suma/internal/candidatecompositioncontract"
	"github.com/ai-mada/suma/internal/platform"
)

func candidateCompositionModels() []any {
	return []any{platform.CandidateCompositionSourceRequest{}, platform.CandidateCompositionCreateRequest{}, platform.CandidateCompositionStartRequest{}, platform.CandidateCompositionCommandRequest{}, platform.CandidateCompositionPlan{}, platform.CandidateCompositionPage{}, platform.CandidateCompositionSourceChoice{}, platform.CandidateCompositionOptions{}, platform.TaskCodeBaselineBindRequest{}, platform.CandidateCompositionBaseline{}, ccmp.CandidateCompositionExecution{}, ccmp.CandidateCompositionEvidence{}, ccmp.CodeBaselineBinding{}, ccmp.CodeInputRequest{}}
}

func candidateCompositionContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/candidate-compositions/options", "get", "", "CandidateCompositionOptions", "200"},
		{"/candidate-compositions", "get", "", "CandidateCompositionPage", "200"},
		{"/candidate-compositions", "post", "CandidateCompositionCreateRequest", "CandidateCompositionPlan", "201"},
		{"/candidate-compositions/{id}", "get", "", "CandidateCompositionPlan", "200"},
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
	for _, name := range []string{"CandidateCompositionSourceRequest", "CandidateCompositionCreateRequest", "CandidateCompositionStartRequest", "CandidateCompositionCommandRequest", "TaskCodeBaselineBindRequest", "CodeInputRequest"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	field("CandidateCompositionCreateRequest", "sources", map[string]any{"minItems": 2, "maxItems": 16})
	field("CandidateCompositionPlan", "strategy", map[string]any{"const": "merge_commit"})
	field("CandidateCompositionExecution", "protocol", map[string]any{"const": ccmp.CandidateCompositionProtocol})
	field("CandidateCompositionEvidence", "protocol", map[string]any{"const": ccmp.CandidateCompositionProtocol})
	field("CodeBaselineBinding", "protocol", map[string]any{"const": ccmp.CodeBaselineProtocol})
	field("CodeInputRequest", "input_kind", map[string]any{"enum": []string{"composition", "code_baseline"}})
	field("CandidateCompositionCommandRequest", "action", map[string]any{"const": "dismiss"})
	for _, name := range []string{"CandidateCompositionStartRequest", "TaskCodeBaselineBindRequest"} {
		field(name, "acknowledge", map[string]any{"const": true})
	}
	for _, name := range []string{"CandidateCompositionCreateRequest", "CandidateCompositionStartRequest", "CandidateCompositionCommandRequest", "TaskCodeBaselineBindRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("TaskCodeBaselineBindRequest", "expected_binding_version", map[string]any{"minimum": 0, "maximum": 2147483646})
	for _, pair := range [][2]string{{"CandidateCompositionSourceRequest", "expected_task_version"}, {"CandidateCompositionCreateRequest", "expected_task_version"}, {"CandidateCompositionCreateRequest", "workflow_version"}, {"CandidateCompositionStartRequest", "expected_version"}, {"CandidateCompositionCommandRequest", "expected_version"}, {"TaskCodeBaselineBindRequest", "expected_task_version"}, {"TaskCodeBaselineBindRequest", "expected_composition_version"}} {
		field(pair[0], pair[1], map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("Record", "code_baseline", map[string]any{"readOnly": true, "description": "Protected task-domain input; generic record/data writes cannot manufacture or replace this binding."})
}
