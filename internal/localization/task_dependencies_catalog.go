package localization

// These diagnostics belong to the atomic business task graph API. Match full
// owned source messages only; titles, logs and user text are never translated.
var taskDependenciesCatalog = []Definition{
	{Key: "task_dependencies.request", SourceTemplate: "Invalid task dependency change request", Texts: map[Locale]string{ZhCN: "任务依赖修改请求无效，请核对项目、任务版本和前置任务", EnUS: "The task dependency change request is invalid. Review the project, task versions and predecessors."}},
	{Key: "task_dependencies.query", SourceTemplate: "Invalid task dependency graph query", Texts: map[Locale]string{ZhCN: "任务依赖查询无效，请提供当前项目", EnUS: "The task dependency graph query is invalid. Provide the current project."}},
	{Key: "task_dependencies.duplicate_task", SourceTemplate: "Task dependency changes contain a duplicate task", Texts: map[Locale]string{ZhCN: "修改清单包含重复任务，每个任务只能提交一次", EnUS: "The change list contains a duplicate task. Submit each task only once."}},
	{Key: "task_dependencies.duplicate_dependency", SourceTemplate: "Task dependencies contain a duplicate task", Texts: map[Locale]string{ZhCN: "前置任务重复，请移除重复项后保存", EnUS: "The predecessor list contains a duplicate task. Remove the duplicate before saving."}},
	{Key: "task_dependencies.unchanged", SourceTemplate: "Task dependency changes must modify each selected task", Texts: map[Locale]string{ZhCN: "修改清单只能包含依赖实际变化的任务，调整顺序不算依赖变更", EnUS: "Include only tasks whose dependencies have changed. Reordering the same predecessors is not a dependency change."}},
	{Key: "task_dependencies.self", SourceTemplate: "Task cannot depend on itself", Texts: map[Locale]string{ZhCN: "任务不能依赖自身", EnUS: "A task cannot depend on itself."}},
	{Key: "task_dependencies.reference", SourceTemplate: "Predecessor task does not exist in the current project", Texts: map[Locale]string{ZhCN: "前置任务不属于当前项目或已不存在，请核对依赖", EnUS: "The predecessor does not exist in the current project. Review the dependency."}},
	{Key: "task_dependencies.cycle", SourceTemplate: "Task dependencies contain a cycle", Texts: map[Locale]string{ZhCN: "任务依赖形成循环，请调整前置关系后保存", EnUS: "The task dependencies contain a cycle. Adjust the predecessor relationships before saving."}},
	{Key: "task_dependencies.changed", SourceTemplate: "Task dependencies changed; keep the draft and reload", Texts: map[Locale]string{ZhCN: "任务依赖或版本已变化，请保留草稿并重新加载后核对", EnUS: "The task dependencies or versions have changed. Keep the draft and reload to review the changes."}},
	{Key: "task_dependencies.version_exhausted", SourceTemplate: "Task version limit reached; review the task before changing dependencies", Texts: map[Locale]string{ZhCN: "任务版本已达上限，请核查任务后再修改依赖", EnUS: "The task has reached its version limit. Review the task before changing dependencies."}},
}
