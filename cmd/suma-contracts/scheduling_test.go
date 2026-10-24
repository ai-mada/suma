package main

import "testing"

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
