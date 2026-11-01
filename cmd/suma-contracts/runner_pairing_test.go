package main

import "testing"

func TestRunnerPairingContractSeparatesBrowserApprovalFromMachineProof(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	for _, model := range []string{"RunnerPairing", "RunnerPairingCreateRequest", "RunnerPairingCommandRequest", "RunnerPairingProposalRequest", "RunnerPairingStatusRequest"} {
		if mapAt(schemas, model)["additionalProperties"] != false {
			t.Fatalf("pairing envelope must be closed: %s", model)
		}
		fields := mapAt(schemas, model, "properties")
		for _, secret := range []string{"token", "secret", "cookie", "password"} {
			if fields[secret] != nil {
				t.Fatalf("pairing JSON must never deliver a secret: %s.%s", model, secret)
			}
		}
	}
	for _, suffix := range []string{"proposal", "status"} {
		op := mapAt(api, "paths", "/runner-pairings/{id}/"+suffix, "post")
		security := op["security"].([]any)
		if len(security) != 1 || security[0].(map[string]any)["runnerPairingProof"] == nil {
			t.Fatal("machine proposal/status must not inherit browser or general Runner authority")
		}
		for _, value := range op["parameters"].([]any) {
			p := value.(map[string]any)
			if p["in"] == "query" && p["name"] == "project_id" {
				t.Fatal("machine scope must come from the exact original JSON operation")
			}
		}
	}
	if mapAt(api, "paths", "/runner-pairings/{id}/status", "post")["x-read-only"] != true {
		t.Fatal("machine status must not become a credential issuance command")
	}
	approve := mapAt(api, "paths", "/runner-pairings/{id}/commands", "post")["security"].([]any)
	if len(approve) != 1 || approve[0].(map[string]any)["cookieAuth"] == nil {
		t.Fatal("pairing approval requires current real browser administration")
	}
}
