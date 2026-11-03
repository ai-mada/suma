package localization

var toolPreparationCatalog = []Definition{
	{Key: "validation.tool_preparation_invalid", Texts: map[Locale]string{ZhCN: "工具配置准备请求无效", EnUS: "The tool preparation request is invalid."}},
	{Key: "validation.tool_preparation_changed", Texts: map[Locale]string{ZhCN: "工具准备依据已变化，请核对原操作", EnUS: "The preparation basis changed. Reconcile the original operation."}},
	{Key: "validation.tool_preparation_busy", Texts: map[Locale]string{ZhCN: "此执行节点有未完成的维护操作，请先核对", EnUS: "This worker has unfinished maintenance. Reconcile it first."}},
	{Key: "validation.tool_preparation_unavailable", Texts: map[Locale]string{ZhCN: "此节点尚未允许当前工具的远程配置准备", EnUS: "This worker has not allowed remote configuration preparation for this tool."}},
	{Key: "validation.tool_preparation_limit", Texts: map[Locale]string{ZhCN: "项目工具准备操作已达数量限制", EnUS: "The project tool preparation limit has been reached."}},
}
