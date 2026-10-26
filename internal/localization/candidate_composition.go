package localization

// Only controlled platform diagnostics are translated. Source code, Git output,
// conflict paths, original evidence and human review text remain literal.
var candidateCompositionCatalog = []Definition{
	{Key: "candidate_composition.invalid", SourceTemplate: "Invalid candidate composition request", Texts: map[Locale]string{ZhCN: "代码合成请求无效", EnUS: "Invalid candidate composition request"}},
	{Key: "candidate_composition.changed", SourceTemplate: "Candidate composition changed; refresh the fixed preview", Texts: map[Locale]string{ZhCN: "代码合成依据已变化，请刷新固定预览", EnUS: "Candidate composition changed; refresh the fixed preview"}},
	{Key: "candidate_composition.source", SourceTemplate: "Candidate composition source acceptance or candidate changed", Texts: map[Locale]string{ZhCN: "来源任务的验收或候选代码已变化", EnUS: "Candidate composition source acceptance or candidate changed"}},
	{Key: "candidate_composition.template", SourceTemplate: "Candidate composition requires one producer and independent candidate tests", Texts: map[Locale]string{ZhCN: "代码合成需要一个产出节点和独立候选测试", EnUS: "Candidate composition requires one producer and independent candidate tests"}},
	{Key: "candidate_composition.base", SourceTemplate: "Candidate composition sources do not share the administrator base", Texts: map[Locale]string{ZhCN: "来源候选不具有共同的管理员代码锚点", EnUS: "Candidate composition sources do not share the administrator base"}},
	{Key: "candidate_composition.expired", SourceTemplate: "Candidate composition preview expired", Texts: map[Locale]string{ZhCN: "代码合成预览已过期", EnUS: "Candidate composition preview expired"}},
	{Key: "candidate_composition.ack", SourceTemplate: "Explicit candidate composition acknowledgement required", Texts: map[Locale]string{ZhCN: "请明确确认本次代码合成操作", EnUS: "Explicit candidate composition acknowledgement required"}},
	{Key: "candidate_composition.authority", SourceTemplate: "Candidate composition initiating credential is no longer authorized", Texts: map[Locale]string{ZhCN: "代码合成发起者当前已无执行权限", EnUS: "Candidate composition initiating credential is no longer authorized"}},
	{Key: "candidate_composition.baseline", SourceTemplate: "Code baseline changed or is no longer accepted", Texts: map[Locale]string{ZhCN: "代码基线已变化或验收依据已失效", EnUS: "Code baseline changed or is no longer accepted"}},
	{Key: "candidate_composition.started", SourceTemplate: "Code baseline requires an unstarted task", Texts: map[Locale]string{ZhCN: "代码基线只能绑定到尚未开始的任务", EnUS: "Code baseline requires an unstarted task"}},
	{Key: "candidate_composition.unknown", SourceTemplate: "Candidate composition requires recovery of the original run", Texts: map[Locale]string{ZhCN: "请恢复原代码合成运行并核对结果", EnUS: "Candidate composition requires recovery of the original run"}},
	{Key: "candidate_composition.conflict", SourceTemplate: "Candidate composition requires a known exact conflict with complete Git evidence", Texts: map[Locale]string{ZhCN: "代码合成需要准确已知的冲突及完整Git证据", EnUS: "Candidate composition requires a known exact conflict with complete Git evidence"}},
	{Key: "candidate_composition.conflict_changed", Texts: map[Locale]string{ZhCN: "原冲突依据已变化或完整Git证据不足，请重新核对", EnUS: "Candidate composition requires a known exact conflict with complete Git evidence"}},
	{Key: "candidate_composition.repair_profile", SourceTemplate: "Conflict repair requires an explicitly enabled native workspace tool profile", Texts: map[Locale]string{ZhCN: "冲突修复需要管理员明确启用的原生工作区工具配置", EnUS: "Conflict repair requires an explicitly enabled native workspace tool profile"}},
	{Key: "candidate_composition.repair_task", SourceTemplate: "Conflict repair task must start through its fixed repair plan", Texts: map[Locale]string{ZhCN: "冲突修复任务必须从其固定修复计划启动", EnUS: "Conflict repair task must start through its fixed repair plan"}},
	{Key: "candidate_composition.repair_capability", SourceTemplate: "invalid conflict repair capability", Texts: map[Locale]string{ZhCN: "冲突修复能力声明无效", EnUS: "invalid conflict repair capability"}},
}
