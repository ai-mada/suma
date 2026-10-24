package main

import "testing"

func TestExecutionPoolContractsSeparateHumanAndMachineAuthority(t *testing.T) {
	api, _ := generate()
	paths := mapAt(api, "paths")
	for _, test := range []struct{ path, method, scope string }{
		{"/execution-pools", "get", "organization"},
		{"/execution-pools/{pool}/runners", "post", "organization"},
		{"/execution-pools/{pool}/policy-previews/{preview}/commit", "post", "organization"},
		{"/project-execution/route-previews", "post", "project"},
		{"/project-execution/route", "get", "project"},
		{"/pool-runner/claim", "post", "machine"},
		{"/pool-runner/results", "post", "machine"},
	} {
		op := mapAt(paths, test.path, test.method)
		if op["x-authorization-scope"] == nil {
			t.Fatalf("missing concrete scope %s", test.path)
		}
		project := false
		for _, param := range op["parameters"].([]any) {
			if param.(map[string]any)["name"] == "project_id" {
				project = true
			}
		}
		if project != (test.scope == "project") {
			t.Fatalf("implicit selected-project authority leaked into %s", test.path)
		}
		security := op["security"].([]any)[0].(map[string]any)
		key := "cookieAuth"
		if test.scope == "machine" {
			key = "poolRunnerAuth"
		}
		if security[key] == nil {
			t.Fatalf("wrong identity for %s", test.path)
		}
	}
	schemas := mapAt(api, "components", "schemas")
	for _, model := range []string{"ExecutionPoolPolicyPreviewRequest", "ExecutionRoutePreviewRequest", "PoolClaimRequest", "PoolRunnerResultRequest"} {
		if mapAt(schemas, model)["additionalProperties"] != false {
			t.Fatalf("unbounded request %s", model)
		}
	}
	token := mapAt(schemas, "ExecutionPoolCredentialResponse", "properties", "token")
	if token["readOnly"] != true || token["x-sensitive"] != true || token["writeOnly"] != nil {
		t.Fatal("one-time token contract does not describe an actual response secret")
	}
	if mapAt(api, "components", "securitySchemes")["poolRunnerAuth"] == nil {
		t.Fatal("pool machine security scheme missing")
	}
}
