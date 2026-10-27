package localization

var modelExecutionCatalog = []Definition{
	{Key: "model_execution.evidence", SourceTemplate: "invalid: model execution evidence differs from fixed node", Texts: map[Locale]string{ZhCN: "执行模型证据与固定节点依据不一致", EnUS: "Model execution evidence differs from the fixed node."}},
	{Key: "model_execution.invalid", SourceTemplate: "Invalid native model execution selection", Texts: map[Locale]string{ZhCN: "模型选择、执行参数或预算无效，请检查后重新预览", EnUS: "Invalid native model execution selection"}},
	{Key: "model_execution.unsupported", SourceTemplate: "Independent model selection requires an approved native workspace tool", Texts: map[Locale]string{ZhCN: "独立选择模型需要已批准并支持此能力的原生工作区工具", EnUS: "Independent model selection requires an approved native workspace tool"}},
	{Key: "model_execution.changed", SourceTemplate: "Model deployment, parameters or rate changed; prepare a new dispatch preview", Texts: map[Locale]string{ZhCN: "模型部署、参数或费率依据已变化，请重新预览执行计划", EnUS: "Model deployment, parameters or rate changed; prepare a new dispatch preview"}},
	{Key: "model_execution.unavailable", SourceTemplate: "The selected model deployment is not in the current authorized worker inventory", Texts: map[Locale]string{ZhCN: "所选模型版本不在当前已授权执行节点的有效库存中", EnUS: "The selected model deployment is not in the current authorized worker inventory"}},
	{Key: "model_execution.binding", SourceTemplate: "invalid: invalid model execution binding", Texts: map[Locale]string{ZhCN: "流程的固定模型绑定无效", EnUS: "invalid: invalid model execution binding"}},
	{Key: "model_execution.tool", SourceTemplate: "invalid: model execution requires fixed native agent tool", Texts: map[Locale]string{ZhCN: "模型执行需要固定原生智能体工具", EnUS: "invalid: model execution requires fixed native agent tool"}},
	{Key: "model_execution.limits", SourceTemplate: "invalid: model execution limits differ", Texts: map[Locale]string{ZhCN: "流程额度与固定模型参数不一致", EnUS: "invalid: model execution limits differ"}},
}
