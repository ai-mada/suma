package main

import "testing"

func TestTaskConversationMentionContractsKeepSelectionAndImmutableSourceExplicit(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"TaskConversationMention", "TaskConversationMentionTarget", "TaskConversationMentionTargetPage", "TaskConversationMessageDetail"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("mention DTO must be closed", name)
		}
	}
	for _, path := range []string{"/task-conversation-mention-targets", "/task-conversation-messages/{id}"} {
		op := mapAt(api, "paths", path, "get")
		if op["x-read-only"] != true || len(mapAt(api, "paths", path)) != 1 {
			t.Fatal("directory/locator must not mutate discussion or read state", path)
		}
	}
	for _, pair := range [][2]string{{"TaskConversationAppendRequest", "mention_resource_ids"}, {"TaskConversationMessage", "mentions"}} {
		model := mapAt(schemas, pair[0])
		for _, field := range model["required"].([]string) {
			if field == pair[1] {
				t.Fatal("zero-mention current messages must omit optional metadata", pair)
			}
		}
		if mapAt(model, "properties", pair[1])["maxItems"] != 20 {
			t.Fatal("explicit recipients must remain bounded", pair)
		}
	}
	kinds := mapAt(schemas, "NotificationInboxItem", "properties", "kind")["enum"].([]string)
	// This domain requires both sources; the review-request domain separately
	// owns the complete current enum when adding its real notification source.
	if len(kinds) < 2 || kinds[0] != "task_assigned" || kinds[1] != "task_mentioned" {
		t.Fatal("both actual sources must be registered", kinds)
	}
	if mapAt(schemas, "NotificationInboxItem", "properties")["message_id"] == nil {
		t.Fatal("intact mention source must carry its exact locator")
	}
}
