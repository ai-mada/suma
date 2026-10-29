package localization

var resourceCalendarCatalog = []Definition{
	{Key: "resource_calendar.request", SourceTemplate: "Invalid resource calendar request", Texts: map[Locale]string{ZhCN: "资源工作日历请求无效，请核对项目、资源及完整日历字段", EnUS: "The resource calendar request is invalid. Check the project, resource and complete schedule."}},
	{Key: "resource_calendar.schedule", SourceTemplate: "Resource calendar schedule is invalid", Texts: map[Locale]string{ZhCN: "工作日历无效，请核对每日工时、日期例外及重复日期", EnUS: "The work calendar is invalid. Check daily hours, exceptions and duplicate dates."}},
	{Key: "resource_calendar.conflict", SourceTemplate: "Resource calendar version changed", Texts: map[Locale]string{ZhCN: "工作日历已变化，请读取当前版本后核对并保存", EnUS: "The work calendar changed. Read the current version, review and save again."}},
}
