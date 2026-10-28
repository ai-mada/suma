package localization

// Exact API diagnostics only; task names, review reasons and issue text retain
// their original content and never become interpolation templates.
var taskDeliveryCatalog = []Definition{
	{Key: "task_delivery.query", SourceTemplate: "Invalid task delivery query", Texts: map[Locale]string{ZhCN: "任务交付查询无效，请提供当前项目", EnUS: "The task delivery query is invalid. Provide the current project."}},
	{Key: "task_delivery.reserved_operation", SourceTemplate: "Operation key prefix is reserved for internal workflow commands", Texts: map[Locale]string{ZhCN: "该操作标识前缀由内部流程保留，请使用独立操作标识", EnUS: "This operation key prefix is reserved for internal workflow commands. Use a separate operation key."}},
}
