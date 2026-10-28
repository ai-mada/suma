package main

import "github.com/ai-mada/suma/internal/platform"

func workspaceTreeModels() []any {
	return []any{platform.WorkspaceMetadata{}, platform.WorkspaceTreeEntry{}, platform.WorkspaceTreePage{}}
}

func workspaceTreeContracts(add operationAdder, api map[string]any) {
	op := add("/workspace/tree", "get", "WorkspaceTreePage", false, false)
	op["description"] = "Read one current authorized directory with stable, bounded pagination. No recursive inventory or absolute host paths. A cursor fixes the original caller, binding, directory and metadata snapshot; changes require a fresh first page. This is not a full-tree or file-content snapshot."
	appendQuery(op, queryParameter("directory", "Workspace-relative directory; empty selects the root. Never absolute, traversing, sensitive or linked.", map[string]any{"type": "string", "maxLength": 1024, "default": ""}))
	appendQuery(op, queryParameter("limit", "Maximum entries in this page.", map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "default": 100}))
	appendQuery(op, queryParameter("before", "Opaque exact-directory snapshot cursor returned by the preceding page. Omit for a fresh first page.", map[string]any{"type": "string", "minLength": 1, "maxLength": 1024}))
	toolContractMap(api, "components", "schemas", "WorkspaceTreeEntry", "properties", "kind")["enum"] = []string{"file", "directory"}
}
