package localization

var workspaceFileContextCatalog = []Definition{
	{Key: "workspace.save.invalid_request", SourceTemplate: "Workspace file request is invalid", Texts: map[Locale]string{ZhCN: "文件保存请求无效，请保留原文件读取结果并核对保存字段", EnUS: "The file save request is invalid. Keep the original file snapshot and review the save fields."}},
	{Key: "workspace.save.context_required", SourceTemplate: "expected_context_sha256 must be a 64-character lowercase SHA-256 digest", Texts: map[Locale]string{ZhCN: "请提供原文件读取结果中的 64 位小写工作区上下文摘要 expected_context_sha256", EnUS: "Provide expected_context_sha256 as the original file's 64-character lowercase workspace context digest."}},
	{Key: "workspace.save.context_changed", SourceTemplate: "Workspace context changed; reload the file before saving", Texts: map[Locale]string{ZhCN: "工作区上下文已变化，请重新加载文件后保存", EnUS: "The workspace context has changed. Reload the file before saving."}},
}
