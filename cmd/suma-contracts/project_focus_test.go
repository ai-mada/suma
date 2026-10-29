package main

import "testing"

func TestProjectFocusContractPreservesBoundedReadFacts(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/project-focus", "get")
	if op["requestBody"] != nil || op["x-read-only"] != true || mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/ProjectFocus" {
		t.Fatal("focus must be a typed read without commands")
	}
	if mapAt(api, "components", "schemas", "ProjectFocusSection", "properties", "items")["maxItems"] != 6 {
		t.Fatal("compact focus lost its server bound")
	}
	properties := mapAt(api, "components", "schemas", "ProjectFocusItem", "properties")
	for _, forbidden := range []string{"output", "logs", "command", "credential", "can_review", "tests_passed", "progress_percent"} {
		if properties[forbidden] != nil {
			t.Fatal("focus may not invent eligibility or expose private execution data", forbidden)
		}
	}
}
