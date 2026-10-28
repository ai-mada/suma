package main

import (
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func intelligentOperationsModels() []any {
	return []any{platform.IntelligentOperationRequirement{}, platform.IntelligentOperationDesign{}, platform.IntelligentOperationKnowledge{}, platform.IntelligentOperationStep{}, platform.IntelligentOperationOutput{}, platform.IntelligentOperationSource{}, platform.IntelligentOperationStepResult{}, platform.IntelligentOperationAnalysis{}, platform.IntelligentOperationCreateRequest{}, platform.IntelligentOperationCommandRequest{}, platform.IntelligentOperationPreviewRequest{}, platform.IntelligentOperationDefault{}, platform.IntelligentOperationImpact{}, platform.IntelligentOperationPreview{}, platform.IntelligentOperationPage{}}
}

func intelligentOperationsContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/intelligent-operations/analyses", "get", "", "IntelligentOperationPage", "200"},
		{"/intelligent-operations/analyses", "post", "IntelligentOperationCreateRequest", "IntelligentOperationAnalysis", "201"},
		{"/intelligent-operations/analyses/{id}", "get", "", "IntelligentOperationAnalysis", "200"},
		{"/intelligent-operations/analyses/{id}/preview", "post", "IntelligentOperationPreviewRequest", "IntelligentOperationPreview", "200"},
		{"/intelligent-operations/analyses/{id}/commands", "post", "IntelligentOperationCommandRequest", "IntelligentOperationAnalysis", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"agent", "runner"}
		op["description"] = "Current browser or scoped project API authority precedes reads, writes and original-operation receipts. One fixed, read-only model call proposes at most eight local requirement/design/knowledge draft steps; no tools, publication or automatic retry. A reviewed preview and explicit acknowledgement apply all local draft effects, immutable results, activity, analysis CAS and receipt in one database transaction. This is not a cross-service or external-effect transaction."
		if route.input != "" {
			request(op, route.input)
		}
		if route.input == "IntelligentOperationPreviewRequest" {
			op["x-read-only"] = true
			op["x-required-project-authority"] = "current project read"
			op["description"] = "Read-only local-draft impact preview. Creates no drafts, reservation or operation receipt. Binds the complete proposal, sources, target versions and fixed policy; apply recomputes the digest under current project write authority."
		} else if route.input != "" {
			op["x-required-project-authority"] = "current project write"
		}
		if route.path == "/intelligent-operations/analyses" && route.method == "get" {
			appendQuery(op, queryParameter("limit", "Canonical positive decimal page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Existing analysis ID in the current tenant/project; stable created_at/id descending keyset.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "x-max-utf8-bytes": 64}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		for key, value := range attrs {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, name := range []string{"IntelligentOperationRequirement", "IntelligentOperationDesign", "IntelligentOperationKnowledge", "IntelligentOperationStep", "IntelligentOperationOutput", "IntelligentOperationSource", "IntelligentOperationStepResult", "IntelligentOperationAnalysis", "IntelligentOperationCreateRequest", "IntelligentOperationCommandRequest", "IntelligentOperationPreviewRequest", "IntelligentOperationDefault", "IntelligentOperationImpact", "IntelligentOperationPreview", "IntelligentOperationPage", "IntelligentOperationsCapability"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	for _, name := range []string{"IntelligentOperationCreateRequest", "IntelligentOperationCommandRequest", "IntelligentOperationPreviewRequest"} {
		field(name, "project_id", map[string]any{"minLength": 1, "maxLength": 64, "x-max-utf8-bytes": 64})
	}
	for _, name := range []string{"IntelligentOperationCreateRequest", "IntelligentOperationCommandRequest"} {
		field(name, "idempotency_key", map[string]any{"minLength": 1, "maxLength": 128, "x-min-utf8-bytes": 8, "x-max-utf8-bytes": 128, "description": "Stable original operation key, 8–128 UTF-8 bytes; no NUL. Same principal and body recover the original result."})
		toolContractMap(schemas, name)["x-max-json-utf8-bytes"] = 65536
	}
	create := toolContractMap(schemas, "IntelligentOperationCreateRequest")
	create["required"] = []string{"project_id", "goal", "locale", "sources", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key"}
	field("IntelligentOperationCreateRequest", "goal", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("IntelligentOperationCreateRequest", "tool_profile_id", map[string]any{"minLength": 1, "maxLength": 64, "x-max-utf8-bytes": 64})
	field("IntelligentOperationCreateRequest", "expected_tool_version", map[string]any{"minimum": 1})
	field("IntelligentOperationCreateRequest", "acknowledge_model_use", map[string]any{"const": true})
	field("IntelligentOperationCreateRequest", "sources", map[string]any{"minItems": 0, "maxItems": 8, "items": map[string]any{"allOf": []any{map[string]any{"$ref": "#/components/schemas/ProjectKnowledgeSourceInput"}, map[string]any{"properties": map[string]any{"version": map[string]any{"maximum": 2147483646}, "quote": map[string]any{"maxLength": 16000, "x-max-utf8-bytes": 16000}}}}}, "description": "Explicit current project sources, unique by kind/id. Only safe description/criteria excerpts enter the model; Record.Data and configuration do not. Empty selects no sources; null is invalid."})
	for _, name := range []string{"IntelligentOperationCreateRequest", "IntelligentOperationAnalysis"} {
		field(name, "locale", map[string]any{"enum": []string{"zh-CN", "en-US"}})
		field(name, "max_output_tokens", map[string]any{"minimum": 1, "maximum": 8192, "default": 4096})
	}
	field("IntelligentOperationAnalysis", "max_steps", map[string]any{"const": 1})
	field("IntelligentOperationAnalysis", "state", map[string]any{"enum": []string{"running", "ready", "failed", "reconciling", "applied", "dismissed"}})
	field("IntelligentOperationAnalysis", "results", map[string]any{"maxItems": 8})
	field("IntelligentOperationAnalysis", "original_output", map[string]any{"description": "Received model evidence, separate from the editable proposal and business schema validity. Does not authorize adoption."})
	field("IntelligentOperationAnalysis", "reason_key", map[string]any{"description": "Server-owned localized system diagnostic; absent for literal human command reasons."})
	for _, name := range []string{"IntelligentOperationCommandRequest", "IntelligentOperationPreviewRequest"} {
		field(name, "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	command := toolContractMap(schemas, "IntelligentOperationCommandRequest")
	command["required"] = []string{"project_id", "expected_version", "action", "reason", "idempotency_key"}
	field("IntelligentOperationCommandRequest", "action", map[string]any{"enum": []string{"revise", "apply", "dismiss"}})
	field("IntelligentOperationCommandRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	toolContractMap(command, "properties")["proposal"] = map[string]any{"$ref": "#/components/schemas/IntelligentOperationOutput"}
	command["oneOf"] = []any{
		map[string]any{"required": []string{"proposal"}, "properties": map[string]any{"action": map[string]any{"const": "revise"}, "preview_sha256": map[string]any{"const": ""}, "acknowledge_drafts": map[string]any{"const": false}}},
		map[string]any{"required": []string{"preview_sha256", "acknowledge_drafts"}, "properties": map[string]any{"action": map[string]any{"const": "apply"}, "preview_sha256": map[string]any{"pattern": "^[a-f0-9]{64}$"}, "acknowledge_drafts": map[string]any{"const": true}}, "not": map[string]any{"required": []string{"proposal"}}},
		map[string]any{"properties": map[string]any{"action": map[string]any{"const": "dismiss"}, "preview_sha256": map[string]any{"const": ""}, "acknowledge_drafts": map[string]any{"const": false}}, "not": map[string]any{"required": []string{"proposal"}}},
	}
	output := toolContractMap(schemas, "IntelligentOperationOutput")
	output["x-max-json-utf8-bytes"] = 32000
	field("IntelligentOperationOutput", "schema_version", map[string]any{"const": 1})
	field("IntelligentOperationOutput", "summary", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("IntelligentOperationOutput", "rationale", map[string]any{"minLength": 1, "maxLength": 8000, "x-max-utf8-bytes": 8000})
	field("IntelligentOperationOutput", "questions", map[string]any{"maxItems": 8, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000, "x-max-utf8-bytes": 1000}})
	field("IntelligentOperationOutput", "steps", map[string]any{"maxItems": 8, "description": "Unique stable step keys with closed acyclic dependencies; server topological order resolves new requirement IDs. Duplicate design targets are invalid."})
	output["allOf"] = []any{map[string]any{"if": map[string]any{"properties": map[string]any{"steps": map[string]any{"maxItems": 0}}}, "then": map[string]any{"properties": map[string]any{"questions": map[string]any{"minItems": 1}}}}}
	step := toolContractMap(schemas, "IntelligentOperationStep")
	step["required"] = []string{"key", "action", "depends_on"}
	field("IntelligentOperationStep", "key", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
	field("IntelligentOperationStep", "action", map[string]any{"enum": []string{"requirement.create_draft", "requirement.design.save_draft", "knowledge.create_draft"}})
	field("IntelligentOperationStep", "depends_on", map[string]any{"maxItems": 8, "uniqueItems": true, "items": map[string]any{"type": "string", "pattern": "^[A-Za-z0-9_-]{1,64}$"}})
	branches := []any{}
	for _, action := range []struct{ action, payload, model string }{{"requirement.create_draft", "requirement", "IntelligentOperationRequirement"}, {"requirement.design.save_draft", "design", "IntelligentOperationDesign"}, {"knowledge.create_draft", "knowledge", "IntelligentOperationKnowledge"}} {
		toolContractMap(step, "properties")[action.payload] = map[string]any{"$ref": "#/components/schemas/" + action.model}
		foreign := []any{}
		for _, payload := range []string{"requirement", "design", "knowledge"} {
			if payload != action.payload {
				foreign = append(foreign, map[string]any{"required": []string{payload}})
			}
		}
		branches = append(branches, map[string]any{"properties": map[string]any{"action": map[string]any{"const": action.action}}, "required": []string{action.payload}, "not": map[string]any{"anyOf": foreign}})
	}
	step["oneOf"] = branches
	toolContractMap(schemas, "IntelligentOperationRequirement")["required"] = []string{"title", "acceptance_criteria"}
	field("IntelligentOperationRequirement", "title", map[string]any{"minLength": 1, "maxLength": 200, "x-max-utf8-bytes": 200})
	field("IntelligentOperationRequirement", "description", map[string]any{"maxLength": 16000, "x-max-utf8-bytes": 16000})
	field("IntelligentOperationRequirement", "priority", map[string]any{"enum": []string{"", "low", "medium", "high", "urgent"}, "description": "Empty or omitted uses the requirement domain's medium default, shown in preview."})
	field("IntelligentOperationRequirement", "acceptance_criteria", map[string]any{"minItems": 1, "maxItems": 16, "items": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000, "x-max-utf8-bytes": 1000}})
	design := toolContractMap(schemas, "IntelligentOperationDesign")
	design["required"] = []string{"content", "tasks"}
	design["oneOf"] = []any{
		map[string]any{"required": []string{"requirement_id"}, "properties": map[string]any{"requirement_id": map[string]any{"minLength": 1, "maxLength": 64, "x-max-utf8-bytes": 64}, "requirement_step_key": map[string]any{"const": ""}}},
		map[string]any{"required": []string{"requirement_step_key"}, "properties": map[string]any{"requirement_step_key": map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"}, "requirement_id": map[string]any{"const": ""}}},
	}
	field("IntelligentOperationDesign", "tasks", map[string]any{"maxItems": 100, "x-max-json-bytes": 262144, "description": "Ordinary requirement-design draft validation and owner authority apply. Saving does not publish or adopt formal tasks."})
	toolContractMap(schemas, "IntelligentOperationKnowledge")["required"] = []string{"kind", "title", "content", "source_indices"}
	field("IntelligentOperationKnowledge", "kind", map[string]any{"enum": []string{"memory", "spec", "skill"}})
	for name, limit := range map[string]int{"title": 200, "summary": 1000, "content": 16000} {
		attrs := map[string]any{"maxLength": limit, "x-max-utf8-bytes": limit}
		if name != "summary" {
			attrs["minLength"] = 1
		}
		field("IntelligentOperationKnowledge", name, attrs)
	}
	field("IntelligentOperationKnowledge", "source_indices", map[string]any{"minItems": 0, "maxItems": 8, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": 7}, "description": "Zero-based subset of frozen sources; empty is legal. Every index must exist in this analysis; no caller-supplied snapshots or provenance."})
	field("IntelligentOperationSource", "sources", map[string]any{"maxItems": 8})
	field("IntelligentOperationPreview", "policy", map[string]any{"const": "suma-local-drafts-v1"})
	field("IntelligentOperationPreview", "steps", map[string]any{"maxItems": 8})
	field("IntelligentOperationPreview", "questions", map[string]any{"maxItems": 8})
	field("IntelligentOperationImpact", "required_permission", map[string]any{"const": "project.write"})
	field("IntelligentOperationImpact", "risk", map[string]any{"const": "draft_only"})
	field("IntelligentOperationImpact", "target_kind", map[string]any{"enum": []string{"requirements", "requirement_designs", "project_knowledge"}})
	field("IntelligentOperationStepResult", "resource_kind", map[string]any{"enum": []string{"requirements", "requirement_designs", "project_knowledge"}})
	field("IntelligentOperationStepResult", "version", map[string]any{"minimum": 1})
	field("IntelligentOperationStepResult", "revision", map[string]any{"minimum": 0, "description": "0 for ordinary requirement records; positive immutable revision for design and knowledge drafts."})
	field("IntelligentOperationsCapability", "protocol", map[string]any{"const": toolcontract.IntelligentOperationsProtocol})
	field("IntelligentOperationsCapability", "model", map[string]any{"minLength": 1, "maxLength": 256, "x-max-utf8-bytes": 256})
	field("IntelligentOperationsCapability", "endpoint_origin", map[string]any{"minLength": 1, "maxLength": 2048, "description": "Fixed HTTPS origin or loopback HTTP origin, without credentials, path, query or fragment. A probe is not model account health."})
	for _, name := range []string{"IntelligentOperationSource", "IntelligentOperationStepResult", "IntelligentOperationAnalysis", "IntelligentOperationDefault", "IntelligentOperationImpact", "IntelligentOperationPreview", "IntelligentOperationPage"} {
		for _, property := range toolContractMap(schemas, name, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
}
