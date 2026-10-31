package main

import "testing"

func TestUnifiedPermissionContractsKeepNativeBranchesAndObservationAuthority(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	paths := mapAt(api, "paths")
	for _, name := range []string{"ToolPermissionRequest", "ToolPermissionChoice", "ToolPermissionDecision", "ToolPermissionDecisionRequest"} {
		if len(mapAt(schemas, name)["oneOf"].([]any)) != 2 || mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("native protocol union not closed", name)
		}
	}
	for _, prefix := range []string{"/runner", "/pool-runner"} {
		op := mapAt(paths, prefix+"/tool-permissions/observations", "post")
		if op["x-machine-only"] != true || op["x-read-only"] == true || op["x-max-request-body-bytes"] != 1<<20 {
			t.Fatal("observation endpoint authority/body bound mismatch")
		}
	}
	if mapAt(schemas, "ToolPermissionRequest", "properties")["session_id"] != nil || mapAt(schemas, "ToolPermissionRequest", "properties")["command"] != nil {
		t.Fatal("native source escaped its protocol branch")
	}
	if mapAt(schemas, "CodexRPCRequestID", "properties", "value")["type"] != "string" {
		t.Fatal("RPC ID converted to floating point")
	}
}
