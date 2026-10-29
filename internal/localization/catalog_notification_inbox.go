package localization

var notificationInboxCatalog = []Definition{
	{Key: "notification_inbox.invalid", SourceTemplate: "Invalid notification inbox request or cursor", Texts: map[Locale]string{ZhCN: "收件箱请求或分页依据无效，请核对过滤、版本和原操作标识", EnUS: "The inbox request or cursor is invalid. Check the filter, version and original operation ID."}},
	{Key: "notification_inbox.changed", SourceTemplate: "The notification has changed; reload before deciding", Texts: map[Locale]string{ZhCN: "通知状态已变化，请重新读取后明确操作", EnUS: "The notification changed. Reload it before deciding."}},
	{Key: "notification_inbox.source_changed", SourceTemplate: "The task assignment source does not match the saved task", Texts: map[Locale]string{ZhCN: "任务分配来源与已保存任务不一致，本次变更未提交", EnUS: "The task assignment does not match the saved task. This change was not committed."}},
	{Key: "notification_inbox.sequence", SourceTemplate: "The notification sequence is invalid or exhausted", Texts: map[Locale]string{ZhCN: "通知序号无效或已达到上限，请检查原始数据", EnUS: "The notification sequence is invalid or exhausted. Check the source data."}},
	{Key: "notification_inbox.source_invalid", SourceTemplate: "The notification source is invalid", Texts: map[Locale]string{ZhCN: "通知来源数据无效，请检查原始事件后重试", EnUS: "The notification source is invalid. Check the original event and retry."}},
}
