package main

import "testing"

func TestProjectKnowledgeContractKeepsApprovalAndFixedContext(t *testing.T) {
	api, _ := generate()
	command := mapAt(api, "paths", "/project-knowledge/{id}/commands", "post")
	security := command["security"].([]any)
	if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
		t.Fatal("publication must keep browser approval boundary")
	}
	if command["x-browser-original-view"] == nil {
		t.Fatal("publication lost original view")
	}
	create := mapAt(api, "paths", "/project-knowledge", "post")
	if len(create["security"].([]any)) != 2 {
		t.Fatal("legal project API editing was removed")
	}
	for _, model := range []string{"ProjectKnowledgeWriteRequest", "ProjectKnowledgeCommandRequest", "ProjectKnowledgeReference", "ProjectKnowledgeSourceInput"} {
		if mapAt(api, "components", "schemas", model)["additionalProperties"] != false {
			t.Fatal("unbounded trusted inputs", model)
		}
	}
	if mapAt(api, "components", "schemas", "TaskDispatchPreviewRequest", "properties", "knowledge")["maxItems"] != 16 {
		t.Fatal("unbounded context references")
	}
	provenance := mapAt(api, "components", "schemas", "ProjectKnowledgeContextItem", "properties", "sources", "items")
	if provenance["$ref"] != "#/components/schemas/ProjectKnowledgeContextSource" {
		t.Fatal("context duplicated source quotes", provenance)
	}
	if _, ok := mapAt(api, "components", "schemas", "ProjectKnowledgeContextSource", "properties")["quote"]; ok {
		t.Fatal("quote should remain in revision evidence, not repeated context")
	}
	query := mapAt(api, "paths", "/project-knowledge", "get")["parameters"].([]any)
	found := false
	for _, raw := range query {
		p := raw.(map[string]any)
		if p["name"] == "q" {
			found = true
			if p["schema"].(map[string]any)["x-max-utf8-bytes"] != 200 {
				t.Fatal("query limit disagrees with service")
			}
		}
	}
	if !found {
		t.Fatal("query contract missing")
	}
}
