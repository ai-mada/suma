package main

import "testing"

func TestProjectNotificationPreferencesContractKeepsPersonalCASAndFalseExplicit(t *testing.T) {
	api, _ := generate()
	paths, schemas := mapAt(api, "paths"), mapAt(api, "components", "schemas")
	read := mapAt(paths, "/notification-preferences", "get")
	write := mapAt(paths, "/notification-preferences/commands", "post")
	if read["x-read-only"] != true || write["x-max-request-body-bytes"] != 16<<10 {
		t.Fatal("preference read/write boundaries are missing")
	}
	for _, operation := range []map[string]any{read, write} {
		projectRequired := false
		for _, raw := range operation["parameters"].([]any) {
			parameter := raw.(map[string]any)
			if parameter["name"] == "project_id" {
				projectRequired = parameter["required"] == true
			}
			if parameter["name"] == "user_id" || parameter["name"] == "tenant_id" || parameter["name"] == "recipient_user_id" {
				t.Fatal("preference API allows another recipient")
			}
		}
		if !projectRequired || len(operation["x-disallowed-token-scopes"].([]string)) != 3 {
			t.Fatal("project and human identity scope are not explicit")
		}
	}
	command := mapAt(schemas, "ProjectNotificationPreferencesCommand")
	fields := mapAt(command, "properties")
	if command["additionalProperties"] != false || len(fields) != 4 || len(command["required"].([]string)) != 4 || mapAt(fields, "followed_discussions_muted")["type"] != "boolean" || mapAt(fields, "expected_version")["minimum"] != 0 {
		t.Fatal("false and version0 must remain required values in a closed command")
	}
	for _, name := range []string{"ProjectNotificationPreferences", "ProjectNotificationPreferencesReceipt"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("preference result is not closed", name)
		}
	}
	if mapAt(schemas, "ProjectNotificationPreferences", "properties", "version")["minimum"] != 0 || len(mapAt(schemas, "ProjectNotificationPreferencesReceipt", "properties")) != 2 {
		t.Fatal("receipt must preserve original operation and observational version0")
	}
}
