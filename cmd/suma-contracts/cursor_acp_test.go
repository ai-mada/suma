package main

import (
	"reflect"
	"testing"
)

// Public capability data must preserve runtime/authentication pairing. A
// generic ACP protocol alone cannot authorize borrowing another CLI's login.
func TestCursorACPContractKeepsRuntimeAuthenticationAndModelAuthority(t *testing.T) {
	api, _ := generate()
	schemas := mapAt(api, "components", "schemas")
	capability := mapAt(schemas, "ACPExecutionCapability")
	branches := capability["oneOf"].([]any)
	if len(branches) != 2 {
		t.Fatal("ACP runtime/authentication pairs are not explicit")
	}
	want := map[string]string{"qoder": "existing_cli_login", "cursor": "explicit_api_key_env"}
	for _, raw := range branches {
		properties := mapAt(raw.(map[string]any), "properties")
		runtime := mapAt(properties, "runtime")["const"].(string)
		if expected, ok := want[runtime]; !ok || mapAt(properties, "auth_strategy")["const"] != expected {
			t.Fatal("cross-runtime credential authority", properties)
		}
		delete(want, runtime)
	}
	if len(want) != 0 {
		t.Fatal("missing runtime authentication branch", want)
	}
	for field, expected := range map[string]any{"model_source": "administrator_session_config", "budget_mode": "observe_only", "permission_mode": "human_once"} {
		if mapAt(capability, "properties", field)["const"] != expected {
			t.Fatal("CLI authority changed", field)
		}
	}
	if !reflect.DeepEqual(mapAt(schemas, "ACPModelDiscoveryResult", "properties", "runtime")["enum"], []any{"qoder", "cursor"}) {
		t.Fatal("model discovery lost its exact runtime identity")
	}
	for _, field := range []string{"argv", "credential", "api_key", "executable", "package_root"} {
		if mapAt(capability, "properties")[field] != nil {
			t.Fatal("local execution authority escaped into public capability", field)
		}
	}
}
