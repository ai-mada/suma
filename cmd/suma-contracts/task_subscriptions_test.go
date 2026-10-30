package main

import (
	"strings"
	"testing"
)

func TestTaskSubscriptionContractKeepsIdentityPrivateAndVersionZeroObservational(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	read := mapAt(paths, "/tasks/{task}/subscription", "get")
	write := mapAt(paths, "/tasks/{task}/subscription/commands", "post")
	if read["x-read-only"] != true {
		t.Fatal("reading an absent preference must not create a subscription")
	}
	for _, operation := range []map[string]any{read, write} {
		seen := map[string]bool{}
		for _, value := range operation["parameters"].([]any) {
			parameter := value.(map[string]any)
			name, _ := parameter["name"].(string)
			if name == "user_id" || name == "recipient_user_id" || name == "tenant_id" {
				t.Fatal("a client must never select the follow recipient", name)
			}
			if name == "project_id" || name == "task" {
				seen[name] = parameter["required"] == true
			}
		}
		if !seen["project_id"] || !seen["task"] {
			t.Fatal("read and command must bind the exact project and task")
		}
	}
	schemas := mapAt(api, "components", "schemas")
	command := mapAt(schemas, "TaskSubscriptionCommand", "properties")
	if len(command) != 5 || command["user_id"] != nil || mapAt(command, "expected_version")["minimum"] != 0 {
		t.Fatal("closed command must admit version0 without a recipient override")
	}
	if mapAt(command, "idempotency_key")["minLength"] != 1 || mapAt(command, "idempotency_key")["x-min-utf8-bytes"] != 8 {
		t.Fatal("a valid multibyte operation key must not acquire an eight-character minimum")
	}
	for _, name := range []string{"TaskSubscription", "TaskSubscriptionCommand", "TaskSubscriptionReceipt"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("follow contract must be closed", name)
		}
	}
	if mapAt(schemas, "TaskSubscription", "properties", "version")["minimum"] != 0 {
		t.Fatal("an absent read cannot be forced to fabricate version1")
	}
	if len(mapAt(schemas, "TaskSubscriptionReceipt", "properties")) != 2 {
		t.Fatal("receipt must acknowledge the original operation and its saved preference only")
	}
}

func TestTaskSubscriptionContractRetainsAllInboxSourcesAndExactMessageLocator(t *testing.T) {
	api, _ := generate()
	item := mapAt(api, "components", "schemas", "NotificationInboxItem", "properties")
	kinds, ok := mapAt(item, "kind")["enum"].([]string)
	if !ok || len(kinds) != 5 {
		t.Fatal("adding follow notifications must retain every existing source kind")
	}
	actual := map[string]bool{}
	for _, kind := range kinds {
		actual[kind] = true
	}
	for _, kind := range []string{"task_assigned", "task_mentioned", "task_review_requested", "task_handoff_requested", "task_followed"} {
		if !actual[kind] {
			t.Fatal("missing source", kind)
		}
	}
	if item["message_id"] == nil || item["body"] != nil || item["message_body"] != nil {
		t.Fatal("follow notifications expose an authorized source locator, never a copied discussion")
	}
	if !strings.Contains(mapAt(item, "message_id")["description"].(string), "task_followed") {
		t.Fatal("later source contracts must not overwrite the shared message locator semantics")
	}
}
