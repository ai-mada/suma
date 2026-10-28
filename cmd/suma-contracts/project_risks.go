package main

import "github.com/ai-mada/suma/internal/platform"

func projectRiskModels() []any {
	return []any{platform.ProjectRisk{}, platform.ProjectRiskFields{}, platform.ProjectRiskReference{}, platform.ProjectRiskSource{}, platform.ProjectRiskReviewInput{}, platform.ProjectRiskReview{}, platform.ProjectRiskLinkBasis{}, platform.ProjectRiskReadiness{}, platform.ProjectRiskBlocker{}, platform.ProjectRiskView{}, platform.ProjectRiskPage{}, platform.ProjectRiskSummary{}, platform.ProjectRiskMatrixCell{}, platform.ProjectRiskCreateRequest{}, platform.ProjectRiskSaveRequest{}, platform.ProjectRiskCommandRequest{}}
}

// Risks share project authority and original receipts. They cannot issue tools,
// change task acceptance or turn model advice into an approval.
func projectRiskContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/risks", "get", "", "ProjectRiskPage", "200"},
		{"/risks", "post", "ProjectRiskCreateRequest", "ProjectRiskView", "201"},
		{"/risks/summary", "get", "", "ProjectRiskSummary", "200"},
		{"/risks/{id}", "get", "", "ProjectRiskView", "200"},
		{"/risks/{id}", "patch", "ProjectRiskSaveRequest", "ProjectRiskView", "200"},
		{"/risks/{id}/commands", "post", "ProjectRiskCommandRequest", "ProjectRiskView", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.path == "/risks" && route.method == "post" {
			op["responses"].(map[string]any)["200"] = map[string]any{"description": "Original receipt recovery or an explicitly already-registered source; a different form is not applied.", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{"$ref": "#/components/schemas/ProjectRiskView"}}}}
		}
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Formal current-project risk aggregate, separate from model planning advice. Current project authorization is checked before original-operation receipt recovery. Code mitigation requires current formal task acceptance and its existing quality evidence. Closure and reopen retain review history and never change task/run/test/release state."
		if route.input != "" {
			request(op, route.input)
			browserOnly(op)
		} else {
			op["x-read-only"] = true
		}
		if route.path == "/risks/{id}/commands" {
			op["x-action-authorities"] = map[string]string{"start": "current project write", "submit_review": "current project write", "return": "current browser project approval", "close": "current browser project approval", "reopen": "current browser project approval"}
		}
		if route.path == "/risks" && route.method == "get" {
			appendQuery(op, queryParameter("state", "Optional exact lifecycle state.", map[string]any{"type": "string", "enum": []string{"open", "mitigating", "in_review", "closed"}}))
			appendQuery(op, queryParameter("owner_id", "Optional exact human resource ID in the current project.", map[string]any{"type": "string", "maxLength": 64}))
			appendQuery(op, queryParameter("limit", "Default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Last loaded risk ID; current project and state permissions are checked on every page.", map[string]any{"type": "string"}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ProjectRisk", "ProjectRiskFields", "ProjectRiskReference", "ProjectRiskSource", "ProjectRiskReviewInput", "ProjectRiskReview", "ProjectRiskLinkBasis", "ProjectRiskReadiness", "ProjectRiskBlocker", "ProjectRiskView", "ProjectRiskPage", "ProjectRiskSummary", "ProjectRiskMatrixCell", "ProjectRiskCreateRequest", "ProjectRiskSaveRequest", "ProjectRiskCommandRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for k, v := range attrs {
			value[k] = v
		}
	}
	for _, name := range []string{"ProjectRisk", "ProjectRiskFields", "ProjectRiskCreateRequest", "ProjectRiskSaveRequest"} {
		field(name, "title", map[string]any{"minLength": 1, "maxLength": 500, "x-max-utf8-bytes": 500})
		field(name, "strategy", map[string]any{"enum": []string{"", "mitigate", "avoid", "transfer", "accept"}})
		field(name, "links", map[string]any{"maxItems": 32})
		for _, score := range []string{"probability", "impact"} {
			field(name, score, map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 1, "maximum": 5}, map[string]any{"type": "null"}}})
		}
	}
	for _, name := range []string{"ProjectRiskCreateRequest", "ProjectRiskSaveRequest", "ProjectRiskCommandRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
		field(name, "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	}
	field("ProjectRisk", "state", map[string]any{"enum": []string{"open", "mitigating", "in_review", "closed"}})
	field("ProjectRiskReference", "kind", map[string]any{"enum": []string{"task", "requirement", "issue"}})
	field("ProjectRiskCommandRequest", "action", map[string]any{"enum": []string{"start", "submit_review", "return", "close", "reopen"}})
	for _, name := range []string{"ProjectRiskReviewInput", "ProjectRiskReview"} {
		field(name, "disposition", map[string]any{"enum": []string{"mitigated", "accepted", "avoided", "transferred", "not_applicable"}})
		for _, score := range []string{"residual_probability", "residual_impact"} {
			field(name, score, map[string]any{"anyOf": []any{map[string]any{"type": "integer", "minimum": 1, "maximum": 5}, map[string]any{"type": "null"}}})
		}
	}
	field("ProjectRiskView", "history", map[string]any{"maxItems": 10, "description": "Bounded most-recent review history; history_total/history_complete disclose omitted records. This window never determines closure eligibility."})
	field("ProjectRiskSummary", "matrix_scope", map[string]any{"const": "active"})
	field("ProjectRiskSummary", "matrix", map[string]any{"minItems": 25, "maxItems": 25, "description": "Full authorized active-risk ordinal matrix. Unassessed risks are counted separately, never placed in a zero cell."})
	toolContractMap(schemas, "ProjectRiskCommandRequest")["allOf"] = []any{
		map[string]any{"if": map[string]any{"properties": map[string]any{"action": map[string]any{"const": "submit_review"}}}, "then": map[string]any{"required": []string{"review"}}},
		map[string]any{"if": map[string]any{"properties": map[string]any{"action": map[string]any{"enum": []string{"close", "return"}}}}, "then": map[string]any{"required": []string{"expected_review_id"}}},
	}
}
