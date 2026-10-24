package localization

// Exact full diagnostics keep each validation failure useful in both languages;
// unknown external/user diagnostics still follow source_untranslated fallback.
var preferencesCatalog = []Definition{
	{Key: "preferences.scope_invalid", Texts: map[Locale]string{ZhCN: "语言偏好写入需要唯一且有效的预期账户与组织范围标头", EnUS: "Language preference writes require unique, valid expected account and organization scope headers."}},
	{Key: "preferences.scope_changed", Texts: map[Locale]string{ZhCN: "当前账户或组织已变化，请核对原请求范围后重试", EnUS: "The current account or organization has changed. Verify the original request scope before retrying."}},
	{Key: "preferences.locale_invalid", Texts: map[Locale]string{ZhCN: "语言偏好必须为支持的语言标签，或使用空字符串继承", EnUS: "The locale preference must be a supported language tag, or an empty string to inherit."}},
	{Key: "preferences.locale_type", Texts: map[Locale]string{ZhCN: "语言偏好的 locale 必须为字符串，空字符串表示继承", EnUS: "The locale preference must be a string. An empty string inherits the next default."}},
	{Key: "preferences.version_invalid", Texts: map[Locale]string{ZhCN: "语言偏好的 expected_version 必须为 0–2147483646 的整数", EnUS: "The locale preference expected_version must be an integer from 0 to 2147483646."}},
	{Key: "preferences.operation_invalid", Texts: map[Locale]string{ZhCN: "语言偏好需要 8–128 字节且不含控制字符的 idempotency_key", EnUS: "Locale preferences require an idempotency_key of 8 to 128 bytes without control characters."}},
	{Key: "preferences.body_unreadable", Texts: map[Locale]string{ZhCN: "语言偏好请求无法读取或超过大小限制", EnUS: "The locale preference request could not be read or exceeded the size limit."}},
	{Key: "preferences.json_invalid", Texts: map[Locale]string{ZhCN: "语言偏好请求必须是无重复字段的 JSON 对象", EnUS: "The locale preference request must be a JSON object without duplicate fields."}},
	{Key: "preferences.fields_invalid", Texts: map[Locale]string{ZhCN: "语言偏好请求必须且仅能包含 locale、expected_version 和 idempotency_key", EnUS: "The locale preference request must contain only locale, expected_version, and idempotency_key, with all three required."}},
	{Key: "preferences.idempotency_conflict", Texts: map[Locale]string{ZhCN: "语言偏好操作标识已用于其他内容", EnUS: "This locale preference operation ID was already used for different content."}},
	{Key: "preferences.version_conflict", Texts: map[Locale]string{ZhCN: "语言偏好已变化，请刷新后重试", EnUS: "The locale preferences have changed. Refresh and try again."}},
	{Key: "preferences.method_invalid", Texts: map[Locale]string{ZhCN: "语言偏好接口仅支持 GET 和 PATCH", EnUS: "The locale preference endpoint supports only GET and PATCH."}},
	{Key: "preferences.account_updated", Texts: map[Locale]string{ZhCN: "更新账户语言偏好", EnUS: "Updated account language preference"}},
	{Key: "preferences.organization_updated", Texts: map[Locale]string{ZhCN: "更新组织默认语言", EnUS: "Updated organization default language"}},
}
