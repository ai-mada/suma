package localization

// Only messages emitted by the manual worklog domain use these exact aliases.
// User notes/reasons and immutable snapshots are never machine-translated.
var worklogCatalog = []Definition{
	{Key: "worklog.request", SourceTemplate: "Invalid manual worklog request", Texts: map[Locale]string{ZhCN: "工时登记请求无效，请核对日期范围、任务版本、人员和分钟数", EnUS: "The manual worklog request is invalid. Check the date range, task version, person and minutes."}},
	{Key: "worklog.scope", SourceTemplate: "Worklog scope changed; reload the first page", Texts: map[Locale]string{ZhCN: "工时记录已变化，请重新加载第一页", EnUS: "The worklog register changed. Reload the first page."}},
	{Key: "worklog.task", SourceTemplate: "Worklog task version changed", Texts: map[Locale]string{ZhCN: "任务版本或状态已变化，请核对当前任务后重新登记", EnUS: "The task version or state changed. Review the current task before registering time."}},
	{Key: "worklog.version_status", SourceTemplate: "Worklog version or status changed", Texts: map[Locale]string{ZhCN: "工时记录版本或状态已变化，请保留草稿并核对当前记录", EnUS: "The worklog version or state changed. Keep your draft and review the current entry."}},
	{Key: "worklog.daily_limit", SourceTemplate: "Worklog daily total would exceed its limit", Texts: map[Locale]string{ZhCN: "本次登记会超过该人员在组织内同一天的工时上限，请核对其他登记", EnUS: "This entry would exceed the person's organization-wide daily worklog limit. Review other entries."}},
	{Key: "worklog.version", SourceTemplate: "Worklog version changed", Texts: map[Locale]string{ZhCN: "工时记录已被修改，请保留草稿并核对当前版本", EnUS: "The worklog entry was modified. Keep your draft and review its current version."}},
}
