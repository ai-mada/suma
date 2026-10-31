package main

import (
	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/platform"
)

func acpRuntimeModels() []any {
	return []any{ac.Capability{}, ac.Evidence{}, ac.PermissionOption{}, ac.PermissionRequest{}, ac.PermissionDecision{}, ac.Permission{}, ac.PermissionList{}, ac.DecisionRequest{}, platform.ToolPermissionOpenRequest{}, platform.ToolPermissionPollRequest{}, platform.PoolToolPermissionOpenRequest{}, platform.PoolToolPermissionPollRequest{}, ac.ACPModelChoice{}, ac.ACPModelCatalogue{}, ac.ACPModelDiscoveryEvidence{}, ac.ACPModelDiscoveryResult{}}
}

func acpRuntimeContracts(add operationAdder, api map[string]any) {
	for _, route := range []struct {
		path, method, input, output string
		browser, readonly           bool
	}{
		{"/runner/tool-permissions", "post", "ToolPermissionOpenRequest", "ToolPermission", false, false},
		{"/runner/tool-permissions/poll", "post", "ToolPermissionPollRequest", "ToolPermission", false, true},
		{"/pool-runner/tool-permissions", "post", "PoolToolPermissionOpenRequest", "ToolPermission", false, false},
		{"/pool-runner/tool-permissions/poll", "post", "PoolToolPermissionPollRequest", "ToolPermission", false, true},
		{"/runs/{run}/nodes/{node}/tool-permissions", "get", "", "ToolPermissionList", true, true},
		{"/tool-permissions/{id}/decision", "post", "ToolPermissionDecisionRequest", "ToolPermission", true, false},
	} {
		op := add(route.path, route.method, route.output, false, route.input != "")
		if route.browser {
			browserOnly(op)
		}
		successStatus(op, "200")
		op["description"] = "Exact original ACP process/attempt/fence and current human/machine authority. Saved decision, Runner observation, remote adoption, process stopping and quality evidence remain independent; a read never starts another agent."
		if route.input != "" {
			request(op, route.input)
			op["x-query-parameters-forbidden"], op["x-max-request-body-bytes"] = true, 64<<10
		}
		if !route.browser {
			security := "runnerAuth"
			if len(route.path) >= len("/pool-runner/") && route.path[:len("/pool-runner/")] == "/pool-runner/" {
				security = "poolRunnerAuth"
			}
			op["security"] = []any{map[string]any{security: []any{}}}
			op["parameters"] = []any{}
			op["x-machine-only"] = true
		}
		if route.readonly {
			op["x-read-only"], op["x-no-agent-effects"] = true, true
		}
		if route.method == "get" {
			for _, raw := range op["parameters"].([]any) {
				p := raw.(map[string]any)
				p["required"] = true
				maximum := 64
				if p["name"] == "node" {
					maximum = 100
				}
				p["schema"] = map[string]any{"type": "string", "minLength": 1, "maxLength": maximum}
			}
			op["parameters"] = append(op["parameters"].([]any), map[string]any{"name": "permission_id", "in": "query", "required": false, "description": "Read at most this exact original same-project/run/node historical permission; grants no execution or current-attempt approval.", "schema": map[string]any{"type": "string", "minLength": 1, "maxLength": 64}})
		}
		op["x-response-cache-control"] = "no-store"
	}
	for _, name := range []string{"ACPExecutionCapability", "ACPExecutionEvidence", "ToolPermissionOption", "ToolPermissionRequest", "ToolPermissionDecision", "ToolPermission", "ToolPermissionList", "ToolPermissionDecisionRequest", "ToolPermissionOpenRequest", "ToolPermissionPollRequest", "PoolToolPermissionOpenRequest", "PoolToolPermissionPollRequest", "ACPModelChoice", "ACPModelCatalogue", "ACPModelDiscoveryEvidence", "ACPModelDiscoveryResult"} {
		object := api["components"].(map[string]any)["schemas"].(map[string]any)[name].(map[string]any)
		object["additionalProperties"] = false
	}
	// These are safe local IPC observations, not new HTTP operations. A model
	// directory never grants a task, provider authentication or fee guarantee.
	schemas := api["components"].(map[string]any)["schemas"].(map[string]any)
	result := schemas["ACPModelDiscoveryResult"].(map[string]any)
	result["description"] = "Explicit local native ACP model observation after actual host termination; no Prompt/configuration writes. Ready is adoptable only under the original desktop scope, installation and nonrenewable observation deadline. Not a provider/model health or billing claim."
	props := result["properties"].(map[string]any)
	props["protocol"] = map[string]any{"type": "string", "enum": []any{ac.ModelDiscoveryProtocol}}
	props["runtime"] = map[string]any{"type": "string", "enum": []any{"qoder"}}
	props["state"] = map[string]any{"type": "string", "enum": []any{"ready", "failed", "unknown"}}
	catalogue := schemas["ACPModelCatalogue"].(map[string]any)["properties"].(map[string]any)
	catalogue["source"] = map[string]any{"type": "string", "enum": []any{"acp_session_config"}}
	catalogue["protocol_version"] = map[string]any{"type": "integer", "enum": []any{1}}
	catalogue["models"].(map[string]any)["minItems"] = 1
	catalogue["models"].(map[string]any)["maxItems"] = 256
}
