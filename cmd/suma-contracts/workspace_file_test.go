package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestWorkspaceFileTypedContextContract(t *testing.T) {
	api, types := generate()
	for name, fields := range map[string][]string{
		"WorkspaceFileDocument":    {"project_id", "path", "content", "hash", "language", "workspace_name", "context_sha256"},
		"WorkspaceFileSaveRequest": {"project_id", "path", "content", "expected_hash", "expected_context_sha256", "idempotency_key"},
	} {
		model := mapAt(api, "components", "schemas", name)
		properties := mapAt(model, "properties")
		required := append([]string(nil), model["required"].([]string)...)
		sort.Strings(required)
		sort.Strings(fields)
		if model["additionalProperties"] != false || len(properties) != len(fields) || !reflect.DeepEqual(required, fields) {
			t.Fatalf("%s lost its exact required fields: %v", name, model)
		}
		for _, field := range fields {
			if mapAt(properties, field)["type"] != "string" || !strings.Contains(types, "  "+field+": string;") {
				t.Fatalf("%s.%s is no longer a required string", name, field)
			}
		}
	}
	for _, method := range []string{"get", "post"} {
		op := mapAt(api, "paths", "/workspace/file", method)
		if mapAt(op, "responses", "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/WorkspaceFileDocument" {
			t.Fatalf("%s workspace file has an untyped response", method)
		}
		if mapAt(op, "responses")["201"] != nil {
			t.Fatalf("%s workspace file declares a second success status", method)
		}
	}
	read := mapAt(api, "paths", "/workspace/file", "get")
	parameters := map[string]map[string]any{}
	for _, raw := range read["parameters"].([]any) {
		parameter := raw.(map[string]any)
		parameters[parameter["name"].(string)] = parameter
	}
	for _, name := range []string{"project_id", "path"} {
		if parameters[name] == nil || parameters[name]["required"] != true {
			t.Fatalf("file GET omits required %s query", name)
		}
	}
	write := mapAt(api, "paths", "/workspace/file", "post")
	if mapAt(write, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/WorkspaceFileSaveRequest" {
		t.Fatal("file POST no longer has its sole typed scope/body contract")
	}
	// Shared authorization and display decorators add headers after the typed
	// route is registered. They must not introduce a second source of scope.
	headers := map[string]bool{}
	for _, raw := range write["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] != "header" {
			t.Fatalf("file POST accepts scope outside its typed body: %v", parameter)
		}
		headers[parameter["name"].(string)] = true
	}
	for _, name := range []string{"X-Suma-Expected-User", "X-Suma-Expected-Organization"} {
		if !headers[name] {
			t.Fatalf("file POST lost its original-view header %s", name)
		}
	}
	for _, pair := range [][2]string{{"WorkspaceFileDocument", "context_sha256"}, {"WorkspaceFileSaveRequest", "expected_context_sha256"}} {
		if mapAt(api, "components", "schemas", pair[0], "properties", pair[1])["pattern"] != "^[a-f0-9]{64}$" {
			t.Fatalf("%s.%s lost lowercase digest constraint", pair[0], pair[1])
		}
	}
	binding := mapAt(api, "paths", "/workspace/binding", "post", "responses")
	if mapAt(binding, "200", "content", "application/json", "schema")["$ref"] != "#/components/schemas/WorkspaceMetadata" || binding["201"] != nil {
		t.Fatal("ordinary binding no longer returns the original typed metadata receipt")
	}
}
