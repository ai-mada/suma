package localization

// Exact reviewed application diagnostics; vendor prose remains original evidence.
var codexExecutionCatalog = []Definition{
	{Key: "codex_skills.invalid", SourceTemplate: "Invalid Codex native skill binding", Texts: map[Locale]string{ZhCN: "Codex 原生技能绑定无效，请检查已发布技能与派发计划。", EnUS: "Invalid Codex native skill binding"}},
	{Key: "codex_skills.changed", SourceTemplate: "Codex native skill binding changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "Codex 原生技能依据已变化，请重新预览派发计划。", EnUS: "Codex native skill binding changed; prepare a new dispatch preview"}},
	{Key: "codex_skills.agent", SourceTemplate: "invalid: native skills require a fixed Codex agent", Texts: map[Locale]string{ZhCN: "原生技能需要固定的 Codex 智能体节点。", EnUS: "invalid: native skills require a fixed Codex agent"}},
	{Key: "codex_skills.bundle", SourceTemplate: "invalid: invalid native skill bundle", Texts: map[Locale]string{ZhCN: "原生技能包的内容或摘要无效。", EnUS: "invalid: invalid native skill bundle"}},
	{Key: "codex_skills.fixed", SourceTemplate: "invalid: invalid fixed Codex skills", Texts: map[Locale]string{ZhCN: "运行固定的 Codex 技能依据无效。", EnUS: "invalid: invalid fixed Codex skills"}},
	{Key: "codex_skills.evidence", SourceTemplate: "invalid: Codex skill evidence differs from fixed node", Texts: map[Locale]string{ZhCN: "Codex 技能凭证与本次固定节点不符。", EnUS: "invalid: Codex skill evidence differs from fixed node"}},
	{Key: "codex_skills.incomplete", SourceTemplate: "invalid: successful Codex run lacks complete skill evidence", Texts: map[Locale]string{ZhCN: "Codex 运行缺少完整技能凭证，不能记为成功。", EnUS: "invalid: successful Codex run lacks complete skill evidence"}},
	{Key: "codex_execution.invalid", SourceTemplate: "Invalid Codex model execution selection", Texts: map[Locale]string{ZhCN: "Codex 模型选择或用量提醒设置无效，请重新检查。", EnUS: "Invalid Codex model execution selection"}},
	{Key: "codex_execution.unsupported", SourceTemplate: "Independent Codex model selection requires an approved app-server tool", Texts: map[Locale]string{ZhCN: "请选择已批准并支持独立模型配置的 Codex 工具。", EnUS: "Independent Codex model selection requires an approved app-server tool"}},
	{Key: "codex_execution.changed", SourceTemplate: "Codex model catalog or local configuration changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "Codex 模型目录或本地配置已变化，请重新预览派发计划。", EnUS: "Codex model catalog or local configuration changed; prepare a new dispatch preview"}},
	{Key: "codex_execution.unavailable", SourceTemplate: "The selected Codex model is not in the current authorized worker catalog", Texts: map[Locale]string{ZhCN: "当前已授权执行节点的目录中没有所选 Codex 模型。", EnUS: "The selected Codex model is not in the current authorized worker catalog"}},
	{Key: "codex_execution.budget", SourceTemplate: "Codex app-server supports observed usage only; enforced budgets are unavailable", Texts: map[Locale]string{ZhCN: "此 Codex 接入仅支持用量观测，无法满足当前硬预算策略。", EnUS: "Codex app-server supports observed usage only; enforced budgets are unavailable"}},
	{Key: "codex_execution.binding", SourceTemplate: "invalid: invalid Codex execution binding", Texts: map[Locale]string{ZhCN: "流程中的 Codex 配置绑定无效。", EnUS: "invalid: invalid Codex execution binding"}},
	{Key: "codex_execution.tool", SourceTemplate: "invalid: Codex execution requires a fixed agent tool", Texts: map[Locale]string{ZhCN: "Codex 执行需要固定的智能体工具。", EnUS: "invalid: Codex execution requires a fixed agent tool"}},
	{Key: "codex_execution.evidence", SourceTemplate: "invalid: Codex configuration evidence differs from fixed node", Texts: map[Locale]string{ZhCN: "Codex 配置确认与固定节点依据不一致。", EnUS: "invalid: Codex configuration evidence differs from fixed node"}},
}
