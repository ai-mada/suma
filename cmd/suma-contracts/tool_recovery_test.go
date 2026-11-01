package main

import "testing"

func TestToolRecoveryContractSeparatesProofClosureCapabilityAndExecution(t *testing.T) {
	api, types := generate()
	paths, schemas := mapAt(api, "paths"), mapAt(api, "components", "schemas")
	for _, path := range []string{"/tool-inspections/{id}/recoveries", "/tool-inspection-recoveries/{id}"} {
		get := mapAt(paths, path, "get")
		if get["x-response-cache-control"] != "no-store" || get["x-disallowed-token-scopes"] == nil {
			t.Fatal("browser recovery read lost current scope or cache isolation", path)
		}
		foundProject := false
		for _, raw := range get["parameters"].([]any) {
			p := raw.(map[string]any)
			if p["name"] == "project_id" && p["in"] == "query" && p["required"] == true {
				foundProject = true
			}
		}
		if !foundProject {
			t.Fatal("current project became implicit", path)
		}
	}
	for _, side := range []struct{ path, scope string }{{"/runner", "runner"}, {"/pool-runner", "pool"}} {
		for _, suffix := range []string{"", "/proofs", "/acknowledgements"} {
			op := mapAt(paths, side.path+"/tool-inspections/recovery"+suffix, "post")
			if op["x-required-token-scope"] != side.scope {
				t.Fatal("node proof used another principal", side.path, suffix)
			}
		}
	}
	for _, name := range []string{"ToolInspectionRecovery", "ToolMaintenanceCapability", "ToolMaintenanceStatus", "ToolInspectionStopProof"} {
		if mapAt(schemas, name)["additionalProperties"] != false {
			t.Fatal("open recovery proof or capability payload", name)
		}
	}
	proof := mapAt(schemas, "ToolInspectionStopProof")
	for _, field := range []string{"probe_returned", "all_native_trees_stopped", "no_pending_inventory"} {
		if mapAt(proof, "properties", field)["const"] != true {
			t.Fatal("missing or false stop facts acquired release authority", field)
		}
	}
	if mapAt(proof, "properties", "native_process_count")["minimum"] != 0 || mapAt(proof, "properties", "stopped_process_count")["x-equal-to"] != "native_process_count" {
		t.Fatal("zero native work or process-tree coverage became ambiguous")
	}
	for _, name := range []string{"ToolProfile", "ToolInventoryRequest", "ToolInventoryReceipt", "PoolToolInventoryRequest"} {
		value := mapAt(schemas, name, "properties", "maintenance")
		if value["$ref"] == nil || value["anyOf"] != nil {
			t.Fatal("optional native declaration gained a nullable bypass", name)
		}
	}
	poll := mapAt(schemas, "ToolInspectionRecoveryPoll", "properties", "recovery")
	if values, ok := poll["anyOf"].([]any); !ok || len(values) != 2 {
		t.Fatal("an empty recovery poll fabricated a record")
	}
	status := mapAt(schemas, "ToolMaintenanceStatus")
	foundChecked := false
	for _, field := range status["required"].([]string) {
		foundChecked = foundChecked || field == "checked_at"
	}
	if !foundChecked || len(status["oneOf"].([]any)) != 3 || types == "" {
		t.Fatal("capability deadline lost its server clock or closed states")
	}
	branches := mapAt(schemas, "ToolInspection")["oneOf"].([]any)
	if len(branches) != 8 || len(mapAt(schemas, "ToolInspectionRecovery")["oneOf"].([]any)) != 4 {
		t.Fatal("unknown closure was collapsed into successful inspection")
	}
	for _, raw := range branches {
		value := raw.(map[string]any)
		if mapAt(value, "properties", "state")["const"] == "superseded" && mapAt(value, "properties", "fence")["const"] != 0 {
			t.Fatal("started work could be hidden as superseded")
		}
	}
}
