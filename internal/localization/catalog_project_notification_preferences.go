package localization

// Only these reviewed API producers become source aliases. Preference payloads
// and saved receipt values stay literal; translating them would change identity.
var projectNotificationPreferencesCatalog = []Definition{
	{Key: "project_notification_preferences.invalid", SourceTemplate: "Invalid project notification preferences request", Texts: map[Locale]string{ZhCN: "项目通知设置请求无效，请核对项目、版本及原操作标识", EnUS: "The project notification preferences request is invalid. Check the project, version and original operation ID."}},
	{Key: "project_notification_preferences.changed", SourceTemplate: "The project notification preferences changed; reload before deciding", Texts: map[Locale]string{ZhCN: "项目通知设置已变化，请重新读取后明确操作；未确认的原操作仍须核对", EnUS: "The project notification preferences changed. Reload before deciding; reconcile any unconfirmed original operation."}},
}
