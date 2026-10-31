package main

import (
	ac "github.com/ai-mada/suma/internal/acpcontract"
	"github.com/ai-mada/suma/internal/platform"
)

func acpRuntimeModels() []any {
	return []any{ac.Capability{}, ac.Evidence{}, ac.PermissionOption{}, ac.PermissionRequest{}, ac.PermissionDecision{}, ac.Permission{}, ac.PermissionList{}, ac.DecisionRequest{}, platform.ToolPermissionOpenRequest{}, platform.ToolPermissionPollRequest{}, platform.PoolToolPermissionOpenRequest{}, platform.PoolToolPermissionPollRequest{}}
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
	for _, name := range []string{"ACPExecutionCapability", "ACPExecutionEvidence", "ToolPermissionOption", "ToolPermissionRequest", "ToolPermissionDecision", "ToolPermission", "ToolPermissionList", "ToolPermissionDecisionRequest", "ToolPermissionOpenRequest", "ToolPermissionPollRequest", "PoolToolPermissionOpenRequest", "PoolToolPermissionPollRequest"} {
		object := api["components"].(map[string]any)["schemas"].(map[string]any)[name].(map[string]any)
		object["additionalProperties"] = false
	}
}
