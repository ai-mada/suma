package main

import "testing"

func TestTaskReviewRequestContractsKeepFormalDecisionSeparate(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, path := range []string{"/task-review-targets", "/task-review-requests/{id}"} {
		if len(mapAt(paths, path)) != 1 || mapAt(paths, path, "get")["x-read-only"] != true {
			t.Fatal("source reads cannot create responsibility or grant acceptance", path)
		}
	}
	props := mapAt(api, "components", "schemas", "TaskReviewRequestCreate", "properties")
	for _, key := range []string{"reviewer_user_id", "author_id", "decision", "can_accept"} {
		if props[key] != nil {
			t.Fatal("client cannot choose authority or a decision", key)
		}
	}
	for _, path := range []string{"/task-review-requests", "/task-review-requests/{id}/commands"} {
		for _, v := range mapAt(paths, path, "post")["parameters"].([]any) {
			if v.(map[string]any)["in"] == "query" {
				t.Fatal("writes preserve a closed original body", path)
			}
		}
	}
	if mapAt(api, "components", "schemas", "TaskReviewRequest", "properties")["can_accept"] != nil {
		t.Fatal("responsibility cannot shortcut formal Review")
	}
	kinds := mapAt(api, "components", "schemas", "NotificationInboxItem", "properties", "kind")["enum"].([]string)
	if len(kinds) != 5 || kinds[2] != "task_review_requested" || kinds[3] != "task_handoff_requested" || kinds[4] != "task_followed" {
		t.Fatal("real request source not registered", kinds)
	}
}
