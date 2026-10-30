package main

import "github.com/ai-mada/suma/internal/platform"

func taskConversationReplyModels() []any {
	return []any{platform.TaskConversationReplyCoverage{}, platform.TaskConversationReplySource{}, platform.TaskConversationReply{}, platform.TaskConversationReplyCreateRequest{}, platform.TaskConversationReplyCommandRequest{}, platform.TaskConversationReplyPage{}}
}

func taskConversationReplyContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct{ path, method, input, output, status string }{
		{"/task-conversation-replies", "get", "", "TaskConversationReplyPage", "200"},
		{"/task-conversation-replies/{id}", "get", "", "TaskConversationReply", "200"},
		{"/task-conversation-replies", "post", "TaskConversationReplyCreateRequest", "TaskConversationReply", "201"},
		{"/task-conversation-replies/{id}/commands", "post", "TaskConversationReplyCommandRequest", "TaskConversationReply", "200"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		successStatus(op, route.status)
		op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
		op["description"] = "Explicit read-only multi-turn task assistant using one managed DAG run, a fixed current model deployment/parameters/budget, frozen task/requirement/message context and one original model request. It never replaces the task run, dispatches development commands or accepts delivery. Only a verified exact original attempt/fence/model receipt can append an assistant message. Unknown work remains blocked; cancel requests are not stopped-process evidence."
		if route.input != "" {
			request(op, route.input)
		} else {
			op["x-read-only"] = true
		}
		if route.path == "/task-conversation-replies" && route.method == "get" {
			task := queryParameter("task_id", "Exact task in the current authorized project.", map[string]any{"type": "string", "minLength": 1, "maxLength": 64})
			task["required"] = true
			appendQuery(op, task)
			appendQuery(op, queryParameter("limit", "Bounded reply history.", map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "default": 20}))
			appendQuery(op, queryParameter("cursor", "Original reply ID scoped to the same tenant/project/task for an earlier page.", map[string]any{"type": "string", "maxLength": 64}))
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, values map[string]any) {
		for key, value := range values {
			toolContractMap(schemas, model, "properties", name)[key] = value
		}
	}
	for _, model := range []string{"TaskConversationReplyCoverage", "TaskConversationReplySource", "TaskConversationReply", "TaskConversationReplyCreateRequest", "TaskConversationReplyCommandRequest", "TaskConversationReplyPage"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
	}
	for _, model := range []string{"TaskConversationReplyCoverage", "TaskConversationReplySource", "TaskConversationReply", "TaskConversationReplyPage"} {
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	for _, model := range []string{"TaskConversationReplyCreateRequest", "TaskConversationReplyCommandRequest"} {
		field(model, "project_id", map[string]any{"minLength": 1, "maxLength": 64})
		field(model, "idempotency_key", map[string]any{"minLength": 8, "maxLength": 128})
	}
	for _, name := range []string{"task_id", "tool_profile_id"} {
		field("TaskConversationReplyCreateRequest", name, map[string]any{"minLength": 1, "maxLength": 64})
	}
	for _, name := range []string{"expected_task_version", "expected_tool_version"} {
		field("TaskConversationReplyCreateRequest", name, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	for _, name := range []string{"expected_last_sequence", "reply_to_sequence"} {
		field("TaskConversationReplyCreateRequest", name, map[string]any{"minimum": 1, "maximum": 9007199254740991})
	}
	field("TaskConversationReplyCreateRequest", "context_message_limit", map[string]any{"minimum": 1, "maximum": 20, "description": "Explicit recent context window; oversized context is rejected, never silently trimmed or summarized."})
	field("TaskConversationReplyCreateRequest", "locale", map[string]any{"enum": []string{"zh-CN", "en-US"}})
	field("TaskConversationReplyCreateRequest", "acknowledge_model_use", map[string]any{"const": true})
	for _, name := range []string{"expected_version", "expected_run_version"} {
		field("TaskConversationReplyCommandRequest", name, map[string]any{"minimum": 1, "maximum": 2147483646})
	}
	field("TaskConversationReplyCommandRequest", "action", map[string]any{"enum": []string{"cancel"}})
	field("TaskConversationReplySource", "messages", map[string]any{"maxItems": 20})
	field("TaskConversationReplySource", "knowledge", map[string]any{"maxItems": 16, "description": "Approved exact knowledge consumed once from the selected messages, including dependencies inherited by verified assistants. Rechecked before new execution and publication; not executable authority."})
	field("TaskConversationReplyPage", "items", map[string]any{"maxItems": 50})
	field("TaskConversationReply", "model_execution", map[string]any{"description": "The exact server-reserved model deployment, one-call parameters, token/cost allowance and rate basis. No provider credentials."})
	field("TaskConversationMessage", "actor_kind", map[string]any{"enum": []string{"human", "api", "assistant"}})
	field("TaskConversationMessage", "reply_to_sequence", map[string]any{"minimum": 0, "maximum": 9007199254740991})
}
