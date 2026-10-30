package localization

var taskConversationKnowledgeCatalog = []Definition{
	{Key: "task_conversation_knowledge.invalid", SourceTemplate: "Invalid task conversation knowledge references", Texts: map[Locale]string{ZhCN: "讨论知识引用无效，请核对准确标识、修订和摘要", EnUS: "The conversation knowledge references are invalid. Check the exact IDs, revisions and digests."}},
	{Key: "task_conversation_knowledge.source", SourceTemplate: "The fixed task conversation knowledge source is invalid", Texts: map[Locale]string{ZhCN: "讨论的固定知识来源无效，无法确认原消息或受保护回复", EnUS: "The fixed knowledge source is invalid. The original message or protected reply cannot be confirmed."}},
	{Key: "task_conversation_knowledge.conflict", SourceTemplate: "Selected messages reference different revisions of the same knowledge", Texts: map[Locale]string{ZhCN: "所选消息引用了同一知识的不同修订，请明确调整消息范围", EnUS: "The selected messages reference different revisions of the same knowledge. Explicitly adjust the message window."}},
	{Key: "task_conversation_knowledge.limit", SourceTemplate: "The selected conversation knowledge exceeds 16 unique references", Texts: map[Locale]string{ZhCN: "所选讨论知识超过16项，请明确缩小引用范围", EnUS: "The selected conversation knowledge exceeds 16 unique references. Explicitly reduce the selection."}},
	{Key: "project_knowledge.revision_invalid", SourceTemplate: "Invalid project knowledge revision request", Texts: map[Locale]string{ZhCN: "项目知识修订请求无效，请核对当前项目、知识和准确修订", EnUS: "The project knowledge revision request is invalid. Check the current project, knowledge and exact revision."}},
	{Key: "project_knowledge.revision_source", SourceTemplate: "The immutable project knowledge revision is inconsistent", Texts: map[Locale]string{ZhCN: "不可变知识修订与原摘要不一致，不能使用最新版本替代", EnUS: "The immutable knowledge revision does not match its digest. The latest revision cannot replace it."}},
}
