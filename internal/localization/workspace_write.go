package localization

// Stable diagnostics for current-authorized remote file saves.
var workspaceWriteCatalog = []Definition{
	{Key: "workspace_write.transport_unavailable", SourceTemplate: "Current worker has not declared remote workspace save support", Texts: map[Locale]string{ZhCN: "当前执行器未声明远程工作区保存能力，请更新执行器并刷新工具库存", EnUS: "The current worker has not declared remote workspace save support. Update the runner and refresh tool inventory."}},
	{Key: "workspace_write.invalid", SourceTemplate: "Invalid workspace save request", Texts: map[Locale]string{ZhCN: "执行工作区保存请求无效", EnUS: "Invalid workspace save request"}},
	{Key: "workspace_write.changed", SourceTemplate: "Workspace save command has changed", Texts: map[Locale]string{ZhCN: "保存命令已变化，请核对原操作", EnUS: "Workspace save command has changed"}},
	{Key: "workspace_write.task_changed", SourceTemplate: "Workspace task execution basis has changed", Texts: map[Locale]string{ZhCN: "任务执行依据已变化，请重新审阅计划", EnUS: "Workspace task execution basis has changed"}},
	{Key: "workspace_write.queue_full", SourceTemplate: "Workspace save queue is full", Texts: map[Locale]string{ZhCN: "保存队列已满，请稍后重试", EnUS: "Workspace save queue is full"}},
}
