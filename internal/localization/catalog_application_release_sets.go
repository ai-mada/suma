package localization

// Exact release-set diagnostics only; user content and original evidence stay unchanged.
var applicationReleaseSetCatalog = []Definition{
	{Key: "release_set.invalid", SourceTemplate: "Invalid application release set request", Texts: map[Locale]string{ZhCN: "应用版本集请求无效", EnUS: "Invalid application release set request"}},
	{Key: "release_set.changed", SourceTemplate: "The application release set or reviewed deployment basis has changed", Texts: map[Locale]string{ZhCN: "应用版本集或已审阅的部署依据已变化，请刷新后重新确认", EnUS: "The application release set or reviewed deployment basis has changed"}},
	{Key: "release_set.blocked", SourceTemplate: "Complete the exact application release set evidence before preparing deployment", Texts: map[Locale]string{ZhCN: "请补齐应用版本集的准确来源与质量依据，再准备部署计划", EnUS: "Complete the exact application release set evidence before preparing deployment"}},
	{Key: "release_set.reference", SourceTemplate: "invalid application release-set plan binding", Texts: map[Locale]string{ZhCN: "应用版本集的计划绑定无效", EnUS: "invalid application release-set plan binding"}},
}
