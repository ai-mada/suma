package main

import "github.com/ai-mada/suma/internal/platform"

func workspaceFileModels() []any {
	return []any{platform.WorkspaceFileDocument{}, platform.WorkspaceFileSaveRequest{}}
}

func workspaceFileContracts(add operationAdder, api map[string]any) {
	read := add("/workspace/file", "get", "WorkspaceFileDocument", false, false)
	successStatus(read, "200")
	read["description"] = "Read one authorized bound-workspace text file. context_sha256 binds the tenant, project, fresh binding identity and the actual opened root directory's OS identity. The context is opaque and must be retained with this file snapshot; equal file bytes or rebinding to the same path do not preserve it."
	read["x-request-schema-coverage"] = "required project_id and workspace-relative path; current authorization, binding and actual opened root identity are server-enforced"
	path := queryParameter("path", "Workspace-relative safe file path; never absolute, traversing, sensitive or linked.", map[string]any{"type": "string", "minLength": 1})
	path["required"] = true
	appendQuery(read, path)

	write := add("/workspace/file", "post", "WorkspaceFileDocument", false, true)
	successStatus(write, "200")
	request(write, "WorkspaceFileSaveRequest")
	// The typed body is the only source of POST scope, never a query override.
	write["parameters"] = []any{}
	write["description"] = "Save existing UTF-8 text with both original file hash and original workspace context. Replay of a completed exact operation restores its immutable original document before checking the current root. workspace_context_changed with not_accepted=true requires a committed terminal rejection receipt, created only after proving the original operation absent under current authorization and the project lock. Exact retries replay that rejection even if the original physical root returns. A prepared operation remains reconciliation-required when its original root context cannot be recovered."
	write["x-request-schema-coverage"] = "typed required save fields; lowercase context digest, content bounds, current authority, original receipts and locked context comparison are server-enforced"

	for _, name := range []string{"WorkspaceFileDocument", "WorkspaceFileSaveRequest"} {
		toolContractMap(api, "components", "schemas", name)["additionalProperties"] = false
	}
	document := toolContractMap(api, "components", "schemas", "WorkspaceFileDocument", "properties")
	document["hash"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
	document["context_sha256"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
	document["context_sha256"].(map[string]any)["description"] = "Opaque original workspace context; not an authority token or a file content hash."
	input := toolContractMap(api, "components", "schemas", "WorkspaceFileSaveRequest", "properties")
	input["project_id"].(map[string]any)["minLength"] = 1
	input["path"].(map[string]any)["minLength"] = 1
	input["expected_hash"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
	input["expected_context_sha256"].(map[string]any)["pattern"] = "^[a-f0-9]{64}$"
	input["expected_context_sha256"].(map[string]any)["description"] = "Copy context_sha256 from the original file GET. Never infer it from current binding metadata or another file request."
	input["idempotency_key"].(map[string]any)["minLength"] = 8
	input["idempotency_key"].(map[string]any)["maxLength"] = 128
}
