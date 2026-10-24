package localization

import (
	"strings"
	"testing"
	"unicode"
)

func TestControlCatalogCoversFrozenPublicSources(t *testing.T) {
	want := map[string]bool{}
	for _, source := range frozenControlSources {
		if want[source] {
			t.Fatalf("duplicate frozen source: %q", source)
		}
		want[source] = true
		key, found := LookupSource(source)
		if !found || !strings.HasPrefix(key, "control.") {
			t.Errorf("public control diagnostic is not mapped: %q => %q", source, key)
			continue
		}
		zh, zhErr := Format(ZhCN, key, nil)
		en, enErr := Format(EnUS, key, nil)
		if zhErr != nil || enErr != nil || !zh.Localized || !en.Localized || zh.Text != source || en.Text == "" || strings.ContainsFunc(en.Text, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
			t.Errorf("incomplete exact-source rendering for %q: zh=%+v/%v en=%+v/%v", source, zh, zhErr, en, enErr)
		}
	}
	if len(want) != 143 || len(controlCatalog) != len(want) {
		t.Fatalf("frozen control scope mismatch: sources=%d definitions=%d", len(want), len(controlCatalog))
	}
	keys := map[string]bool{}
	for _, definition := range controlCatalog {
		if keys[definition.Key] || !want[definition.Texts[ZhCN]] || len(definition.Parameters) != 0 {
			t.Errorf("ambiguous or out-of-scope control definition: %+v", definition)
		}
		keys[definition.Key] = true
	}
}

func TestControlCatalogDoesNotInferDiagnosticPrefixesOrSuffixes(t *testing.T) {
	for _, source := range []string{
		"检索服务暂不可用；",
		"标题长度应为 1–500 个字节；用户自己的说明",
		"用户文本：任务完成须经过测试证据与人工验收",
		"执行节点需要独立 Bearer 凭证 external validator detail",
	} {
		if key, found := LookupSource(source); found {
			t.Errorf("partial or user-defined source was classified as %s: %q", key, source)
		}
		out, err := WithSource(EnUS, "control.unknown_external_diagnostic", nil, source)
		if err != nil || out.Text != source || out.Localized || out.Fallback != "source_untranslated" || out.Params != nil {
			t.Errorf("unknown source was generalized or changed: %+v %v", out, err)
		}
	}
}

// Frozen independently from the catalog from the T-P4-009 source inventory.
var frozenControlSources = []string{
	"请输入有效邮箱",
	"密码至少需要 12 个字符",
	"请填写姓名",
	"平台已初始化，请登录",
	"登录内容超出限制",
	"邮箱或密码不正确",
	"基线页最多200条",
	"基线分页游标无效",
	"请选择项目",
	"需求已变更，请检查拆解草稿",
	"请选择 1–100 项任务",
	"任务草稿格式错误",
	"任务草稿 key 重复",
	"任务已变化，需要重新派发预览",
	"前置任务尚未通过验收",
	"任务已变更",
	"请填写验收意见",
	"验收结论应为 accepted 或 rejected",
	"尚无可验收的执行记录",
	"执行尚未成功，不能通过验收",
	"缺少明确测试节点的执行证据",
	"执行证据已变化，请重新审查",
	"执行已进入补偿，不能通过验收",
	"任务已变更，请重新审查",
	"执行依据已失效，需要重新派发",
	"前置任务的验收依据已变化，需要重新派发",
	"需求已变更，旧执行不能通过验收",
	"验收保存冲突",
	"子流程证据链无效",
	"运行或子运行已失败、正在补偿或已补偿，不能复用测试证据",
	"本次已选测试节点必须通过",
	"expected_version 必须为非负整数",
	"组织名称长度应为 1–200 字节",
	"无效组织成员操作",
	"组织角色应为 owner、admin 或 member",
	"组织成员已更新，请刷新后重试",
	"必须保留至少一名组织所有者",
	"请输入不含显示名称的有效邮箱",
	"单次邀请最多包含 100 个项目授权",
	"邀请项目授权角色无效或项目重复",
	"邀请授权项目不属于当前组织",
	"邀请角色应为 admin 或 member；所有权须显式转交",
	"无效项目授权列表",
	"邀请已更新，请刷新后重试",
	"此邀请已使用或已撤销",
	"邀请已更新",
	"姓名应为 1–200 字节，密码应为 12–1024 字节",
	"无效邀请密钥",
	"邀请不存在或密钥无效",
	"邀请邮箱不匹配",
	"邀请已使用、已撤销或已过期；若先前已接受，请使用普通登录",
	"邀请人已无组织管理权限，请重新申请邀请",
	"无效邀请角色",
	"现有账号必须提供正确密码",
	"账号已是组织成员；邀请不会覆盖其角色或密码",
	"邀请已接受，请使用普通登录",
	"邀请已使用",
	"请选择组织",
	"团队已更新，请刷新后重试",
	"新团队的预期版本应为 0",
	"团队名称应为 1–200 字节，描述最多 20000 字节",
	"团队状态应为 active 或 archived",
	"团队已更新",
	"人类团队成员使用组织身份，不需要 project_id",
	"用户不是当前组织的真实成员",
	"代理或工具成员必须指定资源类型和项目",
	"代理或工具资源不存在、类型不符或不属于当前组织项目",
	"人类资源必须绑定真实 user_id",
	"人类资源用户或项目不属于当前组织",
	"人类资源用户尚未获得当前项目权限",
	"请提供 expected_version；首次添加使用 0",
	"无效团队成员操作",
	"归档团队不允许新增或修改成员",
	"团队成员已更新，请刷新后重试",
	"此团队成员已移除或不存在",
	"团队角色应为 lead、member 或 observer",
	"团队成员已更新",
	"请提供 expected_version；首次授权使用 0",
	"无效项目授权操作",
	"项目角色应为 admin、reviewer、developer 或 viewer",
	"只能授权当前组织的真实成员",
	"项目授权已更新，请刷新后重试",
	"项目授权已移除或不存在",
	"请求无法读取或超过大小限制",
	"请求必须是 JSON 对象",
	"请求包含多余 JSON 内容",
	"凭证名称过长",
	"无效凭证范围",
	"流程格式错误",
	"缺少有效操作标识",
	"发布执行必须从发布计划核对或恢复，不能使用通用运行命令",
	"执行节点请求必须使用 POST",
	"执行节点需要独立 Bearer 凭证",
	"执行进度身份无效",
	"工具进度必须是受限事件对象",
	"领取任务必须提供 8–128 字符的稳定 idempotency_key",
	"领取任务必须提供精确项目及实际能力列表",
	"无效执行信封",
	"无效执行回执",
	"该工具尚未授权或未实现",
	"消息目录仅允许读取",
	"工时应为有效的非负数，且不能超过限制",
	"计划已变化，请保留草稿后刷新",
	"计划说明超出限制",
	"无效计划状态",
	"日期必须使用有效的YYYY-MM-DD格式",
	"迭代必须有先后正确的开始与结束日期",
	"里程碑必须指定目标日期",
	"仍有关联任务尚未通过验收，不能完成计划",
	"计划保存冲突",
	"请选择1–200条排期变更",
	"排期格式无效",
	"排期必须包含estimated_hours，可为null",
	"批次包含重复任务",
	"任务已变化，请重新核对排期",
	"任务排期已变化，请保留草稿后刷新",
	"请先重新打开已完成计划，再调整其任务排期",
	"任务排期日期无效",
	"关联迭代或里程碑不存在于当前项目",
	"请先重新打开计划，再调整任务范围",
	"排期并发冲突",
	"计划完成依据已变化，请重新核对",
	"容量周必须是有效的周一日期",
	"请明确指定容量周",
	"请明确指定available_hours；未知时使用null",
	"容量已变化，请刷新后重新调整",
	"检索服务暂不可用；资料原件仍可访问",
	"检索服务尚未通过查询验证",
	"内容超出单项限制",
	"文档正文与描述合计不得超过 1 MiB UTF-8 字节",
	"任务完成须经过测试证据与人工验收",
	"负责人不属于当前项目",
	"负责人资源已停用",
	"关联需求不属于当前项目",
	"资源类型应为 human、agent 或 tool",
	"此账号已在项目中登记，请更新已有资源",
	"发布登记不能替代实际部署验证",
	"任务依赖存在环路",
	"前置任务不存在或不属于当前项目",
	"记录已变化，请保留草稿并重新加载",
	"资源身份与类型不可替换，请登记独立资源",
	"保存冲突",
	"标题长度应为 1–500 个字节",
}
