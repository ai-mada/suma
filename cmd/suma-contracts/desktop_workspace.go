package main

import "github.com/ai-mada/suma/internal/platform"

func desktopWorkspaceModels() []any {
	return []any{platform.WorkspaceDesktopGrant{}, platform.WorkspaceDesktopBindingRequest{}}
}

func desktopWorkspaceContracts(add operationAdder, api map[string]any) {
	op := add("/workspace/desktop-binding", "post", "WorkspaceMetadata", false, true)
	successStatus(op, "200")
	op["description"] = "Explicitly bind the current project to a native-selected local desktop directory. Current browser approval, an exact short-lived signed selection and original binding CAS are mandatory. Replaying the same committed operation restores its original receipt without a new filesystem effect. The selection does not broaden the global workspace root or grant execution authority."
	request(op, "WorkspaceDesktopBindingRequest")
	browserOnly(op)
	grant := toolContractMap(api, "components", "schemas", "WorkspaceDesktopGrant", "properties")
	grant["protocol"].(map[string]any)["enum"] = []string{"suma-desktop-workspace-v1"}
	grant["path"].(map[string]any)["maxLength"] = 4096
	grant["operation_id"].(map[string]any)["pattern"] = "^desktop-workspace-[a-f0-9]{32}$"
	for _, key := range []string{"user_id", "tenant_id", "project_id", "nonce"} {
		grant[key].(map[string]any)["pattern"] = "^[a-f0-9]{32}$"
	}
	grant["signature"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
	toolContractMap(api, "components", "schemas", "WorkspaceDesktopBindingRequest", "properties", "expected_binding_sha256")["pattern"] = "^(?:[a-f0-9]{64})?$"
}
