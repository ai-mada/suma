package localization

// Provider diagnostics are fixed machine identifiers. They never expose an
// OAuth code, response body or credential while retaining readable CN/EN copy.
var workBuddyConnectionsCatalog = []Definition{
	{Key: "workbuddy_connection.invalid", SourceTemplate: "workbuddy_connection_invalid", Texts: map[Locale]string{ZhCN: "WorkBuddy连接请求无效，请检查所选权限和当前连接", EnUS: "The WorkBuddy connection request is invalid. Check the selected scopes and current connection."}},
	{Key: "workbuddy_connection.unavailable", SourceTemplate: "workbuddy_connection_unavailable", Texts: map[Locale]string{ZhCN: "WorkBuddy应用尚未配置或当前不可用，请联系管理员", EnUS: "The WorkBuddy application is unconfigured or unavailable. Contact an administrator."}},
	{Key: "workbuddy_connection.version", SourceTemplate: "workbuddy_connection_version", Texts: map[Locale]string{ZhCN: "WorkBuddy连接已变化，请刷新当前状态后再明确操作", EnUS: "The WorkBuddy connection has changed. Refresh its current state before choosing another action."}},
	{Key: "workbuddy_connection.reauthorize", SourceTemplate: "workbuddy_reauthorization_required", Texts: map[Locale]string{ZhCN: "WorkBuddy连接需要重新授权，请核对账号和权限范围", EnUS: "The WorkBuddy connection needs new authorization. Check the account and scopes."}},
	{Key: "workbuddy_connection.unknown", SourceTemplate: "workbuddy_effect_unknown", Texts: map[Locale]string{ZhCN: "WorkBuddy授权操作结果尚未确认，请查看当前状态；不要重复发起外部操作", EnUS: "The WorkBuddy authorization outcome is unconfirmed. Check its current state; do not repeat the external operation."}},
}
