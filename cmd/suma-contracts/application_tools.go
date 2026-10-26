package main

import (
	"encoding/json"
	"github.com/ai-mada/suma/internal/platform"
)

func applicationToolModels() []any {
	return []any{platform.ApplicationToolDefinition{}, platform.ApplicationToolList{}, platform.DeveloperConnectionMetadata{},
		platform.ApplicationToolPageInput{}, platform.ApplicationToolIDInput{}, platform.ApplicationToolRequirementInput{}, platform.ApplicationToolTaskInput{}, platform.ApplicationToolSearchInput{}, platform.ApplicationToolKnowledgeInput{}, platform.ApplicationToolKnowledgeListInput{}, platform.ApplicationRequirementDraftInput{}, platform.ApplicationTaskDraftInput{}, platform.ApplicationRequirementDesignSaveInput{}, platform.ApplicationToolDraftResult{}, platform.ApplicationToolDesignDraftResult{}}
}

func applicationToolCatalogue() ([]byte, error) {
	data, err := json.MarshalIndent(platform.ApplicationToolList{Items: platform.DeclaredApplicationTools()}, "", "  ")
	return append(data, '\n'), err
}

// MCP protocol envelopes belong to the pinned official SDK, not suma's REST
// generator. Only the authenticated connection metadata is a suma REST DTO.
func applicationToolContracts(add operationAdder, api map[string]any) {
	op := add("/developer-connection", "get", "DeveloperConnectionMetadata", false, false)
	op["description"] = "Current browser or project API read. Declares available project MCP capabilities and an operator-trusted endpoint; never confirms client connectivity or exercises a credential. Agent and Runner credentials are rejected. No credentials are included."
	successStatus(op, "200")
	for _, name := range []string{"ApplicationToolDefinition", "ApplicationToolList", "DeveloperConnectionMetadata"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	toolContractMap(api, "components", "schemas", "DeveloperConnectionMetadata", "properties", "client_connection_verified")["const"] = false
	toolContractMap(api, "components", "schemas", "DeveloperConnectionMetadata", "properties", "transport")["const"] = "streamable_http"
	toolContractMap(api, "components", "schemas", "DeveloperConnectionMetadata", "properties", "endpoint_source")["enum"] = []string{"configured", "listener_loopback", "unconfigured"}
	toolContractMap(api, "components", "schemas", "ApplicationToolDefinition", "properties", "credential_scopes", "items")["enum"] = []string{"agent", "api"}
}
