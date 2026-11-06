package main

import (
	"github.com/ai-mada/suma/internal/platform"
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
	"strings"
)

func managedToolPreparationModels() []any {
	return []any{tp.ManagedToolPreparationCapability{}, tp.ManagedToolPreparationConfiguration{}, tp.ManagedToolPreparationReference{}, tp.ManagedToolPreparationStopProof{}, tp.ManagedToolPreparationGenerationReceipt{}, tp.ManagedToolPreparation{}, tp.ManagedToolPreparationCreateRequest{}, tp.ManagedToolPreparationClaimRequest{}, tp.PoolManagedToolPreparationClaimRequest{}, tp.ManagedToolPreparationClaim{}, tp.ManagedToolPreparationResultRequest{}, tp.PoolManagedToolPreparationResultRequest{}, tp.ManagedToolPreparationInventoryBinding{}, platform.ToolPreparationPage{}}
}
func managedToolPreparationRoutes(add operationAdder) {
	for _, r := range []struct{ path, method, input, output string }{{"/tool-preparations", "post", "ManagedToolPreparationCreateRequest", "ManagedToolPreparation"}, {"/tool-preparations", "get", "", "ToolPreparationPage"}, {"/tool-preparations/{id}", "get", "", "ManagedToolPreparation"}} {
		op := add(r.path, r.method, r.output, false, true)
		if r.input != "" {
			request(op, r.input)
			successStatus(op, "201")
		}
		if r.method == "get" {
			project := queryParameter("project_id", "Current project.", map[string]any{"type": "string", "minLength": 1})
			project["required"] = true
			appendQuery(op, project)
			if r.path == "/tool-preparations" {
				worker := queryParameter("worker_id", "Exact registered worker.", map[string]any{"type": "string", "minLength": 1})
				worker["required"] = true
				appendQuery(op, worker)
				appendQuery(op, queryParameter("limit", "Bounded page size.", map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}))
				appendQuery(op, queryParameter("cursor", "Original scope-bound cursor.", map[string]any{"type": "string", "maxLength": 2048}))
			}
		}
		op["x-response-cache-control"] = "no-store"
		op["description"] = "Explicit bounded preparation on the exact allowed maintenance owner. Configuration generation, original inventory acknowledgement, paused registration and enablement remain separate facts. Original operation replay does not authorize another native effect."
	}
	for _, ns := range []struct{ prefix, security, scope, claim, result string }{{"/runner", "runnerAuth", "runner", "ManagedToolPreparationClaimRequest", "ManagedToolPreparationResultRequest"}, {"/pool-runner", "poolRunnerAuth", "pool", "PoolManagedToolPreparationClaimRequest", "PoolManagedToolPreparationResultRequest"}} {
		for _, r := range []struct{ suffix, input, output string }{{"claim", ns.claim, "ManagedToolPreparationClaim"}, {"results", ns.result, "ManagedToolPreparation"}} {
			op := add(ns.prefix+"/tool-preparations/"+r.suffix, "post", r.output, false, true)
			request(op, r.input)
			successStatus(op, "200")
			op["security"] = []any{map[string]any{ns.security: []string{}}}
			op["x-required-token-scope"] = ns.scope
			op["description"] = "Authenticated original maintenance owner; durable nonce/reference/fence/source and exact receipt replay. Prepared is awaiting_inventory. Only an original correlated inventory transaction completes acceptance. No arbitrary shell or new task permission."
		}
	}
}
func managedToolPreparationSchemas(api map[string]any) {
	schemas := toolContractMap(api, "components", "schemas")
	names := []string{"ManagedToolPreparationCapability", "ManagedToolPreparationConfiguration", "ManagedToolPreparationReference", "ManagedToolPreparationStopProof", "ManagedToolPreparationGenerationReceipt", "ManagedToolPreparation", "ManagedToolPreparationCreateRequest", "ManagedToolPreparationClaimRequest", "PoolManagedToolPreparationClaimRequest", "ManagedToolPreparationClaim", "ManagedToolPreparationResultRequest", "PoolManagedToolPreparationResultRequest", "ManagedToolPreparationInventoryBinding", "ToolPreparationPage"}
	for _, name := range names {
		m := toolContractMap(schemas, name)
		m["additionalProperties"] = false
		for key, raw := range toolContractMap(m, "properties") {
			p := raw.(map[string]any)
			switch {
			case strings.HasSuffix(key, "sha256"):
				p["pattern"] = "^[a-f0-9]{64}$"
			case key == "version" || key == "expected_scan_version" || key == "installation_scan_version":
				p["minimum"], p["maximum"] = 1, 2147483646
			case key == "fence":
				p["minimum"], p["maximum"] = 1, 9007199254740991
				if name == "ManagedToolPreparation" {
					p["minimum"] = 0
				}
			case key == "native_process_count" || key == "stopped_process_count":
				p["minimum"], p["maximum"] = 0, tp.MaxNativeProcesses
			case key == "adapter_id":
				p["enum"] = tp.AdapterIDs()
			case key == "adapter_ids":
				p["minItems"], p["maxItems"], p["uniqueItems"] = 1, len(tp.AdapterIDs()), true
				p["items"] = map[string]any{"type": "string", "enum": tp.AdapterIDs()}
			}
		}
	}
	toolContractMap(schemas, "ManagedToolPreparationCapability", "properties", "protocol")["const"] = tp.Protocol
	toolContractMap(schemas, "ManagedToolPreparationStopProof", "properties", "protocol")["const"] = tp.StopProtocol
	for _, field := range []string{"probe_returned", "all_native_trees_stopped"} {
		toolContractMap(schemas, "ManagedToolPreparationStopProof", "properties", field)["const"] = true
	}
	toolContractMap(schemas, "ManagedToolPreparationCreateRequest", "properties", "acknowledge_configuration_write")["const"] = true
	toolContractMap(schemas, "ManagedToolPreparationConfiguration", "properties", "model")["x-max-utf8-bytes"] = 256
	toolContractMap(schemas, "ManagedToolPreparationConfiguration", "properties", "model")["description"] = "Fixed model for ACP and Claude CLI only. Must be empty for the other exact native-command preparation adapters; unsupported options are never silently ignored."
	managedToolPreparationConfigurationSchemas(schemas)
	toolContractMap(schemas, "ManagedToolPreparationConfiguration", "properties", "credential_env_ref")["description"] = "Separate local credential variable name, not its value. Cursor requires a nonempty reference; Qoder may use existing CLI login. Native CLI preparation requires an empty string and uses its original CLI identity. Codex app-server also requires an empty string and reuses the node administrator configuration. Reserved process/provider variables are rejected."
	toolContractMap(schemas, "ManagedToolPreparation", "properties", "state")["enum"] = tp.States()
	for _, name := range []string{"ManagedToolPreparationResultRequest", "PoolManagedToolPreparationResultRequest"} {
		toolContractMap(schemas, name, "properties", "outcome")["enum"] = []string{"prepared", "failed", "unknown", "closed_unknown"}
	}
	for _, name := range []string{"ToolInventoryRequest", "ToolInventoryReceipt", "PoolToolInventoryRequest"} {
		toolContractMap(schemas, name, "properties", "preparation")["description"] = "Original preparation reference, input source, accepted generation and local receipt. Same-transaction inventory acknowledgement completes the original operation; replays retain the original observation time."
	}
}

// The adapter and its options are one closed choice. Generate the same closed
// modes as the validator instead of widening model input for every tool.
func managedToolPreparationConfigurationSchemas(schemas map[string]any) {
	variants := []any{}
	for _, id := range tp.AdapterIDs() {
		model := map[string]any{"type": "string", "x-max-utf8-bytes": 256}
		credential := map[string]any{"type": "string"}
		switch tp.ConfigurationMode(id) {
		case "model_and_credential":
			model["minLength"] = 1
			if id == "cursor-cli" {
				credential["minLength"] = 1
			}
		case "fixed_claude_model":
			model["pattern"], model["maxLength"] = tc.ClaudeCLIModelPattern, 256
			credential["const"] = ""
		case "existing_local_configuration":
			model["const"], credential["const"] = "", ""
		}
		configuration := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"model", "credential_env_ref"}, "properties": map[string]any{"model": model, "credential_env_ref": credential}}
		variants = append(variants, map[string]any{"type": "object", "required": []string{"adapter_id", "configuration"}, "properties": map[string]any{"adapter_id": map[string]any{"const": id}, "configuration": configuration}})
	}
	for _, name := range []string{"ManagedToolPreparation", "ManagedToolPreparationCreateRequest"} {
		toolContractMap(schemas, name)["oneOf"] = variants
	}
}
