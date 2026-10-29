package main

import (
	"reflect"
	"testing"
)

func TestAdapterCatalogContractIsReadOnlyAndSeparatesDirectionFromSupport(t *testing.T) {
	api, _ := generate()
	path := mapAt(api, "paths", "/adapter-catalog")
	if len(path) != 1 || path["get"] == nil {
		t.Fatal("catalog must not expose configuration or execution commands")
	}
	op := mapAt(path, "get")
	if op["x-read-only"] != true || op["requestBody"] != nil {
		t.Fatal("catalog must only observe maintained capability declarations")
	}
	projectScope := false
	for _, entry := range op["parameters"].([]any) {
		parameter := entry.(map[string]any)
		if parameter["in"] == "query" && (parameter["name"] != "project_id" || parameter["required"] != true) {
			t.Fatal("catalog scope must be explicit and closed", parameter)
		}
		projectScope = projectScope || parameter["in"] == "query" && parameter["name"] == "project_id"
	}
	if !projectScope {
		t.Fatal("catalog lost its current project authorization boundary")
	}
	for _, check := range []struct {
		model, field string
		values       []string
	}{
		{"AdapterDescriptor", "direction", []string{"suma_to_tool", "tool_to_suma"}},
		{"AdapterDescriptor", "implementation", []string{"implemented", "guide_only", "planned"}},
		{"AdapterDescriptor", "setup_kind", []string{"runner_profile", "client_export", "manual_guide"}},
		{"AdapterCapability", "support", []string{"supported", "unsupported", "unverified"}},
	} {
		if !reflect.DeepEqual(mapAt(api, "components", "schemas", check.model, "properties", check.field)["enum"], check.values) {
			t.Fatal("capability semantics drifted", check)
		}
	}
	for _, model := range []string{"AdapterCatalogPage", "AdapterDescriptor", "AdapterCapability"} {
		schema := mapAt(api, "components", "schemas", model)
		if schema["additionalProperties"] != false {
			t.Fatal("catalog must have a closed source-derived response", model)
		}
		for name, value := range mapAt(schema, "properties") {
			if value.(map[string]any)["readOnly"] != true {
				t.Fatal("catalog field implies client authority", model, name)
			}
		}
	}
}
