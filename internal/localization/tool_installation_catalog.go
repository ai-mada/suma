package localization

var toolInstallationCatalog = []Definition{
	{Key: "validation.tool_installation_scan", Texts: map[Locale]string{ZhCN: "工具安装检测请求无效", EnUS: "The tool installation scan request is invalid."}},
	{Key: "validation.tool_installation_changed", Texts: map[Locale]string{ZhCN: "工具安装检测依据已变化，请核对原请求", EnUS: "The installation scan basis has changed. Reconcile the original request."}},
	{Key: "validation.tool_installation_busy", Texts: map[Locale]string{ZhCN: "执行节点已有未完成的维护操作，请先核对原请求", EnUS: "The worker has an unfinished maintenance operation. Reconcile it first."}},
	{Key: "validation.tool_installation_unavailable", Texts: map[Locale]string{ZhCN: "执行节点尚未声明当前工具安装检测能力", EnUS: "The worker has not declared a current installation scan capability."}},
}
