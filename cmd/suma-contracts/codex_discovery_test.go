package main

import "testing"

func TestCodexDiscoverySchemaSeparatesUsableAndUnknownCatalogues(t *testing.T) {
	api, _ := generate()
	schema := mapAt(api, "components", "schemas", "CodexModelDiscoveryResult")
	if schema["additionalProperties"] != false || len(schema["oneOf"].([]any)) != 2 || mapAt(schema, "properties", "protocol")["const"] != "suma-codex-model-discovery-v1" {
		t.Fatal("native catalogue result is not a closed state-dependent contract")
	}
	properties := mapAt(schema, "properties")
	for _, private := range []string{"executable", "base_url", "api_key_env_ref", "token", "goal", "model"} {
		if properties[private] != nil {
			t.Fatal("private configuration or task selection escaped the result", private)
		}
	}
}
