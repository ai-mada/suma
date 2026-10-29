package localization

// Project focus owns only its controlled API diagnostics. Task titles, run
// facts and user-authored text never become translation templates.
var projectFocusCatalog = []Definition{
	{Key: "project_focus.query", SourceTemplate: "Invalid project focus query", Texts: map[Locale]string{ZhCN: "项目待办查询无效，请使用当前项目且不要附加筛选参数", EnUS: "The project focus query is invalid. Use the current project without extra filters."}},
	{Key: "project_focus.request", SourceTemplate: "Project focus request is invalid", Texts: map[Locale]string{ZhCN: "项目待办请求无效，请提供当前项目", EnUS: "The project focus request is invalid. Provide the current project."}},
}
