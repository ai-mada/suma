package localization

// Application tool descriptions and transport diagnostics are display-only.
// Original project content, operation receipts and tool arguments stay unchanged.
var applicationToolCatalog = []Definition{
	{Key: "application_tool.object_required", Texts: map[Locale]string{ZhCN: "工具参数必须是有效的 JSON 对象", EnUS: "Tool arguments must be a valid JSON object."}},
	{Key: "application_tool_project_requirements", Texts: map[Locale]string{ZhCN: "分页读取当前项目需求", EnUS: "Read a page of current project requirements"}},
	{Key: "application_tool_project_requirement", Texts: map[Locale]string{ZhCN: "读取当前项目的单个正式需求", EnUS: "Read one current project requirement"}},
	{Key: "application_tool_project_task", Texts: map[Locale]string{ZhCN: "读取当前项目的单个正式任务", EnUS: "Read one current project task"}},
	{Key: "application_tool_project_requirement_design", Texts: map[Locale]string{ZhCN: "读取当前需求设计与发布资格", EnUS: "Read the current requirement design and publication eligibility"}},
	{Key: "application_tool_project_task_design", Texts: map[Locale]string{ZhCN: "读取任务原采用设计依据与当前资格", EnUS: "Read the originally adopted task design and its current eligibility"}},
	{Key: "application_tool_project_knowledge_list", Texts: map[Locale]string{ZhCN: "分页读取当前项目知识元数据与草稿状态", EnUS: "Read project knowledge metadata and draft state by page"}},
	{Key: "application_tool_project_requirement_draft_create", Texts: map[Locale]string{ZhCN: "创建当前项目需求草稿，不批准或启动执行", EnUS: "Create a project requirement draft without approval or execution"}},
	{Key: "application_tool_project_task_draft_create", Texts: map[Locale]string{ZhCN: "创建当前项目待办任务，不验收或启动执行", EnUS: "Create a project to-do task without acceptance or execution"}},
	{Key: "application_tool_project_requirement_design_save", Texts: map[Locale]string{ZhCN: "保存版本化需求设计草稿，不发布或采用任务", EnUS: "Save a versioned requirement design draft without publishing or adopting tasks"}},
	{Key: "application_tool.input_invalid", Texts: map[Locale]string{ZhCN: "应用工具参数无效，请核对字段、类型和数量限制", EnUS: "Application tool arguments are invalid. Check the fields, types and item limits."}},
	{Key: "application_tool.output_limit", Texts: map[Locale]string{ZhCN: "工具读取结果超过64KiB，请缩小页面或引用范围", EnUS: "Tool read results exceed 64 KiB. Reduce the page size or reference scope."}},
	{Key: "mcp.output_limit", Texts: map[Locale]string{ZhCN: "工具输出超过限制，请缩小页面或引用范围", EnUS: "Tool output exceeds the limit. Reduce the page size or reference scope."}},
	{Key: "mcp.origin_rejected", Texts: map[Locale]string{ZhCN: "请求来源或主机不在允许范围", EnUS: "The request origin or host is not allowed."}},
	{Key: "mcp.arguments_invalid", Texts: map[Locale]string{ZhCN: "工具参数无效，请按工具声明提供完整且无重复字段的参数", EnUS: "Tool arguments are invalid. Follow the declared schema and provide complete fields without duplicates."}},
	{Key: "mcp.timeout", Texts: map[Locale]string{ZhCN: "工具调用超过时间限制，请核对原操作后重试", EnUS: "The tool call timed out. Check the original operation before retrying."}},
	{Key: "mcp.cancelled", Texts: map[Locale]string{ZhCN: "工具调用已取消，请核对原操作状态", EnUS: "The tool call was cancelled. Check the original operation state."}},
	{Key: "mcp.public_url_invalid", Texts: map[Locale]string{ZhCN: "公开服务地址必须为 HTTPS 源地址，本地回环开发地址可使用 HTTP", EnUS: "The public service URL must be an HTTPS origin. Local loopback development may use HTTP."}},
	{Key: "mcp.public_url_unspecified", Texts: map[Locale]string{ZhCN: "公开服务地址不能使用通配监听地址", EnUS: "The public service URL cannot use a wildcard listening address."}},
	{Key: "application_tool.project_required", Texts: map[Locale]string{ZhCN: "请选择有效项目", EnUS: "Select a valid project."}},
}
