package main

import (
	"github.com/ai-mada/suma/internal/platform"
	tr "github.com/ai-mada/suma/internal/testreportcontract"
)

func testPlanModels() []any {
	return []any{platform.QualityTestCase{}, platform.TestPlanSource{}, platform.TestPlanRevision{}, platform.TestPlanView{}, platform.TestPlanList{}, platform.TestPlanCreateRequest{}, platform.TestPlanSaveRequest{}, platform.TestPlanCommandRequest{}, platform.TestPlanReportRef{}, platform.TestPlanReport{}, platform.TestCaseEvidence{}, platform.TestCaseResult{}, platform.TestPlanExecution{}, platform.TestPlanPackageEvidence{}, platform.TestPlanExecutionList{}, platform.TestPlanDefectRequest{}, platform.TestPlanPackageDefectRequest{}, tr.TestReport{}, tr.TestCaseSelector{}, tr.TestReportCase{}, tr.ParsedTestReport{}}
}

func testPlanContracts(add operationAdder, api map[string]any) {
	for _, r := range []struct{ path, method, input, output, status string }{
		{"/test-plans", "get", "", "TestPlanList", "200"},
		{"/test-plans", "post", "TestPlanCreateRequest", "TestPlanView", "201"},
		{"/test-plans/{id}", "get", "", "TestPlanView", "200"},
		{"/test-plans/{id}", "put", "TestPlanSaveRequest", "TestPlanView", "200"},
		{"/test-plans/{id}/commands", "post", "TestPlanCommandRequest", "TestPlanView", "200"},
		{"/test-plans/{id}/executions", "get", "", "TestPlanExecutionList", "200"},
		{"/test-plans/{id}/executions/{run}", "get", "", "TestPlanExecution", "200"},
		{"/test-plans/{id}/executions/{run}/defects", "post", "TestPlanDefectRequest", "Record", "201"},
		{"/test-plans/{id}/executions/{run}/package-defects", "post", "TestPlanPackageDefectRequest", "Record", "201"},
		{"/test-plans/{id}/reports/{report}", "get", "", "TestPlanReport", "200"},
	} {
		op := add(r.path, r.method, r.output, false, r.input != "")
		successStatus(op, r.status)
		op["x-disallowed-token-scopes"] = []string{"agent", "runner"}
		op["x-required-project-authority"] = "current project read"
		op["description"] = "Task-bound immutable test plan revisions and separate exact-attempt JUnit and package evidence. Current authorization precedes reads, writes and original-operation receipts. Existing task dispatch freezes the published plan; process success, case success and human approval remain distinct."
		if r.input != "" {
			request(op, r.input)
			op["x-required-project-authority"] = "current project write"
		} else {
			op["x-read-only"] = true
		}
		if r.input == "TestPlanCommandRequest" {
			browserOnly(op)
			op["x-required-project-authority"] = "current browser project approval"
		}
		if r.output == "TestPlanReport" {
			op["description"] = "Explicit authorized read of the bounded original XML and parsed cases. Ordinary execution summaries carry references without XML. Report IDs, raw XML and case names cannot grant execution or acceptance rights."
		}
		if r.input == "TestPlanPackageDefectRequest" {
			op["description"] = "Create or recover an issue from the exact current failed package-test attempt. Requires an explicit reason and known test failure against the sealed artifact. Deduplicated by plan, run, node, attempt and fence."
		}
		if r.output == "TestPlanList" || r.output == "TestPlanExecutionList" {
			if r.output == "TestPlanList" {
				appendQuery(op, queryParameter("task_id", "Optional exact current project task. Filtering applies before count and pagination; a cursor from another task is rejected.", map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{1,64}$"}))
			}
			appendQuery(op, queryParameter("limit", "Canonical positive decimal page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			maximum := 64
			description := "Existing plan ID in this tenant/project, descending created_at/id keyset."
			if r.output == "TestPlanExecutionList" {
				maximum = 100
				description = "Existing task run ID in this tenant/project, descending created_at/run_id keyset."
			}
			appendQuery(op, queryParameter("before", description, map[string]any{"type": "string", "minLength": 1, "maxLength": maximum, "x-max-utf8-bytes": maximum}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attributes map[string]any) {
		for key, value := range attributes {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, name := range []string{"QualityTestCase", "TestPlanSource", "TestPlanRevision", "TestPlanView", "TestPlanList", "TestPlanCreateRequest", "TestPlanSaveRequest", "TestPlanCommandRequest", "TestPlanReportRef", "TestPlanReport", "TestCaseEvidence", "TestCaseResult", "TestPlanExecution", "TestPlanPackageEvidence", "TestPlanExecutionList", "TestPlanDefectRequest", "TestPlanPackageDefectRequest", "TestReport", "TestCaseSelector", "TestReportCase", "ParsedTestReport"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	for _, name := range []string{"TestPlanCreateRequest", "TestPlanSaveRequest", "TestPlanCommandRequest", "TestPlanDefectRequest", "TestPlanPackageDefectRequest"} {
		toolContractMap(schemas, name)["x-max-json-utf8-bytes"] = 256 * 1024
		field(name, "project_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
		field(name, "idempotency_key", map[string]any{"minLength": 1, "maxLength": 128, "x-min-utf8-bytes": 8, "x-max-utf8-bytes": 128, "description": "Stable original operation key, 8–128 UTF-8 bytes; changed same-key bodies conflict. Current actor authorization precedes receipt replay."})
	}
	for _, name := range []string{"TestPlanCreateRequest", "TestPlanSaveRequest"} {
		field(name, "expected_task_version", map[string]any{"minimum": 1, "maximum": 2147483646})
		field(name, "expected_requirement_version", map[string]any{"minimum": 0, "maximum": 2147483646, "description": "Exact current linked requirement version, or explicit0 when no requirement exists."})
		field(name, "title", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
		field(name, "description", map[string]any{"maxLength": 4000, "x-max-utf8-bytes": 4000})
		field(name, "cases", map[string]any{"minItems": 0, "maxItems": 100, "x-max-json-utf8-bytes": 128 * 1024, "description": "Complete non-null draft array. Publication requires at least one complete case, unique exact selectors and coverage of every task acceptance criterion."})
	}
	field("TestPlanCreateRequest", "task_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$", "description": "Current authorized project task; exactly one plan per task. Caller input cannot supply run or result bindings."})
	for _, name := range []string{"TestPlanSaveRequest", "TestPlanCommandRequest", "TestPlanDefectRequest", "TestPlanPackageDefectRequest"} {
		field(name, "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	for _, name := range []string{"TestPlanCommandRequest", "TestPlanDefectRequest", "TestPlanPackageDefectRequest"} {
		field(name, "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	}
	field("TestPlanCommandRequest", "action", map[string]any{"enum": []string{"publish", "retire"}})
	field("TestPlanDefectRequest", "case_key", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$", "description": "Currently failed/error case in the exact execution. Issue creation is deduplicated by plan/run/case; passed, skipped or unknown cases cannot use this action."})
	field("TestPlanPackageDefectRequest", "node_id", map[string]any{"pattern": "^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$", "description": "Frozen package-test node in the exact execution."})
	field("TestPlanPackageDefectRequest", "expected_attempt", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("TestPlanPackageDefectRequest", "expected_fence", map[string]any{"minimum": 1, "maximum": 9007199254740991})
	field("QualityTestCase", "key", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
	field("QualityTestCase", "title", map[string]any{"maxLength": 200, "x-max-utf8-bytes": 200})
	for _, name := range []string{"description", "preconditions", "expected_result"} {
		field("QualityTestCase", name, map[string]any{"maxLength": 4000, "x-max-utf8-bytes": 4000})
	}
	field("QualityTestCase", "priority", map[string]any{"enum": []string{"urgent", "high", "medium", "low"}})
	field("QualityTestCase", "steps", map[string]any{"minItems": 0, "maxItems": 20, "items": map[string]any{"type": "string", "maxLength": 1000, "x-max-utf8-bytes": 1000}, "description": "May be empty in a draft; publication requires at least one nonempty step."})
	field("QualityTestCase", "covers", map[string]any{"maxItems": 100, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 1}, "description": "1-based positions in the exact fixed task acceptance_criteria, not requirement positions or inferred matching. Publication covers every current task criterion."})
	for _, name := range []string{"class_name", "name"} {
		field("TestCaseSelector", name, map[string]any{"maxLength": 500, "x-max-utf8-bytes": 500, "description": "Exact JUnit identity with no controls or surrounding whitespace. Empty draft values are legal; publication and actual reports require nonempty name. No fuzzy matching."})
	}
	field("TestPlanExecution", "package_evidence", map[string]any{"maxItems": 1000, "description": "Frozen package-test gates, separate from the JUnit case counts. Empty when there is no bound package gate."})
	field("TestPlanPackageEvidence", "state", map[string]any{"enum": []string{"pending", "verified", "unverified", "failed", "unknown", "skipped", "cancelled"}})
	field("TestPlanPackageEvidence", "attempt", map[string]any{"minimum": 1})
	field("TestPlanPackageEvidence", "fence", map[string]any{"minimum": 1, "maximum": 9007199254740991, "description": "Fence of the exact current attempt, when available."})
	field("TestPlanPackageEvidence", "issue_id", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$", "description": "Protected issue linked to this exact package-test attempt, when present."})
	field("TestPlanPackageEvidence", "can_create_issue", map[string]any{"description": "Current project write access and an eligible, known failed package attempt with no linked issue."})
	field("TestPlanPackageEvidence", "manifest_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$", "description": "Present only with verified current evidence."})
	field("TestReport", "protocol", map[string]any{"const": tr.Protocol})
	field("TestReport", "xml", map[string]any{"maxLength": tr.MaxXMLBytes, "x-max-utf8-bytes": tr.MaxXMLBytes, "description": "Complete original UTF-8 XML, never silently truncated. The domain parser rejects directives, namespaces, duplicate selectors, ambiguous outcomes, inconsistent counters and unsupported structures."})
	field("TestReport", "sha256", map[string]any{"pattern": "^[a-f0-9]{64}$", "description": "SHA256 of exact original UTF-8 bytes, not reserialized or redacted XML."})
	field("TestReportCase", "selector", map[string]any{"allOf": []any{map[string]any{"properties": map[string]any{"name": map[string]any{"minLength": 1}}, "required": []string{"name"}}}})
	for _, name := range []string{"TestReportCase", "TestCaseEvidence"} {
		field(name, "status", map[string]any{"enum": []string{"passed", "failed", "error", "skipped"}})
		field(name, "message", map[string]any{"maxLength": tr.MaxMessageBytes, "x-max-utf8-bytes": tr.MaxMessageBytes, "description": "Bounded diagnostic projection; long text is shortened at a UTF-8 boundary. Complete bounded original XML remains available through the authorized report read."})
		field(name, "duration_ms", map[string]any{"minimum": 0, "maximum": 86400000})
	}
	field("ParsedTestReport", "cases", map[string]any{"maxItems": tr.MaxCases})
	field("TestPlanReport", "xml", map[string]any{"maxLength": tr.MaxXMLBytes, "x-max-utf8-bytes": tr.MaxXMLBytes, "description": "Explicit original-report read; empty when no original was captured. Report validity and process outcome are separate facts."})
	field("TestPlanReport", "cases", map[string]any{"maxItems": tr.MaxCases})
	for _, name := range []string{"TestPlanReportRef", "TestPlanReport", "ParsedTestReport"} {
		for _, counter := range []string{"total", "passed", "failed", "errors", "skipped"} {
			field(name, counter, map[string]any{"minimum": 0, "maximum": tr.MaxCases})
		}
	}
	for _, name := range []string{"TestPlanReportRef", "TestPlanReport"} {
		field(name, "eligibility", map[string]any{"enum": []string{"current", "invalid", "unknown", "stale_attempt", "source_changed"}})
		field(name, "unmapped", map[string]any{"minimum": 0, "maximum": tr.MaxCases})
	}
	for _, name := range []string{"TestPlanView", "TestPlanRevision"} {
		field(name, "state", map[string]any{"enum": []string{"draft", "published", "retired"}})
	}
	field("TestPlanRevision", "cases", map[string]any{"maxItems": 100, "x-max-json-utf8-bytes": 128 * 1024})
	field("TestPlanView", "eligibility", map[string]any{"enum": []string{"none", "current", "source_changed", "retired"}})
	field("TestPlanView", "allowed_actions", map[string]any{"items": map[string]any{"type": "string", "enum": []string{"save", "publish", "retire"}}, "description": "Server-resolved current actions, never caller grants."})
	field("TestCaseResult", "status", map[string]any{"enum": []string{"passed", "failed", "error", "skipped", "not_run", "unknown", "ambiguous"}})
	field("TestPlanExecution", "state", map[string]any{"enum": []string{"pending", "passed", "failed", "incomplete", "unknown", "source_changed", "retired", "run_changed"}})
	field("TestPlanExecution", "cases", map[string]any{"maxItems": 100})
	for _, name := range []string{"total", "passed", "failed", "errors", "skipped", "not_run"} {
		field("TestPlanExecution", name, map[string]any{"minimum": 0, "maximum": 100})
	}
	field("ToolExecutionProfile", "test_report_protocol", map[string]any{"enum": []string{"", tr.Protocol}, "description": "Explicit administrator-enabled capture on an isolated command test. Discovery does not prove a test run, passing cases or human acceptance."})
	for _, name := range []string{"TestPlanSource", "TestPlanRevision", "TestPlanView", "TestPlanList", "TestPlanReportRef", "TestPlanReport", "TestCaseEvidence", "TestCaseResult", "TestPlanExecution", "TestPlanPackageEvidence", "TestPlanExecutionList", "TestReportCase", "ParsedTestReport"} {
		for _, property := range toolContractMap(schemas, name, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
}
