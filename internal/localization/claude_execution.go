package localization

var claudeExecutionCatalog = []Definition{
	{Key: "claude.execution.invalid", SourceTemplate: "Invalid Claude CLI execution configuration", Texts: map[Locale]string{ZhCN: "Claude CLI 执行配置或回执无效", EnUS: "The Claude CLI execution configuration or receipt is invalid."}},
	{Key: "claude.execution.changed", SourceTemplate: "Claude CLI configuration changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "Claude CLI 固定配置已变化，请重新预览派发", EnUS: "The fixed Claude CLI configuration changed. Prepare a new dispatch preview."}},
	{Key: "claude.execution.unavailable", SourceTemplate: "The fixed Claude CLI configuration is not in the current authorized worker inventory", Texts: map[Locale]string{ZhCN: "当前已授权执行节点的工具上报中没有匹配的固定 Claude CLI 配置", EnUS: "The current authorized worker inventory has no matching fixed Claude CLI configuration."}},
	{Key: "claude.execution.enforced_budget_unavailable", SourceTemplate: "Claude CLI supports observed usage only; enforced budgets are unavailable", Texts: map[Locale]string{ZhCN: "Claude CLI 仅支持观测用量，无法执行强制预算限制", EnUS: "Claude CLI supports observed usage only. Enforced budgets are unavailable."}},
}
