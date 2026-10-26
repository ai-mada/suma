package localization

// This closed vocabulary belongs to the expert-team domain. Names, role
// descriptions, task content and received tool output are never translated.
var expertTeamCatalog = []Definition{
	{Key: "expert_team.invalid", SourceTemplate: "Invalid expert team request", Texts: map[Locale]string{ZhCN: "专家团队请求无效", EnUS: "Invalid expert team request"}},
	{Key: "expert_team.changed", SourceTemplate: "Expert team configuration changed; refresh the preview", Texts: map[Locale]string{ZhCN: "专家配置已变化，请刷新后重新预览", EnUS: "Expert team configuration changed; refresh the preview"}},
	{Key: "expert_team.retired", SourceTemplate: "Expert team configuration is retired", Texts: map[Locale]string{ZhCN: "此专家配置已退役", EnUS: "Expert team configuration is retired"}},
	{Key: "expert_team.inactive", SourceTemplate: "Expert team is not active", Texts: map[Locale]string{ZhCN: "此团队当前未启用", EnUS: "Expert team is not active"}},
	{Key: "expert_team.resource", SourceTemplate: "Expert team resource or membership changed", Texts: map[Locale]string{ZhCN: "专家资源或团队成员关系已变化", EnUS: "Expert team resource or membership changed"}},
	{Key: "expert_team.binding", SourceTemplate: "Expert team workflow, tool or knowledge is no longer eligible", Texts: map[Locale]string{ZhCN: "专家配置中的流程、工具或知识当前不可用", EnUS: "Expert team workflow, tool or knowledge is no longer eligible"}},
	{Key: "expert_team.unpublished", SourceTemplate: "Expert team configuration must be published", Texts: map[Locale]string{ZhCN: "请先审核发布专家配置", EnUS: "Expert team configuration must be published"}},
	{Key: "expert_team.expired", SourceTemplate: "Expert team dispatch preview expired", Texts: map[Locale]string{ZhCN: "团队派发预览已过期，请重新预览", EnUS: "Expert team dispatch preview expired"}},
	{Key: "expert_team.plan_changed", SourceTemplate: "Expert team dispatch plan changed", Texts: map[Locale]string{ZhCN: "团队派发计划已变化，请读取当前状态", EnUS: "Expert team dispatch plan changed"}},
	{Key: "expert_team.owned", SourceTemplate: "This task dispatch plan belongs to an expert team batch", Texts: map[Locale]string{ZhCN: "此任务计划属于团队批次，请从原批次派发", EnUS: "This task dispatch plan belongs to an expert team batch"}},
	{Key: "expert_team.ack", SourceTemplate: "Explicit expert team dispatch acknowledgement required", Texts: map[Locale]string{ZhCN: "请明确确认本次团队任务派发", EnUS: "Explicit expert team dispatch acknowledgement required"}},
	{Key: "expert_team.task", SourceTemplate: "Expert team task is active or its dependencies are not accepted", Texts: map[Locale]string{ZhCN: "所选任务正在执行，或前置任务尚未验收", EnUS: "Expert team task is active or its dependencies are not accepted"}},
}
