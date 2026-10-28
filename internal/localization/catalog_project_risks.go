package localization

// Exact current risk-domain producers only. User plans, model advice and
// immutable evidence remain original text and never match by a loose prefix.
var projectRiskCatalog = []Definition{
	{Key: "project_risk.request", SourceTemplate: "Invalid project risk request", Texts: map[Locale]string{ZhCN: "项目风险请求无效", EnUS: "The project risk request is invalid."}},
	{Key: "project_risk.changed", SourceTemplate: "The risk or its review basis has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "风险或复核依据已变化，请重新读取后决定", EnUS: "The risk or its review basis has changed. Reload before deciding."}},
	{Key: "project_risk.state", SourceTemplate: "The current risk state does not allow this action", Texts: map[Locale]string{ZhCN: "当前风险状态不允许此操作", EnUS: "The current risk state does not allow this action."}},
	{Key: "project_risk.owner", SourceTemplate: "The risk owner must be a current authorized human project resource", Texts: map[Locale]string{ZhCN: "风险负责人必须是当前有项目权限的人类资源", EnUS: "The risk owner must be a currently authorized human project resource."}},
	{Key: "project_risk.readiness", SourceTemplate: "Complete the risk assessment and current linked evidence before review", Texts: map[Locale]string{ZhCN: "请先完善风险评估及当前关联依据，再提交复核", EnUS: "Complete the risk assessment and current linked evidence before review."}},
	{Key: "project_risk.mitigation", SourceTemplate: "Mitigated risks require current formally accepted task evidence", Texts: map[Locale]string{ZhCN: "缓解风险需要当前正式验收通过的任务证据", EnUS: "Risk mitigation requires current formally accepted task evidence."}},
	{Key: "project_risk.source", SourceTemplate: "The selected planning suggestion is unavailable or has changed", Texts: map[Locale]string{ZhCN: "所选规划建议不可用或已变化", EnUS: "The selected planning suggestion is unavailable or has changed."}},
}
