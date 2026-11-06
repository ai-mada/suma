package main

import (
	tc "github.com/ai-mada/suma/internal/toolcontract"
	tp "github.com/ai-mada/suma/internal/toolpreparationcontract"
	"testing"
)

func TestManagedToolPreparationContractsKeepExplicitScopeAndOriginalInventory(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	create := toolContractMap(schemas, "ManagedToolPreparationCreateRequest")
	if create["additionalProperties"] != false || toolContractMap(create, "properties", "acknowledge_configuration_write")["const"] != true {
		t.Fatal("configuration write lost its explicit closed input")
	}
	options := toolContractMap(schemas, "ManagedToolPreparationConfiguration", "properties")
	if len(options) != 2 || options["model"] == nil || options["credential_env_ref"] == nil {
		t.Fatal("remote options became an arbitrary host command")
	}
	claim := toolContractMap(schemas, "ManagedToolPreparationClaim", "properties")
	for _, name := range []string{"tenant_id", "project_id", "worker_id", "preparation"} {
		if claim[name] == nil {
			t.Fatal("canonical null claim scope is missing", name)
		}
	}
	for _, name := range []string{"ToolInventoryRequest", "ToolInventoryReceipt", "PoolToolInventoryRequest"} {
		if toolContractMap(schemas, name, "properties")["preparation"] == nil {
			t.Fatal("original inventory did not carry accepted generation", name)
		}
	}
	paths := toolContractMap(api, "paths")
	for _, route := range []string{"/tool-preparations", "/tool-preparations/{id}", "/runner/tool-preparations/claim", "/runner/tool-preparations/results", "/pool-runner/tool-preparations/claim", "/pool-runner/tool-preparations/results"} {
		if paths[route] == nil {
			t.Fatal("bounded preparation route missing", route)
		}
	}
}

func TestManagedCLIInitialSchemaKeepsClosedAdapterConfigurations(t *testing.T) {
	api, _ := generate()
	schemas := toolContractMap(api, "components", "schemas")
	if toolContractMap(schemas, "ManagedToolPreparationCapability", "properties", "adapter_ids")["maxItems"] != len(tp.AdapterIDs()) {
		t.Fatal("factory capacity hardcoded to the old subset")
	}
	for _, name := range []string{"ManagedToolPreparation", "ManagedToolPreparationCreateRequest"} {
		choices, ok := toolContractMap(schemas, name)["oneOf"].([]any)
		if !ok || len(choices) != len(tp.AdapterIDs()) {
			t.Fatal("adapter-specific options are not closed", name)
		}
		modes := map[string]bool{}
		for _, raw := range choices {
			props := toolContractMap(raw.(map[string]any), "properties")
			id := toolContractMap(props, "adapter_id")["const"].(string)
			config := toolContractMap(props, "configuration")
			if config["additionalProperties"] != false || len(toolContractMap(config, "properties")) != 2 {
				t.Fatal("arbitrary machine options exposed")
			}
			model := toolContractMap(config, "properties", "model")
			credential := toolContractMap(config, "properties", "credential_env_ref")
			switch tp.ConfigurationMode(id) {
			case "existing_local_configuration":
				if model["const"] != "" || credential["const"] != "" {
					t.Fatal("native command model override advertised")
				}
			case "fixed_claude_model":
				if model["pattern"] != tc.ClaudeCLIModelPattern || credential["const"] != "" {
					t.Fatal("Claude option validation detached from original CLI")
				}
			case "model_and_credential":
				if model["minLength"] != 1 {
					t.Fatal("fixed ACP model became optional")
				}
			default:
				t.Fatal("unsupported factory advertised")
			}
			modes[id] = true
		}
		if len(modes) != len(tp.AdapterIDs()) || !modes["codex-app-server"] {
			t.Fatal("duplicate factory branches")
		}
	}
}
