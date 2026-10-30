package main

import (
	"reflect"
	"testing"
)

func TestTaskHandoffInboxContractUsesExactReadAndPrivateSourceBinding(t *testing.T) {
	api, _ := generate()
	path := mapAt(api, "paths", "/task-handoffs/{id}")
	op := mapAt(path, "get")
	if len(path) != 1 || op["x-read-only"] != true || op["requestBody"] != nil {
		t.Fatal("exact handoff location must not add a decision or write endpoint")
	}
	queries := map[string]bool{}
	for _, raw := range op["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] == "query" {
			queries[parameter["name"].(string)] = parameter["required"] == true
		}
	}
	if !reflect.DeepEqual(queries, map[string]bool{"project_id": true, "task_id": true}) {
		t.Fatal("exact reads must not scan history pages or infer another task", queries)
	}
	item := mapAt(api, "components", "schemas", "NotificationInboxItem")
	if !reflect.DeepEqual(mapAt(item, "properties", "kind")["enum"], []string{"task_assigned", "task_mentioned", "task_review_requested", "task_handoff_requested", "task_followed"}) {
		t.Fatal("a source domain overwrote the complete inbox kind contract")
	}
	if mapAt(item, "properties", "handoff_id")["readOnly"] != true {
		t.Fatal("handoff location must come from the validated server projection")
	}
	for _, field := range item["required"].([]string) {
		if field == "handoff_id" {
			t.Fatal("unavailable sources and other event kinds must omit a handoff location")
		}
	}
	if mapAt(api, "components", "schemas", "TaskHandoff", "properties")["notification_id"] != nil {
		t.Fatal("private source binding must not become a public handoff field")
	}
}
