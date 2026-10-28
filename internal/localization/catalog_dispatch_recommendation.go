package localization

// Exact domain messages only; model/user content remains immutable original text.
var dispatchRecommendationCatalog = []Definition{
	{Key: "dispatch_recommendation.invalid", SourceTemplate: "Invalid dispatch recommendation request", Texts: map[Locale]string{ZhCN: "派发推荐请求无效", EnUS: "Invalid dispatch recommendation request"}},
	{Key: "dispatch_recommendation.profile_message", SourceTemplate: "Select a current enabled read-only Suma dispatch recommendation profile", Texts: map[Locale]string{ZhCN: "请选择当前启用的只读速码派发推荐配置", EnUS: "Select a current enabled read-only Suma dispatch recommendation profile"}},
	{Key: "dispatch_recommendation.source_changed", SourceTemplate: "The dispatch recommendation source changed; review current candidates again", Texts: map[Locale]string{ZhCN: "推荐依据已变化，请重新核对当前候选项", EnUS: "The dispatch recommendation source changed; review current candidates again"}},
	{Key: "dispatch_recommendation.empty", SourceTemplate: "No eligible execution candidate satisfies the current constraints", Texts: map[Locale]string{ZhCN: "没有执行候选项满足当前约束", EnUS: "No eligible execution candidate satisfies the current constraints"}},
	{Key: "dispatch_recommendation.consent", SourceTemplate: "Confirm that the current task and candidate context will be sent to the selected model", Texts: map[Locale]string{ZhCN: "请确认当前任务及候选上下文将发送给所选模型", EnUS: "Confirm that the current task and candidate context will be sent to the selected model"}},
	{Key: "dispatch_recommendation.output", SourceTemplate: "The model recommendation does not satisfy the fixed candidate contract", Texts: map[Locale]string{ZhCN: "模型推荐不符合固定候选合同", EnUS: "The model recommendation does not satisfy the fixed candidate contract"}},
	{Key: "dispatch_recommendation.version", SourceTemplate: "The dispatch recommendation changed; reload before deciding", Texts: map[Locale]string{ZhCN: "派发推荐已变化，请重新加载后决定", EnUS: "The dispatch recommendation changed; reload before deciding"}},
	{Key: "dispatch_recommendation.unknown", SourceTemplate: "The dispatch recommendation outcome is unknown; inspect the original run", Texts: map[Locale]string{ZhCN: "推荐结果未知，请检查原始运行", EnUS: "The dispatch recommendation outcome is unknown; inspect the original run"}},
	{Key: "dispatch_recommendation.failed", SourceTemplate: "The dispatch recommendation did not complete successfully", Texts: map[Locale]string{ZhCN: "派发推荐未成功完成", EnUS: "The dispatch recommendation did not complete successfully"}},
	{Key: "dispatch_recommendation.authority", SourceTemplate: "The dispatch recommendation creator no longer has current execution authority", Texts: map[Locale]string{ZhCN: "推荐创建者已不具备当前执行权限", EnUS: "The dispatch recommendation creator no longer has current execution authority"}},
	{Key: "dispatch_recommendation.prompt", SourceTemplate: "The recommendation context exceeds 64000 UTF-8 bytes; select a smaller context", Texts: map[Locale]string{ZhCN: "推荐上下文超过 64000 个 UTF-8 字节，请缩小范围", EnUS: "The recommendation context exceeds 64000 UTF-8 bytes; select a smaller context"}},
}
