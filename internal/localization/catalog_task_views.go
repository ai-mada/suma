package localization

var taskViewsCatalog = []Definition{
	{Key: "task_views.query_invalid", SourceTemplate: "Task workspace query is invalid", Texts: map[Locale]string{ZhCN: "任务查询条件无效，请核对筛选、排序和分页参数", EnUS: "The task query is invalid. Check filters, sorting and pagination."}},
	{Key: "task_views.cursor_invalid", SourceTemplate: "Task workspace cursor does not match the current query", Texts: map[Locale]string{ZhCN: "任务查询范围已变化，请保留筛选并重新读取第一页", EnUS: "The task query scope changed. Keep the filters and reload the first page."}},
}
