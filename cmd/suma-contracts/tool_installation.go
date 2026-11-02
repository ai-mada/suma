package main

import (
	"github.com/ai-mada/suma/internal/platform"
	ii "github.com/ai-mada/suma/internal/toolinstallationcontract"
)

func toolInstallationModels() []any {
	return []any{ii.ToolInstallationCapability{}, ii.ToolInstallationScanReference{}, ii.ToolInstallationItem{}, ii.ToolInstallationStopProof{}, ii.ToolInstallationScanResult{}, ii.ToolInstallationScan{}, ii.ToolInstallationScanCreateRequest{}, ii.ToolInstallationScanClaimRequest{}, ii.PoolToolInstallationScanClaimRequest{}, ii.ToolInstallationScanClaim{}, ii.ToolInstallationScanResultRequest{}, ii.PoolToolInstallationScanResultRequest{}, platform.ToolInstallationScanPage{}}
}

func toolInstallationRoutes(add operationAdder) {
	for _, route := range []struct{ path, method, input, output string }{
		{"/tool-installation-scans", "post", "ToolInstallationScanCreateRequest", "ToolInstallationScan"},
		{"/tool-installation-scans", "get", "", "ToolInstallationScanPage"},
		{"/tool-installation-scans/{id}", "get", "", "ToolInstallationScan"},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		browserOnly(op)
		if route.input != "" {
			request(op, route.input)
			successStatus(op, "201")
		} else {
			successStatus(op, "200")
		}
		if route.method == "get" && route.path == "/tool-installation-scans" {
			op["parameters"] = append(op["parameters"].([]any), map[string]any{"in": "query", "name": "worker_id", "required": true, "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 150}}, map[string]any{"in": "query", "name": "limit", "schema": map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}}, map[string]any{"in": "query", "name": "cursor", "schema": map[string]any{"type": "string", "maxLength": 2048}})
		}
		op["x-response-cache-control"] = "no-store"
		op["description"] = "Fixed-directory discovery on one current authorized machine that opted in. Open scans and unresolved maintenance are mutually exclusive. Repeat creation recovers its original receipt; GET its exact ID for the current head. Installation is not configuration, login, model health, dispatch or acceptance."
	}
	for _, namespace := range []struct{ prefix, scope, security, claim, result string }{
		{"/runner", "runner", "runnerAuth", "ToolInstallationScanClaimRequest", "ToolInstallationScanResultRequest"},
		{"/pool-runner", "pool", "poolRunnerAuth", "PoolToolInstallationScanClaimRequest", "PoolToolInstallationScanResultRequest"},
	} {
		for _, route := range []struct{ suffix, input, output string }{{"claim", namespace.claim, "ToolInstallationScanClaim"}, {"results", namespace.result, "ToolInstallationScan"}} {
			op := add(namespace.prefix+"/tool-installation-scans/"+route.suffix, "post", route.output, false, true)
			request(op, route.input)
			successStatus(op, "200")
			op["security"] = []any{map[string]any{namespace.security: []string{}}}
			op["x-required-token-scope"] = namespace.scope
			op["description"] = "Original outbound maintenance owner persists nonce, source, claim/fence and result before transport. Null scan acknowledges an empty claim. Expired claims are never reassigned or reprobed. Complete coverage requires durable native stop evidence. Unknown contains no observations. Only original stop receipts can close unknown; current identity and pool grants are rechecked."
		}
	}
}

// Reflection remains the single field source. Domain validators enforce the
// transactional source and cross-record rules that schemas cannot authorize.
func toolInstallationSchemas(api map[string]any) {
	schemas := toolContractMap(api, "components", "schemas")
	names := []string{"ToolInstallationCapability", "ToolInstallationScanReference", "ToolInstallationItem", "ToolInstallationStopProof", "ToolInstallationScanResult", "ToolInstallationScan", "ToolInstallationScanCreateRequest", "ToolInstallationScanClaimRequest", "PoolToolInstallationScanClaimRequest", "ToolInstallationScanClaim", "ToolInstallationScanResultRequest", "PoolToolInstallationScanResultRequest", "ToolInstallationScanPage"}
	for _, name := range names {
		model := toolContractMap(schemas, name)
		model["additionalProperties"] = false
		for key, raw := range model["properties"].(map[string]any) {
			property := raw.(map[string]any)
			switch key {
			case "id", "project_id", "worker_id", "runner_id", "tenant_id", "requested_by", "scan_id":
				property["pattern"] = `^[A-Za-z0-9][A-Za-z0-9_.-]{0,149}$`
			case "idempotency_key", "claim_id", "instance_id":
				property["pattern"] = `^[A-Za-z0-9][A-Za-z0-9_.:-]{0,149}$`
			case "source_sha256", "receipt_sha256", "executable_sha256":
				property["pattern"] = `^[a-f0-9]{64}$`
			case "version":
				if name == "ToolInstallationItem" {
					property["minLength"], property["x-max-utf8-bytes"] = 1, 256
				} else {
					property["minimum"], property["maximum"] = 1, 2147483647
				}
			case "fence":
				property["minimum"] = 1
				if name == "ToolInstallationScan" {
					property["minimum"] = 0
				}
			case "adapter_ids":
				property["minItems"], property["maxItems"], property["uniqueItems"] = 1, 9, true
				property["items"] = map[string]any{"type": "string", "enum": ii.AdapterIDs()}
			case "items":
				property["maxItems"] = 9
				if name == "ToolInstallationScanPage" {
					property["maxItems"] = 100
				}
			case "native_process_count", "stopped_process_count":
				property["minimum"], property["maximum"] = 0, ii.MaxNativeProcesses
			case "created_at", "expires_at", "claimed_at", "lease_until", "finished_at", "observed_at", "as_of":
				property["format"] = "date-time"
			case "next_cursor":
				property["maxLength"] = 2048
			}
		}
	}
	toolContractMap(schemas, "ToolInstallationCapability", "properties", "protocol")["const"] = ii.Protocol
	item := toolContractMap(schemas, "ToolInstallationItem")
	toolContractMap(item, "properties", "adapter_id")["enum"] = ii.AdapterIDs()
	var correspondence []any
	for _, id := range ii.AdapterIDs() {
		correspondence = append(correspondence, map[string]any{"if": map[string]any{"properties": map[string]any{"adapter_id": map[string]any{"const": id}}}, "then": map[string]any{"properties": map[string]any{"protocol": map[string]any{"const": ii.AdapterProtocol(id)}}}})
	}
	item["allOf"] = correspondence
	toolContractMap(item, "properties", "reason_code")["enum"] = []string{"not_installed", "cursor_editor_only", "cursor_scan_incomplete", "unsupported_launcher", "invalid_package", "node_unavailable", "probe_failed", "cursor_agent_unverified", "cursor_launcher_unsupported", "workspace_required", "configuration_required", "version_unsupported", "acp_configuration_required"}
	toolContractMap(schemas, "ToolInstallationScan", "properties", "state")["enum"] = []string{"pending", "claimed", "completed", "failed", "unknown", "closed_unknown", "expired", "superseded"}
	toolContractMap(schemas, "ToolInstallationScan", "properties", "result_code")["enum"] = []string{"scan_completed", "not_started", "probe_timeout", "probe_interrupted", "worker_restarted", "request_expired", "source_changed"}
	for _, name := range []string{"ToolInstallationScanResultRequest", "PoolToolInstallationScanResultRequest"} {
		toolContractMap(schemas, name, "properties", "outcome")["enum"] = []string{"completed", "failed", "unknown", "closed_unknown"}
		toolContractMap(schemas, name, "properties", "failure_code")["enum"] = []string{"", "not_started", "probe_timeout", "probe_interrupted", "worker_restarted"}
	}
	proof := toolContractMap(schemas, "ToolInstallationStopProof")
	toolContractMap(proof, "properties", "protocol")["const"] = ii.StopProtocol
	for _, field := range []string{"probe_returned", "all_native_trees_stopped"} {
		toolContractMap(proof, "properties", field)["const"] = true
	}
	toolContractMap(proof, "properties", "stopped_process_count")["x-equal-to"] = "native_process_count"
	proof["description"] = "Exact original ref/source and durable receipt. Explicit counts include genuine zero trees; expiry, missing PIDs or a digest alone prove nothing."
	// Unknown maintenance status cannot carry a discovery declaration.
	status := toolContractMap(schemas, "ToolMaintenanceStatus")
	if branches, ok := status["oneOf"].([]any); ok && len(branches) > 0 {
		branches[0].(map[string]any)["not"] = inspectionAnyPresence("protocol", "instance_id", "observed_at", "expires_at", "installed_discovery")
	}
}
