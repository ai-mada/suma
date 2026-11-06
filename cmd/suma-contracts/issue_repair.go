package main

import "github.com/ai-mada/suma/internal/platform"

func issueRepairModels() []any {
	return []any{platform.IssueRepair{}, platform.IssueRepairAutomationSource{}, platform.IssueRegressionBinding{}, platform.IssueRepairClosure{}, platform.IssueRepairCaseOrigin{}, platform.IssueRepairPackageOrigin{}, platform.IssueRepairPackageProof{}, platform.IssueRepairReleaseHandoff{}, platform.IssueRepairReleaseHandoffInput{}, platform.IssueRepairBlocker{}, platform.IssueRepairView{}, platform.IssueRepairPrepareRequest{}, platform.IssueRepairBindRequest{}, platform.IssueRepairCloseRequest{}, platform.IssueRepairReopenRequest{}}
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
		op["description"] = "Current protected issue repair cycle. Reuses ordinary tasks, the single Go scheduler, immutable regression publications and current formal task approval. Package failures also require an independently verified package regression for the original scope. Exact original operations replay only after current authorization."
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
	for _, name := range []string{"IssueRepair", "IssueRepairAutomationSource", "IssueRegressionBinding", "IssueRepairClosure", "IssueRepairCaseOrigin", "IssueRepairPackageOrigin", "IssueRepairPackageProof", "IssueRepairReleaseHandoff", "IssueRepairReleaseHandoffInput", "IssueRepairBlocker", "IssueRepairView", "IssueRepairPrepareRequest", "IssueRepairBindRequest", "IssueRepairCloseRequest", "IssueRepairReopenRequest"} {
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
	field("IssueRepair", "automation_source", map[string]any{"readOnly": true, "description": "Original failed run and verified automation provenance retained when this repair cycle was prepared. Visible to authorized browser sessions. Does not establish acceptance or closure eligibility."})
	field("IssueRepairAutomationSource", "run_id", map[string]any{"readOnly": true, "minLength": 1, "maxLength": 100})
	field("IssueRepairAutomationSource", "origin", map[string]any{"readOnly": true})
	field("IssueRepairView", "history", map[string]any{"maxItems": 10, "description": "Most recent bounded repair cycles. history_total/history_complete explicitly disclose omitted history; this window never determines closure authority."})
	field("IssueRepairView", "original_case", map[string]any{"readOnly": true, "description": "Protected original failed case locator from the frozen execution. Independent of editable issue metadata and current plan publication."})
	for _, name := range []string{"plan_id", "case_key"} {
		field("IssueRepairCaseOrigin", name, map[string]any{"readOnly": true, "pattern": "^[A-Za-z0-9_-]{1,64}$"})
	}
	field("IssueRepairCaseOrigin", "run_id", map[string]any{"readOnly": true, "pattern": "^[A-Za-z0-9_.-]{1,100}$"})
	field("IssueRepairCaseOrigin", "plan_revision", map[string]any{"readOnly": true, "minimum": 1, "maximum": 2147483646})
	field("IssueRepairCaseOrigin", "plan_sha256", map[string]any{"readOnly": true, "pattern": "^[a-f0-9]{64}$"})
	field("IssueRepairView", "original_package", map[string]any{"readOnly": true, "description": "Historical source of an issue from a protected failed package attempt, independent of editable issue metadata."})
	field("IssueRepairView", "release_handoff", map[string]any{"readOnly": true, "description": "Current closed and releasable package repair. Omitted when the current user or current evidence does not permit this handoff."})
	for _, property := range toolContractMap(schemas, "IssueRepairReleaseHandoff", "properties") {
		property.(map[string]any)["readOnly"] = true
	}
	for _, name := range []string{"issue_version", "repair_version", "cycle", "task_version"} {
		field("IssueRepairReleaseHandoff", name, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	for _, name := range []string{"expected_issue_version", "expected_repair_version"} {
		field("IssueRepairReleaseHandoffInput", name, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("IssueRepairReleaseHandoffInput", "issue_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
	field("IssueRepairClosure", "package_regression", map[string]any{"readOnly": true, "description": "Exact sealed package proof retained by this closure cycle. The closure's run_id identifies the parent run."})
	for _, name := range []string{"IssueRepairPackageOrigin", "IssueRepairPackageProof"} {
		for _, property := range toolContractMap(schemas, name, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
		field(name, "node_id", map[string]any{"pattern": "^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$"})
		field(name, "attempt", map[string]any{"minimum": 1, "maximum": 2147483646})
		field(name, "fence", map[string]any{"minimum": 1, "maximum": 9007199254740991})
		field(name, "artifact_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
		field(name, "manifest_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
		field(name, "source_commit", map[string]any{"pattern": "^(?:[a-f0-9]{40}|[a-f0-9]{64})$"})
	}
	field("IssueRepairPackageOrigin", "repository_id", map[string]any{"description": "Exact registered Git integration target ID, when the original build was application-scoped."})
	field("IssueRepairBlocker", "code", map[string]any{"enum": []string{"repair_not_prepared", "issue_closed", "regression_not_bound", "repair_task_not_accepted", "repair_run_changed", "repair_evidence_stale", "regression_plan_changed", "regression_case_failed", "original_case_required", "formal_review_missing", "package_regression_missing", "package_regression_scope_changed"}})
}
