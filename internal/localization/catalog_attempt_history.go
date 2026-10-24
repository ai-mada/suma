package localization

var attemptHistoryCatalog = []Definition{
	{Key: "attempt_history.raw_encoding", SourceTemplate: "selected tool event is not valid UTF-8", Texts: map[Locale]string{ZhCN: "选中的工具事件原文不是有效的 UTF-8 文本", EnUS: "The selected tool event raw text is not valid UTF-8."}},
	{Key: "attempt_history.query", SourceTemplate: "invalid: invalid attempt history query", Texts: map[Locale]string{ZhCN: "尝试历史查询参数无效", EnUS: "The attempt history query is invalid."}},
	{Key: "attempt_history.cursor", SourceTemplate: "invalid: invalid attempt history cursor", Texts: map[Locale]string{ZhCN: "尝试历史游标无效或不属于此尝试", EnUS: "The attempt history cursor is invalid or belongs to another attempt."}},
	{Key: "attempt_history.identity", SourceTemplate: "invalid durable attempt identity", Texts: map[Locale]string{ZhCN: "持久尝试或事件标识无效", EnUS: "The persisted attempt or event identity is invalid."}},
	{Key: "attempt_history.event_size", SourceTemplate: "selected tool event exceeds 2 MiB", Texts: map[Locale]string{ZhCN: "选中的工具事件超过读取大小上限", EnUS: "The selected tool event exceeds the read size limit."}},
	{Key: "attempt_history.receipt", SourceTemplate: "saved runner receipt does not match durable attempt identity", Texts: map[Locale]string{ZhCN: "保存的执行回执与持久尝试标识不符", EnUS: "The saved runner receipt does not match the persisted attempt identity."}},
	{Key: "attempt_history.http_query", SourceTemplate: "工具历史请求参数无效", Texts: map[Locale]string{ZhCN: "工具历史请求参数无效", EnUS: "The attempt history query is invalid."}},
}
