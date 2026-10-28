package main

import (
	mc "github.com/ai-mada/suma/internal/modelcontract"
	"github.com/ai-mada/suma/internal/platform"
)

func dispatchRecommendationModels() []any {
	return []any{platform.DispatchRecommendation{}, platform.DispatchRecommendationChoice{}, platform.DispatchRecommendationProposal{}, platform.DispatchRecommendationSource{}, platform.DispatchRecommendationCandidate{}, platform.DispatchRecommendationCoverage{}, platform.DispatchRecommendationConstraints{}, platform.DispatchRecommendationOptionsRequest{}, platform.DispatchRecommendationCreateRequest{}, platform.DispatchRecommendationAdoptRequest{}, platform.DispatchRecommendationAdoption{}, platform.DispatchRecommendationPage{}, platform.DispatchRecommendationProvenance{}}
}

func dispatchRecommendationContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/dispatch-recommendations/options", "post", "DispatchRecommendationOptionsRequest", "DispatchRecommendationSource", "200"},
		{"/dispatch-recommendations", "post", "DispatchRecommendationCreateRequest", "DispatchRecommendation", "201"},
		{"/dispatch-recommendations", "get", "", "DispatchRecommendationPage", "200"},
		{"/dispatch-recommendations/{id}", "get", "", "DispatchRecommendation", "200"},
		{"/dispatch-recommendations/{id}/adopt", "post", "DispatchRecommendationAdoptRequest", "DispatchRecommendationAdoption", "201"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current authorized task/context and bounded real tool/model alternatives. One read-only analysis uses the same Go ModelBinding/gateway and existing DAG. Editable human adoption prepares the original task dispatch plan; target execution still requires the original explicit start. No autonomous dispatch, approval, fabricated price or performance ranking. Current reader authorization precedes historical payload and receipt recovery; changed source or revoked authority prevents adoption."
		if route.input != "" {
			request(op, route.input)
			browserOnly(op)
		} else {
			op["x-read-only"] = true
		}
		if route.path == "/dispatch-recommendations/options" {
			op["x-read-only"] = true
			op["x-side-effects"] = "No reservation, tool probe, model request or run creation"
		}
		if route.method == "get" && route.path == "/dispatch-recommendations" {
			appendQuery(op, queryParameter("task_id", "Optional exact task ID in this project.", map[string]any{"type": "string"}))
			appendQuery(op, queryParameter("limit", "Bounded page; default20.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Original project-authorized page cursor.", map[string]any{"type": "string", "maxLength": 64}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"DispatchRecommendation", "DispatchRecommendationChoice", "DispatchRecommendationProposal", "DispatchRecommendationSource", "DispatchRecommendationCandidate", "DispatchRecommendationCoverage", "DispatchRecommendationConstraints", "DispatchRecommendationOptionsRequest", "DispatchRecommendationCreateRequest", "DispatchRecommendationAdoptRequest", "DispatchRecommendationAdoption", "DispatchRecommendationPage", "DispatchRecommendationProvenance"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, attrs map[string]any) {
		v := toolContractMap(schemas, model, "properties", name)
		for k, a := range attrs {
			v[k] = a
		}
	}
	for _, name := range []string{"DispatchRecommendationOptionsRequest", "DispatchRecommendationCreateRequest"} {
		field(name, "expected_task_version", map[string]any{"minimum": 1})
		field(name, "workflow_version", map[string]any{"minimum": 1})
		field(name, "knowledge", map[string]any{"maxItems": 16})
	}
	field("DispatchRecommendationConstraints", "max_steps", map[string]any{"minimum": 1, "maximum": mc.MaxSteps, "default": 8})
	field("DispatchRecommendationConstraints", "max_output_tokens", map[string]any{"minimum": 1, "maximum": mc.MaxOutputTokens, "default": 4096})
	field("DispatchRecommendationCandidate", "kind", map[string]any{"enum": []string{"model", "codex", "fixed"}})
	field("DispatchRecommendationSource", "candidates", map[string]any{"maxItems": 24})
	field("DispatchRecommendation", "state", map[string]any{"enum": []string{"running", "reconciling", "failed", "ready", "adopted"}})
	field("DispatchRecommendation", "allowed_actions", map[string]any{"items": map[string]any{"type": "string", "enum": []string{"adopt"}}})
	field("DispatchRecommendationCreateRequest", "goal", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("DispatchRecommendationCreateRequest", "locale", map[string]any{"enum": []string{"zh-CN", "en-US"}})
	field("DispatchRecommendationCreateRequest", "acknowledge_model_use", map[string]any{"const": true})
	field("DispatchRecommendationCreateRequest", "expected_analysis_tool_version", map[string]any{"minimum": 1})
	for _, name := range []string{"DispatchRecommendationCreateRequest", "DispatchRecommendationAdoptRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("DispatchRecommendationAdoptRequest", "expected_version", map[string]any{"minimum": 1})
	field("DispatchRecommendationProposal", "schema_version", map[string]any{"const": 1})
	field("DispatchRecommendationProposal", "recommendations", map[string]any{"minItems": 1, "maxItems": 3})
	field("DispatchRecommendationProposal", "questions", map[string]any{"maxItems": 5})
	field("DispatchRecommendationChoice", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("DispatchRecommendationProvenance", "choice_sha256", map[string]any{"readOnly": true, "pattern": "^[a-f0-9]{64}$", "description": "SHA256 of the adopted five-field choice encoded by Go encoding/json, including explicit nulls; independent of browser JSON key order."})
}
