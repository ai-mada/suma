package main

import "github.com/ai-mada/suma/internal/platform"

func taskPlanningStatusModels() []any {
	return []any{
		platform.TaskPlanningStatusSelection{}, platform.TaskPlanningStatusPreviewRequest{},
		platform.TaskPlanningStatusPreviewItem{}, platform.TaskPlanningStatusPreview{},
		platform.TaskPlanningStatusChangeRequest{}, platform.TaskPlanningStatusChangeReceipt{}, platform.TaskPlanningStatusChangeResult{},
	}
}

func taskPlanningStatusContracts(add operationAdder, api map[string]any) {
	preview := add("/task-planning-status-changes/preview", "post", "TaskPlanningStatusPreview", false, false)
	browserOnly(preview)
	successStatus(preview, "200")
	request(preview, "TaskPlanningStatusPreviewRequest")
	preview["parameters"] = []any{}
	preview["x-read-only"] = true
	preview["x-no-external-effects"] = true
	preview["description"] = "Read the exact selected task versions and authoritative eligibility for planning-status changes. This preview does not create an operation, run or acceptance."
	apply := add("/task-planning-status-changes", "post", "TaskPlanningStatusChangeResult", false, true)
	browserOnly(apply)
	successStatus(apply, "200")
	request(apply, "TaskPlanningStatusChangeRequest")
	apply["parameters"] = []any{}
	apply["description"] = "Change only the planning status of explicit tasks in one transaction. Check current authority, versions and workflow boundaries. Recover the original ordered result using the same operation ID and body."

	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskPlanningStatusSelection", "TaskPlanningStatusPreviewRequest", "TaskPlanningStatusPreviewItem", "TaskPlanningStatusPreview", "TaskPlanningStatusChangeRequest", "TaskPlanningStatusChangeReceipt", "TaskPlanningStatusChangeResult"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	field := func(model, name string, attributes map[string]any) {
		property := toolContractMap(schemas, model, "properties", name)
		for key, value := range attributes {
			property[key] = value
		}
	}
	id := map[string]any{"minLength": 1, "maxLength": 64}
	version := map[string]any{"minimum": 1, "maximum": 2147483647}
	batch := map[string]any{"minItems": 1, "maxItems": 16}
	status := map[string]any{"enum": []string{"draft", "todo", "in_progress", "blocked", "cancelled"}}
	for _, model := range []string{"TaskPlanningStatusPreviewRequest", "TaskPlanningStatusChangeRequest"} {
		field(model, "project_id", id)
		field(model, "tasks", batch)
	}
	field("TaskPlanningStatusSelection", "task_id", id)
	field("TaskPlanningStatusSelection", "expected_version", version)
	field("TaskPlanningStatusPreviewItem", "version", version)
	field("TaskPlanningStatusPreviewItem", "eligibility", map[string]any{"enum": []string{"editable", "workflow_status", "active_run", "review_required", "unverified_run"}})
	field("TaskPlanningStatusPreview", "items", batch)
	field("TaskPlanningStatusChangeRequest", "status", status)
	field("TaskPlanningStatusChangeRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("TaskPlanningStatusChangeReceipt", "status", status)
	field("TaskPlanningStatusChangeReceipt", "previous_version", version)
	field("TaskPlanningStatusChangeReceipt", "version", version)
	field("TaskPlanningStatusChangeResult", "changes", batch)
}
