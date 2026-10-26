package localization

// Closed host vocabulary; neither model proposals nor source records are
// translated or treated as evidence of approval by this presentation catalog.
var knowledgeIntelligenceCatalog = []Definition{
	{Key: "knowledge_intelligence.invalid", SourceTemplate: "Invalid model-assisted knowledge request", Texts: map[Locale]string{ZhCN: "模型辅助知识提炼请求无效", EnUS: "Invalid model-assisted knowledge request"}},
	{Key: "knowledge_intelligence.output", SourceTemplate: "The model output does not satisfy the fixed knowledge proposal contract", Texts: map[Locale]string{ZhCN: "模型输出不符合固定的知识提案格式", EnUS: "The model output does not satisfy the fixed knowledge proposal contract"}},
	{Key: "knowledge_intelligence.source", SourceTemplate: "Knowledge sources have changed; create a new extraction", Texts: map[Locale]string{ZhCN: "知识来源已变化，请重新提炼", EnUS: "Knowledge sources have changed; create a new extraction"}},
	{Key: "knowledge_intelligence.profile", SourceTemplate: "Select a current enabled read-only Suma knowledge extraction profile", Texts: map[Locale]string{ZhCN: "请选择当前已启用的速码只读知识提炼工具", EnUS: "Select a current enabled read-only Suma knowledge extraction profile"}},
	{Key: "knowledge_intelligence.version", SourceTemplate: "The knowledge extraction has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "知识提炼已变化，请读取最新版本后再决定", EnUS: "The knowledge extraction has changed; reload before deciding"}},
	{Key: "knowledge_intelligence.consent", SourceTemplate: "Confirm that the selected source text will be sent to the configured model", Texts: map[Locale]string{ZhCN: "请确认将选中的来源正文交给配置的模型", EnUS: "Confirm that the selected source text will be sent to the configured model"}},
	{Key: "knowledge_intelligence.draft", SourceTemplate: "Confirm adoption as a model-assisted project draft requiring publication review", Texts: map[Locale]string{ZhCN: "请确认生成模型辅助的项目草稿，之后仍需审核发布", EnUS: "Confirm adoption as a model-assisted project draft requiring publication review"}},
	{Key: "knowledge_intelligence.failed", SourceTemplate: "The knowledge extraction execution did not complete successfully", Texts: map[Locale]string{ZhCN: "知识提炼执行未成功完成", EnUS: "The knowledge extraction execution did not complete successfully"}},
	{Key: "knowledge_intelligence.unknown", SourceTemplate: "The knowledge extraction outcome is unknown; inspect the original run", Texts: map[Locale]string{ZhCN: "知识提炼结果未知，请检查原运行", EnUS: "The knowledge extraction outcome is unknown; inspect the original run"}},
	{Key: "knowledge_intelligence.prompt", SourceTemplate: "Knowledge extraction context exceeds 32000 UTF-8 bytes; select smaller source excerpts", Texts: map[Locale]string{ZhCN: "提炼上下文过长，请缩小选中的来源片段", EnUS: "Knowledge extraction context exceeds 32000 UTF-8 bytes; select smaller source excerpts"}},
	{Key: "knowledge_intelligence.authority", SourceTemplate: "The extraction creator no longer has current execution authority", Texts: map[Locale]string{ZhCN: "提炼发起者当前已无执行权限", EnUS: "The extraction creator no longer has current execution authority"}},
}
