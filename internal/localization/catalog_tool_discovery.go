package localization

// Exact diagnostics from tool_discovery.go and tool_discovery_http.go.
// Inventory names and provider output are user/source data, not aliases.
var toolDiscoveryCatalog = []Definition{
	{Key: "tool_discovery.invalid_scope", SourceTemplate: "项目或执行节点标识无效", Texts: map[Locale]string{ZhCN: "项目或执行节点标识无效", EnUS: "The project or execution node identifier is invalid."}},
	{Key: "tool_discovery.grant_changed", SourceTemplate: "资源池授权依据已变化，请重试", Texts: map[Locale]string{ZhCN: "资源池授权依据已变化，请重试", EnUS: "The execution pool authorization changed. Retry with the current grant."}},
	{Key: "tool_discovery.invalid_request", SourceTemplate: "工具检测请求无效", Texts: map[Locale]string{ZhCN: "工具检测请求无效", EnUS: "The tool discovery request is invalid."}},
}
