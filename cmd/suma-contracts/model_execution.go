package main

import "github.com/ai-mada/suma/internal/platform"

func modelExecutionModels() []any {
	return []any{platform.ModelExecutionOptions{}, platform.ModelExecutionOption{}}
}
func modelExecutionContracts(add operationAdder, api map[string]any) {
	op := add("/model-execution/options", "get", "ModelExecutionOptions", false, false)
	op["description"] = "Current project-authorized worker native deployments or Codex app-server catalog. Native supported and Codex codex_supported are distinct. Codex model/effort confirms configuration only, with optional token notice and observed partial usage; it provides no native hard allowance or provider pricing. This read never invokes a model or dispatches work."
	for _, p := range []map[string]any{
		queryParameter("tool_profile_id", "Current approved tool profile ID. Native workspace and Codex app-server capabilities are returned separately.", map[string]any{"type": "string", "minLength": 1}),
		queryParameter("expected_tool_version", "Exact current tool record version.", map[string]any{"type": "integer", "minimum": 1}),
	} {
		p["required"] = true
		appendQuery(op, p)
	}
}
