package main

import (
	"reflect"
	"testing"

	"github.com/ai-mada/suma/internal/platform"
)

func TestWorkBuddyTaskDispatchContractKeepsBrowserOwnerAndExplicitStart(t *testing.T) {
	api, _ := generate()
	schemas, paths := mapAt(api, "components", "schemas"), mapAt(api, "paths")
	if models["WorkBuddyTaskDispatchPlan"] != reflect.TypeOf(platform.WorkBuddyTaskDispatchPlan{}) {
		t.Fatal("plan schema detached from domain type")
	}
	for _, name := range []string{"WorkBuddyTaskDispatchPreviewRequest", "WorkBuddyTaskDispatchPlan", "WorkBuddyTaskDispatchStartRequest"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("open dispatch DTO", name)
		}
	}
	for _, name := range []string{"WorkBuddyTaskDispatchPreviewRequest", "WorkBuddyTaskDispatchStartRequest"} {
		properties := mapAt(schemas, name, "properties")
		for _, forbidden := range []string{"prompt", "url", "token", "owner_id", "assignment", "model", "task_id_override", "workflow"} {
			if properties[forbidden] != nil {
				t.Fatal("caller can replace frozen authority or goal", name, forbidden)
			}
		}
	}
	if mapAt(schemas, "WorkBuddyTaskDispatchPreviewRequest", "properties", "expected_task_version")["maximum"] != 2147483646 || mapAt(schemas, "WorkBuddyTaskDispatchPreviewRequest", "properties", "grant_generation")["maximum"] != 2147483647 {
		t.Fatal("current mutable version boundary diverged from grant generation")
	}
	for _, route := range []struct{ path, method, status, response string }{
		{"/workbuddy-task-dispatch-plans", "post", "201", "WorkBuddyTaskDispatchPlan"},
		{"/workbuddy-task-dispatch-plans/{id}", "get", "200", "WorkBuddyTaskDispatchPlan"},
		{"/workbuddy-task-dispatch-plans/{id}/start", "post", "201", "Run"},
	} {
		op := mapAt(paths, route.path, route.method)
		if !reflect.DeepEqual(op["security"], []any{map[string]any{"cookieAuth": []string{}}}) || op["x-no-provider-effects"] != true {
			t.Fatal("dispatch owner/browser boundary changed", route.path)
		}
		if route.method == "post" && op["x-query-parameters-forbidden"] != true {
			t.Fatal("write query ambiguity", route.path)
		}
		if mapAt(op, "responses", route.status, "content", "application/json", "schema")["$ref"] != "#/components/schemas/"+route.response {
			t.Fatal("response contract drift", route.path)
		}
		for _, raw := range op["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["in"] == "query" && (route.method != "get" || p["name"] != "project_id" || p["required"] != true) {
				t.Fatal("ambiguous query contract", route.path)
			}
		}
	}
}
