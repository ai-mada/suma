package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestIntelligentOperationsRoutesSeparateReadOnlyPreviewFromWrites(t *testing.T) {
	api, types := generate()
	paths := mapAt(api, "paths")
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/intelligent-operations/analyses", "get", "", "IntelligentOperationPage", "200"},
		{"/intelligent-operations/analyses", "post", "IntelligentOperationCreateRequest", "IntelligentOperationAnalysis", "201"},
		{"/intelligent-operations/analyses/{id}", "get", "", "IntelligentOperationAnalysis", "200"},
		{"/intelligent-operations/analyses/{id}/preview", "post", "IntelligentOperationPreviewRequest", "IntelligentOperationPreview", "200"},
		{"/intelligent-operations/analyses/{id}/commands", "post", "IntelligentOperationCommandRequest", "IntelligentOperationAnalysis", "200"},
	} {
		if paths[route.path] == nil {
			t.Fatal("unified input route is absent", route.path)
		}
		op := mapAt(paths, route.path, route.method)
		if mapAt(op, "responses", route.status, "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.output || len(op["security"].([]any)) != 2 {
			t.Fatal("route lost its exact response or browser/project-API authority", route)
		}
		if route.input != "" && mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.input {
			t.Fatal("route uses a generic request envelope", route)
		}
	}
	preview := mapAt(api, "components", "schemas", "IntelligentOperationPreviewRequest")
	if !reflect.DeepEqual(preview["required"], []string{"project_id", "expected_version"}) || len(mapAt(preview, "properties")) != 2 || mapAt(paths, "/intelligent-operations/analyses/{id}/preview", "post")["x-read-only"] != true {
		t.Fatal("read-only preview acquired mutation fields or an operation key")
	}
	create := mapAt(api, "components", "schemas", "IntelligentOperationCreateRequest")
	if !reflect.DeepEqual(create["required"], []string{"project_id", "goal", "locale", "sources", "tool_profile_id", "expected_tool_version", "acknowledge_model_use", "idempotency_key"}) || mapAt(create, "properties", "sources")["maxItems"] != 8 || mapAt(create, "properties", "max_output_tokens")["maximum"] != 8192 {
		t.Fatal("creation lost its actual required fields or fixed bounds")
	}
	if !strings.Contains(types, "export interface IntelligentOperationAnalysis {") || !strings.Contains(types, "export interface IntelligentOperationsCapability {") {
		t.Fatal("current domain or reflected tool capability is missing from generated types")
	}
}

func TestIntelligentOperationsSchemasCloseActionPayloadsAndCommands(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"IntelligentOperationCreateRequest", "IntelligentOperationCommandRequest", "IntelligentOperationPreviewRequest", "IntelligentOperationOutput", "IntelligentOperationStep", "IntelligentOperationRequirement", "IntelligentOperationDesign", "IntelligentOperationKnowledge", "IntelligentOperationsCapability"} {
		if schemas[name] == nil || mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("closed intelligent operations schema is absent or accepts unknown fields", name)
		}
	}
	step := mapAt(schemas, "IntelligentOperationStep")
	branches, ok := step["oneOf"].([]any)
	if !ok || len(branches) != 3 {
		t.Fatal("typed actions are not mutually exclusive")
	}
	for index, action := range []string{"requirement.create_draft", "requirement.design.save_draft", "knowledge.create_draft"} {
		payload := []string{"requirement", "design", "knowledge"}[index]
		branch := branches[index].(map[string]any)
		if mapAt(branch, "properties", "action")["const"] != action || !reflect.DeepEqual(branch["required"], []string{payload}) || branch["not"] == nil || mapAt(step, "properties", payload)["$ref"] == nil {
			t.Fatal("action can carry an absent, null or foreign typed payload", action)
		}
	}
	command := mapAt(schemas, "IntelligentOperationCommandRequest")
	commands, ok := command["oneOf"].([]any)
	if !ok || len(commands) != 3 || mapAt(commands[1], "properties", "acknowledge_drafts")["const"] != true || mapAt(commands[1], "properties", "preview_sha256")["pattern"] != "^[a-f0-9]{64}$" {
		t.Fatal("apply lost its explicit fixed-preview acknowledgement")
	}
	if mapAt(command, "properties", "proposal")["$ref"] != "#/components/schemas/IntelligentOperationOutput" || mapAt(schemas, "IntelligentOperationOutput")["x-max-json-utf8-bytes"] != 32000 || mapAt(schemas, "IntelligentOperationKnowledge", "properties", "source_indices", "items")["minimum"] != 0 {
		t.Fatal("proposal is nullable, unbounded or uses one-based knowledge indices")
	}
	if mapAt(schemas, "IntelligentOperationPreview", "properties", "policy")["const"] != "suma-local-drafts-v1" || mapAt(schemas, "IntelligentOperationStepResult", "properties", "resource_id")["readOnly"] != true {
		t.Fatal("fixed local-draft policy or immutable result authority is missing")
	}
}
