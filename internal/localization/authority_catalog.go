package localization

// Authority diagnostics are product messages, never translations of user or
// tool evidence. Additional locales extend this catalog through the registry.
var authorityCatalog = []Definition{
	{Key: "auth.workspace_acceptance_required", Texts: map[Locale]string{ZhCN: "先前保存缺少可信受理记录，需要核对", EnUS: "The earlier save has no trusted acceptance record. Reconciliation is required."}},
	{Key: "auth.artifact_basis_changed", Texts: map[Locale]string{ZhCN: "制品读取依据已变化，请重试", EnUS: "The artifact read basis has changed. Try again."}},
	{Key: "auth.release_credential_changed", Texts: map[Locale]string{ZhCN: "发布执行凭证已变化，请重试", EnUS: "The release execution credential has changed. Try again."}},
	{Key: "auth.release_authority_changed", Texts: map[Locale]string{ZhCN: "发布授权已变化，请重试", EnUS: "Release authorization has changed. Try again."}},
	{Key: "auth.workspace_binding_changed", Texts: map[Locale]string{ZhCN: "工作区绑定已变化，请重新加载", EnUS: "The workspace binding has changed. Reload the workspace."}},
	{Key: "auth.ownership_changed", Texts: map[Locale]string{ZhCN: "授权归属已变化，请重试", EnUS: "The authorization ownership has changed. Try again."}},
	{Key: "auth.pool_ownership_changed", Texts: map[Locale]string{ZhCN: "资源池归属已变化，请重试", EnUS: "The execution pool ownership has changed. Try again."}},
	{Key: "agent.tool_overview", Texts: map[Locale]string{ZhCN: "读取当前授权项目的真实状态与活动", EnUS: "Read the current authorized project's actual status and activity"}},
	{Key: "agent.tool_tasks", Texts: map[Locale]string{ZhCN: "读取当前项目任务", EnUS: "Read tasks in the current project"}},
	{Key: "agent.tool_search", Texts: map[Locale]string{ZhCN: "检索当前项目资料并返回来源", EnUS: "Search current project material and return its sources"}},

	{Key: "auth.authority_changed", Texts: map[Locale]string{ZhCN: "权限集合已改变，请重试", EnUS: "Permissions have changed. Try again."}},
	{Key: "auth.account_changed", Texts: map[Locale]string{ZhCN: "账号状态已改变，请重试", EnUS: "The account state has changed. Try again."}},
	{Key: "auth.read_required", Texts: map[Locale]string{ZhCN: "读取操作不能为空", EnUS: "A read operation is required."}},
	{Key: "activity.invitation_accepted", Texts: map[Locale]string{ZhCN: "接受组织邀请", EnUS: "Accepted the organization invitation"}},
}
