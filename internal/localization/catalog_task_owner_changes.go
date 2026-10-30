package localization

// These fixed source messages contain no task, identity or credential payload.
// User-owned task names and directory facts remain untranslated source values.
var taskOwnerChangesCatalog = []Definition{
	{Key: "task_owner_changes.invalid", SourceTemplate: "Invalid task owner change request", Texts: map[Locale]string{ZhCN: "任务负责人变更请求无效，请核对所选任务、版本和确认内容", EnUS: "The task owner change request is invalid. Check the tasks, versions and confirmation."}},
	{Key: "task_owner_changes.query", SourceTemplate: "Invalid task owner target query or cursor", Texts: map[Locale]string{ZhCN: "负责人查询或分页依据无效，请重新读取人员目录", EnUS: "The owner directory query or cursor is invalid. Read the directory again."}},
	{Key: "task_owner_changes.changed", SourceTemplate: "Selected tasks changed; keep the selection and preview again", Texts: map[Locale]string{ZhCN: "所选任务已变化，请保留选择并明确重新预览", EnUS: "The selected tasks changed. Keep the selection and explicitly preview again."}},
	{Key: "task_owner_changes.target_unavailable", SourceTemplate: "The selected canonical human owner is no longer available", Texts: map[Locale]string{ZhCN: "所选人员当前已不符合条件，请重新选择有效人员", EnUS: "The selected person is no longer eligible. Choose a currently available person."}},
	{Key: "task_owner_changes.target_changed", SourceTemplate: "The selected owner resource changed; reload the people directory", Texts: map[Locale]string{ZhCN: "所选人员资源已变化，请重新读取人员目录", EnUS: "The selected person resource changed. Read the people directory again."}},
	{Key: "task_owner_changes.preview_changed", SourceTemplate: "The task owner change preview changed; preview again before confirming", Texts: map[Locale]string{ZhCN: "负责人变更预览依据已变化，请重新预览后再确认", EnUS: "The owner change preview basis changed. Preview again before confirming."}},
	{Key: "task_owner_changes.unchanged", SourceTemplate: "The selected tasks already have this owner", Texts: map[Locale]string{ZhCN: "所选任务的负责人均未变化，无需提交", EnUS: "Every selected task already has this owner. There is no change to apply."}},
	{Key: "task_owner_changes.version_exhausted", SourceTemplate: "Task version limit reached; review the task before changing its owner", Texts: map[Locale]string{ZhCN: "任务版本已达上限，请先核查任务后再变更负责人", EnUS: "The task version limit was reached. Review the task before changing its owner."}},
	{Key: "records.version_exhausted", SourceTemplate: "Record version limit reached; review the record before changing it", Texts: map[Locale]string{ZhCN: "记录版本已达上限，请先核查记录后再变更", EnUS: "The record version limit was reached. Review the record before changing it."}},
}
