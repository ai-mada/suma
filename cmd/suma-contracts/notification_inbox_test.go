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
	for _, key := range []string{"title", "current_task", "body", "task_status"} {
		if mapAt(api, "components", "schemas", "NotificationInboxCommandReceipt", "properties")[key] != nil {
			t.Fatal("replay must not retain task content", key)
		}
	}
}
