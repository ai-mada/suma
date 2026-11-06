package localization

// Exact API diagnostics only; task names, review reasons and issue text retain
// their original content and never become interpolation templates.
var taskDeliveryCatalog = []Definition{
	{Key: "task_delivery.query", SourceTemplate: "Invalid task delivery query", Texts: map[Locale]string{ZhCN: "任务交付查询无效，请提供当前项目", EnUS: "The task delivery query is invalid. Provide the current project."}},
	{Key: "task_delivery.reserved_operation", SourceTemplate: "Operation key prefix is reserved for internal workflow commands", Texts: map[Locale]string{ZhCN: "该操作标识前缀由内部流程保留，请使用独立操作标识", EnUS: "This operation key prefix is reserved for internal workflow commands. Use a separate operation key."}},
	{Key: "task_delivery.ascii_operation", SourceTemplate: "Operation key must use visible ASCII characters on OceanBase", Texts: map[Locale]string{ZhCN: "OceanBase 下的操作标识须使用可见 ASCII 字符（不含空格）", EnUS: "On OceanBase, use visible ASCII characters without spaces for the operation key."}},
}
