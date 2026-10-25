package main

import "testing"

func TestTaskDesignViewContractKeepsReadAndOriginalSnapshot(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/tasks/{id}/design-context", "get")
	if len(op["security"].([]any)) != 2 {
		t.Fatal("project/browser read contract changed")
	}
	props := mapAt(api, "components", "schemas", "TaskDesignContextView", "properties")
	for _, name := range []string{"context", "requirement", "adopted_task", "bound", "eligibility"} {
		if props[name].(map[string]any)["readOnly"] != true {
			t.Fatal("source read is not declared read-only", name)
		}
	}
	if _, exists := props["adoption"]; exists {
		t.Fatal("task read must not repeat the full unrelated batch")
	}
}
