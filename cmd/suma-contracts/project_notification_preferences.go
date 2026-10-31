package main

import "github.com/ai-mada/suma/internal/platform"

func projectNotificationPreferencesModels() []any {
	return []any{platform.ProjectNotificationPreferences{}, platform.ProjectNotificationPreferencesCommand{}, platform.ProjectNotificationPreferencesReceipt{}}
}

// These endpoints manage the current person's preference only. They grant no
// task-write right and never acknowledge notifications or create a recipient.
func projectNotificationPreferencesContracts(add operationAdder, api map[string]any) {
	read := add("/notification-preferences", "get", "ProjectNotificationPreferences", false, false)
	write := add("/notification-preferences/commands", "post", "ProjectNotificationPreferencesReceipt", false, true)
	request(write, "ProjectNotificationPreferencesCommand")
	read["x-read-only"] = true
	write["x-max-request-body-bytes"] = 16 << 10
	for _, operation := range []map[string]any{read, write} {
		successStatus(operation, "200")
		operation["x-disallowed-token-scopes"] = []string{"runner", "agent", "pool"}
		operation["description"] = "Current browser/user-API project read authority manages only its own project notification preference. A missing row is observational version0 and unmuted. Exact CAS and original receipt recovery change only future followed-discussion delivery; mentions, handoffs, history and read acknowledgments are unaffected. An old receipt is not current state."
		for _, raw := range operation["parameters"].([]any) {
			parameter := raw.(map[string]any)
			if parameter["name"] == "project_id" {
				parameter["required"] = true
				parameter["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": 64, "pattern": "^(?!\\.{1,2}$)[A-Za-z0-9_.-]{1,64}$"}
			}
		}
	}
	schemas := toolContractMap(api, "components", "schemas")
	for _, name := range []string{"ProjectNotificationPreferences", "ProjectNotificationPreferencesCommand", "ProjectNotificationPreferencesReceipt"} {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, raw := range toolContractMap(model, "properties") {
			field := raw.(map[string]any)
			if name != "ProjectNotificationPreferencesCommand" {
				field["readOnly"] = true
			}
			switch key {
			case "project_id", "user_id":
				field["minLength"], field["maxLength"] = 1, 64
				field["pattern"] = "^(?!\\.{1,2}$)[A-Za-z0-9_.-]{1,64}$"
			case "version", "expected_version":
				field["minimum"], field["maximum"] = 0, 2147483647
			case "idempotency_key":
				field["minLength"], field["maxLength"] = 1, 128
				field["x-min-utf8-bytes"], field["x-max-utf8-bytes"] = 8, 128
			case "updated_at":
				field["description"] = "Empty only for an absent version0 preference; otherwise the unchanged or newly committed UTC RFC3339 timestamp. GET never creates a row."
			case "followed_discussions_muted":
				field["description"] = "Mute future followed-task discussion delivery in this project only; exact mentions and all other notification sources are unaffected."
			}
		}
	}
}
