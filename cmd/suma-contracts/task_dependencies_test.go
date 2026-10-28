package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestTaskDependencyGraphAndAtomicChangeContracts(t *testing.T) {
	api, types := generate()
	for name, fields := range map[string][]string{
		"TaskDependencyGraph":         {"project_id", "total", "limit", "complete", "tasks"},
		"TaskDependencyNode":          {"task_id", "version", "title", "status", "depends_on"},
		"TaskDependencyChangeRequest": {"project_id", "changes", "idempotency_key"},
		"TaskDependencyChange":        {"task_id", "expected_version", "depends_on"},
		"TaskDependencyChangeResult":  {"project_id", "operation_id", "changes"},
		"TaskDependencyChangeReceipt": {"task_id", "previous_version", "version", "depends_on"},
	} {
		model := mapAt(api, "components", "schemas", name)
		required := append([]string(nil), model["required"].([]string)...)
		sort.Strings(required)
		sort.Strings(fields)
		if model["additionalProperties"] != false || len(mapAt(model, "properties")) != len(fields) || !reflect.DeepEqual(required, fields) {
			t.Fatalf("%s lost its exact required contract: %v", name, model)
		}
		if !strings.Contains(types, "export interface "+name+" {") {
			t.Fatalf("%s is missing from generated TypeScript", name)
		}
	}
	for method, model := range map[string]string{"get": "TaskDependencyGraph", "post": "TaskDependencyChangeResult"} {
		op := mapAt(api, "paths", "/task-dependencies", method)
		if mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+model || mapAt(op, "responses")["201"] != nil {
			t.Fatalf("%s must return the sole typed 200 response", method)
		}
	}
	read := mapAt(api, "paths", "/task-dependencies", "get")
	projectRequired := false
	for _, raw := range read["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] == "query" {
			if parameter["name"] != "project_id" || parameter["required"] != true {
				t.Fatalf("graph GET has an unsupported or optional scope query: %v", parameter)
			}
			projectRequired = true
		}
	}
	if !projectRequired {
		t.Fatal("graph GET lost required project scope")
	}
	write := mapAt(api, "paths", "/task-dependencies", "post")
	if mapAt(write, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/TaskDependencyChangeRequest" {
		t.Fatal("atomic dependency write lost its typed request")
	}
	headers := map[string]bool{}
	for _, raw := range write["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] != "header" {
			t.Fatalf("dependency write permits a second scope source: %v", parameter)
		}
		headers[parameter["name"].(string)] = true
	}
	for _, name := range []string{"X-Suma-Expected-User", "X-Suma-Expected-Organization"} {
		if !headers[name] {
			t.Fatalf("dependency write lost original-view header %s", name)
		}
	}
	graph := mapAt(api, "components", "schemas", "TaskDependencyGraph")
	if mapAt(graph, "properties", "limit")["const"] != 1000 || mapAt(graph, "properties", "tasks")["maxItems"] != 1000 {
		t.Fatal("complete graph read no longer has the fixed 1000-task boundary")
	}
	condition := graph["allOf"].([]any)[0].(map[string]any)
	if mapAt(condition, "if", "properties", "complete")["const"] != false || mapAt(condition, "then", "properties", "tasks")["maxItems"] != 0 {
		t.Fatal("overflow can masquerade as a partial authoritative graph")
	}
	changes := mapAt(api, "components", "schemas", "TaskDependencyChangeRequest", "properties", "changes")
	if changes["minItems"] != 1 || changes["maxItems"] != 100 {
		t.Fatal("atomic dirty-task batch lost its explicit bounds")
	}
	for _, model := range []string{"TaskDependencyNode", "TaskDependencyChange", "TaskDependencyChangeReceipt"} {
		dependencies := mapAt(api, "components", "schemas", model, "properties", "depends_on")
		if dependencies["type"] != "array" || dependencies["uniqueItems"] != true || mapAt(dependencies, "items")["minLength"] != 1 {
			t.Fatalf("%s predecessors permit ambiguous, empty or duplicate identifiers", model)
		}
	}
	version := mapAt(api, "components", "schemas", "TaskDependencyChange", "properties", "expected_version")
	if version["minimum"] != 1 || version["maximum"] != 2147483647 {
		t.Fatal("dependency CAS input no longer matches exactVersion")
	}
}
