package main

import "github.com/ai-mada/suma/internal/platform"

func skillCatalogModels() []any {
	return []any{platform.SkillCatalogOrigin{}, platform.SkillCatalogSummary{}, platform.SkillCatalogDetail{}, platform.SkillCatalogPage{}, platform.SkillCatalogWriteRequest{}, platform.SkillCatalogCommandRequest{}, platform.SkillCatalogAdoptRequest{}, platform.SkillCatalogRevisionPage{}, platform.SkillCatalogAdoption{}, platform.ApplicationSkillCatalogListInput{}, platform.ApplicationSkillCatalogGetInput{}}
}

func skillCatalogContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/skill-catalog", "get", "", "SkillCatalogPage", "200"},
		{"/skill-catalog", "post", "SkillCatalogWriteRequest", "SkillCatalogDetail", "201"},
		{"/skill-catalog/{id}", "get", "", "SkillCatalogDetail", "200"},
		{"/skill-catalog/{id}", "patch", "SkillCatalogWriteRequest", "SkillCatalogDetail", "200"},
		{"/skill-catalog/{id}/commands", "post", "SkillCatalogCommandRequest", "SkillCatalogDetail", "200"},
		{"/skill-catalog/{id}/adoptions", "post", "SkillCatalogAdoptRequest", "ProjectKnowledgeDetail", "201"},
		{"/skill-catalog/{id}/revisions", "get", "", "SkillCatalogRevisionPage", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["description"] = "Organization-reviewed skill text with immutable project publication pointers. Current project read is required for discovery; API credentials stay project scoped. Sharing/updating requires a browser organization administrator and current origin-project approval. Adoption requires browser target-project approval and creates a draft only. Source drift or retirement blocks new adoption; existing reviewed copies and runs remain immutable. Source quotes and original document metadata are not shared. Stable keys and exact CAS apply before replay. See contracts/skill-catalog.md."
		if route.input != "" {
			request(op, route.input)
			browserOnly(op)
		}
		if route.method == "get" && route.path == "/skill-catalog" {
			for _, filter := range []struct {
				name  string
				limit int
			}{{"domain", 80}, {"scenario", 80}, {"tag", 40}, {"q", 200}} {
				appendQuery(op, queryParameter(filter.name, "Bounded literal keyword or metadata filter; not semantic/vector similarity.", map[string]any{"type": "string", "maxLength": filter.limit}))
			}
			appendQuery(op, queryParameter("state", "Optional published or retired filter.", map[string]any{"type": "string", "enum": []string{"published", "retired"}}))
		}
		if route.method == "get" && (route.path == "/skill-catalog" || route.path == "/skill-catalog/{id}/revisions") {
			appendQuery(op, queryParameter("limit", "Exact integer page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100}))
			appendQuery(op, queryParameter("cursor", "Opaque current tenant/project/filter-bound cursor.", map[string]any{"type": "string", "maxLength": 4096}))
		}
	}
	for _, name := range []string{"SkillCatalogWriteRequest", "SkillCatalogCommandRequest", "SkillCatalogAdoptRequest", "ApplicationSkillCatalogListInput", "ApplicationSkillCatalogGetInput"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(api, "components", "schemas", model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, model := range []string{"SkillCatalogWriteRequest", "SkillCatalogCommandRequest", "SkillCatalogAdoptRequest"} {
		field(model, "expected_version", map[string]any{"minimum": 0, "maximum": 2147483646, "description": "Exact JSON integer lexeme. Create requires zero, all other mutations require the current positive version."})
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
		field(model, "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	}
	field("SkillCatalogCommandRequest", "action", map[string]any{"const": "retire"})
	field("SkillCatalogWriteRequest", "acknowledge_sharing", map[string]any{"const": true})
	field("SkillCatalogAdoptRequest", "acknowledge_draft", map[string]any{"const": true})
	for _, model := range []string{"ApplicationSkillCatalogGetInput", "SkillCatalogAdoptRequest"} {
		field(model, "origin_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	}
	field("ApplicationSkillCatalogGetInput", "version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("SkillCatalogWriteRequest", "published_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	field("SkillCatalogWriteRequest", "published_revision", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("SkillCatalogWriteRequest", "expected_knowledge_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	for _, model := range []string{"SkillCatalogSummary", "SkillCatalogDetail"} {
		field(model, "state", map[string]any{"enum": []string{"published", "retired"}})
	}
}
