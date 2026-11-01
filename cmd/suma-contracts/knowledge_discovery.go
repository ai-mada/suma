package main

import "github.com/ai-mada/suma/internal/platform"

func knowledgeDiscoveryModels() []any {
	return []any{platform.ProjectKnowledgeDiscoveryQuery{}, platform.ProjectKnowledgeDiscoveryHit{}, platform.ProjectKnowledgeDiscovery{}}
}

// Discovery is a publication-only read contract. It deliberately does not
// redefine the management directory, historical revision API or semantic index.
func knowledgeDiscoveryContracts(add operationAdder, api map[string]any) {
	op := add("/project-knowledge/discovery", "get", "ProjectKnowledgeDiscovery", false, false)
	op["description"] = "Discover currently eligible published memory, SPEC and skills by literal Unicode keyword, or browse with an empty query. Latest drafts never supply result text. Every hit is reauthorized against the current publication and sources. A current-read keyset traverses creation order; it is not a cross-request snapshot or semantic index. Explicitly preview and select the exact reference through the existing task contract."
	op["x-read-only"], op["x-response-cache-control"] = true, "no-store"
	op["x-query-unknown-fields"] = "reject"
	op["x-no-side-effects"] = "No publication, model call, adoption, task start, tool execution or permission grant."
	op["x-max-response-utf8-bytes"] = 48 * 1024
	appendQuery(op, queryParameter("q", "Literal published title/summary/content substring; empty browses. Trimmed and Unicode-lowercased; never matches JSON encoding, sources or latest drafts.", map[string]any{"type": "string", "maxLength": 200, "x-max-utf8-bytes": 200, "x-reject-control-characters": true}))
	appendQuery(op, queryParameter("kind", "Optional publication kind.", map[string]any{"type": "string", "enum": []string{"memory", "spec", "skill"}}))
	appendQuery(op, queryParameter("limit", "Maximum returned hits, not a full-project total. Canonical positive decimal only.", map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 10, "x-canonical-decimal": true}))
	appendQuery(op, queryParameter("cursor", "Opaque current actor/organization/project/query/kind/limit/order-bound continuation. Empty starts a new traversal. Changes in permissions or publication are checked again on every page.", map[string]any{"type": "string", "maxLength": 2048, "x-max-utf8-bytes": 2048}))
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ProjectKnowledgeDiscoveryQuery", "ProjectKnowledgeDiscoveryHit", "ProjectKnowledgeDiscovery"} {
		toolContractMap(schemas, name)["additionalProperties"] = false
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(schemas, model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	field("ProjectKnowledgeDiscoveryQuery", "q", map[string]any{"maxLength": 200, "x-max-utf8-bytes": 200, "x-reject-control-characters": true})
	field("ProjectKnowledgeDiscoveryQuery", "kind", map[string]any{"enum": []string{"memory", "spec", "skill"}})
	field("ProjectKnowledgeDiscoveryQuery", "limit", map[string]any{"minimum": 1, "maximum": 20, "default": 10})
	field("ProjectKnowledgeDiscoveryQuery", "cursor", map[string]any{"maxLength": 2048, "x-max-utf8-bytes": 2048})
	field("ProjectKnowledgeDiscoveryHit", "kind", map[string]any{"enum": []string{"memory", "spec", "skill"}})
	field("ProjectKnowledgeDiscoveryHit", "matched_field", map[string]any{"enum": []string{"none", "title", "summary", "content"}, "description": "none only when browsing with an empty normalized query. Other values identify the first matching published field."})
	field("ProjectKnowledgeDiscoveryHit", "title", map[string]any{"maxLength": 200, "x-max-utf8-bytes": 200})
	field("ProjectKnowledgeDiscoveryHit", "summary", map[string]any{"maxLength": 1000, "x-max-utf8-bytes": 1000})
	field("ProjectKnowledgeDiscoveryHit", "snippet", map[string]any{"maxLength": 600, "x-max-utf8-bytes": 600, "description": "Compact publication excerpt; exact full text and provenance remain available through the original revision API."})
	field("ProjectKnowledgeDiscoveryHit", "source_count", map[string]any{"minimum": 0, "maximum": 8})
	field("ProjectKnowledgeDiscoveryHit", "published_at", map[string]any{"format": "date-time"})
	field("ProjectKnowledgeDiscovery", "mode", map[string]any{"const": "keyword"})
	field("ProjectKnowledgeDiscovery", "scope", map[string]any{"const": "current_project_publications"})
	field("ProjectKnowledgeDiscovery", "items", map[string]any{"maxItems": 20})
	field("ProjectKnowledgeDiscovery", "scanned", map[string]any{"minimum": 0, "maximum": 200})
	field("ProjectKnowledgeDiscovery", "scan_limit", map[string]any{"const": 200})
	field("ProjectKnowledgeDiscovery", "next_cursor", map[string]any{"maxLength": 2048, "x-max-utf8-bytes": 2048})
	field("ProjectKnowledgeDiscovery", "exhausted", map[string]any{"description": "Current traversal reached its remaining tail. An empty non-exhausted page is partial and must offer continuation, never a whole-project empty conclusion."})
	result := toolContractMap(schemas, "ProjectKnowledgeDiscovery")
	result["x-max-response-utf8-bytes"] = 48 * 1024
	result["allOf"] = []any{map[string]any{
		"if":   map[string]any{"properties": map[string]any{"exhausted": map[string]any{"const": true}}},
		"then": map[string]any{"properties": map[string]any{"next_cursor": map[string]any{"const": ""}}},
		"else": map[string]any{"properties": map[string]any{"next_cursor": map[string]any{"minLength": 1}}},
	}}
}
