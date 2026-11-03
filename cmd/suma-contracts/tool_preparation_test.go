package main

import "testing"

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
