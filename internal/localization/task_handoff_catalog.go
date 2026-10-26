package localization

// Only host-owned complete diagnostics have aliases. Notes, decision reasons,
// IDs and immutable source evidence stay in their original language.
var taskHandoffCatalog = []Definition{
	{Key: "task_handoff.basis", SourceTemplate: "Task handoff basis has changed; create a new proposal", Texts: map[Locale]string{ZhCN: "任务交接依据已变化，请重新发起交接", EnUS: "Task handoff basis has changed; create a new proposal"}},
	{Key: "task_handoff.run_unsafe", SourceTemplate: "The linked execution is active or has an unknown outcome", Texts: map[Locale]string{ZhCN: "关联执行仍在进行或结果未知，请先核对原执行", EnUS: "The linked execution is active or has an unknown outcome"}},
	{Key: "task_handoff.recipient", SourceTemplate: "The handoff recipient is not an active human with project write access", Texts: map[Locale]string{ZhCN: "接收人必须是已启用且有项目写权限的真实人员资源", EnUS: "The handoff recipient is not an active human with project write access"}},
	{Key: "task_handoff.team", SourceTemplate: "The recipient is not an active member or lead of the selected team", Texts: map[Locale]string{ZhCN: "接收人必须是所选团队的有效成员或负责人", EnUS: "The recipient is not an active member or lead of the selected team"}},
	{Key: "task_handoff.pending", SourceTemplate: "This task already has a pending handoff", Texts: map[Locale]string{ZhCN: "此任务已有待接收的交接，请先处理原交接", EnUS: "This task already has a pending handoff"}},
	{Key: "task_handoff.permission", SourceTemplate: "Only the designated recipient or original sender may perform this handoff action", Texts: map[Locale]string{ZhCN: "此交接操作只允许指定接收人或原发起人执行", EnUS: "Only the designated recipient or original sender may perform this handoff action"}},
	{Key: "task_handoff.version", SourceTemplate: "The handoff has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "交接已变化，请刷新后再决定", EnUS: "The handoff has changed; reload before deciding"}},
	{Key: "task_handoff.acknowledge", SourceTemplate: "Confirm that changing the owner invalidates previous execution and acceptance evidence", Texts: map[Locale]string{ZhCN: "请确认负责人变更将使旧执行和验收依据失效", EnUS: "Confirm that changing the owner invalidates previous execution and acceptance evidence"}},
	{Key: "task_handoff.same_owner", SourceTemplate: "The recipient is already the task owner", Texts: map[Locale]string{ZhCN: "接收人已经是当前任务负责人", EnUS: "The recipient is already the task owner"}},
	{Key: "task_handoff.request", SourceTemplate: "Invalid task handoff request", Texts: map[Locale]string{ZhCN: "任务交接请求无效", EnUS: "Invalid task handoff request"}},
	{Key: "task_handoff.command", SourceTemplate: "Invalid task handoff command", Texts: map[Locale]string{ZhCN: "任务交接操作无效", EnUS: "Invalid task handoff command"}},
	{Key: "task_handoff.page", SourceTemplate: "Invalid task handoff page", Texts: map[Locale]string{ZhCN: "任务交接分页参数无效", EnUS: "Invalid task handoff page"}},
	{Key: "task_handoff.cursor", SourceTemplate: "Invalid task handoff cursor", Texts: map[Locale]string{ZhCN: "任务交接游标无效或不属于当前任务", EnUS: "Invalid task handoff cursor"}},
	{Key: "task_handoff.query", Texts: map[Locale]string{ZhCN: "交接筛选或分页参数无效", EnUS: "The handoff filters or pagination parameters are invalid."}},
	{Key: "task_handoff.fields", Texts: map[Locale]string{ZhCN: "交接请求缺少完整且有效的字段", EnUS: "The handoff request is missing complete, valid fields."}},
	{Key: "task_handoff.scope", Texts: map[Locale]string{ZhCN: "交接请求的项目范围不一致", EnUS: "The handoff request has conflicting project scopes."}},
}
