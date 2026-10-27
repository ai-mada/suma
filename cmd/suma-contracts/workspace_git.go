package main

import "github.com/ai-mada/suma/internal/platform"

func workspaceGitModels() []any {
	return []any{platform.WorkspaceGitStatus{}, platform.WorkspaceGitChange{}, platform.WorkspaceGitDiff{}}
}

func workspaceGitContracts(add operationAdder, api map[string]any) {
	status := add("/workspace/git", "get", "WorkspaceGitStatus", false, false)
	status["description"] = "Read current authorized bound-workspace Git observations only. No staging, checkout, commit, merge, network or execution. At most 200 displayable safe paths; restricted paths remain undisclosed. This is not candidate or test evidence."
	diff := add("/workspace/git/diff", "get", "WorkspaceGitDiff", false, false)
	diff["description"] = "Read exact bounded original/modified UTF-8 content for the current Git index or worktree. Recheck current authority and metadata; binary responses contain no source text. The selected-file observation is current, not a saved review or test proof."
	file := queryParameter("path", "Workspace-relative non-secret file path; never absolute or traversing.", map[string]any{"type": "string", "minLength": 1, "maxLength": 4096})
	file["required"] = true
	appendQuery(diff, file)
	appendQuery(diff, queryParameter("source", "Compare HEAD to index, or index to the current worktree.", map[string]any{"type": "string", "enum": []string{"worktree", "index"}, "default": "worktree"}))
	toolContractMap(api, "components", "schemas", "WorkspaceGitStatus", "properties", "reason_code")["enum"] = []string{"ready", "not_repository", "git_unavailable", "unsupported_layout", "unsafe_config"}
	toolContractMap(api, "components", "schemas", "WorkspaceGitDiff", "properties", "source")["enum"] = []string{"worktree", "index"}
}
