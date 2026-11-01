package localization

// Parameterized diagnostics use explicit producer keys. Prefix matching cannot
// identify their meaning safely; field identifiers and original decoder/tool
// details are data, not templates. Invalid parameters retain the raw diagnostic.
var validationCatalog = []Definition{
	{Key: "validation.knowledge_discovery", Texts: map[Locale]string{ZhCN: "项目知识发现筛选或游标无效", EnUS: "Invalid project knowledge discovery filters or cursor."}},
	{Key: "validation.tool_recovery_changed", Texts: map[Locale]string{ZhCN: "核对依据已变化，请读取原记录；无法核实原节点时请隔离旧身份并重新配对", EnUS: "The reconciliation source changed. Read the original record; isolate and re-pair the node if its identity cannot be verified."}},
	{Key: "validation.tool_recovery_busy", Texts: map[Locale]string{ZhCN: "本次检查已有未结束的核对，请先读取原核对记录", EnUS: "This inspection already has an unfinished reconciliation. Read its original record first."}},
	{Key: "validation.tool_maintenance_required", Texts: map[Locale]string{ZhCN: "执行节点尚未声明当前维护协议，请连接支持该协议的连续执行节点", EnUS: "The node has not declared the required maintenance protocol. Connect a compatible continuous worker."}},
	{Key: "validation.field_required", Parameters: []Parameter{{Name: "field", Kind: Identifier}}, Texts: map[Locale]string{ZhCN: "缺少{{.field}}", EnUS: "{{.field}} is required."}},
	{Key: "validation.field_integer", Parameters: []Parameter{{Name: "field", Kind: Identifier}}, Texts: map[Locale]string{ZhCN: "{{.field}}必须为整数", EnUS: "{{.field}} must be an integer."}},
	{Key: "validation.field_version_integer", Parameters: []Parameter{{Name: "field", Kind: Identifier}}, Texts: map[Locale]string{ZhCN: "{{.field}}必须为有效版本整数", EnUS: "{{.field}} must be a valid version integer."}},
	{Key: "validation.field_safe_integer", Parameters: []Parameter{{Name: "field", Kind: Identifier}}, Texts: map[Locale]string{ZhCN: "{{.field}}必须为安全非负整数", EnUS: "{{.field}} must be a safe non-negative integer."}},
	{Key: "validation.field_string", Parameters: []Parameter{{Name: "field", Kind: Identifier}}, Texts: map[Locale]string{ZhCN: "{{.field}}必须为字符串", EnUS: "{{.field}} must be a string."}},
	{Key: "validation.schedule_field_required", Parameters: []Parameter{{Name: "field", Kind: Identifier}}, Texts: map[Locale]string{ZhCN: "排期必须包含完整字段：{{.field}}", EnUS: "The schedule must include the complete field: {{.field}}"}},
	{Key: "validation.automation_detail", Parameters: []Parameter{{Name: "detail", Kind: Text}}, Texts: map[Locale]string{ZhCN: "自动化字段或类型无效: {{.detail}}", EnUS: "Invalid automation fields or types: {{.detail}}"}},
	{Key: "validation.release_detail", Parameters: []Parameter{{Name: "detail", Kind: Text}}, Texts: map[Locale]string{ZhCN: "发布请求存在未知字段或错误类型: {{.detail}}", EnUS: "The release request contains unknown fields or incorrect types: {{.detail}}"}},
	{Key: "validation.tool_inventory_detail", Parameters: []Parameter{{Name: "detail", Kind: Text}}, Texts: map[Locale]string{ZhCN: "工具探测数据无效：{{.detail}}", EnUS: "Invalid tool observation: {{.detail}}"}},
	{Key: "validation.tool_profile_detail", Parameters: []Parameter{{Name: "detail", Kind: Text}}, Texts: map[Locale]string{ZhCN: "固定工具档案无效：{{.detail}}", EnUS: "Invalid pinned tool profile: {{.detail}}"}},
	{Key: "validation.tool_profile_filter", Texts: map[Locale]string{ZhCN: "工具列表筛选无效", EnUS: "Invalid tool list filters."}},
	{Key: "validation.tool_inspection", Texts: map[Locale]string{ZhCN: "工具重检请求无效", EnUS: "Invalid tool inspection request."}},
	{Key: "validation.tool_inspection_changed", Texts: map[Locale]string{ZhCN: "工具重检依据已变化，请查看原请求后重新配置", EnUS: "The inspection source changed. Review the original request before reconfiguring."}},
	{Key: "validation.tool_inspection_busy", Texts: map[Locale]string{ZhCN: "执行节点已有未完成的工具重检，请先核对原请求", EnUS: "This worker already has an unfinished inspection. Reconcile the original request first."}},
	{Key: "validation.tool_inspection_limit", Texts: map[Locale]string{ZhCN: "项目工具重检记录已达上限", EnUS: "The project inspection history limit has been reached."}},
}
