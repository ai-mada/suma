package localization

// Native diagnostics are the authoritative shared definitions for Go display
// validation and the generated SDK catalog. English is the complete original
// producer source from 7ff1b1453a4beaf8893855296b39683c907bf048; only named
// placeholders use the common named-dot-v1 grammar. SourceTemplate is private
// provenance and does not register an English source alias.
var nativeCatalog = []Definition{
	{Key: "native.workspace.config", SourceTemplate: "Workspace bridge configuration is invalid", Texts: map[Locale]string{ZhCN: "工作区连接配置无效", EnUS: "Workspace bridge configuration is invalid"}},
	{Key: "native.workspace.catalog", SourceTemplate: "Workspace tool identity or catalogue is invalid", Texts: map[Locale]string{ZhCN: "工作区工具身份或目录无效", EnUS: "Workspace tool identity or catalogue is invalid"}},
	{Key: "native.workspace.transport", SourceTemplate: "Workspace tool connection failed; inspect the current execution", Texts: map[Locale]string{ZhCN: "工作区工具连接失败，请核对当前执行", EnUS: "Workspace tool connection failed; inspect the current execution"}},
	{Key: "native.workspace.effects", SourceTemplate: "Workspace effects require reconciliation; do not restart the model", Texts: map[Locale]string{ZhCN: "工作区操作效果需要核对，请勿重新运行模型", EnUS: "Workspace effects require reconciliation; do not restart the model"}},
	{Key: "native.catalog.not_found", SourceTemplate: "CLI not found on PATH", Texts: map[Locale]string{ZhCN: "在 PATH 中未找到 CLI", EnUS: "CLI not found on PATH"}},
	{Key: "native.catalog.probe_exited", SourceTemplate: "CLI probe exited {{.status}}", Parameters: []Parameter{{Name: "status", Kind: Text}}, Texts: map[Locale]string{ZhCN: "CLI 探测退出，状态码 {{.status}}", EnUS: "CLI probe exited {{.status}}"}},
	{Key: "native.catalog.probe_failed", SourceTemplate: "CLI probe failed; inspect the original reason", Texts: map[Locale]string{ZhCN: "CLI 探测失败，请检查原始原因", EnUS: "CLI probe failed; inspect the original reason"}},
	{Key: "native.catalog.probe_output_limit", SourceTemplate: "CLI probe exceeded output limit", Texts: map[Locale]string{ZhCN: "CLI 探测输出超过上限", EnUS: "CLI probe exceeded output limit"}},
	{Key: "native.catalog.probe_timeout", SourceTemplate: "CLI probe timed out", Texts: map[Locale]string{ZhCN: "CLI 探测超时", EnUS: "CLI probe timed out"}},
	{Key: "native.catalog.requirements_mismatch", SourceTemplate: "Installed CLI help does not match adapter requirements", Texts: map[Locale]string{ZhCN: "已安装 CLI 的帮助信息不符合适配器要求", EnUS: "Installed CLI help does not match adapter requirements"}},
	{Key: "native.catalog.shell_wrapper", SourceTemplate: "Shell wrapper is not probed automatically", Texts: map[Locale]string{ZhCN: "不会自动探测 Shell 包装脚本", EnUS: "Shell wrapper is not probed automatically"}},
	{Key: "native.cli.busy", SourceTemplate: "An agent request is already running", Texts: map[Locale]string{ZhCN: "已有智能体请求正在运行", EnUS: "An agent request is already running"}},
	{Key: "native.cli.invalid_arguments", SourceTemplate: "Unsupported arguments; use --help.", Texts: map[Locale]string{ZhCN: "不支持的参数，请查看 --help。", EnUS: "Unsupported arguments; use --help."}},
	{Key: "native.cli.invalid_json", SourceTemplate: "Request must be a JSON object", Texts: map[Locale]string{ZhCN: "请求必须是有效的 JSON 对象", EnUS: "Request must be a JSON object"}},
	{Key: "native.cli.invalid_language", SourceTemplate: "--language must be zh-CN or en-US (supported aliases are accepted)", Texts: map[Locale]string{ZhCN: "--language 必须为 zh-CN 或 en-US，也支持已声明的语言别名", EnUS: "--language must be zh-CN or en-US (supported aliases are accepted)"}},
	{Key: "native.cli.request_limit", SourceTemplate: "Request exceeded 64 KiB", Texts: map[Locale]string{ZhCN: "请求超过 64 KiB", EnUS: "Request exceeded 64 KiB"}},
	{Key: "native.config.invalid_locale", SourceTemplate: "Unsupported explicit native display language", Texts: map[Locale]string{ZhCN: "原生显示语言必须为 zh-CN 或 en-US，也支持已声明的语言别名", EnUS: "Unsupported explicit native display language"}},
	{Key: "native.config.missing_control_auth", SourceTemplate: "Control token and project are required for tools", Texts: map[Locale]string{ZhCN: "使用工具需要控制服务凭证和项目标识", EnUS: "Control token and project are required for tools"}},
	{Key: "native.config.missing_model", SourceTemplate: "Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent", Texts: map[Locale]string{ZhCN: "运行智能体前请配置 SUMA_MODEL 和 SUMA_MODEL_BASE_URL", EnUS: "Configure SUMA_MODEL and SUMA_MODEL_BASE_URL to run an agent"}},
	{Key: "native.config.missing_provider", SourceTemplate: "Configure SUMA_MODEL_BASE_URL for an OpenAI-compatible provider", Texts: map[Locale]string{ZhCN: "请配置兼容 OpenAI 的提供方地址 SUMA_MODEL_BASE_URL", EnUS: "Configure SUMA_MODEL_BASE_URL for an OpenAI-compatible provider"}},
	{Key: "native.endpoint.rejected", SourceTemplate: "Endpoint rejected request (HTTP {{.status}})", Parameters: []Parameter{{Name: "status", Kind: Integer}}, Texts: map[Locale]string{ZhCN: "端点拒绝请求（HTTP {{.status}}）", EnUS: "Endpoint rejected request (HTTP {{.status}})"}},
	{Key: "native.endpoint.unsafe", SourceTemplate: "HTTPS or a loopback HTTP endpoint is required", Texts: map[Locale]string{ZhCN: "端点必须使用 HTTPS 或本机回环 HTTP 地址", EnUS: "HTTPS or a loopback HTTP endpoint is required"}},
	{Key: "native.limit.output_allowance", SourceTemplate: "Agent output allowance exhausted", Texts: map[Locale]string{ZhCN: "智能体输出额度已用尽", EnUS: "Agent output allowance exhausted"}},
	{Key: "native.limit.output_truncated", SourceTemplate: "Model output was truncated", Texts: map[Locale]string{ZhCN: "模型输出被截断", EnUS: "Model output was truncated"}},
	{Key: "native.limit.steps", SourceTemplate: "Agent reached the configured step limit", Texts: map[Locale]string{ZhCN: "智能体已达到配置的步骤上限", EnUS: "Agent reached the configured step limit"}},
	{Key: "native.limit.tool_calls", SourceTemplate: "At most eight calls are allowed per step", Texts: map[Locale]string{ZhCN: "每一步最多允许八次工具调用", EnUS: "At most eight calls are allowed per step"}},
	{Key: "native.request.invalid_goal", SourceTemplate: "A goal of 1–32000 characters is required", Texts: map[Locale]string{ZhCN: "目标必须包含 1–32000 个字符", EnUS: "A goal of 1–32000 characters is required"}},
	{Key: "native.request.invalid_limits", SourceTemplate: "Explicit max_steps/max_tokens must be positive integers within the supported bounds", Texts: map[Locale]string{ZhCN: "显式 max_steps/max_tokens 必须是支持范围内的正整数", EnUS: "Explicit max_steps/max_tokens must be positive integers within the supported bounds"}},
	{Key: "native.response.empty", SourceTemplate: "Empty upstream response", Texts: map[Locale]string{ZhCN: "上游响应为空", EnUS: "Empty upstream response"}},
	{Key: "native.response.invalid_json", SourceTemplate: "Upstream response is not JSON", Texts: map[Locale]string{ZhCN: "上游响应不是有效 JSON", EnUS: "Upstream response is not JSON"}},
	{Key: "native.response.no_choices", SourceTemplate: "Model returned no choices", Texts: map[Locale]string{ZhCN: "模型未返回候选结果", EnUS: "Model returned no choices"}},
	{Key: "native.response.no_text", SourceTemplate: "Model returned no text", Texts: map[Locale]string{ZhCN: "模型未返回文本", EnUS: "Model returned no text"}},
	{Key: "native.response.object_required", SourceTemplate: "Expected an object", Texts: map[Locale]string{ZhCN: "响应必须为对象", EnUS: "Expected an object"}},
	{Key: "native.response.too_large", SourceTemplate: "Upstream response exceeded 2 MiB", Texts: map[Locale]string{ZhCN: "上游响应超过 2 MiB", EnUS: "Upstream response exceeded 2 MiB"}},
	{Key: "native.run.aborted", SourceTemplate: "Agent aborted or timed out", Texts: map[Locale]string{ZhCN: "智能体已取消或超时", EnUS: "Agent aborted or timed out"}},
	{Key: "native.run.failed", SourceTemplate: "Agent request failed; inspect provider and control configuration", Texts: map[Locale]string{ZhCN: "智能体请求失败，请检查提供方与控制服务配置", EnUS: "Agent request failed; inspect provider and control configuration"}},
	{Key: "native.tools.alias_collision", SourceTemplate: "Provider tool alias collision", Texts: map[Locale]string{ZhCN: "提供方工具别名冲突", EnUS: "Provider tool alias collision"}},
	{Key: "native.tools.catalog_missing", SourceTemplate: "Control service did not return a tool catalog", Texts: map[Locale]string{ZhCN: "控制服务未返回工具目录", EnUS: "Control service did not return a tool catalog"}},
	{Key: "native.tools.duplicate_names", SourceTemplate: "Control service returned duplicate tool names", Texts: map[Locale]string{ZhCN: "控制服务返回了重复的工具名称", EnUS: "Control service returned duplicate tool names"}},
	{Key: "native.tools.invalid_arguments", SourceTemplate: "Tool arguments must be a JSON object", Texts: map[Locale]string{ZhCN: "工具参数必须是 JSON 对象", EnUS: "Tool arguments must be a JSON object"}},
	{Key: "native.tools.invalid_name", SourceTemplate: "Invalid tool name", Texts: map[Locale]string{ZhCN: "工具名称无效", EnUS: "Invalid tool name"}},
	{Key: "native.tools.malformed_call", SourceTemplate: "Malformed tool call", Texts: map[Locale]string{ZhCN: "工具调用格式无效", EnUS: "Malformed tool call"}},
	{Key: "native.tools.output_too_large", SourceTemplate: "Tool result exceeded 64 KiB", Texts: map[Locale]string{ZhCN: "工具结果超过 64 KiB", EnUS: "Tool result exceeded 64 KiB"}},
	{Key: "native.tools.too_many", SourceTemplate: "Too many tools", Texts: map[Locale]string{ZhCN: "工具数量超过上限", EnUS: "Too many tools"}},
	{Key: "native.tools.unknown_tool", SourceTemplate: "Model requested a tool outside the authenticated catalog", Texts: map[Locale]string{ZhCN: "模型请求了认证目录以外的工具", EnUS: "Model requested a tool outside the authenticated catalog"}},
	{Key: "native.usage.allowance_exceeded", SourceTemplate: "Provider reported output beyond the reserved request allowance", Texts: map[Locale]string{ZhCN: "提供方报告的输出量超过该请求的预留额度", EnUS: "Provider reported output beyond the reserved request allowance"}},
	{Key: "native.usage.conflicting_counters", SourceTemplate: "Conflicting model usage counters", Texts: map[Locale]string{ZhCN: "模型用量计数互相冲突", EnUS: "Conflicting model usage counters"}},
	{Key: "native.usage.integer_literal_required", SourceTemplate: "Model usage counters must use exact integer literals", Texts: map[Locale]string{ZhCN: "模型用量计数必须使用精确整数字面量", EnUS: "Model usage counters must use exact integer literals"}},
	{Key: "native.usage.invalid_counter", SourceTemplate: "Model usage counters must be nonnegative safe integers", Texts: map[Locale]string{ZhCN: "模型用量计数必须是非负安全整数", EnUS: "Model usage counters must be nonnegative safe integers"}},
	{Key: "native.usage.lossless_unavailable", SourceTemplate: "Runtime cannot verify exact usage counters; Node22 JSON source context is required", Texts: map[Locale]string{ZhCN: "运行时无法验证精确用量计数，需要 Node22 JSON 原文上下文支持", EnUS: "Runtime cannot verify exact usage counters; Node22 JSON source context is required"}},
}

// IsNativeMessageKey accepts only the fixed native diagnostic vocabulary. A
// future native-prefixed key does not become trusted through its prefix alone.
func IsNativeMessageKey(key string) bool {
	switch key {
	case
		"native.workspace.config",
		"native.workspace.catalog",
		"native.workspace.transport",
		"native.workspace.effects",
		"native.catalog.not_found",
		"native.catalog.probe_exited",
		"native.catalog.probe_failed",
		"native.catalog.probe_output_limit",
		"native.catalog.probe_timeout",
		"native.catalog.requirements_mismatch",
		"native.catalog.shell_wrapper",
		"native.cli.busy",
		"native.cli.invalid_arguments",
		"native.cli.invalid_json",
		"native.cli.invalid_language",
		"native.cli.request_limit",
		"native.config.invalid_locale",
		"native.config.missing_control_auth",
		"native.config.missing_model",
		"native.config.missing_provider",
		"native.endpoint.rejected",
		"native.endpoint.unsafe",
		"native.limit.output_allowance",
		"native.limit.output_truncated",
		"native.limit.steps",
		"native.limit.tool_calls",
		"native.request.invalid_goal",
		"native.request.invalid_limits",
		"native.response.empty",
		"native.response.invalid_json",
		"native.response.no_choices",
		"native.response.no_text",
		"native.response.object_required",
		"native.response.too_large",
		"native.run.aborted",
		"native.run.failed",
		"native.tools.alias_collision",
		"native.tools.catalog_missing",
		"native.tools.duplicate_names",
		"native.tools.invalid_arguments",
		"native.tools.invalid_name",
		"native.tools.malformed_call",
		"native.tools.output_too_large",
		"native.tools.too_many",
		"native.tools.unknown_tool",
		"native.usage.allowance_exceeded",
		"native.usage.conflicting_counters",
		"native.usage.integer_literal_required",
		"native.usage.invalid_counter",
		"native.usage.lossless_unavailable":
		return true
	default:
		return false
	}
}

// NativeCatalog returns a deep snapshot for deterministic SDK generation.
// Mutating its entries, parameter slices or text maps cannot alter formatting
// or expand the fixed native-key allowlist.
func NativeCatalog() []Definition {
	out := make([]Definition, 0, len(nativeCatalog))
	for _, d := range nativeCatalog {
		out = append(out, Definition{Key: d.Key, Parameters: append([]Parameter{}, d.Parameters...), Texts: map[Locale]string{ZhCN: d.Texts[ZhCN], EnUS: d.Texts[EnUS]}, SourceTemplate: d.SourceTemplate})
	}
	return out
}
