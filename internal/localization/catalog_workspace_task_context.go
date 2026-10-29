package localization

// Only the reviewed context producer owns this source. User task text and
// captured command output remain content rather than translated diagnostics.
var workspaceTaskContextCatalog = []Definition{
	{Key: "workspace.task_context.invalid", SourceTemplate: "Invalid workspace task context request", Texts: map[Locale]string{ZhCN: "工作台任务上下文请求无效", EnUS: "The workspace task context request is invalid."}},
}
