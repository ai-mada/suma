package main

import (
	dc "github.com/ai-mada/suma/internal/developmentcontract"
	"github.com/ai-mada/suma/internal/platform"
)

func developmentContinuationModels() []any {
	return []any{dc.DevelopmentContinuationSource{}, dc.DevelopmentContinuationBinding{}, platform.TaskDevelopmentContinuationOption{}, platform.TaskDevelopmentContinuationOptions{}}
}

func developmentContinuationContracts(add operationAdder, api map[string]any) {
	op := add("/tasks/{id}/development-continuation-options", "get", "TaskDevelopmentContinuationOptions", false, false)
	op["x-read-only"] = true
	op["description"] = "Current task readers inspect sealed code from its latest terminal run. A selected source is development input, not an accepted baseline or inherited test result. The cursor binds current actor, task version and original run version; refresh after drift. A reviewed new discussion reply and explicit original plan/Start are required to execute."
	appendQuery(op, queryParameter("limit", "Default20, maximum100.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
	appendQuery(op, queryParameter("before", "Opaque current-actor/project/task/run-version cursor.", map[string]any{"type": "string", "maxLength": 4096}))
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"DevelopmentContinuationSource", "DevelopmentContinuationBinding", "TaskDevelopmentContinuationOption", "TaskDevelopmentContinuationOptions"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
		if model != "DevelopmentContinuationSource" {
			for _, raw := range toolContractMap(schemas, model, "properties") {
				raw.(map[string]any)["readOnly"] = true
			}
		}
	}
	for _, field := range []string{"attempt", "fence", "expected_run_version"} {
		toolContractMap(schemas, "DevelopmentContinuationSource", "properties", field)["minimum"] = 1
	}
	toolContractMap(schemas, "DevelopmentContinuationBinding", "properties", "protocol")["const"] = dc.ContinuationProtocol
	toolContractMap(schemas, "DevelopmentContinuationBinding", "properties", "binding_sha256")["pattern"] = "^[a-f0-9]{64}$"
	toolContractMap(schemas, "TaskDevelopmentContinuationOptions", "properties", "reason")["enum"] = []string{"task_accepted", "no_run", "run_not_terminal", "no_sealed_candidate"}
	toolContractMap(schemas, "TaskDevelopmentContinuationOptions", "properties", "reason")["description"] = "Closed product-owned reason code, localized by the client. Omitted when a source is available."
	toolContractMap(schemas, "TaskDiscussionDevelopmentCreateRequest", "properties", "continuation_source")["description"] = "Optional explicit source selection; current authority resolves all candidate and evidence fields. Never submit a complete binding or inherit old acceptance."
	for _, model := range []string{"TaskDispatchPlan", "TaskDiscussionDevelopmentPlan", "Assignment"} {
		property := toolContractMap(schemas, model, "properties", "development_continuation")
		property["readOnly"] = true
		property["description"] = "Protected original same-task code input; independent tests and human acceptance remain new facts. Workflow config cannot grant this authority."
	}
}
