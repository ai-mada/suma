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
}
