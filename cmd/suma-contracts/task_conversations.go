package main

import "github.com/ai-mada/suma/internal/platform"

func taskConversationModels() []any {
	return []any{platform.TaskConversationMessage{}, platform.TaskConversationPage{}, platform.TaskConversationAppendRequest{}}
}

// Conversation messages are collaboration facts. Neither a persisted note nor
// its source run reference is dispatch, an assistant reply or a review grant.
func taskConversationContracts(add operationAdder, api map[string]any) {
	read := add("/task-conversations", "get", "TaskConversationPage", false, false)
	successStatus(read, "200")
	read["x-read-only"] = true
	task := queryParameter("task_id", "Exact task in the authorized project, including accepted tasks.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
	task["required"] = true
	appendQuery(read, task)
	appendQuery(read, queryParameter("limit", "Latest bounded page returned in ascending sequence order.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
	appendQuery(read, queryParameter("before", "Exclusive sequence for an earlier page in the same authorized task. Omit for the latest page.", map[string]any{"type": "integer", "minimum": 1, "maximum": 9007199254740991}))
	write := add("/task-conversations", "post", "TaskConversationMessage", false, true)
	successStatus(write, "201")
	request(write, "TaskConversationAppendRequest")
	for _, op := range []map[string]any{read, write} {
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Native durable human/project-API task discussion. Current project authority is checked before every read/write/replay. The server derives immutable author, sequence, task/requirement versions and original run reference. Append and original idempotent receipt commit atomically without changing task version, run, dispatch, model, test or acceptance facts. Messages are plain text, never executable HTML or agent instructions."
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		for key, value := range attrs {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, model := range []string{"TaskConversationMessage", "TaskConversationPage", "TaskConversationAppendRequest"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"TaskConversationMessage", "TaskConversationPage"} {
		for _, property := range toolContractMap(schemas, model, "properties") {
			property.(map[string]any)["readOnly"] = true
		}
	}
	for _, name := range []string{"project_id", "task_id"} {
		field("TaskConversationAppendRequest", name, map[string]any{"minLength": 1, "maxLength": 64})
	}
	field("TaskConversationAppendRequest", "expected_task_version", map[string]any{"minimum": 1, "maximum": 2147483646})
	field("TaskConversationAppendRequest", "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	field("TaskConversationAppendRequest", "body", map[string]any{"minLength": 1, "maxLength": 8000, "x-max-utf8-bytes": 32000, "description": "Nonblank valid Unicode text without NUL. Original whitespace is preserved."})
	field("TaskConversationMessage", "actor_kind", map[string]any{"enum": []string{"human", "api"}})
	field("TaskConversationMessage", "sequence", map[string]any{"minimum": 1, "maximum": 9007199254740991})
	field("TaskConversationPage", "next_before", map[string]any{"minimum": 0, "maximum": 9007199254740991})
	field("TaskConversationPage", "limit", map[string]any{"minimum": 1, "maximum": 50})
	field("TaskConversationPage", "items", map[string]any{"maxItems": 50})
	field("TaskConversationPage", "can_post", map[string]any{"description": "Current read-time capability only. Every append independently rechecks current authority."})
}
