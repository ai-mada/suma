package main

import (
	"reflect"
	"testing"
)

func TestSchedulingContractsExposeActualAdmissionAndDurableAttempts(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, path := range []string{"/scheduling/policy", "/scheduling/policy-previews", "/scheduling/policy-previews/{id}", "/scheduling/policy-previews/{id}/commit", "/scheduling/queue", "/runs/{id}/nodes/{node}/scheduling", "/runs/{id}/nodes/{node}/attempts", "/runs/{id}/nodes/{node}/attempts/{attempt}"} {
		if paths[path] == nil {
			t.Fatalf("missing implemented scheduling route %s", path)
		}
	}
	if mapAt(paths, "/scheduling/policy-previews/{id}/commit", "post")["x-disallowed-token-scopes"] == nil {
		t.Fatal("policy change missing human authority")
	}
	schemas := mapAt(api, "components", "schemas")
	for _, pair := range [][2]string{{"SchedulingPolicyInput", "budget_limit"}, {"NodeAdmissionSnapshot", "blockers"}, {"ExecutionAttempt", "result"}, {"Assignment", "dispatch_state"}, {"RunnerClaimRequest", "idempotency_key"}} {
		if mapAt(schemas, pair[0], "properties")[pair[1]] == nil {
			t.Fatal("missing durable scheduling field", pair)
		}
	}
	usage := mapAt(schemas, "ToolUsage", "properties", "input_tokens")
	if usage["type"] != "integer" || usage["maximum"] != int64(9007199254740991) || usage["anyOf"] != nil {
		t.Fatal("optional reported zero is not a strict safe integer", usage)
	}
}

func TestSchedulingQueueFilterContract(t *testing.T) {
	api, _ := generate()
	params := map[string]map[string]any{}
	for _, raw := range mapAt(api, "paths", "/scheduling/queue", "get")["parameters"].([]any) {
		entry := raw.(map[string]any)
		name, _ := entry["name"].(string)
		params[name] = entry
	}
	for _, name := range []string{"route", "run_id"} {
		if param := params[name]; param["in"] != "query" || (param["required"] != nil && param["required"] != false) {
			t.Fatalf("missing optional queue filter %s: %#v", name, param)
		}
	}
	if route := mapAt(params["route"], "schema"); !reflect.DeepEqual(route["enum"], []string{"project", "pool"}) {
		t.Fatalf("wrong frozen route filter: %#v", route)
	}
	if run := mapAt(params["run_id"], "schema"); run["pattern"] != "^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$" || run["maxLength"] != 100 {
		t.Fatalf("wrong exact run filter: %#v", run)
	}
}
