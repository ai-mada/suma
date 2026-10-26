package main

import (
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func knowledgeIntelligenceModels() []any {
	return []any{platform.KnowledgeIntelligenceOutput{}, platform.KnowledgeIntelligenceSource{}, platform.KnowledgeIntelligenceAnalysis{}, platform.KnowledgeIntelligenceCreateRequest{}, platform.KnowledgeIntelligenceCommandRequest{}, platform.KnowledgeIntelligencePage{}, platform.KnowledgeIntelligenceAdoption{}, toolcontract.KnowledgeExtractionCapability{}}
}

func knowledgeIntelligenceContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/knowledge-intelligence/analyses", "get", "", "KnowledgeIntelligencePage", "200"},
		{"/knowledge-intelligence/analyses", "post", "KnowledgeIntelligenceCreateRequest", "KnowledgeIntelligenceAnalysis", "201"},
		{"/knowledge-intelligence/analyses/{id}", "get", "", "KnowledgeIntelligenceAnalysis", "200"},
		{"/knowledge-intelligence/analyses/{id}/commands", "post", "KnowledgeIntelligenceCommandRequest", "KnowledgeIntelligenceAnalysis", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		if route.input != "" {
			request(op, route.input)
			browserOnly(op)
		}
		op["description"] = "Current-authorized model-assisted knowledge proposals. Create freezes real sources and a dedicated read-only model profile into one managed DAG execution. No tools or automatic retry. Raw output is preserved independently of schema validity and later source drift. Explicit adoption creates a project draft only; ordinary project approval is still required for publication. All writes use exact CAS and stable operation keys."
		if route.path == "/knowledge-intelligence/analyses" && route.method == "get" {
			appendQuery(op, queryParameter("limit", "Bounded exact integer page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
			appendQuery(op, queryParameter("before", "Opaque project analysis boundary ID.", map[string]any{"type": "string", "maxLength": 64}))
		}
	}
	field := func(model, name string, extra map[string]any) {
		p := toolContractMap(api, "components", "schemas", model, "properties", name)
		for k, v := range extra {
			p[k] = v
		}
	}
	for _, name := range []string{"KnowledgeIntelligenceCreateRequest", "KnowledgeIntelligenceCommandRequest", "KnowledgeIntelligenceOutput", "KnowledgeExtractionCapability"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	field("KnowledgeExtractionCapability", "protocol", map[string]any{"const": toolcontract.KnowledgeExtractionProtocol})
	field("KnowledgeExtractionCapability", "model", map[string]any{"minLength": 1, "maxLength": 200})
	field("KnowledgeExtractionCapability", "endpoint_origin", map[string]any{"minLength": 1, "maxLength": 2000})
	for _, model := range []string{"KnowledgeIntelligenceAnalysis", "KnowledgeIntelligenceCreateRequest"} {
		field(model, "kind", map[string]any{"enum": []string{"memory", "spec", "skill"}})
		field(model, "locale", map[string]any{"enum": []string{"zh-CN", "en-US"}})
		field(model, "max_steps", map[string]any{"const": 1, "default": 1})
		field(model, "max_output_tokens", map[string]any{"minimum": 1, "maximum": 8192, "default": 4096})
	}
	field("KnowledgeIntelligenceCreateRequest", "sources", map[string]any{"minItems": 1, "maxItems": 8})
	field("KnowledgeIntelligenceCreateRequest", "acknowledge_model_use", map[string]any{"const": true})
	field("KnowledgeIntelligenceCreateRequest", "goal", map[string]any{"minLength": 1, "maxLength": 4000, "x-max-utf8-bytes": 4000})
	field("KnowledgeIntelligenceCommandRequest", "action", map[string]any{"enum": []string{"revise", "adopt", "dismiss"}})
	field("KnowledgeIntelligenceCommandRequest", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("KnowledgeIntelligenceCommandRequest", "reason", map[string]any{"minLength": 1, "maxLength": 2000, "x-max-utf8-bytes": 2000})
	field("KnowledgeIntelligenceCommandRequest", "acknowledge_draft", map[string]any{"description": "Must be true for explicit adopt; not publication approval."})
	for _, model := range []string{"KnowledgeIntelligenceCreateRequest", "KnowledgeIntelligenceCommandRequest"} {
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	field("KnowledgeIntelligenceAnalysis", "state", map[string]any{"enum": []string{"running", "ready", "failed", "reconciling", "adopted", "dismissed"}})
	field("KnowledgeIntelligenceOutput", "schema_version", map[string]any{"const": 1})
	for name, limit := range map[string]int{"title": 200, "summary": 1000, "content": 16000, "rationale": 8000} {
		field("KnowledgeIntelligenceOutput", name, map[string]any{"maxLength": limit, "x-max-utf8-bytes": limit})
	}
	field("KnowledgeIntelligenceOutput", "source_indices", map[string]any{"minItems": 1, "maxItems": 8, "uniqueItems": true, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": 7}, "description": "Zero-based subset of the frozen source list. Never caller-supplied source snapshots or authority."})
}
