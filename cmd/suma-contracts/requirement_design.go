package main

import "github.com/ai-mada/suma/internal/platform"

func requirementDesignModels() []any {
	return []any{platform.RequirementDesignContent{}, platform.RequirementDesignTask{}, platform.RequirementDesignDiagnostic{}, platform.RequirementDesignRevision{}, platform.RequirementDesignView{}, platform.RequirementDesignSaveRequest{}, platform.RequirementDesignCommandRequest{}, platform.RequirementDesignPreviewRequest{}, platform.RequirementDesignChange{}, platform.RequirementDesignPreview{}, platform.RequirementDesignAdoptRequest{}, platform.RequirementDesignAdoptedTask{}, platform.RequirementDesignAdoption{}, platform.RequirementDesignRevisionList{}, platform.RequirementDesignAdoptionList{}, platform.RequirementDesignContext{}}
}

func requirementDesignContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/requirements/{id}/design", "get", "", "RequirementDesignView"},
		{"/requirements/{id}/design", "put", "RequirementDesignSaveRequest", "RequirementDesignView"},
		{"/requirements/{id}/design/commands", "post", "RequirementDesignCommandRequest", "RequirementDesignView"},
		{"/requirements/{id}/design/preview", "post", "RequirementDesignPreviewRequest", "RequirementDesignPreview"},
		{"/requirements/{id}/design/adopt", "post", "RequirementDesignAdoptRequest", "RequirementDesignAdoption"},
		{"/requirements/{id}/design/revisions", "get", "", "RequirementDesignRevisionList"},
		{"/requirements/{id}/design/adoptions", "get", "", "RequirementDesignAdoptionList"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		if route.input != "" {
			request(op, route.input)
		}
		if route.input == "RequirementDesignCommandRequest" || route.input == "RequirementDesignAdoptRequest" {
			browserOnly(op)
			op["x-required-project-authority"] = "current browser write and approval"
		}
		if route.input == "RequirementDesignPreviewRequest" {
			op["x-read-only"] = true
			op["description"] = "Read-only complete task delta preview, not a reservation or execution. The digest is recomputed during adoption."
		} else {
			op["description"] = "Requirement-bound immutable design revisions and reviewed task changes. Saving does not publish, adopt or start execution. Current authorization precedes every receipt; stable task keys prevent duplicate creation across revisions."
		}
		if route.output == "RequirementDesignRevisionList" || route.output == "RequirementDesignAdoptionList" {
			appendQuery(op, queryParameter("limit", "Bounded immutable history page.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Opaque tenant/project/requirement-bound cursor.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(schemas, model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, model := range []string{"RequirementDesignContent", "RequirementDesignTask", "RequirementDesignSaveRequest", "RequirementDesignCommandRequest", "RequirementDesignPreviewRequest", "RequirementDesignAdoptRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	toolContractMap(schemas, "RequirementDesignContent")["x-total-max-utf8-bytes"] = 16000
	for _, name := range []string{"product", "experience", "technical"} {
		field("RequirementDesignContent", name, map[string]any{"maxLength": 16000, "x-max-utf8-bytes": 16000})
	}
	field("RequirementDesignTask", "key", map[string]any{"pattern": "^[A-Za-z0-9_-]{1,64}$"})
	field("RequirementDesignTask", "title", map[string]any{"maxLength": 200, "x-max-utf8-bytes": 200})
	field("RequirementDesignTask", "description", map[string]any{"maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("RequirementDesignTask", "priority", map[string]any{"enum": []string{"urgent", "high", "medium", "low"}})
	field("RequirementDesignTask", "depends_on", map[string]any{"maxItems": 100, "uniqueItems": true})
	field("RequirementDesignTask", "acceptance_criteria", map[string]any{"maxItems": 20, "items": map[string]any{"type": "string", "maxLength": 1000, "x-max-utf8-bytes": 1000}})
	field("RequirementDesignTask", "covers", map[string]any{"maxItems": 100, "items": map[string]any{"type": "integer"}, "description": "1-based criteria indexes of the exact stored requirement version. Publication requires unique valid coverage; incomplete drafts expose diagnostics."})
	field("RequirementDesignSaveRequest", "tasks", map[string]any{"maxItems": 100, "x-max-json-bytes": 262144})
	field("RequirementDesignSaveRequest", "expected_version", map[string]any{"minimum": 0, "maximum": 2147483646})
	for _, model := range []string{"RequirementDesignCommandRequest", "RequirementDesignPreviewRequest", "RequirementDesignAdoptRequest"} {
		field(model, "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	for _, model := range []string{"RequirementDesignSaveRequest", "RequirementDesignCommandRequest"} {
		field(model, "expected_requirement_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	for _, model := range []string{"RequirementDesignSaveRequest", "RequirementDesignCommandRequest", "RequirementDesignAdoptRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, model := range []string{"RequirementDesignPreviewRequest", "RequirementDesignAdoptRequest"} {
		field(model, "revision", map[string]any{"minimum": 1, "maximum": 2147483647})
		field(model, "sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	}
	field("RequirementDesignAdoptRequest", "plan_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	field("RequirementDesignCommandRequest", "action", map[string]any{"enum": []string{"publish", "retire"}})
	field("RequirementDesignChange", "action", map[string]any{"enum": []string{"create", "update", "keep", "exclude"}})
	field("RequirementDesignView", "eligibility", map[string]any{"enum": []string{"none", "current", "requirement_changed", "retired", "publication_changed"}})
	field("TaskContext", "design", map[string]any{"readOnly": true, "description": "Resolved from protected task binding and immutable reviewed requirement design. Passed once, without the full task proposal. Complete goal remains <=32000 UTF8 bytes. New publications and task drift block fresh dispatch and acceptance of old design runs."})
}
