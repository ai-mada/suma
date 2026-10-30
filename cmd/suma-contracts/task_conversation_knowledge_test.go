package main

import "testing"

func TestTaskConversationKnowledgeContractKeepsPinsAndHistoricalReadDistinct(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, field := range [][2]string{{"TaskConversationAppendRequest", "knowledge"}, {"TaskConversationMessage", "knowledge"}, {"TaskConversationReplySource", "knowledge"}} {
		model := mapAt(schemas, field[0])
		if mapAt(model, "properties", field[1])["maxItems"] != 16 {
			t.Fatal("knowledge context must remain bounded", field)
		}
		for _, required := range model["required"].([]string) {
			if required == field[1] {
				t.Fatal("no-knowledge immutable sources must omit optional fields", field)
			}
		}
	}
	for _, name := range []string{"ProjectKnowledgeReference", "TaskConversationKnowledgeReference", "TaskConversationAppendRequest"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("caller text cannot become a trusted knowledge snapshot", name)
		}
	}
	op := mapAt(api, "paths", "/project-knowledge/{id}/revisions/{revision}", "get")
	if op["x-read-only"] != true || len(op["security"].([]any)) != 2 || len(mapAt(api, "paths", "/project-knowledge/{id}/revisions/{revision}")) != 1 {
		t.Fatal("exact history must use only current read authority")
	}
	found := false
	for _, raw := range op["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "revision" && p["in"] == "path" {
			found = true
			s := p["schema"].(map[string]any)
			if s["type"] != "integer" || s["minimum"] != 1 || s["maximum"] != 2147483647 || s["x-canonical-decimal"] != true {
				t.Fatal("revision path must match the exact canonical runtime parser", s)
			}
		}
	}
	if !found {
		t.Fatal("exact revision parameter missing")
	}
}
