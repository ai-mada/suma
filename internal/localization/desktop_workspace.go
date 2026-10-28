package localization

var desktopWorkspaceCatalog = []Definition{
	{Key: "desktop_workspace.unavailable", SourceTemplate: "Desktop workspace selection is unavailable", Texts: map[Locale]string{ZhCN: "当前服务未启用桌面项目目录选择", EnUS: "Desktop workspace selection is unavailable on this service."}},
	{Key: "desktop_workspace.invalid", SourceTemplate: "Desktop workspace request is invalid", Texts: map[Locale]string{ZhCN: "桌面目录绑定请求无效，请重新选择项目目录", EnUS: "The desktop workspace request is invalid. Choose the project directory again."}},
	{Key: "desktop_workspace.grant_invalid", SourceTemplate: "Desktop workspace grant is invalid", Texts: map[Locale]string{ZhCN: "目录选择证明与当前账号、组织或项目不符，请重新选择", EnUS: "The directory selection proof does not match the current account, organization or project. Choose it again."}},
	{Key: "desktop_workspace.grant_expired", SourceTemplate: "Desktop workspace grant has expired; choose the directory again", Texts: map[Locale]string{ZhCN: "本次目录选择已过期，请重新选择后绑定", EnUS: "The directory selection has expired. Choose the directory again before binding."}},
	{Key: "desktop_workspace.binding_changed", SourceTemplate: "Workspace binding changed; reload before choosing a directory", Texts: map[Locale]string{ZhCN: "项目工作区绑定已变化，请刷新后重新选择目录", EnUS: "The workspace binding has changed. Refresh before choosing a directory again."}},
	{Key: "desktop_workspace.operation_conflict", SourceTemplate: "Desktop workspace operation ID was already used for different content", Texts: map[Locale]string{ZhCN: "此绑定操作标识已用于其他内容，请核对原操作", EnUS: "This binding operation ID was already used for different content. Review the original operation."}},
	{Key: "desktop_workspace.root_invalid", SourceTemplate: "Desktop workspace root proof is invalid; choose the directory again", Texts: map[Locale]string{ZhCN: "项目目录授权证明无效，请在桌面中重新选择目录", EnUS: "The project directory authorization is invalid. Choose the directory again in the desktop app."}},
	{Key: "desktop_workspace.root_changed", SourceTemplate: "Desktop workspace directory identity changed; choose the directory again", Texts: map[Locale]string{ZhCN: "原项目目录已被替换，请重新选择目录，不要沿用旧授权", EnUS: "The original project directory was replaced. Choose it again instead of reusing its previous authorization."}},
	{Key: "desktop_workspace.root_unavailable", SourceTemplate: "Desktop workspace directory is unavailable; choose the directory again", Texts: map[Locale]string{ZhCN: "项目目录当前不可用，请检查目录后在桌面中重新选择", EnUS: "The project directory is unavailable. Check it and choose it again in the desktop app."}},
}
