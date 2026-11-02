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

func TestExecutionPoolRunnerExactReadContract(t *testing.T) {
	api, _ := generate()
	op := mapAt(api, "paths", "/execution-pools/{pool}/runners/{runner}", "get")
	if op["x-read-only"] != true || op["x-response-cache-control"] != "no-store" || op["x-query-parameters-forbidden"] != true || op["requestBody"] != nil {
		t.Fatal("exact registered runner read is not a closed, uncached read-only operation")
	}
	security := op["security"].([]any)[0].(map[string]any)
	if security["cookieAuth"] == nil || op["x-authorization-scope"] != "current organization owner/admin; exact browser credential" {
		t.Fatal("exact pool runner read expanded machine or project authority")
	}
	parameters := map[string]bool{}
	for _, raw := range op["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] == "query" {
			t.Fatal("exact runner read accepts an implicit query scope")
		}
		if parameter["in"] == "path" {
			parameters[parameter["name"].(string)] = true
		}
	}
	if len(parameters) != 2 || !parameters["pool"] || !parameters["runner"] {
		t.Fatal("exact pool and runner identities are not required path inputs")
	}
	response := mapAt(op, "responses", "200", "content", "application/json", "schema")
	if response["$ref"] != "#/components/schemas/ExecutionPoolRunner" {
		t.Fatal("read reused a credential response or a list instead of the public runner DTO")
	}
	fields := mapAt(api, "components", "schemas", "ExecutionPoolRunner", "properties")
	if fields["token"] != nil || fields["token_hash"] != nil {
		t.Fatal("public runner DTO discloses a credential")
	}
}
