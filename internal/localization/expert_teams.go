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
	{Key: "expert_team.graph", SourceTemplate: "Expert team task dependencies are invalid", Texts: map[Locale]string{ZhCN: "团队任务依赖无效，请检查循环、缺失或跨项目引用", EnUS: "Expert team task dependencies are invalid"}},
	{Key: "expert_team.source", SourceTemplate: "Expert team pending task source changed; explicitly revise the plan", Texts: map[Locale]string{ZhCN: "未派发任务的依据已变化，请明确修订计划", EnUS: "Expert team pending task source changed; explicitly revise the plan"}},
	{Key: "expert_team.waiting", SourceTemplate: "Expert team task is waiting for formal predecessor acceptance", Texts: map[Locale]string{ZhCN: "正在等待前置任务正式验收", EnUS: "Expert team task is waiting for formal predecessor acceptance"}},
	{Key: "expert_team.wrong_run", SourceTemplate: "Expert team predecessor acceptance belongs to a different run", Texts: map[Locale]string{ZhCN: "前置任务的验收对应其他运行，不能推进本计划", EnUS: "Expert team predecessor acceptance belongs to a different run"}},
	{Key: "expert_team.stopped", SourceTemplate: "Expert team plan is stopped; active runs retain their own controls", Texts: map[Locale]string{ZhCN: "已停止后续派发；已启动运行仍使用各自的控制与回执", EnUS: "Expert team plan is stopped; active runs retain their own controls"}},
	{Key: "expert_team.paused", SourceTemplate: "Expert team plan is paused; active runs continue", Texts: map[Locale]string{ZhCN: "已暂停后续派发；已启动运行继续执行", EnUS: "Expert team plan is paused; active runs continue"}},
	{Key: "expert_team.authority", SourceTemplate: "Expert team activation credential is no longer authorized; explicitly resume", Texts: map[Locale]string{ZhCN: "启动凭据当前不可用，请以有效身份明确继续计划", EnUS: "Expert team activation credential is no longer authorized; explicitly resume"}},
	{Key: "expert_team.revision", SourceTemplate: "Expert team revision may only change unstarted tasks", Texts: map[Locale]string{ZhCN: "修订只能调整尚未派发的任务", EnUS: "Expert team revision may only change unstarted tasks"}},
	{Key: "expert_team.run_blocked", SourceTemplate: "Expert team run requires explicit recovery or review; no automatic retry", Texts: map[Locale]string{ZhCN: "运行需要明确恢复或评审，不会自动重试", EnUS: "Expert team run requires explicit recovery or review; no automatic retry"}},
}
