package main

import "github.com/ai-mada/suma/internal/platform"

func issueRepairModels() []any {
	return []any{platform.IssueRepair{}, platform.IssueRegressionBinding{}, platform.IssueRepairClosure{}, platform.IssueRepairBlocker{}, platform.IssueRepairView{}, platform.IssueRepairPrepareRequest{}, platform.IssueRepairBindRequest{}, platform.IssueRepairCloseRequest{}, platform.IssueRepairReopenRequest{}}
}

func issueRepairContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, status string }{
		{"/issues/{id}/repair", "get", "", "200"},
		{"/issues/{id}/repair/prepare", "post", "IssueRepairPrepareRequest", "201"},
		{"/issues/{id}/repair/regression", "post", "IssueRepairBindRequest", "200"},
		{"/issues/{id}/repair/close", "post", "IssueRepairCloseRequest", "200"},
		{"/issues/{id}/repair/reopen", "post", "IssueRepairReopenRequest", "200"},
	} {
		op := add(route.path, route.method, "IssueRepairView", false, route.input != "")
		successStatus(op, route.status)
		op["description"] = "Current protected issue repair cycle. Reuses ordinary tasks, the single Go scheduler, immutable regression publications and current formal task approval. Mutable record data never proves a repair. Exact original operations replay only after current authorization."
		op["x-disallowed-token-scopes"] = []string{"agent", "runner"}
		op["x-required-project-authority"] = "current project read"
		if route.input == "" {
			op["x-read-only"] = true
		} else {
			request(op, route.input)
			op["x-required-project-authority"] = "current project write"
		}
		if route.input == "IssueRepairCloseRequest" || route.input == "IssueRepairReopenRequest" {
			browserOnly(op)
			op["x-required-project-authority"] = "current browser project approval"
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"IssueRepair", "IssueRegressionBinding", "IssueRepairClosure", "IssueRepairBlocker", "IssueRepairView", "IssueRepairPrepareRequest", "IssueRepairBindRequest", "IssueRepairCloseRequest", "IssueRepairReopenRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, attributes map[string]any) {
		for key, value := range attributes {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, name := range []string{"IssueRepairPrepareRequest", "IssueRepairBindRequest", "IssueRepairCloseRequest", "IssueRepairReopenRequest"} {
		toolContractMap(schemas, name)["x-max-json-utf8-bytes"] = 256 * 1024
		field(name, "project_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
		field(name, "expected_issue_version", map[string]any{"minimum": 1, "maximum": 2147483646})
		field(name, "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
		field(name, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128, "x-min-utf8-bytes": 8, "x-max-utf8-bytes": 128})
		if name != "IssueRepairPrepareRequest" {
			field(name, "expected_repair_version", map[string]any{"minimum": 1, "maximum": 2147483646})
		}
	}
	field("IssueRepairPrepareRequest", "title", map[string]any{"minLength": 1, "maxLength": 500, "x-max-utf8-bytes": 500})
	field("IssueRepairPrepareRequest", "description", map[string]any{"maxLength": 200000, "x-max-utf8-bytes": 200000})
	field("IssueRepairPrepareRequest", "owner_id", map[string]any{"pattern": "^([A-Za-z0-9_-]{1,64})?$"})
	field("IssueRepairPrepareRequest", "acceptance_criteria", map[string]any{"minItems": 1, "maxItems": 100, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000}})
	field("IssueRepairBindRequest", "expected_plan_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("IssueRepairBindRequest", "plan_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
	field("IssueRepairBindRequest", "case_keys", map[string]any{"minItems": 1, "maxItems": 100, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{1,64}$"}})
	field("IssueRepairCloseRequest", "run_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,100}$"})
	field("IssueRepairView", "history", map[string]any{"maxItems": 10, "description": "Most recent bounded repair cycles. history_total/history_complete explicitly disclose omitted history; this window never determines closure authority."})
	field("IssueRepairBlocker", "code", map[string]any{"enum": []string{"repair_not_prepared", "issue_closed", "regression_not_bound", "repair_task_not_accepted", "repair_run_changed", "repair_evidence_stale", "regression_plan_changed", "regression_case_failed", "original_case_required", "formal_review_missing"}})
}
