package main

import "github.com/ai-mada/suma/internal/platform"

func intelligentPlanningModels() []any {
	return []any{platform.PlanningAnalysis{}, platform.PlanningAnalysisCreateRequest{}, platform.PlanningAnalysisCommandRequest{}, platform.PlanningAnalysisPage{}, platform.PlanningSource{}, platform.PlanningSourceTask{}, platform.PlanningOutput{}, platform.PlanningScheduleAction{}, platform.PlanningRisk{}, platform.PlanningAlert{}, platform.PlanningAlertPage{}, platform.PlanningAlertCommandRequest{}}
}

func intelligentPlanningContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/planning/analyses", "get", "", "PlanningAnalysisPage", "200"},
		{"/planning/analyses", "post", "PlanningAnalysisCreateRequest", "PlanningAnalysis", "201"},
		{"/planning/analyses/{id}", "get", "", "PlanningAnalysis", "200"},
		{"/planning/analyses/{id}/commands", "post", "PlanningAnalysisCommandRequest", "PlanningAnalysis", "200"},
		{"/planning/alerts", "get", "", "PlanningAlertPage", "200"},
		{"/planning/alerts/{id}/commands", "post", "PlanningAlertCommandRequest", "PlanningAlert", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current project authority precedes reads, writes and operation replay. A bounded real model run produces a reviewable schedule proposal and model risk suggestions. Source and original output are immutable; applying a revision requires current approval, explicit acknowledgement and same-transaction source revalidation. Risk alerts are durable in-app suggestions, not confirmed incidents or permission to send external messages."
		if route.input != "" {
			request(op, route.input)
		} else if route.output != "PlanningAnalysis" {
			appendQuery(op, queryParameter("limit", "Exact decimal integer page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Existing ID in this tenant/project and filter; stable created_at/id descending keyset.", map[string]any{"type": "string", "maxLength": 64}))
			if route.output == "PlanningAlertPage" {
				appendQuery(op, queryParameter("state", "Optional alert state.", map[string]any{"type": "string", "enum": []string{"open", "acknowledged", "resolved"}}))
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		for key, value := range attrs {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, model := range []string{"PlanningAnalysisCreateRequest", "PlanningAnalysisCommandRequest", "PlanningAlertCommandRequest", "PlanningOutput", "PlanningScheduleAction", "PlanningRisk"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"PlanningAnalysisCreateRequest", "PlanningAnalysisCommandRequest", "PlanningAlertCommandRequest"} {
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("PlanningCapability", "protocol", map[string]any{"const": "suma-schedule-plan-v1"})
	field("PlanningCapability", "model", map[string]any{"minLength": 1, "maxLength": 256, "x-max-utf8-bytes": 256})
	field("PlanningCapability", "endpoint_origin", map[string]any{"minLength": 1, "maxLength": 2048, "description": "Fixed HTTPS origin or loopback HTTP origin, without path, query, fragment or credentials. Not model account health."})
	toolContractMap(schemas, "PlanningCapability")["additionalProperties"] = false
	field("PlanningAnalysisCreateRequest", "goal", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("PlanningAnalysisCreateRequest", "locale", map[string]any{"enum": []string{"zh-CN", "en-US"}})
	field("PlanningAnalysisCreateRequest", "task_ids", map[string]any{"minItems": 1, "maxItems": 20, "uniqueItems": true})
	field("PlanningAnalysisCreateRequest", "expected_tool_version", map[string]any{"minimum": 1})
	field("PlanningAnalysisCreateRequest", "max_steps", map[string]any{"minimum": 1, "maximum": 4, "default": 3})
	field("PlanningAnalysisCreateRequest", "max_output_tokens", map[string]any{"minimum": 1, "maximum": 8192, "default": 4096})
	field("PlanningAnalysisCreateRequest", "acknowledge_model_use", map[string]any{"const": true})
	toolContractMap(schemas, "PlanningAnalysisCreateRequest")["required"] = []string{"project_id", "goal", "locale", "task_ids", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key"}
	for _, model := range []string{"PlanningAnalysisCommandRequest", "PlanningAlertCommandRequest"} {
		field(model, "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
		field(model, "reason", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	}
	toolContractMap(schemas, "PlanningAnalysisCommandRequest")["required"] = []string{"project_id", "expected_version", "action", "reason", "idempotency_key"}
	field("PlanningAnalysisCommandRequest", "action", map[string]any{"enum": []string{"revise", "apply", "dismiss"}})
	field("PlanningAlertCommandRequest", "action", map[string]any{"enum": []string{"acknowledge", "resolve"}})
	field("PlanningAnalysis", "state", map[string]any{"enum": []string{"running", "ready", "failed", "reconciling", "applied", "dismissed"}})
	field("PlanningAlert", "state", map[string]any{"enum": []string{"open", "acknowledged", "resolved"}})
	field("PlanningOutput", "schema_version", map[string]any{"const": 1})
	field("PlanningOutput", "summary", map[string]any{"minLength": 1, "maxLength": 2000})
	field("PlanningOutput", "rationale", map[string]any{"minLength": 1, "maxLength": 8000})
	field("PlanningOutput", "actions", map[string]any{"minItems": 1, "maxItems": 20})
	field("PlanningOutput", "risks", map[string]any{"maxItems": 20})
	field("PlanningRisk", "severity", map[string]any{"enum": []string{"info", "warning", "critical"}})
	for _, model := range []string{"PlanningAnalysis", "PlanningAnalysisPage", "PlanningSource", "PlanningSourceTask", "PlanningAlert", "PlanningAlertPage"} {
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
}
