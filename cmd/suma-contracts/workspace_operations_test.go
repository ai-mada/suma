package main

import (
	"strings"
	"testing"
)

func TestWorkspaceOperationContractsSeparateHostAuthorityFromSDK(t *testing.T) {
	api, wire := generate()
	schemas := mapAt(api, "components", "schemas")
	if mapAt(schemas, "Event", "properties")["run_id"] == nil || mapAt(schemas, "ToolEvent", "properties")["kind"] == nil || mapAt(schemas, "WorkspaceOperationEvent", "properties")["phase"] == nil {
		t.Fatal("host facts shadowed existing public event types")
	}
	for _, route := range []string{"/runner/progress", "/pool-runner/progress"} {
		if len(mapAt(api, "paths", route, "post", "requestBody", "content", "application/json", "schema")["oneOf"].([]any)) != 2 {
			t.Fatal("mixed provenance request accepted by contract")
		}
	}
	for _, name := range []string{"HostOperationProgressRequest", "PoolHostOperationProgressRequest", "WorkspaceOperationEvent"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatalf("open host envelope %s", name)
		}
	}
	hostWire := strings.SplitN(strings.SplitN(wire, "export interface WorkspaceOperationEvent", 2)[1], "\n}", 2)[0]
	if !strings.Contains(wire, "export interface WorkspaceOperationCoverage") || strings.Contains(hostWire, "exit_code?: number | null") {
		t.Fatal("required absence/null semantics lost in generated host facts")
	}
	for _, secret := range []string{"content", "output", "command", "args", "env", "token", "endpoint"} {
		if mapAt(schemas, "WorkspaceOperationEvent", "properties")[secret] != nil {
			t.Fatalf("host projection exposes %s", secret)
		}
	}
}
