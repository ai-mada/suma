package main

import "github.com/ai-mada/suma/internal/platform"

func taskSubscriptionModels() []any {
	return []any{platform.TaskSubscription{}, platform.TaskSubscriptionCommand{}, platform.TaskSubscriptionReceipt{}}
}

// A personal follow is separate from task ownership and business-write rights.
// Both routes bind the original task and current project; the recipient can
// never be supplied by a client. An absent preference remains a read-only v0.
func taskSubscriptionContracts(add operationAdder, api map[string]any) {
	read := add("/tasks/{task}/subscription", "get", "TaskSubscription", false, false)
	write := add("/tasks/{task}/subscription/commands", "post", "TaskSubscriptionReceipt", false, true)
	request(write, "TaskSubscriptionCommand")
	read["x-read-only"] = true
	for _, op := range []map[string]any{read, write} {
		successStatus(op, "200")
		op["x-disallowed-token-scopes"] = []string{"runner", "agent", "pool"}
		op["description"] = "Current browser/user-API project read authority manages only its own task follow. Reading a missing row is observational version0. Follow/unfollow uses exact CAS and the original operation receipt; it never changes task ownership, dispatches work or marks notifications read. Discussion notifications are projected by original source transactions, with current recipient authority and mention deduplication."
		for _, value := range op["parameters"].([]any) {
			parameter := value.(map[string]any)
			if parameter["name"] == "project_id" || parameter["name"] == "task" {
				parameter["required"] = true
				parameter["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^(?!\\.{1,2}$)[A-Za-z0-9_.-]{1,64}$"}
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"TaskSubscription", "TaskSubscriptionCommand", "TaskSubscriptionReceipt"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, value := range toolContractMap(model, "properties") {
			field := value.(map[string]any)
			if name != "TaskSubscriptionCommand" {
				field["readOnly"] = true
			}
			switch key {
			case "project_id", "task_id", "user_id":
				field["minLength"], field["maxLength"] = 1, 64
				field["pattern"] = "^(?!\\.{1,2}$)[A-Za-z0-9_.-]{1,64}$"
			case "version", "expected_version":
				field["minimum"], field["maximum"] = 0, 2147483647
			case "idempotency_key":
				// Runtime bounds count UTF-8 bytes, not Unicode code points. A
				// short multibyte key must not be rejected by a conflicting schema.
				field["minLength"], field["maxLength"] = 1, 128
				field["x-min-utf8-bytes"], field["x-max-utf8-bytes"] = 8, 128
			case "action":
				field["enum"] = []string{"follow", "unfollow"}
			case "updated_at":
				field["description"] = "Empty only for an absent version0 preference; otherwise the saved UTC RFC3339 timestamp. No row is created by GET."
			}
		}
	}
	toolContractMap(schemas, "NotificationInboxItem", "properties", "message_id")["description"] = "Exact immutable source message for task_mentioned or task_followed only after current source and private reverse recipient binding match. Opening never acknowledges the notification."
}
