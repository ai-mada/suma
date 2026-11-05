package main

import "testing"

func TestTaskDeliveryBoundedReadContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/tasks/{id}/delivery", "get")
	if mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskDelivery" || op["requestBody"] != nil {
		t.Fatal("delivery must remain a typed read-only endpoint")
	}
	for _, name := range []string{"TaskDeliveryNodeSummary", "TaskDeliveryExpertPlans", "TaskDeliveryIssues"} {
		model := mapAt(api, "components", "schemas", name)
		if model["additionalProperties"] != false || mapAt(model, "properties", "items")["maxItems"] != 20 || mapAt(model, "properties", "limit")["const"] != 20 {
			t.Fatalf("%s lost bounded snapshot contract", name)
		}
	}
	properties := mapAt(api, "components", "schemas", "TaskDeliveryRun", "properties")
	for _, forbidden := range []string{"output", "logs", "command", "config", "credential", "can_review"} {
		if properties[forbidden] != nil {
			t.Fatalf("delivery read gained an unrelated field: %s", forbidden)
		}
	}
	trace := mapAt(properties, "workbuddy_delivery")
	if trace["$ref"] != "#/components/schemas/TaskDeliveryRunTarget" || trace["anyOf"] != nil {
		t.Fatal("WorkBuddy trace must be a non-null target when present")
	}
	target := mapAt(api, "components", "schemas", "TaskDeliveryRunTarget")
	if target["additionalProperties"] != false || len(mapAt(target, "properties")) != 3 || mapAt(target, "properties", "attempt")["minimum"] != 1 {
		t.Fatal("WorkBuddy trace leaked non-routing fields or allows unstarted attempts")
	}
	repository := mapAt(api, "paths", "/tasks/{id}/delivery/repository-source", "get")
	if mapAt(repository, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskDeliveryRepositorySource" || repository["requestBody"] != nil {
		t.Fatal("Repository observation must remain a typed read-only endpoint")
	}
	expected := map[string]string{"project_id": "query", "task_version": "query", "run_id": "query", "target_id": "query", "target_version": "query", "id": "path"}
	for _, raw := range repository["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] == "header" {
			continue // Shared authorization and locale headers are added later.
		}
		name := parameter["name"].(string)
		if expected[name] == "" || expected[name] != parameter["in"] || parameter["required"] != true {
			t.Fatalf("Unexpected repository observation selector: %v", parameter)
		}
		delete(expected, name)
	}
	if len(expected) != 0 {
		t.Fatalf("Missing repository observation selectors: %v", expected)
	}
	observed := mapAt(api, "components", "schemas", "TaskDeliveryRepositorySource")
	if observed["additionalProperties"] != false || len(mapAt(observed, "properties")) != 11 {
		t.Fatal("Repository observation leaked non-evidence fields")
	}
}
