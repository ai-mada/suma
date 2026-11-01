package localization

var turnSteeringCatalog = []Definition{
	{Key: "turn_steering.invalid", SourceTemplate: "Invalid turn steering request", Texts: map[Locale]string{ZhCN: "补充指令请求无效，请核对原消息和执行目标", EnUS: "Invalid turn steering request"}},
	{Key: "turn_steering.changed", SourceTemplate: "The live execution or discussion source changed; refresh before sending", Texts: map[Locale]string{ZhCN: "当前执行或讨论来源已变化，请刷新后重新核对", EnUS: "The live execution or discussion source changed; refresh before sending"}},
}
