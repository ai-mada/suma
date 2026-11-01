package main

import (
	"testing"

	ic "github.com/ai-mada/suma/internal/toolinspectioncontract"
)

func TestToolReinspectionContractsKeepExactAuthorityAndNullableEmptyClaim(t *testing.T) {
	api, types := generate()
	paths, schemas := mapAt(api, "paths"), mapAt(api, "components", "schemas")
	create := mapAt(paths, "/tool-profiles/{id}/inspections", "post")
	if create["x-disallowed-token-scopes"] == nil || create["security"] == nil || types == "" {
		t.Fatal("browser maintenance gained machine authority or lost its public types")
	}
	for _, pair := range []struct{ path, scope string }{{"/runner", "runner"}, {"/pool-runner", "pool"}} {
		for _, suffix := range []string{"claim", "results"} {
			op := mapAt(paths, pair.path+"/tool-inspections/"+suffix, "post")
			if op["x-required-token-scope"] != pair.scope {
				t.Fatal("inspection crossed a machine authority boundary", pair.path, suffix)
			}
		}
	}
	claim := mapAt(schemas, "ToolInspectionClaim", "properties", "inspection")
	branches, ok := claim["anyOf"].([]any)
	if !ok || len(branches) != 2 || branches[1].(map[string]any)["type"] != "null" {
		t.Fatal("an empty claim acquired a fabricated request")
	}
	if mapAt(schemas, "ToolInspectionReference", "properties", "fence")["minimum"] != 1 || mapAt(schemas, "ToolInspection", "properties", "fence")["minimum"] != 0 || mapAt(schemas, "ToolInspectionList", "properties", "items")["maxItems"] != 10 {
		t.Fatal("unclaimed request, execution reference or bounded history changed")
	}
	for _, field := range []string{"executable", "args", "env", "install", "model", "enable"} {
		if mapAt(schemas, "ToolInspectionCreateRequest", "properties")[field] != nil {
			t.Fatal("reinspection accepted arbitrary execution or configuration", field)
		}
	}
	for _, name := range []string{"ToolInspectionFailureRequest", "PoolToolInspectionFailureRequest"} {
		if len(mapAt(schemas, name)["oneOf"].([]any)) != 2 {
			t.Fatal("known not-started and unknown effects became interchangeable")
		}
	}
	if branches, ok := mapAt(schemas, "ToolInspection")["oneOf"].([]any); !ok || len(branches) != 8 {
		t.Fatal("inspection states lost their paired claim, result and inventory facts")
	}
	for _, name := range []string{"ToolInventoryRequest", "PoolToolInventoryRequest", "ToolInventoryReceipt"} {
		ref := mapAt(schemas, name, "properties", "inspection")
		if ref["$ref"] != "#/components/schemas/ToolInspectionReference" || ref["anyOf"] != nil {
			t.Fatal("explicit null could erase the original inspection reference", name)
		}
	}
	if branches, ok := mapAt(schemas, "ToolInventoryReceipt")["oneOf"].([]any); !ok || len(branches) != 2 {
		t.Fatal("inventory ACK acquired an unpaired inspection result")
	}
	operation := mapAt(schemas, "ToolInspectionCreateRequest", "properties", "idempotency_key")
	if !ic.Operation("工具重检") || operation["minLength"] != 1 || operation["x-min-utf8-bytes"] != 8 || operation["x-max-utf8-bytes"] != 128 {
		t.Fatal("byte-bounded operation IDs became character-bounded")
	}
}
