package main

import "testing"

func TestWorklogContractPreservesHumanSnapshotsAndExplicitWriteBasis(t *testing.T) {
	api, _ := generate()
	for path, requestType := range map[string]string{"/worklogs": "WorklogCreateRequest", "/worklogs/{id}/corrections": "WorklogCorrectionRequest", "/worklogs/{id}/void": "WorklogVoidRequest"} {
		if mapAt(api, "paths", path, "post", "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+requestType {
			t.Fatal("wrong manual mutation", path)
		}
		for _, item := range mapAt(api, "paths", path, "post")["parameters"].([]any) {
			if item.(map[string]any)["in"] == "query" {
				t.Fatal("typed worklog writes reject query fields", path)
			}
		}
	}
	if mapAt(api, "components", "schemas", "WorklogCorrectionRequest", "properties")["member_id"] != nil {
		t.Fatal("correction must not change the human identity")
	}
	if mapAt(api, "components", "schemas", "WorklogCreateRequest", "properties", "expected_task_version")["minimum"] != 1 {
		t.Fatal("explicit current task basis required")
	}
	if mapAt(api, "components", "schemas", "Worklog", "properties", "source")["const"] != "human_manual" {
		t.Fatal("manual reporting cannot masquerade as measured usage")
	}
	if mapAt(api, "components", "schemas", "WorklogCreateRequest", "properties", "minutes")["maximum"] != 1440 {
		t.Fatal("integer daily bound missing")
	}
	key := mapAt(api, "components", "schemas", "WorklogCreateRequest", "properties", "idempotency_key")
	if key["x-min-utf8-bytes"] != 8 || key["x-max-utf8-bytes"] != 128 || key["minLength"] != 2 {
		t.Fatal("operation key byte contract must not reject valid Unicode keys", key)
	}
	params := mapAt(api, "paths", "/worklogs", "get")["parameters"].([]any)
	required := map[string]bool{}
	for _, item := range params {
		p := item.(map[string]any)
		if p["required"] == true {
			required[p["name"].(string)] = true
		}
	}
	if !required["from_date"] || !required["to_date"] || !required["project_id"] {
		t.Fatal("list must declare complete bounded scope", required)
	}
}
