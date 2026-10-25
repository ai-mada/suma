package main

import "testing"

func TestCandidateTransferContractKeepsMachineScopeAndBinaryBytes(t *testing.T) {
	api, _ := generate()
	for _, namespace := range []string{"runner", "pool-runner"} {
		base := "/" + namespace + "/candidate-transfers"
		for _, path := range []string{base, base + "/resolve", base + "/{id}/seal"} {
			op := mapAt(api, "paths", path, "post")
			security := op["security"].([]any)
			if len(security) != 1 || security[0].(map[string]any)["bearerAuth"] == nil {
				t.Fatal("machine-only candidate route changed", path)
			}
			if mapAt(op, "requestBody", "content", "application/json", "schema")["$ref"] == nil {
				t.Fatal("candidate request lost its typed contract", path)
			}
		}
		put := mapAt(api, "paths", base+"/{id}/bundle", "put")
		if mapAt(put, "requestBody", "content", "application/octet-stream", "schema")["format"] != "binary" {
			t.Fatal("bundle became JSON text")
		}
		get := mapAt(api, "paths", base+"/{id}/bundle", "get")
		if mapAt(get, "responses", "200", "content", "application/octet-stream", "schema")["format"] != "binary" {
			t.Fatal("download lost binary response")
		}
	}
	props := mapAt(api, "components", "schemas", "CandidateAssignmentRef", "properties")
	for _, field := range []string{"pool_grant_id", "pool_grant_version", "pool_credential_id", "pool_credential_version", "dispatch_id", "lease_until"} {
		if props[field] == nil {
			t.Fatal("pool callback identity is incomplete", field)
		}
	}
	if mapAt(api, "components", "schemas", "CandidateAssignmentRef", "properties", "lease_until")["type"] != "string" {
		t.Fatal("lease timestamp wire type changed")
	}
	// The pre-existing release identity remains a different reflected type;
	// registering the new contract must not replace its global schema by name.
	if _, exists := mapAt(api, "components", "schemas", "AssignmentRef", "properties")["pool_grant_id"]; exists {
		t.Fatal("candidate identity replaced the release contract")
	}
}
