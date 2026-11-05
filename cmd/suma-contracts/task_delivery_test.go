package main

import (
	"slices"
	"strings"
	"testing"

	cc "github.com/ai-mada/suma/internal/candidatecontract"
)

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

func TestTaskDeliveryRepositoryIntakeContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", taskDeliveryRepositoryIntakePath, "post")
	if mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskDeliveryRepositoryIntakeRequest" ||
		mapAt(op, "responses", "201", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskDeliveryRepositoryIntake" ||
		mapAt(op, "responses")["200"] != nil {
		t.Fatal("repository intake must return its typed 201 receipt")
	}
	security := op["security"].([]any)
	if len(security) != 1 || mapAt(security[0])["cookieAuth"] == nil {
		t.Fatal("repository intake must be browser-only")
	}
	for _, raw := range op["parameters"].([]any) {
		if mapAt(raw)["in"] == "query" {
			t.Fatal("repository intake must reject query selectors")
		}
	}
	request := mapAt(api, "components", "schemas", "TaskDeliveryRepositoryIntakeRequest")
	want := []string{"project_id", "task_version", "run_id", "target_id", "target_version", "expected_head", "expected_base", "idempotency_key"}
	required := request["required"].([]string)
	if request["additionalProperties"] != false || len(required) != len(want) || len(mapAt(request, "properties")) != len(want) {
		t.Fatal("repository intake request must require the exact scoped fields")
	}
	for _, field := range want {
		if !slices.Contains(required, field) {
			t.Fatalf("repository intake request lost %q", field)
		}
	}
	receipt := mapAt(api, "components", "schemas", "TaskDeliveryRepositoryIntake")
	if receipt["additionalProperties"] != false || len(mapAt(receipt, "properties")) != 14 {
		t.Fatal("repository intake receipt leaked fields outside its public summary")
	}
}

func TestTaskDeliveryRepositoryDispatchContract(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	preview := mapAt(paths, taskDeliveryRepositoryDispatchPath, "post")
	const model = "TaskDeliveryRepositoryDispatchPreviewRequest"
	if mapAt(preview, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+model ||
		mapAt(preview, "responses", "201", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskDispatchPlan" || mapAt(preview, "responses")["200"] != nil {
		t.Fatal("repository source preview lost its typed plan contract")
	}
	security := preview["security"].([]any)
	if len(security) != 1 || mapAt(security[0])["cookieAuth"] == nil || !strings.Contains(preview["description"].(string), "browser project approver") {
		t.Fatal("repository source preview must require a browser project approver")
	}
	pathFields := map[string]bool{"id": true, "intake": true}
	for _, raw := range preview["parameters"].([]any) {
		field := mapAt(raw)
		if field["in"] == "query" {
			t.Fatal("repository source preview accepts no query")
		}
		if field["in"] == "path" {
			name := field["name"].(string)
			if !pathFields[name] || field["required"] != true {
				t.Fatal("unexpected repository source preview path parameter", field)
			}
			delete(pathFields, name)
		}
	}
	if len(pathFields) != 0 || mapAt(preview, "responses")["428"] == nil {
		t.Fatal("repository source preview lost its exact path or original browser view")
	}
	request := mapAt(api, "components", "schemas", model)
	fields := []string{"project_id", "expected_task_version", "producer_profile_id", "expected_producer_profile_version", "test_profile_id", "expected_test_profile_version", "idempotency_key"}
	if request["additionalProperties"] != false || len(request["required"].([]string)) != len(fields) || len(mapAt(request, "properties")) != len(fields) {
		t.Fatal("repository source preview must have a closed body")
	}
	for _, field := range fields {
		if !slices.Contains(request["required"].([]string), field) {
			t.Fatal("missing required source preview field", field)
		}
	}
	start := mapAt(paths, "/task-dispatch-plans/{id}/start", "post")
	if !strings.Contains(start["description"].(string), "current browser project approval on initial start and receipt recovery") {
		t.Fatal("task dispatch start must document conditional repository source approval")
	}

	for _, route := range []struct{ namespace, security, scope string }{{"runner", "runnerAuth", "runner"}, {"pool-runner", "poolRunnerAuth", "pool"}} {
		path := "/" + route.namespace + "/repository-source-intakes/{id}/bundle"
		bundle := mapAt(paths, path, "get")
		security := bundle["security"].([]any)
		if len(security) != 1 || len(mapAt(security[0])) != 1 || mapAt(security[0])[route.security] == nil || bundle["x-required-token-scope"] != route.scope || bundle["requestBody"] != nil {
			t.Fatal("repository bundle lost its exact machine authority", path)
		}
		content := mapAt(bundle, "responses", "200", "content")
		format := mapAt(content, "application/octet-stream", "schema")
		if len(content) != 1 || format["format"] != "binary" || format["maxLength"] != cc.MaxBundleBytes || mapAt(bundle, "responses")["201"] != nil {
			t.Fatal("repository bundle no longer advertises bounded binary bytes", path)
		}
		expected := map[string]string{"id": "path", "project_id": "query", "X-Suma-Assignment": "header"}
		for _, raw := range bundle["parameters"].([]any) {
			field := mapAt(raw)
			name := field["name"].(string)
			if field["required"] != true && field["in"] == "header" {
				continue // Shared optional presentation headers.
			}
			if expected[name] != field["in"] || field["required"] != true {
				t.Fatal("unexpected repository source bundle parameter", path, field)
			}
			delete(expected, name)
		}
		if len(expected) != 0 {
			t.Fatal("repository source bundle is missing required scope fields", path, expected)
		}
	}
}
