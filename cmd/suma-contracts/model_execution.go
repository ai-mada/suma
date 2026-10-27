package main

import "github.com/ai-mada/suma/internal/platform"

func modelExecutionModels() []any {
	return []any{platform.ModelExecutionOptions{}, platform.ModelExecutionOption{}}
}
func modelExecutionContracts(add operationAdder, api map[string]any) {
	op := add("/model-execution/options", "get", "ModelExecutionOptions", false, false)
	op["description"] = "Current project-authorized worker model deployment observations and matching operator rate. Does not probe provider health, request a model, dispatch work or certify vendor billing. Dispatch preview freezes the selected exact revision and rate."
	for _, p := range []map[string]any{
		queryParameter("tool_profile_id", "Current approved native workspace tool profile ID.", map[string]any{"type": "string", "minLength": 1}),
		queryParameter("expected_tool_version", "Exact current tool record version.", map[string]any{"type": "integer", "minimum": 1}),
	} {
		p["required"] = true
		appendQuery(op, p)
	}
}
