package main

import "testing"

func TestTaskCollectionContractSharesLiteralFiltersAndBoundsGroupPages(t *testing.T) {
	api, _ := generate()
	if mapAt(api, "paths")["/task-collection/items"] != nil {
		t.Fatal("must not publish a second task-row reader")
	}
	query := func(op map[string]any) map[string]map[string]any {
		out := map[string]map[string]any{}
		for _, entry := range op["parameters"].([]any) {
			p := entry.(map[string]any)
			if p["in"] == "query" {
				out[p["name"].(string)] = p
			}
		}
		return out
	}
	summary := mapAt(api, "paths", "/task-collection/summary", "get")
	sq, wq := query(summary), query(mapAt(api, "paths", "/task-workspace", "get"))
	for _, key := range []string{"q", "status", "priority", "owner_id", "requirement_id", "unassigned", "status_unset", "priority_unset"} {
		if sq[key] == nil || wq[key] == nil {
			t.Fatal("missing shared filter", key)
		}
	}
	if sq["sort"] != nil || sq["limit"] != nil || sq["cursor"] != nil {
		t.Fatal("summary must not accept row continuation")
	}
	if sq["group_limit"]["schema"].(map[string]any)["maximum"] != 100 || sq["group_cursor"] == nil {
		t.Fatal("unbounded group headers")
	}
	if sq["status"]["schema"].(map[string]any)["enum"] != nil || sq["status"]["schema"].(map[string]any)["default"] != nil {
		t.Fatal("literal status must not use a sentinel")
	}
	if mapAt(api, "components", "schemas", "TaskCollectionSummary", "properties", "groups")["maxItems"] != 100 {
		t.Fatal("missing response bound")
	}
	for _, key := range []string{"groups_total", "groups_limit", "next_group_cursor"} {
		if mapAt(api, "components", "schemas", "TaskCollectionSummary", "properties")[key] == nil {
			t.Fatal("missing group metadata", key)
		}
	}
	if len(mapAt(api, "paths", "/task-collection/summary")) != 1 || summary["x-read-only"] != true {
		t.Fatal("summary must only observe")
	}
}
