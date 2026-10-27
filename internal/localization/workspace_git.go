package localization

var workspaceGitCatalog = []Definition{
	{Key: "workspace_git.invalid", SourceTemplate: "Workspace Git request is invalid", Texts: map[Locale]string{ZhCN: "工作区Git读取请求无效，请检查项目、文件及差异来源", EnUS: "Workspace Git request is invalid. Check the project, file and difference source."}},
	{Key: "workspace_git.changed", SourceTemplate: "Workspace Git metadata or binding changed; refresh and retry", Texts: map[Locale]string{ZhCN: "工作区Git依据或绑定已变化，请刷新后重试", EnUS: "Workspace Git metadata or binding changed; refresh and retry."}},
	{Key: "workspace_git.failed", SourceTemplate: "Workspace Git inspection failed", Texts: map[Locale]string{ZhCN: "工作区Git读取失败，请检查仓库与Git配置", EnUS: "Workspace Git inspection failed. Check the repository and Git configuration."}},
	{Key: "workspace_git.limit", SourceTemplate: "Workspace Git inspection exceeds its fixed limit", Texts: map[Locale]string{ZhCN: "工作区Git读取超过固定时间或内容上限，请缩小文件范围后重试", EnUS: "Workspace Git inspection exceeds its fixed time or content limit. Narrow the file scope and retry."}},
	{Key: "workspace_git.conflicted", SourceTemplate: "Resolve this conflicted file before previewing its Git differences", Texts: map[Locale]string{ZhCN: "此文件存在合并冲突，请在文件编辑器处理后重新读取差异", EnUS: "Resolve this conflicted file in the editor before previewing its Git differences."}},
	{Key: "workspace_git.not_changed", SourceTemplate: "The selected file has no changes in this Git source", Texts: map[Locale]string{ZhCN: "此文件在所选Git来源中已无变更，请刷新列表", EnUS: "The selected file has no changes in this Git source. Refresh the list."}},
	{Key: "workspace_git.unsupported", SourceTemplate: "The selected Git file cannot be previewed safely", Texts: map[Locale]string{ZhCN: "所选Git文件不能安全预览，请检查文件类型、路径或仓库布局", EnUS: "The selected Git file cannot be previewed safely. Check its type, path or repository layout."}},
	{Key: "native.execution.invalid_limits", SourceTemplate: "Native agent execution limits are invalid", Texts: map[Locale]string{ZhCN: "原生智能体执行限额无效，请填写支持范围内的正整数步数与输出总额度", EnUS: "Native agent execution limits are invalid. Use positive integer steps and output allowance within the supported bounds."}},
}
