package localization

// Closed host-owned messages; never translate model or user content.
var intelligentPlanningCatalog = []Definition{
	{Key: "intelligent_planning.request", SourceTemplate: "Invalid intelligent planning request", Texts: map[Locale]string{ZhCN: "规划请求无效", EnUS: "Invalid intelligent planning request"}},
	{Key: "intelligent_planning.output", SourceTemplate: "The model planning output does not satisfy the fixed proposal contract", Texts: map[Locale]string{ZhCN: "模型输出不符合已固定的规划方案格式", EnUS: "The model planning output does not satisfy the fixed proposal contract"}},
	{Key: "intelligent_planning.source", SourceTemplate: "Planning source has changed; create a new analysis", Texts: map[Locale]string{ZhCN: "规划依据已变化，请重新创建分析", EnUS: "Planning source has changed; create a new analysis"}},
	{Key: "intelligent_planning.profile", SourceTemplate: "Select a current enabled read-only Suma planning profile", Texts: map[Locale]string{ZhCN: "请选择当前已启用的速码只读规划工具", EnUS: "Select a current enabled read-only Suma planning profile"}},
	{Key: "intelligent_planning.prompt_limit", SourceTemplate: "Planning context exceeds 32000 UTF-8 bytes; select a smaller task scope", Texts: map[Locale]string{ZhCN: "规划上下文超过32000字节，请减少所选任务范围", EnUS: "Planning context exceeds 32000 UTF-8 bytes; select a smaller task scope"}},
	{Key: "intelligent_planning.version", SourceTemplate: "The planning analysis has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "规划分析已变化，请刷新后再决定", EnUS: "The planning analysis has changed; reload before deciding"}},
	{Key: "intelligent_planning.model_confirmation", SourceTemplate: "Confirm that the selected task content will be sent to the configured model", Texts: map[Locale]string{ZhCN: "请确认将所选任务内容发送至已配置的模型", EnUS: "Confirm that the selected task content will be sent to the configured model"}},
	{Key: "intelligent_planning.apply_confirmation", SourceTemplate: "Confirm the proposed schedule changes before applying them", Texts: map[Locale]string{ZhCN: "请确认方案中的排期变更后再采纳", EnUS: "Confirm the proposed schedule changes before applying them"}},
	{Key: "intelligent_planning.execution_failed", SourceTemplate: "The planning execution did not complete successfully", Texts: map[Locale]string{ZhCN: "规划执行未成功完成", EnUS: "The planning execution did not complete successfully"}},
	{Key: "intelligent_planning.execution_unknown", SourceTemplate: "The planning execution outcome is unknown; inspect the original run", Texts: map[Locale]string{ZhCN: "规划执行结果未知，请核对原运行", EnUS: "The planning execution outcome is unknown; inspect the original run"}},
	{Key: "intelligent_planning.creator_authority", SourceTemplate: "The planning creator no longer has current execution authority", Texts: map[Locale]string{ZhCN: "规划发起人已不具备当前执行权限", EnUS: "The planning creator no longer has current execution authority"}},
	{Key: "intelligent_planning.alert", SourceTemplate: "Invalid planning alert decision or version", Texts: map[Locale]string{ZhCN: "规划预警决策或版本无效", EnUS: "Invalid planning alert decision or version"}},
	{Key: "intelligent_planning.query", SourceTemplate: "Invalid intelligent planning query", Texts: map[Locale]string{ZhCN: "规划查询或分页参数无效", EnUS: "Invalid intelligent planning query"}},
	{Key: "intelligent_planning.fields", SourceTemplate: "Incomplete intelligent planning request", Texts: map[Locale]string{ZhCN: "规划请求缺少完整且有效的字段", EnUS: "Incomplete intelligent planning request"}},
}
