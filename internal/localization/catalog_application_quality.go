package localization

// Only controlled read-tool descriptions and cursor diagnostics are localized.
// Case titles, recorded outcomes and original reports remain source facts.
var applicationQualityCatalog = []Definition{
	{Key: "application_tool_project_task_delivery", Texts: map[Locale]string{ZhCN: "读取当前任务的交付、测试与人工验收事实，不派发或修改状态", EnUS: "Read current task delivery, test and human acceptance facts without dispatching or changing state"}},
	{Key: "application_tool_project_quality_execution", Texts: map[Locale]string{ZhCN: "分页读取固定任务运行的逐例质量与紧凑证据引用，不读取报告原件或批准验收", EnUS: "Read a fixed task run's case results and compact evidence by page, without report originals or acceptance approval"}},
	{Key: "application_quality.invalid_case_cursor", SourceTemplate: "The case cursor does not belong to the selected run's frozen test plan", Texts: map[Locale]string{ZhCN: "用例游标不属于所选运行固定的测试计划，请重新读取对应页面", EnUS: "The case cursor does not belong to the selected run's frozen test plan. Read the matching page again."}},
}
