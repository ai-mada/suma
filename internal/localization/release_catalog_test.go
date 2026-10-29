package localization

import (
	"strings"
	"testing"
	"unicode"
)

// These sources come from the frozen producer inventory, independently of the
// display catalog. Removing a mapping must leave a failing consumer contract.
var releaseDiagnosticSources = []string{
	"计划已执行；查看实际部署，后续变更需要新计划",
	"下载需携带当前测试执行身份",
	"停止或结果未知的执行不能形成成功制品证明",
	"制品证明必须来自同一固定源码，构建及包测试不得改变代码",
	"命令与登记的制品配置不符",
	"实际封存制品与执行回执不一致",
	"构建证明需来自明确构建命令",
	"制品不属于本次构建身份或固定配置",
	"包测试必须使用管理员固定的测试命令",
	"缺少本次测试冻结的制品输入",
	"测试制品、来源代码与冻结输入不一致",
	"原构建证明不存在",
	"未知制品证明种类",
	"构建或包测试当前尝试已变化",
	"缺少可信执行的最终制品回执",
	"制品证明与最终回执不一致",
	"制品输入仅能取自本次测试的明确构建祖先",
	"包测试需要唯一的管理员测试配置",
	"流程定义无效",
	"制品来源必须是流程的静态构建祖先",
	"上游尚无可用构建证明",
	"测试已固定制品，请恢复原解析操作",
	"执行身份不完整",
	"执行租约已变化，不能新增制品或证明",
	"制品构建需绑定管理员登记命令",
	"当前项目制品保留配额为 1 GiB；需要管理员扩展存储策略",
	"本次构建已有上传会话，请恢复原操作",
	"制品存储定位或长度无效",
	"制品文件缺失或已变化",
	"制品文件身份已变化",
	"制品文件长度已变化",
	"制品真实内容摘要不匹配",
	"制品摘要、长度或操作标识无效",
	"收到的实际文件长度或 SHA-256 不匹配",
	"文件不属于已固定的制品清单",
	"上传已封存，不能新增文件",
	"已封存，请恢复原封存操作",
	"文件尚未完整收到",
	"文件长度与清单不一致",
	"封存清单已变化",
	"制品不完整",
	"文件摘要无效",
	"执行节点标识应为 1–100 位字母、数字、点、短横线或下划线",
	"发布命令配置无效",
	"命令需唯一标识、种类及真实配置摘要",
	"执行节点标识已有登记；请使用不同标识，不能覆盖历史凭证",
	"发布凭证仅能以明确版本撤销",
	"发布执行凭证已变化，请刷新",
	"候选范围必须由本次构建和包测试所在运行实际交付并验收",
	"缺少对应版本的正式人工验收",
	"候选构建和包测试依据不一致",
	"候选缺少已验收交付范围",
	"需求、任务、执行或人工验收依据已变化，需要重新建立候选",
	"候选范围摘要不一致",
	"请填写 1–200 字节的候选名称",
	"请选择 1–100 个已验收任务作为交付范围",
	"请选择同一已封存制品及其真实构建、包测试证明",
	"任务版本格式无效",
	"交付范围有重复任务",
	"任务版本已变化，请刷新候选范围",
	"limit 应为 1–100",
	"分页对象无效",
	"分页游标无效或不属于当前项目",
	"发布请求格式错误",
	"该项目尚未登记此发布提供方",
	"该站点已不在管理员当前发布登记中",
	"发布记录已变化，请刷新",
	"环境必须提供名称和管理员登记的提供方/站点",
	"站点未登记",
	"站点已绑定环境，归档也不复用身份",
	"环境版本已变化",
	"环境仍有占用操作，须先核对",
	"已创建环境不能换绑提供方或站点",
	"环境名称或状态无效",
	"计划版本或内容已变化，请刷新后重新审核",
	"此操作的固定审批已失效",
	"部署操作或执行身份无效",
	"此操作已冻结在原执行尝试，必须核对原操作",
	"提供方已受理原操作，请按 operation_id 查询，不重领副作用",
	"发布票据已过期",
	"副作用代数不匹配",
	"授权查询绑定无效",
	"请求 JSON 层级超过128",
	"请求包含重复 JSON 字段",
	"JSON 结构无效",
	"请求 JSON 无效、层级超限或存在重复字段",
	"提供方请求不可读或超限",
	"提供方请求存在未知字段或错误类型",
	"提供方观测与固定操作不匹配",
	"提供方观测摘要不匹配",
	"提供方操作状态无效",
	"必须显式提供实际副作用状态",
	"必须显式提供实际健康状态",
	"提供方错误信息过长",
	"实际 active 指针不匹配本代制品",
	"未开始观测与实际 active 指针矛盾",
	"健康观测不属于本代实际制品",
	"健康检查重复或缺少实际时间",
	"实际 HTTP 内容或状态不匹配固定健康策略",
	"失败健康观测也须绑定实际 active 制品",
	"失败健康观测不完整",
	"失败健康观测不属于固定策略",
	"部署成功需要实际激活、本代 HTTP 健康与持久 fencing ACK",
	"已发生或未知的副作用不能标为取消成功",
	"同观测版本已保存不同内容",
	"终态观测不可覆盖",
	"部署版本已变化",
	"核对管理执行需要发布调度器",
	"核对期间部署记录已变化，请刷新",
	"缺少原执行身份",
	"原执行身份已变化",
	"未配置固定提供方核对连接",
	"首轮核对仅允许管理员固定回环提供方",
	"提供方不可达，原操作保持占用",
	"提供方尚未返回可核对的原操作记录",
	"提供方观测超限或不可读",
	"提供方返回非结构化观测",
	"发布结果缺少真实提供方成功证据",
	"缺少原操作的完整执行证据",
	"发布结果不能代替当前持久提供方观测",
	"提供方尚未证明已知且持久fencing的终态",
	"未知或失败不能提升为成功",
	"失败核对与提供方事实不一致",
	"候选、环境和发布动作无效",
	"环境已归档",
	"环境仍被原操作占用，必须先核对",
	"环境代数已变化",
	"环境代数已到限制",
	"回退必须选择历史成功操作",
	"回退目标必须是同环境真实健康部署的原候选",
	"普通部署不能传回退来源",
	"健康策略要求的文件不在实际封存制品中",
	"必须确认最新计划预览摘要",
	"预览依据已变化，请重新预览",
	"计划摘要损坏",
	"环境策略或绑定已变化",
	"环境代数或占用已变化",
	"环境屏障不属于此操作",
	"候选发布依据已变化",
	"审批结论或理由无效",
	"已派发计划仅能撤销后续授权，不能覆盖历史审批",
	"计划尚无可撤销审批",
	"发布调度器未配置",
	"执行回执不属于此计划",
	"计划已派发，请读取已有部署",
	"回执与管理执行状态不一致",
	"此固定计划尚未批准",
}

func TestReleaseCatalogCoversExactOwnedSources(t *testing.T) {
	if len(releaseDiagnosticSources) != 147 || len(releaseCatalog) != 147 {
		t.Fatalf("release diagnostic coverage changed: sources=%d definitions=%d", len(releaseDiagnosticSources), len(releaseCatalog))
	}
	seen := map[string]bool{}
	for _, source := range releaseDiagnosticSources {
		key, ok := LookupSource(source)
		if !ok || !strings.HasPrefix(key, "release.") || seen[key] {
			t.Errorf("exact release source has no unique semantic key: source=%q key=%q found=%t", source, key, ok)
			continue
		}
		seen[key] = true
		for _, locale := range []Locale{ZhCN, EnUS} {
			rendered, err := WithSource(locale, key, nil, source)
			if err != nil || !rendered.Localized || rendered.Locale != locale || rendered.Key != key || rendered.Fallback != "" || len(rendered.Params) != 0 {
				t.Errorf("release diagnostic not completely rendered: %+v, %v", rendered, err)
			}
			if locale == ZhCN && rendered.Text != source {
				t.Errorf("Chinese diagnostic changed original bytes: got %q want %q", rendered.Text, source)
			}
			if locale == EnUS && (rendered.Text == source || strings.ContainsFunc(rendered.Text, func(r rune) bool { return unicode.Is(unicode.Han, r) })) {
				t.Errorf("English diagnostic is untranslated: %q", rendered.Text)
			}
		}
		for _, changed := range []string{"用户原文：" + source, source + " D:/user/private.txt"} {
			if _, matched := LookupSource(changed); matched {
				t.Errorf("release source matched a prefix, suffix or user evidence: %q", changed)
			}
		}
	}
}

func TestReleaseCatalogKeepsUnknownProviderEvidence(t *testing.T) {
	const source = "提供方原始错误：部署结果未知；D:/customer/私有文件"
	if _, matched := LookupSource(source); matched {
		t.Fatal("unknown provider evidence matched a product source")
	}
	rendered, err := WithSource(EnUS, "legacy.release_provider_error", nil, source)
	if err != nil || rendered.Localized || rendered.Text != source || rendered.Fallback != "source_untranslated" || len(rendered.Params) != 0 {
		t.Fatalf("unknown provider evidence hidden by a generic translation: %+v %v", rendered, err)
	}
}

func TestReleaseCatalogPreservesEvidenceAndAuthorityConstraints(t *testing.T) {
	// Literal expectations protect the distinctions between execution success,
	// human acceptance, fixed source evidence and authority to repeat an effect.
	for _, example := range []struct{ key, source, english string }{
		{"release.proof.unsuccessful_execution", "停止或结果未知的执行不能形成成功制品证明", "Stopped executions or executions with an unknown outcome cannot produce successful artifact proof."},
		{"release.proof.fixed_source_required", "制品证明必须来自同一固定源码，构建及包测试不得改变代码", "Artifact proofs must come from the same fixed source revision; builds and package tests must not change the code."},
		{"release.proof.fixed_test_command_required", "包测试必须使用管理员固定的测试命令", "Package tests must use the test command fixed by an administrator."},
		{"release.artifact.retention_quota_exceeded", "当前项目制品保留配额为 1 GiB；需要管理员扩展存储策略", "The current project artifact retention quota is 1 GiB; an administrator must extend the storage policy."},
		{"release.candidate.actual_delivery_required", "候选范围必须由本次构建和包测试所在运行实际交付并验收", "The candidate scope must have been actually delivered by the run containing this build and package test, with human acceptance for that same run."},
		{"release.candidate.human_acceptance_missing", "缺少对应版本的正式人工验收", "Formal human acceptance for the corresponding version is missing."},
		{"release.worker.identity_already_registered", "执行节点标识已有登记；请使用不同标识，不能覆盖历史凭证", "The worker ID is already registered. Use a different ID; historical credentials cannot be overwritten."},
		{"release.worker.explicit_revocation_version_required", "发布凭证仅能以明确版本撤销", "Release credentials may only be revoked with an explicit version."},
		{"release.authorization.provider_already_accepted", "提供方已受理原操作，请按 operation_id 查询，不重领副作用", "The provider has accepted the original operation. Query it by operation_id; do not request authorization for its side effects again."},
		{"release.observation.success_unproven", "部署成功需要实际激活、本代 HTTP 健康与持久 fencing ACK", "Deployment success requires actual activation, HTTP health for this generation, and a durable fencing ACK."},
		{"release.observation.cancel_unproven", "已发生或未知的副作用不能标为取消成功", "Side effects that have occurred or whose outcome is unknown cannot be marked as successfully cancelled."},
		{"release.provider.unreachable_operation_occupied", "提供方不可达，原操作保持占用", "The provider is unreachable. The environment remains reserved for the original operation."},
		{"release.deployment.known_fenced_terminal_required", "提供方尚未证明已知且持久fencing的终态", "The provider has not yet proved a known terminal state with durable fencing."},
		{"release.deployment.unknown_or_failed_not_success", "未知或失败不能提升为成功", "An unknown or failed outcome cannot be promoted to success."},
		{"release.plan.dispatched_approval_immutable", "已派发计划仅能撤销后续授权，不能覆盖历史审批", "For a dispatched plan, only subsequent authorization may be revoked; historical approval cannot be overwritten."},
		{"release.plan.rollback_basis_invalid", "回退目标必须是同环境真实健康部署的原候选", "The rollback target must be the original candidate of an actual healthy deployment in the same environment."},
	} {
		key, ok := LookupSource(example.source)
		if !ok || key != example.key {
			t.Errorf("precise source lost its stable key: source=%q key=%q want=%q", example.source, key, example.key)
			continue
		}
		rendered, err := WithSource(EnUS, key, nil, example.source)
		if err != nil || !rendered.Localized || rendered.Text != example.english || rendered.Fallback != "" {
			t.Errorf("translation changed a release evidence or authority constraint: key=%s got=%q want=%q err=%v", key, rendered.Text, example.english, err)
		}
	}
}
