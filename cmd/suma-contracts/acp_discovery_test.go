package main

import (
	"strings"
	"testing"
)

func TestACPDiscoveryGeneratedContractIsLocalObservation(t *testing.T) {
	api, wire := generate()
	schemas := api["components"].(map[string]any)["schemas"].(map[string]any)
	for _, name := range []string{"ACPModelChoice", "ACPModelCatalogue", "ACPModelDiscoveryEvidence", "ACPModelDiscoveryResult"} {
		object, ok := schemas[name].(map[string]any)
		if !ok || object["additionalProperties"] != false || !strings.Contains(wire, "export interface "+name) {
			t.Fatalf("missing closed shared observation %s", name)
		}
	}
	result := schemas["ACPModelDiscoveryResult"].(map[string]any)["properties"].(map[string]any)
	for _, key := range []string{"session_id", "executable", "argv", "credential", "repository_path"} {
		if _, exists := result[key]; exists {
			t.Fatalf("local sensitive field escaped: %s", key)
		}
	}
	for path := range api["paths"].(map[string]any) {
		if strings.Contains(path, "acp-model") {
			t.Fatal("local discovery became a public HTTP command")
		}
	}
}
