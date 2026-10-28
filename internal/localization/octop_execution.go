package localization

// Stable product messages. Raw third-party errors remain separate evidence.
var octopExecutionCatalog = []Definition{
	{Key: "octop.execution.invalid", SourceTemplate: "Invalid Octop ACP execution configuration", Texts: map[Locale]string{ZhCN: "Octop ACP 执行配置或回执无效", EnUS: "The Octop ACP execution configuration or receipt is invalid."}},
	{Key: "octop.execution.changed", SourceTemplate: "Octop ACP configuration changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "Octop ACP 配置已变化，请重新预览派发计划", EnUS: "The Octop ACP configuration changed. Prepare a new dispatch preview."}},
	{Key: "octop.execution.unavailable", SourceTemplate: "The fixed Octop ACP configuration is not in the current authorized worker inventory", Texts: map[Locale]string{ZhCN: "当前已授权执行节点清单中没有该固定 Octop ACP 配置", EnUS: "The fixed Octop ACP configuration is not in the current authorized runner inventory."}},
	{Key: "octop.execution.enforced_budget_unavailable", SourceTemplate: "Octop ACP supports observed usage only; enforced budgets are unavailable", Texts: map[Locale]string{ZhCN: "Octop ACP 仅支持观测用量，无法执行强制预算限制", EnUS: "Octop ACP supports observed usage only. Enforced budgets are unavailable."}},
}
