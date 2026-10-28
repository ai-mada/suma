package main

import "testing"

func TestRequirementTraceabilityReadContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/requirements/{id}/traceability", "get")
	if mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/RequirementTraceability" || op["requestBody"] != nil {
		t.Fatal("requirement traceability must remain one typed authorized read")
	}
	for _, name := range []string{"RequirementTraceability", "RequirementTraceabilityCriterion", "RequirementTraceabilityTask"} {
		if mapAt(api, "components", "schemas", name)["additionalProperties"] != false {
			t.Fatalf("%s must reject unrelated execution authority", name)
		}
	}
	properties := mapAt(api, "components", "schemas", "RequirementTraceability", "properties")
	if mapAt(properties, "tasks")["maxItems"] != 100 || mapAt(properties, "criteria")["maxItems"] != 100 {
		t.Fatal("published task projection lost its complete bounded scope")
	}
	for _, name := range []string{"logs", "commands", "config", "credentials", "can_review", "test_case_coverage"} {
		if properties[name] != nil {
			t.Fatalf("read must not invent coverage or expose private data: %s", name)
		}
	}
}
