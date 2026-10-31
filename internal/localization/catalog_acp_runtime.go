package localization

var acpRuntimeCatalog = []Definition{
	{Key: "acp_execution.invalid", SourceTemplate: "Invalid fixed ACP execution configuration", Texts: map[Locale]string{ZhCN: "ACP固定执行配置无效，请核对原工具与模型配置", EnUS: "The fixed ACP execution configuration is invalid. Check the original tool and model configuration."}},
	{Key: "acp_execution.changed", SourceTemplate: "ACP configuration changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "ACP配置已变化，请重新预览后派发", EnUS: "ACP configuration changed. Prepare a new dispatch preview."}},
	{Key: "acp_execution.unavailable", SourceTemplate: "The fixed ACP configuration is not in the current authorized worker inventory", Texts: map[Locale]string{ZhCN: "当前授权执行节点尚未核实此ACP固定配置", EnUS: "The current authorized runner inventory has not verified this fixed ACP configuration."}},
	{Key: "acp_execution.budget", SourceTemplate: "ACP supports observed usage only; enforced budgets are unavailable", Texts: map[Locale]string{ZhCN: "ACP目前仅观察用量，无法执行平台强制预算，请选择可控模型网关工具", EnUS: "ACP currently supports usage observation only. Select a controlled model gateway tool for enforced budgets."}},
	{Key: "tool_permission.invalid", SourceTemplate: "Invalid ACP tool permission request", Texts: map[Locale]string{ZhCN: "本次ACP权限请求无效，请核对原执行与准确选项", EnUS: "This ACP permission request is invalid. Check the original execution and exact option."}},
	{Key: "tool_permission.changed", SourceTemplate: "The ACP tool permission source changed; reload before deciding", Texts: map[Locale]string{ZhCN: "原ACP权限依据已变化，请重新读取后明确决定", EnUS: "The original ACP permission basis changed. Reload before deciding."}},
	{Key: "tool_permission.limited", SourceTemplate: "ACP tool permission attempt limit reached", Texts: map[Locale]string{ZhCN: "本次执行已达到权限请求上限，请核对原任务与执行记录", EnUS: "This execution reached its permission request limit. Check the original task and execution history."}},
}
