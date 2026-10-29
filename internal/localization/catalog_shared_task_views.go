package localization

var sharedTaskViewsCatalog = []Definition{
	{Key: "shared_task_views.invalid", SourceTemplate: "Task view request is invalid", Texts: map[Locale]string{ZhCN: "任务视图内容或读取范围无效，请检查后重试", EnUS: "The task view content or read scope is invalid. Check it and retry."}},
	{Key: "shared_task_views.conflict", SourceTemplate: "Task view changed; reload the current view", Texts: map[Locale]string{ZhCN: "任务视图已被修改，请读取当前版本并比较后再保存", EnUS: "The task view changed. Read and compare the current version before saving."}},
	{Key: "shared_task_views.updated", SourceTemplate: "Task view updated", Texts: map[Locale]string{ZhCN: "任务视图已更新", EnUS: "Task view updated"}},
}
