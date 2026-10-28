package localization

// Current product diagnostics; original vendor output stays separate evidence.
var openCodeExecutionCatalog = []Definition{
	{Key: "opencode.execution.invalid", SourceTemplate: "Invalid OpenCode SDK execution configuration", Texts: map[Locale]string{ZhCN: "OpenCode SDK 执行配置或回执无效", EnUS: "The OpenCode SDK execution configuration or receipt is invalid."}},
	{Key: "opencode.execution.changed", SourceTemplate: "OpenCode SDK configuration changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "OpenCode SDK 配置已变化，请重新预览派发计划", EnUS: "The OpenCode SDK configuration changed. Prepare a new dispatch preview."}},
	{Key: "opencode.execution.unavailable", SourceTemplate: "The fixed OpenCode SDK configuration is not in the current authorized worker inventory", Texts: map[Locale]string{ZhCN: "当前已授权执行节点清单中没有该固定 OpenCode SDK 配置", EnUS: "The fixed OpenCode SDK configuration is not in the current authorized runner inventory."}},
	{Key: "opencode.execution.enforced_budget_unavailable", SourceTemplate: "OpenCode SDK supports observed usage only; enforced budgets are unavailable", Texts: map[Locale]string{ZhCN: "OpenCode SDK 仅支持观测用量，无法执行强制预算限制", EnUS: "OpenCode SDK supports observed usage only. Enforced budgets are unavailable."}},
}
