package localization

import (
	"strings"
	"testing"
)

func TestExecutionCatalogPreservesSpecificDiagnosticMeaning(t *testing.T) {
	for _, example := range []struct{ source, key, english string }{
		{"仅已启用规则可人工触发", "execution.automation.trigger_requires_enabled", "Only enabled rules can be triggered manually."},
		{"固定流程版本不存在或不属于当前项目", "execution.automation.workflow_version_unavailable", "The pinned workflow version does not exist or does not belong to the current project."},
		{"生成的补偿流程只作历史证据，不能作为自动化模板", "execution.automation.compensation_template_forbidden", "Generated compensation workflows are historical evidence only and cannot be used as automation templates."},
		{"secret 必须为 64 位小写十六进制原文", "execution.ingress.secret_invalid", "secret must be the original 64-character lowercase hexadecimal value."},
		{"请显式提供 expected_version、concurrency_limit、resource_limits 及 budget_limit（null 表示不限制）", "execution.scheduling.policy_fields_required", "Explicitly provide expected_version, concurrency_limit, resource_limits, and budget_limit (null means unlimited)."},
		{"任务已经验收；请通过变更需求创建明确的新开发范围", "execution.dispatch.task_already_accepted", "The task has already been accepted. Create an explicit new development scope through a requirement change."},
		{"固定任务与需求上下文超过32000 UTF-8字节；请先精简任务或拆分，不会截断验收条件", "execution.dispatch.context_too_large", "The pinned task and requirement context exceeds 32000 UTF-8 bytes. Simplify or split the task first; acceptance criteria will not be truncated."},
		{"原执行已进入补偿，需要重新交付", "execution.evidence.compensation_requires_redelivery", "The original execution has entered compensation. A new delivery is required."},
		{"资源池制品与生产部署需独立授权协议", "execution.machine.artifacts_deployment_authorization_required", "Pool artifacts and production deployments require a separate authorization protocol."},
		{"持久事件序号超过客户端安全范围", "execution.stream.sequence_out_of_range", "The persisted event sequence exceeds the client-safe integer range."},
	} {
		t.Run(example.key, func(t *testing.T) {
			key, ok := LookupSource(example.source)
			if !ok || key != example.key {
				t.Fatalf("specific execution diagnostic missing: %q => %q, found=%v", example.source, key, ok)
			}
			for locale, want := range map[Locale]string{ZhCN: example.source, EnUS: example.english} {
				got, err := WithSource(locale, key, nil, example.source)
				if err != nil || !got.Localized || got.Fallback != "" || got.Key != example.key || got.Locale != locale || got.Text != want {
					t.Fatalf("diagnostic lost its specific meaning: locale=%s rendered=%+v err=%v", locale, got, err)
				}
			}
			if _, ok := LookupSource(example.source + " 用户工具原输出"); ok {
				t.Fatal("execution source matching consumed external diagnostic content")
			}
		})
	}
}

// This finite list is the independently inventoried set of complete public
// execution diagnostics. New external suffixes must continue to fall back raw.
func TestExecutionCatalogAssignedDiagnosticsHaveExactBilingualMappings(t *testing.T) {
	required := []string{
		"无效自动化请求",
		"规则名称或固定流程版本无效",
		"trigger 必须为 cron 或 manual",
		"misfire 无效",
		"overlap 无效",
		"catch_up_limit 必须为 1..100",
		"人工规则不接受 cron",
		"受保护发布流程不能被自动化引用",
		"生成的补偿流程只作历史证据，不能作为自动化模板",
		"固定流程版本不存在或不属于当前项目",
		"limit 必须为 1..200",
		"无效分页游标",
		"分页游标不属于当前范围",
		"expected_version 必须为正整数",
		"无效自动化命令",
		"仅 revise 必须携带完整 spec",
		"自动化规则版本已变化",
		"已归档规则不能修改或触发",
		"规则已经启用",
		"仅已启用规则可人工触发",
		"无效时间分页游标",
		"无效修订游标",
		"入口名称无效",
		"provider 必须为 github 或 suma",
		"events 必须为 1..8 个白名单事件",
		"events 含无效或重复事件",
		"仓库名称或数字 ID 无效",
		"GitHub 必须固定 owner/repo 和正十进制 repository_id",
		"ref 必须为完整 refs/heads 分支或 refs/tags 标签",
		"conclusions 最多 16 项",
		"conclusions 含无效或重复状态",
		"必须确认当前规则版本和修订",
		"自动化规则版本已变化或已归档",
		"项目最多允许 50 个事件入口",
		"入口命令无效",
		"规则版本确认字段与命令不匹配",
		"仅 revise 必须携带 spec",
		"secret 必须为 64 位小写十六进制原文",
		"仅 rotate_key 接受 secret",
		"入口版本已变化或已撤销",
		"启用需要密钥及当前已启用规则修订",
		"该来源密钥不能重新使用",
		"分页游标不属于当前入口范围",
		"无效修订分页游标",
		"无效交付分页游标",
		"事件签名无效或凭据不可用",
		"公共事件入口不接受会话、查询、来源或编码头",
		"事件必须使用 JSON content-type",
		"事件接收繁忙，请稍后重试",
		"事件正文超过 2 MiB",
		"事件正文不可读",
		"事件 JSON 结构超过解析预算",
		"事件 JSON 字段重复或超过解析预算",
		"事件 JSON 结构无效",
		"事件 JSON 无效、重复或超过解析预算",
		"事件包含多余 JSON 内容",
		"事件受限上下文字段无效",
		"事件 JSON 无效、重复或层级超限",
		"Suma 事件字段或类型无效",
		"GitHub 事件必须是 JSON 对象",
		"GitHub repository.id 无效",
		"GitHub ping 正文结构无效",
		"GitHub push 缺少必要字段",
		"GitHub workflow_run 缺少必要字段",
		"GitHub workflow_run 上下文无效",
		"completed workflow_run 缺少 conclusion",
		"不支持的 GitHub 事件",
		"入口凭据或配置已变化，请重新核对",
		"同一交付标识已用于不同正文",
		"入口每分钟最多接收 120 个新交付身份",
		"自动化触发已被处理",
		"执行容量必须为 0–9007199254740991 的整数，最多配置 64 类资源",
		"资源名称或容量无效；执行槽位及预算请使用独立字段",
		"该预览已提交，请查看当前调度策略",
		"调度预览已过期，请重新预览",
		"调度策略依据已变化，请重新预览",
		"请提供项目及当前策略版本",
		"调度策略已经变更，请刷新后重新预览",
		"请提供项目及当前预览版本",
		"调度预览版本已经变化，请刷新核对",
		"调度预览已由另一操作提交",
		"limit 必须是 1–100 的整数",
		"请显式提供 expected_version、concurrency_limit、resource_limits 及 budget_limit（null 表示不限制）",
		"attempt 必须是正整数",
		"任务已经验收；请通过变更需求创建明确的新开发范围",
		"计划已派发，不能创建另一运行；请打开原运行或恢复原操作",
		"派发预览已过期，请根据当前任务重新预览",
		"任务内容或排期已变化，请重新派发预览",
		"关联需求依据已变化，请重新派发预览",
		"任务的需求关联已变化，请重新派发预览",
		"派发预览需要项目、任务版本、固定流程、开发节点及工具版本",
		"任务已经变更，请刷新后重新预览",
		"请提供当前派发计划版本",
		"派发计划已变化，请刷新并核对",
		"派发计划已被处理",
		"请固定流程版本与完整执行工具配置",
		"任务开发派发需要隔离工作树写入工具；只读工具请通过独立流程运行",
		"任务智能体派发需要已核实的受支持结构化协议；通用命令请使用明确的命令流程",
		"请选择固定流程中的智能体开发节点",
		"流程需要从所选开发节点取得固定候选的独立测试节点",
		"所选工具与流程原有额外执行能力约束不一致",
		"智能体节点缺少固定开发指令",
		"固定任务与需求上下文超过32000 UTF-8字节；请先精简任务或拆分，不会截断验收条件",
		"智能体任务必须先安排工具执行并核对固定任务上下文；直接关联运行不会发送任务内容",
		"任务流程嵌套过深，无法完整核对派发依据",
		"项目、执行节点或工具清单大小无效",
		"工具命令标识不能重复",
		"工具进度与固定档案不匹配",
		"工具执行证据与固定档案不匹配",
		"工具档案状态无效",
		"工具名称、执行节点或项目无效",
		"项目工具档案已达 1000 项限制",
		"工具操作、项目或版本无效",
		"工具档案版本已变化或已撤销",
		"工具档案版本已变化",
		"工具档案依据已变化，请重新预览",
		"工作树证据必须包含明确成功、干净且已提交的固定代码版本",
		"测试必须由执行节点管理员指定，并保持固定候选提交不变",
		"缺少代码或测试的固定工作树证据，不能正式验收",
		"候选代码尚未被本次运行的测试覆盖，请显式交接候选提交后重新测试",
		"任务证据依赖链无效",
		"前置任务验收已失效",
		"前置任务内容与验收依据不一致",
		"前置需求版本已变化",
		"前置任务依赖版本已变化",
		"任务验收并发变化",
		"前置验收缺少有效执行依据",
		"执行证据不再有效",
		"原执行已进入补偿，需要重新交付",
		"执行证据引用超出限制",
		"limit 应为 1–200",
		"分页游标无效",
		"分页游标无效或不属于当前列表",
		"资源池机器更新必须使用 POST",
		"机器请求超过 2 MiB 或不可读",
		"机器请求字段无效",
		"机器请求包含额外内容",
		"结果与固定派发身份不一致",
		"资源池制品与生产部署需独立授权协议",
		"该资源池机器协议尚未提供此独立能力",
		"机器回写缺少一致的固定租户、项目和派发范围",
		"工具清单操作标识或大小无效",
		"工具清单数据无效",
		"工具清单命令重复",
		"工具清单原操作内容已变化",
		"工具进度与本项目固定档案不匹配",
		"工具结果与本项目固定档案不匹配",
		"事件游标必须为非负安全整数",
		"事件游标不可重复",
		"after 与 Last-Event-ID 冲突",
		"查询参数格式错误",
		"事件连接只接受 project_id 和 after",
		"项目参数不可重复",
		"事件游标不属于当前运行",
		"持久事件序号超过客户端安全范围",
		"实时连接已达上限，请稍后重连",
		"当前连接不支持实时事件",
		"当前连接不支持有界实时事件写入",
	}
	for _, source := range required {
		key, ok := LookupSource(source)
		if !ok || !strings.HasPrefix(key, "execution.") {
			t.Errorf("assigned complete execution diagnostic has no owned key: %q => %q", source, key)
			continue
		}
		for _, locale := range []Locale{ZhCN, EnUS} {
			got, err := WithSource(locale, key, nil, source)
			if err != nil || !got.Localized || got.Fallback != "" || strings.TrimSpace(got.Text) == "" || locale == ZhCN && got.Text != source || locale == EnUS && got.Text == source {
				t.Errorf("assigned diagnostic incomplete: source=%q locale=%s got=%+v err=%v", source, locale, got, err)
			}
		}
		if _, ok := LookupSource(source + " external tool output"); ok {
			t.Errorf("source matched an external suffix: %q", source)
		}
	}
}
