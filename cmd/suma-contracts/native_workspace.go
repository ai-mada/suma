package main

import nwc "github.com/ai-mada/suma/internal/nativeworkspacecontract"

func nativeWorkspaceModels() []any {
	return []any{nwc.NativeWorkspaceEvidence{}, nwc.NativeWorkspaceCheckResult{}, nwc.NativeWorkspaceFileEffect{}, nwc.NativeWorkspaceSeal{}}
}

func nativeWorkspaceSchemas(api map[string]any) {
	for _, name := range []string{"NativeWorkspaceEvidence", "NativeWorkspaceSeal"} {
		toolContractMap(api, "components", "schemas", name, "properties", "schema_version")["const"] = 1
	}
	for _, name := range []string{"NativeWorkspaceEvidence", "NativeWorkspaceCheckResult", "NativeWorkspaceFileEffect", "NativeWorkspaceSeal"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	toolContractMap(api, "components", "schemas", "NativeWorkspaceEvidence", "properties", "state")["enum"] = []string{"open", "closed", "unknown"}
	toolContractMap(api, "components", "schemas", "NativeWorkspaceSeal", "properties", "state")["enum"] = []string{"prepared", "unknown", "sealed", "no_change"}
}
