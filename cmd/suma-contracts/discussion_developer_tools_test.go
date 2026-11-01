package main

import "testing"

// Fixed CLI profiles use their own immutable configuration. A public contract
// that makes either platform model field mandatory would silently exclude them,
// even though the ordinary dispatcher supports their development workflows.
func TestDiscussionHandoffContractAllowsAdapterSpecificModelSelection(t *testing.T) {
	api, _ := generate()
	schema := mapAt(api, "components", "schemas", "TaskDiscussionDevelopmentCreateRequest")
	properties := mapAt(schema, "properties")
	for _, field := range []string{"model_execution_selection", "codex_execution_selection"} {
		if properties[field] == nil || mapAt(properties, field)["description"] == nil {
			t.Fatalf("missing adapter selection contract: %s", field)
		}
		for _, required := range schema["required"].([]string) {
			if required == field {
				t.Fatalf("fixed profiles must be allowed to omit %s", field)
			}
		}
	}
	mutuallyExclusive := mapAt(schema, "not")["required"].([]string)
	if len(mutuallyExclusive) != 2 || mutuallyExclusive[0] != "model_execution_selection" || mutuallyExclusive[1] != "codex_execution_selection" || schema["oneOf"] != nil {
		t.Fatal("model fields must be mutually exclusive without requiring a selection for fixed profiles")
	}
	if schema["additionalProperties"] != false || properties["knowledge"] != nil {
		t.Fatal("discussion handoff must retain server-verified source knowledge rather than accept arbitrary client references")
	}
}
