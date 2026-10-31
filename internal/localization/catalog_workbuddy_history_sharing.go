package localization

// Sharing request errors have stable protocol identifiers. Only their safe
// presentation is translated; selected source text remains literal user data.
var workBuddyHistorySharingCatalog = []Definition{
	{Key: "workbuddy_history_sharing.invalid", SourceTemplate: "workbuddy_cloud_history_share_invalid", Texts: map[Locale]string{ZhCN: "共享请求无效，请核对所选记录、目标任务与内容上限", EnUS: "The sharing request is invalid. Check the selected records, target task and content limits."}},
	{Key: "workbuddy_history_sharing.changed", SourceTemplate: "workbuddy_cloud_history_share_changed", Texts: map[Locale]string{ZhCN: "来源、任务或预览依据已变化，请重新预览；未确认的原操作仍须沿原请求核对", EnUS: "The source, task or preview has changed. Preview again; reconcile any unconfirmed original operation with its original request."}},
}
