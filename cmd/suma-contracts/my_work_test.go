package main

import "testing"

func TestMyWorkContractKeepsIdentityServerOwned(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/my-work", "get")
	for _, item := range op["parameters"].([]any) {
		p := item.(map[string]any)
		if p["name"] == "user_id" || p["name"] == "owner_id" {
			t.Fatal("caller cannot choose personal identity", p)
		}
	}
	if mapAt(api, "components", "schemas", "MyWorkPage", "properties", "items")["maxItems"] != 50 {
		t.Fatal("personal work must remain bounded")
	}
	if mapAt(api, "paths", "/my-work")["post"] != nil {
		t.Fatal("reading personal work may not dispatch or accept a task")
	}
	if mapAt(api, "components", "schemas", "MyWorkStatusCount", "properties", "total")["minimum"] != 0 {
		t.Fatal("exact counts need explicit semantics")
	}
}
