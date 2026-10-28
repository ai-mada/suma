package localization

var workspaceTreeCatalog = []Definition{
	{Key: "workspace_tree.invalid", SourceTemplate: "Workspace tree request is invalid", Texts: map[Locale]string{ZhCN: "目录浏览请求无效，请检查项目、目录和分页参数", EnUS: "Workspace tree request is invalid. Check the project, directory and page parameters."}},
	{Key: "workspace_tree.forbidden", SourceTemplate: "Workspace directory path is not allowed", Texts: map[Locale]string{ZhCN: "此目录不在允许浏览的工作区路径范围内", EnUS: "This directory is outside the allowed workspace paths."}},
	{Key: "workspace_tree.unavailable", SourceTemplate: "Workspace directory is unavailable", Texts: map[Locale]string{ZhCN: "目录不可用，请检查当前绑定、目录是否存在及读取权限", EnUS: "The directory is unavailable. Check the binding, directory and read permissions."}},
	{Key: "workspace_tree.directory_limit", SourceTemplate: "Workspace directory exceeds the 10000-entry limit", Texts: map[Locale]string{ZhCN: "此目录超过一万项扫描上限，请使用项目子目录作为工作区", EnUS: "This directory exceeds the 10,000-entry scan limit. Bind a project subdirectory as the workspace."}},
	{Key: "workspace_tree.changed", SourceTemplate: "Workspace directory changed; reload and retry", Texts: map[Locale]string{ZhCN: "目录或工作区绑定已变化，请刷新目录后重试", EnUS: "The directory or workspace binding has changed. Refresh the directory and try again."}},
}
