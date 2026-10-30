package main

import "github.com/ai-mada/suma/internal/platform"

func taskOwnerChangeModels() []any {
	return []any{platform.TaskOwnerTarget{}, platform.TaskOwnerTargetQuery{}, platform.TaskOwnerTargetPage{}, platform.TaskOwnerSelection{}, platform.TaskOwnerChangePreviewRequest{}, platform.TaskOwnerChangePreviewItem{}, platform.TaskOwnerChangePreview{}, platform.TaskOwnerChangeRequest{}, platform.TaskOwnerChangeReceipt{}, platform.TaskOwnerChangeResult{}}
}

// The preview and command share exact selections, but only the command creates
// effects. The server remains responsible for canonical identities, current
// authority, all-or-nothing CAS, invalidation and immutable original receipts.
func taskOwnerChangeContracts(add operationAdder, api map[string]any) {
	targets := add("/task-owner-targets", "get", "TaskOwnerTargetPage", false, false)
	successStatus(targets, "200")
	targets["x-read-only"] = true
	targets["description"] = "Current project writer's canonical active human directory. Literal bounded search and scope-bound keyset pagination; each page rechecks current access. A target's existing role is unchanged. Unknown or duplicate query fields are rejected."
	appendQuery(targets, queryParameter("q", "Literal name search, not a query expression.", map[string]any{"type": "string", "maxLength": 200}))
	appendQuery(targets, queryParameter("limit", "Exact integer page size; count is independent of loaded items.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
	appendQuery(targets, queryParameter("cursor", "Original scope-bound cursor from this directory and filter.", map[string]any{"type": "string", "maxLength": 4096}))
	preview := add("/task-owner-changes/preview", "post", "TaskOwnerChangePreview", false, false)
	successStatus(preview, "200")
	request(preview, "TaskOwnerChangePreviewRequest")
	preview["parameters"] = []any{}
	preview["x-read-only"] = true
	preview["x-no-external-effects"] = true
	preview["description"] = "Read a current authorized preview for 1..16 explicit task versions and one canonical human resource version. No task, receipt, notification, approval or execution is created. Mixed unchanged owners are shown without an empty edit; downstream impacts are described without invented frozen counts."
	apply := add("/task-owner-changes", "post", "TaskOwnerChangeResult", false, true)
	successStatus(apply, "200")
	request(apply, "TaskOwnerChangeRequest")
	apply["parameters"] = []any{}
	apply["description"] = "Atomically adopt owner changes under current project authority. Recover the original operation before checking legitimately newer versions; otherwise check every selected task, target and preview digest before writes. Skip unchanged owners, notify only changed assignments, invalidate once for changed roots, reread final selected versions and commit one immutable receipt. A digest is not permission, and this command does not dispatch work or approve delivery."
	models := []string{"TaskOwnerTarget", "TaskOwnerTargetQuery", "TaskOwnerTargetPage", "TaskOwnerSelection", "TaskOwnerChangePreviewRequest", "TaskOwnerChangePreviewItem", "TaskOwnerChangePreview", "TaskOwnerChangeRequest", "TaskOwnerChangeReceipt", "TaskOwnerChangeResult"}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, key string, attrs map[string]any) {
		property := toolContractMap(schemas, model, "properties", key)
		for k, value := range attrs {
			property[k] = value
		}
	}
	for _, model := range models {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"TaskOwnerChangePreviewRequest", "TaskOwnerChangeRequest"} {
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "owner_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "expected_owner_version", map[string]any{"minimum": 1, "maximum": 2147483647})
		field(model, "tasks", map[string]any{"minItems": 1, "maxItems": 16, "description": "Explicit unique task IDs and original exact integer versions in original order. No implicit all-query selection."})
	}
	field("TaskOwnerSelection", "task_id", map[string]any{"minLength": 1, "maxLength": 64})
	field("TaskOwnerSelection", "expected_version", map[string]any{"minimum": 1, "maximum": 2147483647})
	field("TaskOwnerChangeRequest", "preview_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$", "description": "Exact current preview basis, not an access credential."})
	field("TaskOwnerChangeRequest", "acknowledge_invalidation", map[string]any{"const": true})
	field("TaskOwnerChangeRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("TaskOwnerTargetPage", "items", map[string]any{"maxItems": 100})
	field("TaskOwnerTargetPage", "limit", map[string]any{"minimum": 1, "maximum": 100})
	field("TaskOwnerTargetPage", "total", map[string]any{"minimum": 0})
	field("TaskOwnerChangePreview", "tasks", map[string]any{"minItems": 1, "maxItems": 16})
	field("TaskOwnerChangePreview", "preview_sha256", map[string]any{"pattern": "^[a-f0-9]{64}$"})
	field("TaskOwnerChangeResult", "changes", map[string]any{"minItems": 1, "maxItems": 16})
	for _, model := range []string{"TaskOwnerTarget", "TaskOwnerTargetPage", "TaskOwnerChangePreview", "TaskOwnerChangePreviewItem", "TaskOwnerChangeResult", "TaskOwnerChangeReceipt"} {
		for _, property := range toolContractMap(schemas, model, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
}
