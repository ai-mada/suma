package localization

// 键表示稳定的显示语义。相同 HTTP code 可以对应多个具体消息；翻译不
// 改变机器状态、代码、用户内容或任何持久化业务字段。
var catalog = append(append(append(commonCatalog, poolCatalog...), preferencesCatalog...), workspaceCatalog...)

var commonCatalog = []Definition{
	{Key: "common.forbidden", Texts: map[Locale]string{ZhCN: "当前身份没有此操作权限", EnUS: "Your current identity does not have permission for this operation."}},
	{Key: "common.not_found", Texts: map[Locale]string{ZhCN: "记录不存在或不可访问", EnUS: "The record does not exist or is not accessible."}},
	{Key: "common.service_unavailable", Texts: map[Locale]string{ZhCN: "服务暂时无法完成操作", EnUS: "The service cannot complete this operation right now."}},
	{Key: "common.invalid_input", Texts: map[Locale]string{ZhCN: "输入无效", EnUS: "The input is invalid."}},
	{Key: "common.version_conflict", Texts: map[Locale]string{ZhCN: "数据已变化，请刷新后重试", EnUS: "The data has changed. Refresh and try again."}},
	{Key: "workflow.not_found", Texts: map[Locale]string{ZhCN: "运行或流程不存在", EnUS: "The run or workflow does not exist."}},
	{Key: "auth.sign_in_required", Texts: map[Locale]string{ZhCN: "请登录", EnUS: "Please sign in."}},
	{Key: "auth.session_expired", Texts: map[Locale]string{ZhCN: "登录已失效", EnUS: "Your sign-in session has expired."}},
	{Key: "http.origin_rejected", Texts: map[Locale]string{ZhCN: "请求来源不在允许范围", EnUS: "The request origin is not allowed."}},
	{Key: "pool.bearer_required", Texts: map[Locale]string{ZhCN: "资源池执行节点需要独立 Bearer 凭证", EnUS: "A pool runner requires its own Bearer credential."}},
	{Key: "pool.paused", Texts: map[Locale]string{ZhCN: "资源池已暂停，停止新派发", EnUS: "The execution pool is paused. New dispatches are stopped."}},
	{Key: "validation.max_length", Parameters: []Parameter{{Name: "field", Kind: Identifier}, {Name: "limit", Kind: Integer}}, Texts: map[Locale]string{ZhCN: "{{.field}} 的长度不能超过 {{.limit}}", EnUS: "{{.field}} must not exceed {{.limit}} characters."}},
}
var sourceAliases = map[string]string{"登录已过期": "auth.session_expired"}
var codes = map[string]string{
	"forbidden": "common.forbidden", "not_found": "common.not_found", "internal_error": "common.service_unavailable",
	"unauthenticated": "auth.sign_in_required", "invalid_input": "common.invalid_input", "version_conflict": "common.version_conflict",
}
