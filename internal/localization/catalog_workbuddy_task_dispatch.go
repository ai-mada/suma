package localization

// These exact control-plane producers are reviewed aliases. Task content,
// prompts, provider replies and receipt identity remain literal data.
var workBuddyTaskDispatchCatalog = []Definition{
	{Key: "workbuddy_task_dispatch.invalid", SourceTemplate: "Invalid WorkBuddy task dispatch request", Texts: map[Locale]string{ZhCN: "WorkBuddy任务派发请求无效，请核对任务、账号及版本", EnUS: "The WorkBuddy task dispatch request is invalid. Check the task, account and versions."}},
	{Key: "workbuddy_task_dispatch.changed", SourceTemplate: "The WorkBuddy task dispatch basis changed; preview again", Texts: map[Locale]string{ZhCN: "任务或账号等派发依据已变化，请重新预览；未确认的原操作仍须核对", EnUS: "The task, account or dispatch basis changed. Preview again; reconcile any unconfirmed original operation."}},
	{Key: "workbuddy_task_dispatch.baseline", SourceTemplate: "WorkBuddy task dispatch does not support the bound code baseline", Texts: map[Locale]string{ZhCN: "此任务已绑定代码基线，当前云执行尚不能核验该基线；请选择支持该基线的开发工具", EnUS: "This task has a bound code baseline that cloud execution cannot yet verify. Choose a development tool that supports this baseline."}},
}
