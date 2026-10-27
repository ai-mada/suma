package localization

// Exact host diagnostics only; reported usage, model IDs and operator evidence stay raw.
var usageAccountingCatalog = []Definition{
	{Key: "usage_accounting.request", SourceTemplate: "Invalid usage accounting request", Texts: map[Locale]string{ZhCN: "用量或额度请求无效", EnUS: "Invalid usage accounting request"}},
	{Key: "usage_accounting.version", SourceTemplate: "Usage accounting has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "用量或额度配置已变化，请刷新后再决定", EnUS: "Usage accounting has changed; reload before deciding"}},
	{Key: "usage_accounting.profile", SourceTemplate: "Pricing requires a current enabled fixed Suma model configuration", Texts: map[Locale]string{ZhCN: "计价需要当前已启用且模型依据固定的速码工具", EnUS: "Pricing requires a current enabled fixed Suma model configuration"}},
	{Key: "usage_accounting.confirmation", SourceTemplate: "Confirm the usage and provide its evidence source before settling", Texts: map[Locale]string{ZhCN: "请确认用量并填写证据来源后再结算", EnUS: "Confirm the usage and provide its evidence source before settling"}},
	{Key: "usage_accounting.reduce", SourceTemplate: "Confirmed usage cannot reduce an already reported counter", Texts: map[Locale]string{ZhCN: "确认用量不能减少已上报的计数", EnUS: "Confirmed usage cannot reduce an already reported counter"}},
	{Key: "usage_accounting.enforce", SourceTemplate: "Confirm that enforcement can prevent new tool attempts", Texts: map[Locale]string{ZhCN: "请确认启用额度限制可能阻止新的工具执行", EnUS: "Confirm that enforcement can prevent new tool attempts"}},
	{Key: "usage_accounting.unavailable", SourceTemplate: "Model token or cost allowance is unavailable", Texts: map[Locale]string{ZhCN: "模型Token或费用额度当前不可用", EnUS: "Model token or cost allowance is unavailable"}},
	{Key: "usage_accounting.output", SourceTemplate: "The output token reservation must cover this attempt's output budget", Texts: map[Locale]string{ZhCN: "输出Token预留必须覆盖本次执行的输出预算", EnUS: "The output token reservation must cover this attempt's output budget"}},
	{Key: "usage_accounting.query", SourceTemplate: "Invalid usage query", Texts: map[Locale]string{ZhCN: "用量查询或分页参数无效", EnUS: "Invalid usage query"}},
	{Key: "usage_accounting.fields", SourceTemplate: "Incomplete usage request", Texts: map[Locale]string{ZhCN: "用量请求缺少完整且有效的字段", EnUS: "Incomplete usage request"}},
}
