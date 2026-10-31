package main

func enhanceToolPermissionSchemas(schemas map[string]any) {
	for _, name := range []string{"ToolPermissionRequest", "ToolPermissionChoice", "ToolPermissionDecision", "ToolPermissionDecisionRequest"} {
		object := toolContractMap(schemas, name)
		toolContractMap(object, "properties", "protocol")["enum"] = []string{"acp-stdio-v1", "codex-app-server-v1"}
		object["oneOf"] = []any{
			map[string]any{"required": []string{"acp"}, "properties": map[string]any{"protocol": map[string]any{"const": "acp-stdio-v1"}}, "not": map[string]any{"required": []string{"codex"}}},
			map[string]any{"required": []string{"codex"}, "properties": map[string]any{"protocol": map[string]any{"const": "codex-app-server-v1"}}, "not": map[string]any{"required": []string{"acp"}}},
		}
	}
	toolContractMap(schemas, "ToolPermissionRequest")["description"] = "One exact native protocol branch, bounded to64KiB encoded JSON. No bare ACP fallback; identities are not translated between protocols."
	toolContractMap(schemas, "ToolPermission", "properties", "allowed_actions")["items"] = map[string]any{"type": "string", "enum": []string{"allow_once", "reject_once", "accept", "decline", "cancel"}}
	toolContractMap(schemas, "ToolPermission", "properties", "state")["enum"] = []string{"pending", "decided", "expired", "superseded"}
	toolContractMap(schemas, "ToolPermissionList", "properties", "items")["maxItems"] = 32
	toolContractMap(schemas, "ACPToolPermissionChoice", "properties", "kind")["enum"] = []string{"allow_once", "reject_once"}
	toolContractMap(schemas, "ToolPermissionOption", "properties", "kind")["enum"] = []string{"allow_once", "reject_once", "allow_always", "reject_always"}
	toolContractMap(schemas, "ACPToolPermissionSource", "properties", "options")["maxItems"] = 16
	toolContractMap(schemas, "ACPToolPermissionSource", "properties", "options")["minItems"] = 1
	toolContractMap(schemas, "CodexApprovalChoice", "properties", "decision")["enum"] = []string{"accept", "decline", "cancel"}
	toolContractMap(schemas, "CodexRPCRequestID", "properties", "kind")["enum"] = []string{"string", "number"}
	toolContractMap(schemas, "CodexRPCRequestID", "properties", "value")["maxLength"] = 256
	toolContractMap(schemas, "CodexRPCRequestID")["description"] = "Direction-specific original JSON-RPC identity. Number kind uses a canonical signed int64 decimal string, never a JavaScript floating point value."
	toolContractMap(schemas, "CodexApprovalRequest", "properties", "method")["enum"] = []string{"item/commandExecution/requestApproval", "item/fileChange/requestApproval"}
	toolContractMap(schemas, "CodexApprovalRequest", "properties", "changes")["maxItems"] = 128
	toolContractMap(schemas, "CodexApprovalFileChange", "properties", "kind")["enum"] = []string{"add", "delete", "update"}
	toolContractMap(schemas, "CodexApprovalObservation", "properties", "sequence")["minimum"] = 1
	toolContractMap(schemas, "CodexApprovalObservation", "properties", "sequence")["maximum"] = 9007199254740991
	toolContractMap(schemas, "CodexApprovalObservation", "properties", "item_status")["enum"] = []string{"inProgress", "completed", "failed", "declined"}
	toolContractMap(schemas, "CodexApprovalObservation")["description"] = "Cumulative original-machine observations. A resolved request can also be cleanup; it does not establish command execution, process stopping, tests or human acceptance. Original operation replay returns its stored snapshot, separately from current reads."
	toolContractMap(schemas, "CodexEvidence", "properties", "approvals")["maxItems"] = 32
	for _, name := range []string{"ToolPermissionRequest", "ToolPermissionDecisionRequest", "ToolPermissionOpenRequest", "PoolToolPermissionOpenRequest", "ToolPermissionObservationRequest", "PoolToolPermissionObservationRequest"} {
		if property, ok := toolContractMap(schemas, name, "properties")["idempotency_key"].(map[string]any); ok {
			property["minLength"], property["maxLength"] = 8, 128
		}
	}
}
