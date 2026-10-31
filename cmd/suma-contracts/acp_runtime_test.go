package main

import (
	"reflect"
	"testing"

	"github.com/ai-mada/suma/internal/platform"
)

func TestACPPermissionContractsKeepSeparateMachineAndHumanAuthority(t *testing.T) {
	api, _ := generate()
	schemas, paths := mapAt(api, "components", "schemas"), mapAt(api, "paths")
	schemes := mapAt(api, "components", "securitySchemes")
	if models["ToolPermissionOpenRequest"] != reflect.TypeOf(platform.ToolPermissionOpenRequest{}) || models["PoolToolPermissionOpenRequest"] != reflect.TypeOf(platform.PoolToolPermissionOpenRequest{}) {
		t.Fatal("machine request schemas detached from real handler types")
	}
	for _, route := range []struct{ path, method, authority string }{{"/runner/tool-permissions", "post", "runnerAuth"}, {"/runner/tool-permissions/poll", "post", "runnerAuth"}, {"/pool-runner/tool-permissions", "post", "poolRunnerAuth"}, {"/pool-runner/tool-permissions/poll", "post", "poolRunnerAuth"}, {"/runs/{run}/nodes/{node}/tool-permissions", "get", "cookieAuth"}, {"/tool-permissions/{id}/decision", "post", "cookieAuth"}} {
		op := mapAt(paths, route.path, route.method)
		security := op["security"].([]any)
		if len(security) != 1 {
			t.Fatal("permission authority is ambiguous", route.path)
		}
		value := security[0].(map[string]any)
		if len(value) != 1 || value[route.authority] == nil || schemes[route.authority] == nil {
			t.Fatal("wrong or undefined credential authority", route.path, value)
		}
		if mapAt(op, "responses")["200"] == nil || mapAt(op, "responses")["201"] != nil || mapAt(op, "responses")["202"] != nil {
			t.Fatal("operation receipt must match actual HTTP status", route.path)
		}
	}
	for _, name := range []string{"ACPExecutionCapability", "ToolPermissionRequest", "ToolPermissionDecisionRequest", "ToolPermissionOpenRequest", "PoolToolPermissionOpenRequest"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("open permission/configuration request", name)
		}
	}
	for _, forbidden := range []string{"assignment", "fence", "runtime", "command", "token", "permission_mode", "expected_actor"} {
		if mapAt(schemas, "ToolPermissionDecisionRequest", "properties")[forbidden] != nil {
			t.Fatal("human can replace execution authority", forbidden)
		}
	}
	get := mapAt(paths, "/runs/{run}/nodes/{node}/tool-permissions", "get")
	found := false
	for _, raw := range get["parameters"].([]any) {
		p := raw.(map[string]any)
		if p["name"] == "permission_id" {
			found = p["required"] == false
		}
	}
	if !found || get["x-no-agent-effects"] != true {
		t.Fatal("exact original historical read gained an execution effect")
	}
}
