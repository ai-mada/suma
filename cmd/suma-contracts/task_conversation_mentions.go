package main

import "github.com/ai-mada/suma/internal/platform"

func taskConversationMentionModels() []any {
	return []any{platform.TaskConversationMention{}, platform.TaskConversationMentionTarget{}, platform.TaskConversationMentionTargetPage{}, platform.TaskConversationMessageDetail{}}
}

// Explicit structured recipient selection is independent of the plain-text
// body. Directory/detail reads never acknowledge a notification or start AI.
func taskConversationMentionContracts(add operationAdder, api map[string]any) {
	targets := add("/task-conversation-mention-targets", "get", "TaskConversationMentionTargetPage", false, false)
	detail := add("/task-conversation-messages/{id}", "get", "TaskConversationMessageDetail", false, false)
	for _, op := range []map[string]any{targets, detail} {
		successStatus(op, "200")
		op["x-read-only"] = true
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Current human/user-API project and task read authority. Exact immutable source metadata; no dispatch, model request or notification acknowledgement."
		task := queryParameter("task_id", "Exact currently authorized task.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
		task["required"] = true
		appendQuery(op, task)
	}
	appendQuery(targets, queryParameter("q", "Literal bounded current-person search; exclusive with resource_ids.", map[string]any{"type": "string", "minLength": 1, "maxLength": 200}))
	appendQuery(targets, queryParameter("limit", "Bounded current-person page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
	appendQuery(targets, queryParameter("cursor", "Opaque canonical continuation scoped to current identity, project, task, query and page size.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096}))
	appendQuery(targets, queryParameter("resource_ids", "At most20 unique comma-separated resource IDs; current-authorized selected subset in supplied order, exclusive with q/cursor.", map[string]any{"type": "string", "minLength": 1, "maxLength": 1299}))
	schemas := toolContractMap(api, "components", "schemas")
	for _, model := range []string{"TaskConversationMention", "TaskConversationMentionTarget", "TaskConversationMentionTargetPage", "TaskConversationMessageDetail"} {
		schema := toolContractMap(schemas, model)
		schema["additionalProperties"] = false
		for _, v := range toolContractMap(schema, "properties") {
			v.(map[string]any)["readOnly"] = true
		}
	}
	field := func(model, key string, attrs map[string]any) {
		for k, v := range attrs {
			toolContractMap(schemas, model, "properties", key)[k] = v
		}
	}
	field("TaskConversationAppendRequest", "mention_resource_ids", map[string]any{"maxItems": 20, "uniqueItems": true, "description": "Explicit current canonical human resource IDs. No plain-text @ parsing; original operation recovery precedes new recipient validation. All recipients, message and notifications commit atomically."})
	field("TaskConversationMessage", "mentions", map[string]any{"maxItems": 20, "description": "Server-resolved immutable recipient snapshots at posting; omitted when absent. Shared hydration preserves frozen assistant/development source digests across later renames or revocation."})
	field("TaskConversationMentionTargetPage", "items", map[string]any{"maxItems": 50})
	field("TaskConversationMentionTargetPage", "limit", map[string]any{"minimum": 1, "maximum": 50})
	field("NotificationInboxItem", "kind", map[string]any{"enum": []string{"task_assigned", "task_mentioned"}})
	field("NotificationInboxItem", "message_id", map[string]any{"minLength": 1, "maxLength": 64, "description": "Exact original mentioned message; only returned when task, human/API source and immutable recipient relation are intact."})
}
