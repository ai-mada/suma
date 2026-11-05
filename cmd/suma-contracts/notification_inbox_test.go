package main

import "testing"

func TestNotificationInboxContractKeepsRecipientAndSourcesServerOwned(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	if len(mapAt(paths, "/notification-inbox")) != 1 {
		t.Fatal("clients must not create notifications")
	}
	read := mapAt(paths, "/notification-inbox", "get")
	if read["x-read-only"] != true {
		t.Fatal("listing must be observational")
	}
	for _, value := range read["parameters"].([]any) {
		p := value.(map[string]any)
		if p["name"] == "user_id" || p["name"] == "recipient" {
			t.Fatal("current recipient must not be supplied")
		}
		if p["name"] == "limit" && p["schema"].(map[string]any)["maximum"] != 50 {
			t.Fatal("unbounded inbox")
		}
	}
	write := mapAt(paths, "/notification-inbox/{id}/commands", "post")
	for _, value := range write["parameters"].([]any) {
		if value.(map[string]any)["in"] == "query" {
			t.Fatal("write scope belongs to closed body")
		}
	}
	properties := mapAt(api, "components", "schemas", "NotificationInboxCommand", "properties")
	if len(properties) != 4 || properties["recipient_user_id"] != nil {
		t.Fatal("notification lifecycle must not change recipient/source")
	}
	if mapAt(api, "components", "schemas", "NotificationInboxItem", "properties", "sequence")["type"] != "string" {
		t.Fatal("sequence must preserve BIGINT precision")
	}
	item := mapAt(api, "components", "schemas", "NotificationInboxItem", "properties")
	kinds, _ := mapAt(item, "kind")["enum"].([]string)
	runKind := false
	for _, kind := range kinds {
		runKind = runKind || kind == "task_run_failed"
	}
	if !runKind || mapAt(item, "run_id")["readOnly"] != true {
		t.Fatal("run failures must carry a read-only source locator")
	}
	for _, key := range []string{"title", "current_task", "body", "task_status", "run_id"} {
		if mapAt(api, "components", "schemas", "NotificationInboxCommandReceipt", "properties")[key] != nil {
			t.Fatal("replay must not retain task content", key)
		}
	}
}

func TestNotificationInboxContractBindsBatchSelectionAndBrowser(t *testing.T) {
	api, _ := generate()
	batch := mapAt(api, "paths", "/notification-inbox/batch-commands", "post")
	security := batch["security"].([]any)
	if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
		t.Fatal("batch notification updates must use the current browser session")
	}
	for _, parameter := range batch["parameters"].([]any) {
		if parameter.(map[string]any)["in"] == "query" {
			t.Fatal("batch targets belong to the closed request body")
		}
	}
	item := mapAt(api, "components", "schemas", "NotificationInboxBatchTarget", "properties")
	pick := mapAt(api, "components", "schemas", "NotificationInboxBatchCommand", "properties", "items")
	if len(item) != 3 || item["expected_version"] == nil || pick["minItems"] != 1 || pick["maxItems"] != 20 {
		t.Fatal("batch must contain bounded explicit versioned targets")
	}
}

func TestNotificationOrganizationInboxContractUsesCurrentBrowserScope(t *testing.T) {
	api, _ := generate()
	path := mapAt(api, "paths", "/notification-inbox/organization")
	op := mapAt(path, "get")
	if len(path) != 1 || op["x-read-only"] != true || op["requestBody"] != nil {
		t.Fatal("organization inbox must expose only a read")
	}
	security := op["security"].([]any)
	if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
		t.Fatal("organization inbox must use the current browser session")
	}
	queries := map[string]bool{}
	for _, value := range op["parameters"].([]any) {
		param := value.(map[string]any)
		if param["in"] != "query" {
			continue
		}
		name, _ := param["name"].(string)
		if queries[name] || name != "filter" && name != "limit" && name != "cursor" {
			t.Fatal("current organization and recipient are server owned", param)
		}
		queries[name] = true
	}
	if len(queries) != 3 {
		t.Fatalf("unexpected organization query parameters: %+v", queries)
	}
	page := mapAt(api, "components", "schemas", "NotificationOrganizationInboxPage", "properties")
	item := mapAt(api, "components", "schemas", "NotificationOrganizationInboxItem", "properties")
	if page["organization_id"] == nil || page["scope_revision"] == nil || page["project_id"] != nil ||
		item["project_id"] == nil || item["notification"] == nil {
		t.Fatal("scope and authorized source coordinates must remain distinct")
	}
}
