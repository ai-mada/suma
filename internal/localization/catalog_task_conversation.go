package localization

// These messages belong only to the typed task-conversation producer. Message
// bodies and captured author names remain user content, never translation keys.
var taskConversationCatalog = []Definition{
	{Key: "task_conversation.invalid", SourceTemplate: "Invalid task conversation request", Texts: map[Locale]string{ZhCN: "任务讨论请求无效", EnUS: "The task discussion request is invalid."}},
	{Key: "task_conversation.task_changed", SourceTemplate: "The task has changed; reload its context before posting", Texts: map[Locale]string{ZhCN: "任务已变化，请先刷新任务依据再发送；当前草稿会保留", EnUS: "The task has changed. Refresh its context before posting; your draft is preserved."}},
	{Key: "task_conversation.sequence_limit", SourceTemplate: "The task conversation sequence limit has been reached", Texts: map[Locale]string{ZhCN: "任务讨论已达到序号上限，请联系管理员", EnUS: "The task discussion sequence limit has been reached. Contact an administrator."}},
}
