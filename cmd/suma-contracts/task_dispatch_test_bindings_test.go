package main

import "testing"

func TestTaskDispatchTestBindingContractsUseOriginalPreviewAndClosedPins(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskDispatchPreviewRequest", "TaskDispatchPlan"} {
		if toolContractMap(schemas, model, "properties", "test_bindings")["maxItems"] != 64 {
			t.Fatal("explicit test bindings lost their limit", model)
		}
	}
	for model, count := range map[string]int{"TaskDispatchTestSelection": 3, "TaskDispatchTestBinding": 6} {
		schema := toolContractMap(schemas, model)
		if schema["additionalProperties"] != false || len(toolContractMap(schema, "properties")) != count {
			t.Fatal("test choices or frozen pins admit unrelated authority", model)
		}
	}
	if toolContractMap(schemas, "TaskDispatchTestSelection", "properties", "expected_tool_version")["minimum"] != 1 {
		t.Fatal("test choices do not require the current tool version")
	}
	if toolContractMap(api, "paths", "/task-dispatch-plans", "post")["requestBody"] == nil {
		t.Fatal("original preview contract disappeared")
	}
}
