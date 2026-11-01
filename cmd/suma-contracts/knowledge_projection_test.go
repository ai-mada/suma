package main

import (
	"reflect"
	"strings"
	"testing"

	ic "github.com/ai-mada/suma/internal/indexcontract"
)

func TestKnowledgeProjectionContractsKeepNamespaceCurrentReferencesAndCoverage(t *testing.T) {
	api, types := generate()
	if wireModelName(reflect.TypeOf(ic.Source{})) != "KnowledgeIndexSource" || ts(reflect.TypeOf(ic.Locator{})) != "KnowledgeIndexLocator" {
		t.Fatal("index domain namespace was lost")
	}
	for _, route := range []string{"/project-knowledge/index-search", "/project-knowledge/index-status"} {
		op := mapAt(api, "paths", route, "get")
		if op["x-read-only"] != true || op["x-response-cache-control"] != "no-store" {
			t.Fatal("indexed knowledge read acquired effects or stale cache", route)
		}
	}
	hit := mapAt(api, "components", "schemas", "ProjectKnowledgeIndexHit", "properties")
	if hit["reference"].(map[string]any)["$ref"] != "#/components/schemas/ProjectKnowledgeReference" || hit["source"].(map[string]any)["$ref"] != "#/components/schemas/KnowledgeIndexSource" || hit["locator"].(map[string]any)["$ref"] != "#/components/schemas/KnowledgeIndexLocator" || hit["snippet"].(map[string]any)["x-max-utf8-bytes"] != 600 {
		t.Fatal("indexed hit did not keep exact reference, domain source and canonical location")
	}
	status := mapAt(api, "components", "schemas", "ProjectKnowledgeIndexStatus", "properties")
	if len(status["latest_event"].(map[string]any)["anyOf"].([]any)) != 2 || len(status["last_ack"].(map[string]any)["anyOf"].([]any)) != 2 || status["coverage"].(map[string]any)["const"] != "outbox_delivery_only" {
		t.Fatal("unknown delivery or bounded coverage became fake readiness")
	}
	if _, leaked := mapAt(api, "components", "schemas", "ProjectKnowledgeProjectionEvent", "properties")["content"]; leaked {
		t.Fatal("status shipped full source text")
	}
	for _, field := range []string{"total", "next_cursor", "score"} {
		if _, found := mapAt(api, "components", "schemas", "ProjectKnowledgeIndexSearch", "properties")[field]; found {
			t.Fatal("provider candidates became a whole-project claim", field)
		}
	}
	if mapAt(api, "components", "schemas", "KnowledgeIndexLocator", "properties", "unit")["const"] != "unicode_codepoints" || mapAt(api, "components", "schemas", "KnowledgeIndexReceipt", "properties", "request_sha256")["pattern"] != "^[a-f0-9]{64}$" {
		t.Fatal("range units or exact raw request receipt missing")
	}
	if len(mapAt(api, "components", "schemas", "KnowledgeIndexSource")["oneOf"].([]any)) != 2 || hit["source"].(map[string]any)["properties"].(map[string]any)["revision"].(map[string]any)["minimum"] != 1 {
		t.Fatal("unpublished tombstone marker became a valid searchable publication")
	}
	local, err := knowledgeIndexRuntimeTypes(types)
	if err != nil || strings.Count(string(local), "export interface ") != 7 || strings.Contains(string(local), "ProjectKnowledge") || !strings.Contains(string(local), "source: KnowledgeIndexSource;") || !strings.Contains(string(local), `"lancedb_hybrid"`) {
		t.Fatal("native package stopped sharing the authoritative seven wire types", err)
	}
}
