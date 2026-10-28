package localization

// Only these exact Go-owned producers may select these display translations.
// Workspace files, tool/model messages and arbitrary error text remain original.
var workspaceOperationCatalog = []Definition{
	{Key: "workspace_operation.coverage", SourceTemplate: "Invalid Go workspace operation coverage", Texts: map[Locale]string{ZhCN: "工作区操作采集范围无效", EnUS: "The workspace operation capture coverage is invalid."}},
	{Key: "workspace_operation.progress", SourceTemplate: "Invalid Go workspace operation progress", Texts: map[Locale]string{ZhCN: "工作区宿主操作记录无效，请核对原执行与事件身份", EnUS: "The workspace host record is invalid. Check its original execution and event identity."}},
	{Key: "workspace_operation.identity", SourceTemplate: "invalid: invalid Go workspace operation identity", Texts: map[Locale]string{ZhCN: "工作区操作身份与原执行不一致", EnUS: "The workspace operation identity does not match the original execution."}},
	{Key: "workspace_operation.profile", SourceTemplate: "invalid: host workspace profile differs from fixed node", Texts: map[Locale]string{ZhCN: "工作区操作的工具档案与固定节点不一致", EnUS: "The workspace operation profile does not match the fixed node."}},
	{Key: "workspace_operation.capacity", SourceTemplate: "invalid: host workspace invocation capacity reached", Texts: map[Locale]string{ZhCN: "本次执行的工作区操作记录已达到上限", EnUS: "Workspace operation records have reached the limit for this execution."}},
}
