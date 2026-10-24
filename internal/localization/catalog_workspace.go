package localization

// Only complete, server-owned diagnostics are matched. Workspace paths, file
// contents, unknown I/O errors and persisted save evidence remain unchanged.
var workspaceCatalog = []Definition{
	{Key: "workspace.project_not_configured", Texts: map[Locale]string{ZhCN: "请先配置当前项目的工作区", EnUS: "Configure a workspace for the current project first."}},
	{Key: "workspace.directory_unavailable", Texts: map[Locale]string{ZhCN: "工作区目录不可用", EnUS: "The workspace directory is unavailable."}},
	{Key: "workspace.read.text_limit", Texts: map[Locale]string{ZhCN: "编辑器仅打开 1 MiB 以内的文本文件", EnUS: "The editor can only open text files of at most 1 MiB."}},
	{Key: "workspace.save.hash_conflict", Texts: map[Locale]string{ZhCN: "文件保存冲突，请重新加载", EnUS: "The file could not be saved because it changed. Reload it."}},
	{Key: "workspace.save.reconciliation_required", Texts: map[Locale]string{ZhCN: "先前保存结果不确定；请将文件恢复为原内容或已提交内容后重试同一请求", EnUS: "The previous save result is uncertain. Restore the file to its original or submitted content, then retry the same request."}},
	{Key: "workspace.save.operation_invalid", Texts: map[Locale]string{ZhCN: "请提供 8–128 字符的 idempotency_key", EnUS: "Provide an idempotency_key of 8 to 128 characters."}},
	{Key: "workspace.save.content_limit", Texts: map[Locale]string{ZhCN: "仅支持 1 MiB 以内的文本内容", EnUS: "Only text content of at most 1 MiB is supported."}},
	{Key: "workspace.save.idempotency_conflict", Texts: map[Locale]string{ZhCN: "该操作标识已用于其他内容", EnUS: "This operation ID was already used for different content."}},
	{Key: "workspace.save.file_changed", Texts: map[Locale]string{ZhCN: "文件已被其他操作修改，请比较后保存", EnUS: "Another operation has changed the file. Compare the contents before saving."}},
	{Key: "workspace.binding.server_root_missing", Texts: map[Locale]string{ZhCN: "服务端尚未配置 SUMA_WORKSPACE_ROOT", EnUS: "SUMA_WORKSPACE_ROOT is not configured on the server."}},
	{Key: "workspace.binding.outside_allowed_root", Texts: map[Locale]string{ZhCN: "工作区必须位于服务端允许根目录内", EnUS: "The workspace must be inside the root directory allowed by the server."}},
	{Key: "workspace.binding.directory_required", Texts: map[Locale]string{ZhCN: "工作区必须是目录", EnUS: "The workspace must be a directory."}},
	{Key: "workspace.method_invalid", Texts: map[Locale]string{ZhCN: "不支持此请求方法", EnUS: "This request method is not supported."}},
	{Key: "search.query_invalid", Texts: map[Locale]string{ZhCN: "请输入 1–500 字节的查询", EnUS: "Enter a query of 1 to 500 bytes."}},
	{Key: "search.query_length", Texts: map[Locale]string{ZhCN: "查询长度超出限制", EnUS: "The query length exceeds the limit."}},
	{Key: "search.keyword.coverage", Texts: map[Locale]string{ZhCN: "当前授权项目的全部需求、任务、问题和文档；返回最多 50 条", EnUS: "All requirements, tasks, issues, and documents in the currently authorized project; at most 50 results are returned."}},
}
