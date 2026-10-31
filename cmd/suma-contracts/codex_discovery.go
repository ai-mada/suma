package main

// The native discovery result is a shared process/SDK contract, not a new HTTP
// capability. Successful and uncertain observations cannot exchange catalogs.
func codexDiscoveryContracts(api map[string]any) {
	schemas := api["components"].(map[string]any)["schemas"].(map[string]any)
	schema := schemas["CodexModelDiscoveryResult"].(map[string]any)
	schema["additionalProperties"] = false
	properties := schema["properties"].(map[string]any)
	properties["protocol"].(map[string]any)["const"] = "suma-codex-model-discovery-v1"
	properties["state"].(map[string]any)["enum"] = []string{"ready", "failed", "unknown"}
	properties["error_code"].(map[string]any)["enum"] = []string{"codex_discovery_invalid", "codex_discovery_unknown", "codex_discovery_installation_changed", "codex_discovery_credential_unavailable", "codex_discovery_catalogue_unavailable", "codex_discovery_not_stopped", "codex_discovery_failed"}
	schema["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"state": map[string]any{"const": "ready"}}, "required": []string{"catalogue"}, "not": map[string]any{"required": []string{"error_code"}}},
		map[string]any{"properties": map[string]any{"state": map[string]any{"enum": []string{"failed", "unknown"}}}, "required": []string{"error_code"}, "not": map[string]any{"required": []string{"catalogue"}}},
	}
}
