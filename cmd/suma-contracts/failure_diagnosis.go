package main

import (
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func failureDiagnosisModels() []any {
	return []any{platform.FailureRepairDraft{}, platform.FailureDiagnosisObservation{}, platform.FailureDiagnosisHypothesis{}, platform.FailureDiagnosisOutput{}, platform.FailureDiagnosisEvidence{}, platform.FailureDiagnosisSource{}, platform.FailureDiagnosisContext{}, platform.FailureDiagnosisIssueSource{}, platform.FailureDiagnosisAnalysis{}, platform.FailureDiagnosisCreateRequest{}, platform.FailureDiagnosisCommandRequest{}, platform.FailureDiagnosisPage{}, platform.FailureDiagnosisAdoption{}, toolcontract.FailureDiagnosisCapability{}}
}

func failureDiagnosisContracts(add operationAdder, api map[string]any) {
	issueSource := add("/failure-diagnosis/issue-source", "get", "FailureDiagnosisIssueSource", false, false)
	issueSource["description"] = "Read the original failed run for a current project issue using the verified failure projection. An issue without a formal failure source omits run_id."
	issueID := queryParameter("issue_id", "Exact issue ID in the current project.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
	issueID["required"] = true
	appendQuery(issueSource, issueID)

	for _, route := range []struct{ path, method, input, output, status string }{
		{"/failure-diagnosis/context", "get", "", "FailureDiagnosisContext", "200"},
		{"/failure-diagnosis/analyses", "get", "", "FailureDiagnosisPage", "200"},
		{"/failure-diagnosis/analyses", "post", "FailureDiagnosisCreateRequest", "FailureDiagnosisAnalysis", "201"},
		{"/failure-diagnosis/analyses/{id}", "get", "", "FailureDiagnosisAnalysis", "200"},
		{"/failure-diagnosis/analyses/{id}/commands", "post", "FailureDiagnosisCommandRequest", "FailureDiagnosisAnalysis", "200"},
		{"/issues/{id}/intake", "get", "", "Record", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
			browserOnly(op)
		}
		op["description"] = "Current-authorized diagnosis of frozen, known failed execution evidence. One dedicated read-only model call, no tools or automatic retries. Model observations and hypotheses require human review. Explicit adoption creates one repair requirement draft per source issue; existing requirements are opened without overwrite. Creating a draft never recovers a run, merges code, closes an issue or accepts a task."
		if route.method == "get" && (route.path == "/failure-diagnosis/context" || route.path == "/failure-diagnosis/analyses") {
			for _, query := range []map[string]any{
				queryParameter("run_id", "Original failed run lookup hint; scope is checked from persisted records.", map[string]any{"type": "string", "minLength": 1, "maxLength": 100}),
				queryParameter("node_id", "Exact current failed node.", map[string]any{"type": "string", "minLength": 1, "maxLength": 150}),
				queryParameter("attempt", "Canonical positive decimal current attempt.", map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483646}),
			} {
				query["required"] = true
				appendQuery(op, query)
			}
			if route.path == "/failure-diagnosis/analyses" {
				appendQuery(op, queryParameter("limit", "Exact bounded page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
				appendQuery(op, queryParameter("before", "Opaque cursor bound to project and original failure identity.", map[string]any{"type": "string", "maxLength": 4096}))
			}
		}
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(api, "components", "schemas", model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, name := range []string{"FailureRepairDraft", "FailureDiagnosisObservation", "FailureDiagnosisHypothesis", "FailureDiagnosisOutput", "FailureDiagnosisCreateRequest", "FailureDiagnosisCommandRequest", "FailureDiagnosisCapability"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	field("Record", "failure_diagnosis_adoption", map[string]any{"readOnly": true, "description": "Authoritative immutable relation loaded from the adoption table; generic writes cannot manufacture it."})
	field("FailureDiagnosisContext", "blocker_diagnostics", map[string]any{"readOnly": true, "description": "Positionally aligned optional display metadata. Canonical blocker bytes remain unchanged; clients render message_key in their current language."})
	field("FailureDiagnosisIssueSource", "run_id", map[string]any{"description": "Verified original failed run. Absent for an issue with no formal execution failure projection."})
	field("FailureDiagnosisAnalysis", "reason_diagnostic", map[string]any{"readOnly": true, "description": "Present only for platform warnings; absent for literal human decision reasons."})
	field("FailureDiagnosisCapability", "protocol", map[string]any{"const": toolcontract.FailureDiagnosisProtocol})
	field("FailureDiagnosisCapability", "model", map[string]any{"minLength": 1, "maxLength": 200})
	field("FailureDiagnosisCapability", "endpoint_origin", map[string]any{"minLength": 1, "maxLength": 2000})
	field("FailureDiagnosisAnalysis", "state", map[string]any{"enum": []string{"running", "ready", "failed", "reconciling", "adopted", "dismissed"}})
	for _, model := range []string{"FailureDiagnosisAnalysis", "FailureDiagnosisCreateRequest"} {
		field(model, "locale", map[string]any{"enum": []string{"zh-CN", "en-US"}})
		field(model, "max_steps", map[string]any{"const": 1, "default": 1})
		field(model, "max_output_tokens", map[string]any{"minimum": 1, "maximum": 8192, "default": 4096})
	}
	field("FailureDiagnosisCreateRequest", "acknowledge_model_use", map[string]any{"const": true})
	field("FailureDiagnosisCreateRequest", "expected_source_sha256", map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"})
	field("FailureDiagnosisCommandRequest", "action", map[string]any{"enum": []string{"revise", "adopt", "dismiss"}})
	field("FailureDiagnosisCommandRequest", "acknowledge_draft", map[string]any{"description": "Must be true for adopt. Other commands do not acquire execution authority."})
	for _, spec := range []struct{ model, name string }{{"FailureDiagnosisCreateRequest", "expected_issue_version"}, {"FailureDiagnosisCreateRequest", "attempt"}, {"FailureDiagnosisCreateRequest", "expected_tool_version"}, {"FailureDiagnosisCommandRequest", "expected_version"}} {
		field(spec.model, spec.name, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	for _, spec := range []struct {
		model, name string
		limit       int
	}{{"FailureRepairDraft", "title", 200}, {"FailureRepairDraft", "description", 16000}, {"FailureDiagnosisObservation", "statement", 2000}, {"FailureDiagnosisHypothesis", "statement", 2000}, {"FailureDiagnosisOutput", "summary", 2000}, {"FailureDiagnosisCreateRequest", "goal", 2000}, {"FailureDiagnosisCommandRequest", "reason", 2000}} {
		field(spec.model, spec.name, map[string]any{"minLength": 1, "maxLength": spec.limit, "x-max-utf8-bytes": spec.limit})
	}
	for _, model := range []string{"FailureDiagnosisCreateRequest", "FailureDiagnosisCommandRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("FailureRepairDraft", "priority", map[string]any{"enum": []string{"low", "medium", "high"}})
	field("FailureDiagnosisHypothesis", "confidence", map[string]any{"enum": []string{"low", "medium", "high"}})
	for _, model := range []string{"FailureDiagnosisObservation", "FailureDiagnosisHypothesis"} {
		field(model, "evidence_indices", map[string]any{"minItems": 1, "maxItems": 6, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": 5}})
	}
	field("FailureDiagnosisOutput", "schema_version", map[string]any{"const": 1})
	field("FailureDiagnosisOutput", "observations", map[string]any{"minItems": 1, "maxItems": 8})
	field("FailureDiagnosisOutput", "hypotheses", map[string]any{"minItems": 0, "maxItems": 8})
	for _, name := range []string{"recommended_actions", "test_plan"} {
		field("FailureDiagnosisOutput", name, map[string]any{"minItems": 1, "maxItems": 8, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000}})
	}
	field("FailureRepairDraft", "acceptance_criteria", map[string]any{"minItems": 1, "maxItems": 16, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000, "x-max-utf8-bytes": 1000}})
	field("FailureDiagnosisSource", "evidence", map[string]any{"minItems": 1, "maxItems": 6})
	field("FailureDiagnosisEvidence", "selection", map[string]any{"enum": []string{"full", "tail"}})
	field("FailureDiagnosisEvidence", "text", map[string]any{"maxLength": 8000, "x-max-utf8-bytes": 8000, "description": "The explicit selected full/tail evidence. sha256 and total_bytes refer to the original complete source."})
}
