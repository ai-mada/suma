package main

import "testing"

func TestPoolRunnerPairingContractKeepsOrganizationAndMachinePurposesSeparate(t *testing.T) {
	api, types := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, name := range []string{"PoolRunnerPairing", "PoolRunnerPairingPage", "PoolRunnerPairingCreateRequest", "PoolRunnerPairingCommandRequest", "PoolRunnerPairingProposalRequest", "PoolRunnerPairingStatusRequest"} {
		model := mapAt(schemas, name)
		if model["additionalProperties"] != false {
			t.Fatalf("Pool pairing must be closed: %s", name)
		}
		for _, field := range []string{"token", "secret", "cookie", "password", "project_id"} {
			if mapAt(model, "properties")[field] != nil {
				t.Fatalf("Pool pairing cannot accept secret/project authority: %s.%s", name, field)
			}
		}
	}
	if mapAt(schemas, "PoolRunnerPairing", "properties", "purpose")["const"] != "pool_runner" || len(types) == 0 {
		t.Fatal("wire types and fixed purpose must be generated together")
	}
	if mapAt(schemas, "PoolRunnerPairingProposalRequest", "properties", "device_name")["x-max-utf8-bytes"] != 100 {
		t.Fatal("self-reported machine name must share the original100-byte bound")
	}
	for _, suffix := range []string{"proposal", "status"} {
		op := mapAt(api, "paths", "/pool-runner-pairings/{id}/"+suffix, "post")
		security := op["security"].([]any)
		if len(security) != 1 || security[0].(map[string]any)["poolRunnerPairingProof"] == nil {
			t.Fatal("preapproved pairing proof cannot be a Pool execution or browser credential")
		}
		for _, raw := range op["parameters"].([]any) {
			parameter := raw.(map[string]any)
			if parameter["in"] == "query" && parameter["name"] == "project_id" {
				t.Fatal("machine pairing cannot inherit a project query")
			}
		}
	}
	if mapAt(api, "paths", "/pool-runner-pairings/{id}/status", "post")["x-read-only"] != true {
		t.Fatal("status must not issue or rotate credentials")
	}
	security := mapAt(api, "paths", "/execution-pools/{pool_id}/runner-pairings/{id}/commands", "post")["security"].([]any)
	if len(security) != 1 || security[0].(map[string]any)["cookieAuth"] == nil {
		t.Fatal("approval remains current human Pool administration")
	}
}
