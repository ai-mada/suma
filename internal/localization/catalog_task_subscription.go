package localization

var taskSubscriptionCatalog = []Definition{
	{Key: "task_subscription.invalid", SourceTemplate: "Invalid task subscription request", Texts: map[Locale]string{ZhCN: "任务关注请求无效，请核对任务、版本及原操作标识", EnUS: "The task subscription request is invalid. Check the task, version and original operation ID."}},
	{Key: "task_subscription.changed", SourceTemplate: "The task subscription changed; reload before deciding", Texts: map[Locale]string{ZhCN: "任务关注状态已变化，请重新读取后明确操作", EnUS: "The task subscription changed. Reload it before deciding."}},
	{Key: "task_subscription.source_invalid", SourceTemplate: "The task subscription notification source is invalid", Texts: map[Locale]string{ZhCN: "任务关注通知来源不一致，本次消息未提交", EnUS: "The task subscription notification source is invalid. This message was not committed."}},
}
