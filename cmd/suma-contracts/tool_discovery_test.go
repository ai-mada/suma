package main

import "testing"

func TestToolDiscoveryContract(t *testing.T) {
	api, _ := generate()
	paths := api["paths"].(map[string]any)
	op := mapAt(paths, "/tool-discovery", "get")
	if op == nil || paths["/tool-discovery"].(map[string]any)["post"] != nil {
		t.Fatal("discovery must be one read-only operation")
	}
	parameters := op["parameters"].([]any)
	required := map[string]bool{}
	for _, raw := range parameters {
		parameter := raw.(map[string]any)
		if parameter["required"] == true {
			required[parameter["name"].(string)] = true
		}
	}
	if !required["project_id"] || !required["worker_id"] || op["x-disallowed-token-scopes"] == nil {
		t.Fatal("discovery lost its exact browser/project/worker scope")
	}
	schemas := api["components"].(map[string]any)["schemas"].(map[string]any)
	if schemas["ToolDiscovery"] == nil {
		t.Fatal("discovery schema missing")
	}
}
