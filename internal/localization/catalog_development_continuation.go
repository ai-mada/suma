package localization

// Only platform-owned continuation diagnostics are translated. Commit IDs,
// machine evidence and human instructions remain their exact original values.
var developmentContinuationCatalog = []Definition{
	{Key: "development_continuation.invalid", SourceTemplate: "Invalid development continuation request", Texts: map[Locale]string{ZhCN: "代码续作请求无效，请核对当前任务和候选来源", EnUS: "Invalid development continuation request"}},
	{Key: "development_continuation.changed", SourceTemplate: "The development continuation source changed; refresh the task and review its candidate", Texts: map[Locale]string{ZhCN: "代码续作来源已变化，请刷新任务并重新核对候选", EnUS: "The development continuation source changed; refresh the task and review its candidate"}},
}
