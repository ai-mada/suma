package main

import (
	"encoding/json"
	"github.com/ai-mada/suma/internal/platform"
)

func applicationToolModels() []any {
	return []any{platform.ApplicationToolDefinition{}, platform.ApplicationToolList{}, platform.DeveloperConnectionMetadata{},
		platform.ApplicationQualityExecutionInput{}, platform.ApplicationQualityEvidenceReference{}, platform.ApplicationQualityCase{}, platform.ApplicationQualityExecutionPage{},
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
	applicationQualityContracts(api)
}

// Application tools and the REST declarations share the same public shapes.
// The MCP wire envelope remains owned by the official SDK. These limits describe
// the compact projection; they never grant access to full reports or execution.
func applicationQualityContracts(api map[string]any) {
	for _, model := range []string{"ApplicationQualityExecutionInput", "ApplicationQualityEvidenceReference", "ApplicationQualityCase", "ApplicationQualityExecutionPage"} {
		schema := toolContractMap(api, "components", "schemas", model)
		schema["additionalProperties"] = false
		if model != "ApplicationQualityExecutionInput" {
			schema["readOnly"] = true
		}
	}
	input := toolContractMap(api, "components", "schemas", "ApplicationQualityExecutionInput", "properties")
	for name, maximum := range map[string]int{"task_id": 64, "run_id": 100, "after_case": 64} {
		input[name] = map[string]any{"type": "string", "minLength": 1, "maxLength": maximum, "x-max-utf8-bytes": maximum}
	}
	input["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 20, "default": 10}
	toolContractMap(api, "components", "schemas", "ApplicationQualityExecutionPage", "properties", "cases")["maxItems"] = 20
	toolContractMap(api, "components", "schemas", "ApplicationQualityExecutionPage", "properties", "source_commits")["maxItems"] = 16
	toolContractMap(api, "components", "schemas", "ApplicationQualityExecutionPage", "properties", "complete")["description"] = "All frozen cases have been included up to this page; this is not task acceptance or production verification."
	toolContractMap(api, "components", "schemas", "ApplicationQualityCase", "properties", "evidence")["maxItems"] = 3
	toolContractMap(api, "components", "schemas", "ApplicationQualityCase", "properties", "evidence_complete")["description"] = "All evidence references for this case fit in the compact response. The full quality workspace retains the original evidence."
}
