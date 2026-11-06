package localization

// Only these exact domain-owned sources are aliases. User issue descriptions,
// review reasons and original test failures stay literal project content.
var issueRepairCatalog = []Definition{
	{Key: "issue_repair.invalid", SourceTemplate: "Invalid issue repair request", Texts: map[Locale]string{ZhCN: "问题修复请求无效，请检查必填项和版本", EnUS: "Invalid issue repair request"}},
	{Key: "issue_repair.version", SourceTemplate: "The issue or repair cycle has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "问题或修复周期已变化，请读取最新版本后再决定", EnUS: "The issue or repair cycle has changed; reload before deciding"}},
	{Key: "issue_repair.state", SourceTemplate: "The current issue repair cycle does not allow this action", Texts: map[Locale]string{ZhCN: "当前问题修复周期不允许此操作", EnUS: "The current issue repair cycle does not allow this action"}},
	{Key: "issue_repair.protected", SourceTemplate: "Issue lifecycle and repair evidence require dedicated repair commands", Texts: map[Locale]string{ZhCN: "请通过问题修复面板更新生命周期，修复和关闭证据由专用操作维护", EnUS: "Issue lifecycle and repair evidence require dedicated repair commands"}},
	{Key: "issue_repair.regression", SourceTemplate: "Bind current published regression cases, including the original failed case and selector", Texts: map[Locale]string{ZhCN: "请绑定当前已发布的回归用例，包含原失败用例及相同测试标识", EnUS: "Bind current published regression cases, including the original failed case and selector"}},
	{Key: "issue_repair.gate", SourceTemplate: "Current accepted repair and complete passing regression evidence are required", Texts: map[Locale]string{ZhCN: "关闭问题需要当前已正式验收的修复，以及完整通过的回归证据", EnUS: "Current accepted repair and complete passing regression evidence are required"}},
	{Key: "issue_repair.source", SourceTemplate: "The protected original defect source is unavailable or inconsistent", Texts: map[Locale]string{ZhCN: "原缺陷的受保护来源不可用或不一致，请核对正式测试记录", EnUS: "The protected original defect source is unavailable or inconsistent"}},
	{Key: "issue_repair.release_stale", SourceTemplate: "The closed repair or its release evidence has changed", Texts: map[Locale]string{ZhCN: "已关闭的修复或发布证据已变化，请重新核对本轮修复", EnUS: "The closed repair or its release evidence has changed"}},
}
