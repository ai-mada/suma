package localization

// Stable messages for the read-only outbound workspace broker.
var workspaceObservationCatalog = []Definition{
	{Key: "workspace_observation.invalid", SourceTemplate: "Invalid workspace observation request", Texts: map[Locale]string{ZhCN: "执行工作区读取请求无效", EnUS: "Invalid workspace observation request"}},
	{Key: "workspace_observation.unavailable", SourceTemplate: "The exact workspace attempt is no longer available", Texts: map[Locale]string{ZhCN: "指定的工作区执行尝试已不可用", EnUS: "The exact workspace attempt is no longer available"}},
	{Key: "workspace_observation.lease_changed", SourceTemplate: "Workspace observation lease has changed or expired", Texts: map[Locale]string{ZhCN: "工作区读取租约已变化或过期", EnUS: "Workspace observation lease has changed or expired"}},
	{Key: "workspace_observation.caller_changed", SourceTemplate: "Workspace observation belongs to its original current caller", Texts: map[Locale]string{ZhCN: "读取请求只属于仍有效的原始调用身份", EnUS: "Workspace observation belongs to its original current caller"}},
	{Key: "workspace_observation.operation_changed", SourceTemplate: "Workspace observation operation content changed", Texts: map[Locale]string{ZhCN: "工作区读取操作内容已变化，请核对原请求", EnUS: "Workspace observation operation content changed"}},
	{Key: "workspace_observation.queue_full", SourceTemplate: "Workspace observation queue is full", Texts: map[Locale]string{ZhCN: "工作区读取队列已满，请稍后重试", EnUS: "Workspace observation queue is full"}},
}
