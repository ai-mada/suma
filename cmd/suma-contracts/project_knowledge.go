package main

import "github.com/ai-mada/suma/internal/platform"

func projectKnowledgeModels() []any {
	return []any{platform.ProjectKnowledgeReference{}, platform.ProjectKnowledgeSourceInput{}, platform.ProjectKnowledgeSource{}, platform.ProjectKnowledgeContextItem{}, platform.ProjectKnowledgeRevision{}, platform.ProjectKnowledgeRevisionDetail{}, platform.ProjectKnowledgeSummary{}, platform.ProjectKnowledgeDetail{}, platform.ProjectKnowledgeList{}, platform.ProjectKnowledgeRevisionList{}, platform.ProjectKnowledgeWriteRequest{}, platform.ProjectKnowledgeExtractionRequest{}, platform.ProjectKnowledgeCommandRequest{}, platform.DeliveryKnowledgeSourceView{}, platform.DeliveryKnowledgeBasis{}, platform.DeliveryKnowledgeTestEvidence{}, platform.DeliveryKnowledgeCandidateEvidence{}}
}

func projectKnowledgeContracts(add operationAdder, api map[string]any) {
	paths := api["paths"].(map[string]any)
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/tasks/{id}/delivery-knowledge-source", "get", "", "DeliveryKnowledgeSourceView", "200"},
		{"/project-knowledge", "get", "", "ProjectKnowledgeList", "200"},
		{"/project-knowledge", "post", "ProjectKnowledgeWriteRequest", "ProjectKnowledgeDetail", "201"},
		{"/project-knowledge/extractions", "post", "ProjectKnowledgeExtractionRequest", "ProjectKnowledgeDetail", "201"},
		{"/project-knowledge/{id}", "get", "", "ProjectKnowledgeDetail", "200"},
		{"/project-knowledge/{id}", "patch", "ProjectKnowledgeWriteRequest", "ProjectKnowledgeDetail", "200"},
		{"/project-knowledge/{id}/commands", "post", "ProjectKnowledgeCommandRequest", "ProjectKnowledgeDetail", "200"},
		{"/project-knowledge/{id}/revisions", "get", "", "ProjectKnowledgeRevisionList", "200"},
		{"/project-knowledge/{id}/revisions/{revision}", "get", "", "ProjectKnowledgeRevisionDetail", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
		}
		if route.path == "/project-knowledge/{id}/commands" {
			browserOnly(op)
			op["x-required-project-authority"] = "current write and approval"
		}
		op["description"] = "Project-scoped reviewed text knowledge. memory/spec/skill are data, not executable plugins or permission grants. Every mutation appends an immutable revision and checks the current credential before a receipt. Draft edits preserve the prior publication; current source changes or retirement block new adoption, without rewriting old runs."
		if route.path == "/project-knowledge/{id}/revisions/{revision}" {
			op["x-read-only"] = true
			op["description"] = "Read the exact immutable revision under current project read authority, with verified content digest. Never substitutes latest or grants eligibility for a new model request."
			for _, raw := range op["parameters"].([]any) {
				parameter := raw.(map[string]any)
				if parameter["in"] == "path" && parameter["name"] == "revision" {
					parameter["schema"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483647, "x-canonical-decimal": true}
				}
			}
		}
		if route.path == "/project-knowledge" && route.method == "get" {
			appendQuery(op, queryParameter("kind", "Optional memory, spec or skill filter.", map[string]any{"type": "string", "enum": []string{"memory", "spec", "skill"}}))
			appendQuery(op, queryParameter("state", "Current aggregate state; published may have a newer draft.", map[string]any{"type": "string", "enum": []string{"draft", "published", "retired"}}))
			appendQuery(op, queryParameter("q", "Title/summary keyword; maximum200 UTF8 bytes.", map[string]any{"type": "string", "maxLength": 200, "x-max-utf8-bytes": 200}))
		}
		if route.path == "/project-knowledge" && route.method == "get" || route.path == "/project-knowledge/{id}/revisions" {
			limit := 50
			if route.path == "/project-knowledge/{id}/revisions" {
				limit = 20
			}
			appendQuery(op, queryParameter("limit", "Bounded page size; no fractional or exponential lexeme.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": limit}))
			appendQuery(op, queryParameter("cursor", "Opaque tenant/project/filter-bound cursor.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ProjectKnowledgeReference", "ProjectKnowledgeSourceInput", "ProjectKnowledgeWriteRequest", "ProjectKnowledgeExtractionRequest", "ProjectKnowledgeCommandRequest"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(schemas, model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, model := range []string{"ProjectKnowledgeWriteRequest", "ProjectKnowledgeExtractionRequest", "ProjectKnowledgeRevision", "ProjectKnowledgeContextItem", "ProjectKnowledgeSummary"} {
		field(model, "kind", map[string]any{"enum": []string{"memory", "spec", "skill"}})
	}
	for _, model := range []string{"ProjectKnowledgeWriteRequest", "ProjectKnowledgeExtractionRequest"} {
		field(model, "title", map[string]any{"maxLength": 200, "x-max-utf8-bytes": 200})
		field(model, "summary", map[string]any{"maxLength": 1000, "x-max-utf8-bytes": 1000})
		field(model, "sources", map[string]any{"maxItems": 8})
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	// Extraction permits server-derived title/empty summary, but always requires
	// at least one current formal source. It is explicitly non-model extraction.
	toolContractMap(schemas, "ProjectKnowledgeExtractionRequest")["required"] = []string{"project_id", "kind", "sources", "idempotency_key"}
	field("ProjectKnowledgeExtractionRequest", "sources", map[string]any{"minItems": 1})
	field("ProjectKnowledgeWriteRequest", "content", map[string]any{"minLength": 1, "maxLength": 16000, "x-max-utf8-bytes": 16000})
	field("ProjectKnowledgeWriteRequest", "expected_version", map[string]any{"minimum": 0, "maximum": 2147483646, "description": "Creation requires0; update requires positive current CAS. Exact JSON integer lexeme required."})
	field("ProjectKnowledgeCommandRequest", "action", map[string]any{"enum": []string{"publish", "retire"}})
	field("ProjectKnowledgeCommandRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("ProjectKnowledgeCommandRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	for _, model := range []string{"ProjectKnowledgeSourceInput", "ProjectKnowledgeSource", "ProjectKnowledgeContextSource"} {
		field(model, "kind", map[string]any{"enum": []string{"documents", "requirements", "tasks", "issues", "accepted_delivery"}})
	}
	field("ProjectKnowledgeSourceInput", "basis_sha256", map[string]any{"description": "Required exact lowercase SHA256 of the explicitly previewed server basis for accepted_delivery. Other source kinds cannot provide a nonempty value. Never submit a delivery proof object.", "pattern": "^([a-f0-9]{64})?$"})
	for _, name := range []string{"DeliveryKnowledgeSourceView", "DeliveryKnowledgeBasis", "DeliveryKnowledgeTestEvidence", "DeliveryKnowledgeCandidateEvidence"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field("DeliveryKnowledgeBasis", "protocol", map[string]any{"const": "suma-accepted-delivery-v1"})
	field("DeliveryKnowledgeBasis", "sha256", map[string]any{"pattern": "^[a-f0-9]{64}$", "description": "Go canonical digest with this field empty and evidence arrays sorted by their exact identities. Provenance, not a new approval or permission grant."})
	field("DeliveryKnowledgeBasis", "tests", map[string]any{"maxItems": 256})
	field("DeliveryKnowledgeBasis", "candidates", map[string]any{"maxItems": 128})
	field("DeliveryKnowledgeCandidateEvidence", "kind", map[string]any{"enum": []string{"workspace_revision", "sealed_bundle"}, "description": "Only a verified sealed bundle may carry transfer/bundle fields. An accepted test-only task has no candidate."})
	preview := toolContractMap(paths, "/tasks/{id}/delivery-knowledge-source", "get")
	preview["x-read-only"], preview["x-no-agent-effects"], preview["x-response-cache-control"] = true, true, "no-store"
	preview["description"] = "Read the current same-project formally accepted task and exact quality evidence as a source preview. No model, proposal, draft or publication is created. Current authorization and actual task/review/run/evidence remain required; success or a review request alone is ineligible."
	field("ProjectKnowledgeSourceInput", "version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("ProjectKnowledgeReference", "revision", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("ProjectKnowledgeReference", "sha256", map[string]any{"pattern": "^[a-f0-9]{64}$", "description": "Digest of the explicitly selected publication, not authority."})
	field("ProjectKnowledgeRevision", "method", map[string]any{"enum": []string{"manual", "extractive", "catalog_adoption", "model_assisted"}})
	field("ProjectKnowledgeSummary", "eligibility", map[string]any{"enum": []string{"none", "current", "source_changed", "retired", "publication_changed"}, "description": "Current eligibility of this exact published revision; historical receipt content is not overwritten."})
	field("TaskDispatchPreviewRequest", "knowledge", map[string]any{"maxItems": 16, "description": "Explicit reviewed publication references; defaultempty. Resolved server-side and fixed into context. Total goal remains<=32000 UTF8 bytes; no truncation."})
	field("TaskContext", "knowledge", map[string]any{"maxItems": 16, "readOnly": true, "description": "Server-resolved immutable snapshots. Content does not grant platform/tool permissions."})
	paths["/task-dispatch-plans"].(map[string]any)["post"].(map[string]any)["x-fixed-knowledge-contract"] = "Current publication/source checked at preview and atomic start; draft edits preserve prior publication. Changed publication or retired/stale source requires a new reviewed preview."
}
