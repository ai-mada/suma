package main

import (
	"github.com/ai-mada/suma/internal/platform"
	"github.com/ai-mada/suma/internal/toolcontract"
)

func adapterCatalogModels() []any {
	return []any{platform.AdapterCatalogPage{}, toolcontract.AdapterDescriptor{}, toolcontract.AdapterCapability{}}
}

func adapterCatalogContracts(add operationAdder, api map[string]any) {
	op := add("/adapter-catalog", "get", "AdapterCatalogPage", false, false)
	successStatus(op, "200")
	op["x-read-only"] = true
	op["x-disallowed-token-scopes"] = []string{"runner", "agent"}
	op["description"] = "Current human project-authorized read of maintained source adapter capabilities and connection directions. Implemented is source support, not installed, healthy, authorized to execute or vendor-verified. No credentials, runtime observations or task execution are returned. Unknown or duplicate query fields are rejected; only project_id is accepted. Response is no-store."
	schemas := toolContractMap(api, "components", "schemas")
	field := func(model, name string, attrs map[string]any) {
		value := toolContractMap(schemas, model, "properties", name)
		for key, item := range attrs {
			value[key] = item
		}
	}
	for _, model := range []string{"AdapterCatalogPage", "AdapterDescriptor", "AdapterCapability"} {
		toolContractMap(schemas, model)["additionalProperties"] = false
		for _, value := range toolContractMap(schemas, model, "properties") {
			value.(map[string]any)["readOnly"] = true
		}
	}
	field("AdapterDescriptor", "direction", map[string]any{"enum": []string{"suma_to_tool", "tool_to_suma"}, "description": "suma_to_tool executes through the declared Suma adapter; tool_to_suma connects an external client to Suma. Neither implies the reverse direction."})
	field("AdapterDescriptor", "entrypoint_kind", map[string]any{"enum": []string{"cli", "sdk", "app_server", "mcp_client", "cloud_api"}})
	field("AdapterCatalogPage", "items", map[string]any{"maxItems": 64})
	field("AdapterDescriptor", "implementation", map[string]any{"enum": []string{"implemented", "guide_only", "planned"}, "description": "Implementation state of this exact entrypoint, not provider health or project dispatch eligibility."})
	field("AdapterDescriptor", "setup_kind", map[string]any{"enum": []string{"runner_profile", "client_export", "manual_guide"}})
	field("AdapterCapability", "support", map[string]any{"enum": []string{"supported", "unsupported", "unverified"}, "description": "Source capability for this exact entrypoint. Missing verification never implies supported."})
	field("AdapterCapability", "key", map[string]any{"enum": []string{"task_start", "history_read", "session_load", "new_turn", "interrupted_resume", "cancel_request", "terminal_observation", "host_stop_proof", "usage_observation", "provider_billing", "candidate_capture", "independent_test", "model_selection", "gateway_budget", "workspace_read", "workspace_write", "human_input", "configuration_export", "project_access"}})
	for _, model := range []string{"AdapterDescriptor", "AdapterCapability"} {
		field(model, "source_refs", map[string]any{"description": "Stable repository source or contract references. These do not authorize commands or expose secrets."})
		field(model, "limitations", map[string]any{"description": "Machine-readable limitation codes for localized explanatory text, not executable actions."})
	}
}
