package localization

// Exact platform-owned diagnostics only; model questions and human answers
// retain their original content and are never translated as error prefixes.
var toolInputsCatalog = []Definition{
	{Key: "tool_inputs.invalid", SourceTemplate: "Invalid tool input request", Texts: map[Locale]string{ZhCN: "问题回答请求无效，请核对原问题和当前执行", EnUS: "Invalid tool input request"}},
	{Key: "tool_inputs.changed", SourceTemplate: "The tool input source changed; reload before answering", Texts: map[Locale]string{ZhCN: "问题来源已变化，请重新加载后回答", EnUS: "The tool input source changed; reload before answering"}},
}
