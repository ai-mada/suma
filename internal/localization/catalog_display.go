package localization

// Display identities require an explicit producer binding; these definitions
// never register reverse aliases for arbitrary stored or user text.
var displayCatalog = []Definition{
	{Key: "display.tool.profile_revoked", SourceTemplate: "工具档案已永久撤销", Texts: map[Locale]string{ZhCN: "工具档案已永久撤销", EnUS: "The tool profile has been permanently revoked."}},
	{Key: "display.tool.profile_paused", SourceTemplate: "工具档案已暂停，尚未授权派发", Texts: map[Locale]string{ZhCN: "工具档案已暂停，尚未授权派发", EnUS: "The tool profile is paused and is not authorized for dispatch."}},
	{Key: "display.tool.activation_revoked", SourceTemplate: "工具档案启用人的当前审批权限已失效", Texts: map[Locale]string{ZhCN: "工具档案启用人的当前审批权限已失效", EnUS: "The person who enabled the tool profile no longer has approval authority."}},
	{Key: "display.tool.worker_expired", SourceTemplate: "执行节点凭证已过期或撤销", Texts: map[Locale]string{ZhCN: "执行节点凭证已过期或撤销", EnUS: "The runner credential has expired or been revoked."}},
	{Key: "display.tool.worker_creator_revoked", SourceTemplate: "执行节点登记人的当前项目权限已失效", Texts: map[Locale]string{ZhCN: "执行节点登记人的当前项目权限已失效", EnUS: "The person who registered the runner no longer has project permission."}},
	{Key: "display.tool.worker_unregistered", SourceTemplate: "执行节点未登记、未获项目授权或机器凭证已撤销", Texts: map[Locale]string{ZhCN: "执行节点未登记、未获项目授权或机器凭证已撤销", EnUS: "The runner is unregistered, lacks project authorization, or its machine credential has been revoked."}},
	{Key: "display.tool.pool_paused", SourceTemplate: "资源池已暂停，停止新派发", Texts: map[Locale]string{ZhCN: "资源池已暂停，停止新派发", EnUS: "The execution pool is paused. New dispatches are stopped."}},
	{Key: "display.tool.pool_unavailable", SourceTemplate: "资源池当前状态不允许新派发", Texts: map[Locale]string{ZhCN: "资源池当前状态不允许新派发", EnUS: "The current execution pool state does not allow new dispatches."}},
	{Key: "display.tool.pool_expired", SourceTemplate: "资源池执行节点凭证已过期", Texts: map[Locale]string{ZhCN: "资源池执行节点凭证已过期", EnUS: "The execution pool runner credential has expired."}},
	{Key: "display.tool.inventory_missing", SourceTemplate: "执行节点尚未上报工具清单", Texts: map[Locale]string{ZhCN: "执行节点尚未上报工具清单", EnUS: "The runner has not reported its tool inventory."}},
	{Key: "display.tool.inventory_stale", SourceTemplate: "工具清单已过期，等待执行节点重新探测", Texts: map[Locale]string{ZhCN: "工具清单已过期，等待执行节点重新探测", EnUS: "The tool inventory has expired. Waiting for the runner to probe again."}},
	{Key: "display.tool.command_missing", SourceTemplate: "当前执行节点未声明此命令", Texts: map[Locale]string{ZhCN: "当前执行节点未声明此命令", EnUS: "The current runner has not declared this command."}},
	{Key: "display.tool.profile_mismatch", SourceTemplate: "当前工具配置、版本或程序摘要与固定档案不匹配", Texts: map[Locale]string{ZhCN: "当前工具配置、版本或程序摘要与固定档案不匹配", EnUS: "The current tool configuration, version, or executable digest does not match the pinned profile."}},
	{Key: "display.tool.reported_unavailable", SourceTemplate: "执行节点报告工具不可用：{{.reason}}", Parameters: []Parameter{{Name: "reason", Kind: Text}}, Texts: map[Locale]string{ZhCN: "执行节点报告工具不可用：{{.reason}}", EnUS: "The runner reported the tool unavailable: {{.reason}}"}},
	{Key: "display.tool.dispatchable", SourceTemplate: "执行节点已报告可领取；模型认证、执行与验收结果需分别核验", Texts: map[Locale]string{ZhCN: "执行节点已报告可领取；模型认证、执行与验收结果需分别核验", EnUS: "The runner reported it can claim work; model authentication, execution, and acceptance results require separate verification."}},
}
