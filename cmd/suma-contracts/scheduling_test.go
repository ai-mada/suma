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
	for _, name := range []string{"route", "run_id", "tool_profile_id"} {
		if param := params[name]; param["in"] != "query" || (param["required"] != nil && param["required"] != false) {
			t.Fatalf("missing optional queue filter %s: %#v", name, param)
		}
	}
	if route := mapAt(params["route"], "schema"); !reflect.DeepEqual(route["enum"], []string{"project", "pool"}) {
		t.Fatalf("wrong frozen route filter: %#v", route)
	}
	for _, name := range []string{"run_id", "tool_profile_id"} {
		schema := mapAt(params[name], "schema")
		if schema["pattern"] != "^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$" || schema["maxLength"] != 100 {
			t.Fatalf("wrong exact ID filter %s: %#v", name, schema)
		}
	}
}

func TestSchedulingToolBindingIsOptionalClosedMetadata(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	admission := mapAt(schemas, "NodeAdmissionSnapshot")
	if value := mapAt(admission, "properties", "tool_binding"); value["$ref"] != "#/components/schemas/SchedulingToolBinding" || value["readOnly"] != true {
		t.Fatalf("unexpected tool binding schema: %#v", value)
	}
	for _, name := range admission["required"].([]string) {
		if name == "tool_binding" {
			t.Fatal("unbound nodes cannot require tool metadata")
		}
	}
	binding := mapAt(schemas, "SchedulingToolBinding")
	fields := mapAt(binding, "properties")
	if binding["additionalProperties"] != false || len(fields) != 3 ||
		mapAt(fields, "profile_id")["maxLength"] != 100 || mapAt(fields, "command_id")["maxLength"] != 100 ||
		mapAt(fields, "profile_sha256")["pattern"] != "^[a-f0-9]{64}$" {
		t.Fatalf("binding must expose only the fixed identity, digest and command: %#v", binding)
	}
}
