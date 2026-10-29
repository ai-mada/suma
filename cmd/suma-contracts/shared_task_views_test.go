package main

import "testing"

func TestSharedTaskViewsContractKeepsScopesVersionsAndClosedPreferences(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, route := range []struct{ path, method, request string }{
		{"/task-views", "post", "SharedTaskViewCreateRequest"},
		{"/task-views/{id}", "patch", "SharedTaskViewUpdateRequest"},
		{"/task-views/{id}/archive", "post", "SharedTaskViewArchiveRequest"},
	} {
		op := mapAt(paths, route.path, route.method)
		if mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.request {
			t.Fatal("untyped mutation", route.path)
		}
		model := mapAt(api, "components", "schemas", route.request)
		if model["additionalProperties"] != false || mapAt(model, "properties", "idempotency_key")["minLength"] != 8 {
			t.Fatal("missing closed command or stable operation", route.request)
		}
		if route.method != "post" || route.path != "/task-views" {
			if mapAt(model, "properties", "expected_version")["minimum"] != 1 {
				t.Fatal("missing observed version", route.request)
			}
		}
	}
	prefs := mapAt(api, "components", "schemas", "SharedTaskViewPreferences")
	if prefs["additionalProperties"] != false || mapAt(prefs, "properties")["action"] != nil {
		t.Fatal("preferences must not allow arbitrary commands")
	}
	for _, name := range []string{"unassigned", "status_unset", "priority_unset"} {
		if mapAt(prefs, "properties", name)["type"] != "boolean" {
			t.Fatal("missing explicit empty-field semantics", name)
		}
	}
	if mapAt(api, "components", "schemas", "SharedTaskViewPage", "properties", "items")["maxItems"] != 100 {
		t.Fatal("missing bounded list")
	}
	for _, name := range []string{"can_edit", "can_delete", "owner_id", "version", "state"} {
		if mapAt(api, "components", "schemas", "SharedTaskView", "properties", name)["readOnly"] != true {
			t.Fatal("server authority must be read-only", name)
		}
	}
}
