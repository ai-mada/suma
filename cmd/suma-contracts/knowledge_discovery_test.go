package main

import (
	"reflect"
	"testing"
)

func TestKnowledgeDiscoveryContractKeepsPublicationReferenceAndPartialCoverage(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/project-knowledge/discovery", "get")
	if op["x-read-only"] != true || op["x-response-cache-control"] != "no-store" || op["x-max-response-utf8-bytes"] != 48*1024 {
		t.Fatal("discovery lost read-only or compact output boundary")
	}
	parameters := map[string]map[string]any{}
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		parameters[p["name"].(string)] = p
	}
	if parameters["project_id"]["required"] != true || parameters["q"]["schema"].(map[string]any)["x-max-utf8-bytes"] != 200 || parameters["limit"]["schema"].(map[string]any)["maximum"] != 20 || parameters["cursor"]["schema"].(map[string]any)["maxLength"] != 2048 {
		t.Fatal("query contract does not match the bounded current-project reader")
	}
	for _, name := range []string{"ProjectKnowledgeDiscovery", "ProjectKnowledgeDiscoveryHit", "ProjectKnowledgeDiscoveryQuery"} {
		if mapAt(api, "components", "schemas", name)["additionalProperties"] != false {
			t.Fatal("open discovery shape", name)
		}
	}
	hit := mapAt(api, "components", "schemas", "ProjectKnowledgeDiscoveryHit", "properties")
	if hit["reference"].(map[string]any)["$ref"] != "#/components/schemas/ProjectKnowledgeReference" || hit["snippet"].(map[string]any)["x-max-utf8-bytes"] != 600 {
		t.Fatal("discovery invented a reference or an unbounded body")
	}
	for _, unwanted := range []string{"latest", "content", "sources", "score", "published_eligible"} {
		if _, found := hit[unwanted]; found {
			t.Fatal("hit leaked draft, full evidence or implied semantic score", unwanted)
		}
	}
	result := mapAt(api, "components", "schemas", "ProjectKnowledgeDiscovery")
	properties := result["properties"].(map[string]any)
	for _, unwanted := range []string{"total", "index_watermark", "score"} {
		if _, found := properties[unwanted]; found {
			t.Fatal("bounded scan claims global or indexed coverage", unwanted)
		}
	}
	if properties["scan_limit"].(map[string]any)["const"] != 200 || properties["mode"].(map[string]any)["const"] != "keyword" || len(result["allOf"].([]any)) != 1 {
		t.Fatal("partial continuation semantics are missing")
	}
	if !reflect.DeepEqual(hit["matched_field"].(map[string]any)["enum"], []string{"none", "title", "summary", "content"}) {
		t.Fatal("browse and keyword matches were conflated")
	}
	// The pre-existing directory and historical read keep their distinct
	// contracts; adding discovery must not turn latest drafts into publications.
	directory := mapAt(api, "paths", "/project-knowledge", "get", "responses", "200", "content", "application/json", "schema")
	if directory["$ref"] != "#/components/schemas/ProjectKnowledgeList" {
		t.Fatal("management directory replaced by discovery")
	}
}
