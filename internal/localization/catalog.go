package localization

// 键表示稳定的显示语义。相同 HTTP code 可以对应多个具体消息；翻译不
// 改变机器状态、代码、用户内容或任何持久化业务字段。
var catalog = combineCatalogs(commonCatalog, poolCatalog, preferencesCatalog, workspaceCatalog, workspaceFileContextCatalog, controlCatalog, executionCatalog, releaseCatalog, validationCatalog, dagCatalog, nativeCatalog, toolDiagnosticCatalog, attemptHistoryCatalog, displayCatalog, authorityCatalog, knowledgeCatalog, requirementDesignCatalog, candidateTransferCatalog, applicationToolCatalog, applicationQualityCatalog, nativeDevelopmentCatalog, teamCLICatalog, taskHandoffCatalog, taskDependenciesCatalog, taskDeliveryCatalog, requirementTraceabilityCatalog, intelligentOperationsCatalog, testPlanCatalog, intelligentPlanningCatalog, usageAccountingCatalog, skillCatalog, knowledgeIntelligenceCatalog, expertTeamCatalog, repositoryIntegrationCatalog, failureDiagnosisCatalog, candidateCompositionCatalog, taskDispatchOptionsCatalog, workspaceGitCatalog, workspaceTreeCatalog, desktopWorkspaceCatalog, modelExecutionCatalog, workspaceObservationCatalog, workspaceWriteCatalog, codexExecutionCatalog, openCodeExecutionCatalog, octopExecutionCatalog, issueRepairCatalog, workspaceOperationCatalog, projectRiskCatalog, dispatchRecommendationCatalog, projectApplicationCatalog, applicationReleaseSetCatalog, applicationReleaseRolloutCatalog, workspaceTaskContextCatalog, taskConversationCatalog, taskConversationReplyCatalog, interactiveDevelopmentCatalog, projectFocusCatalog)

// Build a separate immutable catalog so adding a domain cannot reuse and mutate
// another domain's backing array. Lookup and formatting remain request-scoped.
func combineCatalogs(groups ...[]Definition) []Definition {
	count := 0
	for _, group := range groups {
		count += len(group)
	}
	out := make([]Definition, 0, count)
	for _, group := range groups {
		out = append(out, group...)
	}
	return out
}

var commonCatalog = []Definition{
	{Key: "auth.view_context_required", Texts: map[Locale]string{ZhCN: "需要原始账户与组织上下文，请刷新页面后重试", EnUS: "The original account and organization context is required. Refresh the page and try again."}},
	{Key: "auth.view_context_changed", Texts: map[Locale]string{ZhCN: "账户或组织已变化，请核对原请求后重试", EnUS: "The account or organization has changed. Review the original request before retrying."}},
	{Key: "common.forbidden", Texts: map[Locale]string{ZhCN: "当前身份没有此操作权限", EnUS: "Your current identity does not have permission for this operation."}},
	{Key: "common.not_found", Texts: map[Locale]string{ZhCN: "记录不存在或不可访问", EnUS: "The record does not exist or is not accessible."}},
	{Key: "common.service_unavailable", Texts: map[Locale]string{ZhCN: "服务暂时无法完成操作", EnUS: "The service cannot complete this operation right now."}},
	{Key: "common.invalid_input", Texts: map[Locale]string{ZhCN: "输入无效", EnUS: "The input is invalid."}},
	{Key: "common.version_conflict", Texts: map[Locale]string{ZhCN: "数据已变化，请刷新后重试", EnUS: "The data has changed. Refresh and try again."}},
	{Key: "workflow.not_found", Texts: map[Locale]string{ZhCN: "运行或流程不存在", EnUS: "The run or workflow does not exist."}},
	{Key: "auth.sign_in_required", Texts: map[Locale]string{ZhCN: "请登录", EnUS: "Please sign in."}},
	{Key: "auth.session_expired", Texts: map[Locale]string{ZhCN: "登录已失效", EnUS: "Your sign-in session has expired."}},
	{Key: "http.origin_rejected", Texts: map[Locale]string{ZhCN: "请求来源不在允许范围", EnUS: "The request origin is not allowed."}},
	{Key: "pool.bearer_required", Texts: map[Locale]string{ZhCN: "资源池执行节点需要独立 Bearer 凭证", EnUS: "A pool runner requires its own Bearer credential."}},
	{Key: "pool.paused", Texts: map[Locale]string{ZhCN: "资源池已暂停，停止新派发", EnUS: "The execution pool is paused. New dispatches are stopped."}},
	{Key: "validation.max_length", Parameters: []Parameter{{Name: "field", Kind: Identifier}, {Name: "limit", Kind: Integer}}, Texts: map[Locale]string{ZhCN: "{{.field}} 的长度不能超过 {{.limit}}", EnUS: "{{.field}} must not exceed {{.limit}} characters."}},
}

// These domains have explicitly owned English API producers. Register only
// that reviewed domain; SourceTemplate elsewhere never becomes a global alias.
var sourceAliases = func() map[string]string {
	out := map[string]string{"登录已过期": "auth.session_expired"}
	for _, d := range projectFocusCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range interactiveDevelopmentCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range taskConversationReplyCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range taskConversationCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceTaskContextCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range applicationQualityCatalog {
		if d.SourceTemplate != "" {
			out[d.SourceTemplate] = d.Key
		}
	}
	for _, d := range taskDeliveryCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range intelligentOperationsCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range testPlanCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range requirementTraceabilityCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range taskDependenciesCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range openCodeExecutionCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range codexExecutionCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceWriteCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceFileContextCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceObservationCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range issueRepairCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceOperationCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range applicationReleaseRolloutCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range applicationReleaseSetCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range projectApplicationCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range dispatchRecommendationCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range projectRiskCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range modelExecutionCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range desktopWorkspaceCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceTreeCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range workspaceGitCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range taskDispatchOptionsCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range candidateCompositionCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range failureDiagnosisCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range repositoryIntegrationCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range expertTeamCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, d := range knowledgeIntelligenceCatalog {
		out[d.SourceTemplate] = d.Key
	}
	for _, definition := range skillCatalog {
		if definition.SourceTemplate != "" {
			out[definition.SourceTemplate] = definition.Key
		}
	}
	for _, definition := range candidateTransferCatalog {
		out[definition.SourceTemplate] = definition.Key
	}
	for _, definition := range taskHandoffCatalog {
		if definition.SourceTemplate != "" {
			out[definition.SourceTemplate] = definition.Key
		}
	}
	for _, definition := range intelligentPlanningCatalog {
		out[definition.SourceTemplate] = definition.Key
	}
	for _, definition := range usageAccountingCatalog {
		out[definition.SourceTemplate] = definition.Key
	}
	return out
}()
var codes = map[string]string{
	"forbidden": "common.forbidden", "not_found": "common.not_found", "internal_error": "common.service_unavailable",
	"unauthenticated": "auth.sign_in_required", "invalid_input": "common.invalid_input", "version_conflict": "common.version_conflict",
}
