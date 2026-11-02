package main

import (
	"strings"
	"testing"
)

func TestToolInstallationContractsKeepFixedPurposeAndExplicitStopCounts(t *testing.T) {
	api, types := generate()
	paths, schemas := mapAt(api, "paths"), mapAt(api, "components", "schemas")
	for _, verb := range []string{"get", "post"} {
		op := mapAt(paths, "/tool-installation-scans", verb)
		if op["x-disallowed-token-scopes"] == nil || op["x-response-cache-control"] != "no-store" {
			t.Fatal("scan lost browser-only/current response scope", verb)
		}
	}
	found := map[string]bool{}
	for _, raw := range mapAt(paths, "/tool-installation-scans", "get")["parameters"].([]any) {
		parameter := raw.(map[string]any)
		if parameter["in"] == "query" && parameter["required"] == true {
			found[parameter["name"].(string)] = true
		}
	}
	if !found["project_id"] || !found["worker_id"] {
		t.Fatal("scan target became implicit")
	}
	for _, side := range []struct{ prefix, scope string }{{"/runner", "runner"}, {"/pool-runner", "pool"}} {
		for _, suffix := range []string{"claim", "results"} {
			if mapAt(paths, side.prefix+"/tool-installation-scans/"+suffix, "post")["x-required-token-scope"] != side.scope {
				t.Fatal("scan accepted another machine purpose")
			}
		}
	}
	ids := mapAt(schemas, "ToolInstallationScanCreateRequest", "properties", "adapter_ids")
	if ids["uniqueItems"] != true || ids["minItems"] != 1 || ids["maxItems"] != 9 {
		t.Fatal("fixed directory became unbounded")
	}
	properties := mapAt(schemas, "ToolInstallationItem", "properties")
	for _, forbidden := range []string{"executable", "command", "args", "path", "env", "token"} {
		if properties[forbidden] != nil {
			t.Fatal("private machine launch data became public", forbidden)
		}
	}
	proof := mapAt(schemas, "ToolInstallationStopProof")
	for _, field := range []string{"native_process_count", "stopped_process_count"} {
		present := false
		for _, raw := range proof["required"].([]string) {
			if raw == field {
				present = true
			}
		}
		if !present || mapAt(proof, "properties", field)["minimum"] != 0 {
			t.Fatal("missing process counts can masquerade as a real zero")
		}
	}
	for _, field := range []string{"probe_returned", "all_native_trees_stopped"} {
		if mapAt(proof, "properties", field)["const"] != true {
			t.Fatal("unknown stop state became proof")
		}
	}
	claim := mapAt(schemas, "ToolInstallationScanClaim", "properties", "scan")
	null := false
	for _, raw := range claim["anyOf"].([]any) {
		if raw.(map[string]any)["type"] == "null" {
			null = true
		}
	}
	if !null || !strings.Contains(types, "installed_discovery?: ToolInstallationCapability") {
		t.Fatal("empty claim or optional non-null capability contract drifted")
	}
}
