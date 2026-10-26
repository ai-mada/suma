package localization

// Only closed domain diagnostics are translated. Git paths, complete diffs,
// immutable observations, actor reasons and source content remain literal.
var repositoryIntegrationCatalog = []Definition{
	{Key: "repository_integration.invalid", SourceTemplate: "Invalid repository integration request", Texts: map[Locale]string{ZhCN: "代码集成请求无效", EnUS: "Invalid repository integration request"}},
	{Key: "repository_integration.changed", SourceTemplate: "Repository integration version changed; refresh the original plan", Texts: map[Locale]string{ZhCN: "代码集成版本已变化，请刷新原计划", EnUS: "Repository integration version changed; refresh the original plan"}},
	{Key: "repository_integration.basis", SourceTemplate: "Accepted task, review, tests or sealed candidate basis changed", Texts: map[Locale]string{ZhCN: "已验收任务、评审、测试或密封候选代码依据已变化", EnUS: "Accepted task, review, tests or sealed candidate basis changed"}},
	{Key: "repository_integration.target", SourceTemplate: "Repository target binding or observed Git state changed", Texts: map[Locale]string{ZhCN: "目标仓库绑定或实际 Git 状态已变化", EnUS: "Repository target binding or observed Git state changed"}},
	{Key: "repository_integration.busy", SourceTemplate: "Repository target has an unresolved operation; reconcile the original operation", Texts: map[Locale]string{ZhCN: "目标仓库存在未核对操作，请核对原操作", EnUS: "Repository target has an unresolved operation; reconcile the original operation"}},
	{Key: "repository_integration.approval", SourceTemplate: "Current browser approval of the exact prepared diff is required", Texts: map[Locale]string{ZhCN: "需要浏览器中对本次完整差异的有效审批", EnUS: "Current browser approval of the exact prepared diff is required"}},
	{Key: "repository_integration.ack", SourceTemplate: "Explicit repository integration acknowledgement is required", Texts: map[Locale]string{ZhCN: "请明确确认本次代码集成操作", EnUS: "Explicit repository integration acknowledgement is required"}},
	{Key: "repository_integration.provider", SourceTemplate: "Repository integration provider is unavailable or not configured", Texts: map[Locale]string{ZhCN: "代码集成服务不可用或未配置", EnUS: "Repository integration provider is unavailable or not configured"}},
	{Key: "repository_integration.unknown", SourceTemplate: "Repository effect is unknown; inspect and reconcile the original operation", Texts: map[Locale]string{ZhCN: "代码集成结果未知，请检查并核对原操作", EnUS: "Repository effect is unknown; inspect and reconcile the original operation"}},
	{Key: "repository_integration.observation", SourceTemplate: "Repository provider observation does not match the fixed operation", Texts: map[Locale]string{ZhCN: "集成服务回执与固定操作不一致", EnUS: "Repository provider observation does not match the fixed operation"}},
	{Key: "repository_integration.expired", SourceTemplate: "Repository integration plan or authorization expired", Texts: map[Locale]string{ZhCN: "代码集成计划或授权已过期", EnUS: "Repository integration plan or authorization expired"}},
}
