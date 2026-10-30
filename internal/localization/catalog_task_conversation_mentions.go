package localization

var taskConversationMentionCatalog = []Definition{
	{Key: "task_conversation_mentions.invalid", SourceTemplate: "Invalid task conversation mention request or cursor", Texts: map[Locale]string{ZhCN: "讨论提及请求或分页依据无效，请核对当前任务和接收人", EnUS: "The mention request or cursor is invalid. Check the current task and recipients."}},
	{Key: "task_conversation_mentions.unavailable", SourceTemplate: "One or more mention recipients are no longer available; reload the current people directory", Texts: map[Locale]string{ZhCN: "部分提及人员当前不可用，请重新读取人员目录后再发送", EnUS: "Some mention recipients are unavailable. Reload the current people directory before sending."}},
	{Key: "task_conversation_mentions.source_invalid", SourceTemplate: "The task conversation mention source is invalid", Texts: map[Locale]string{ZhCN: "讨论提及来源数据无效，请检查原消息与提及关系", EnUS: "The mention source is invalid. Check the original message and mention relationship."}},
}
