# UI 原始值插入合同

状态：T-P4-013 同类问题修复冻结基线；限定实现、独立复核、完整网页门禁及实际图交互已有[本地验证证据](../docs/quality/platform-display-verification.md)。负责人：工作台／主集成。关联：FND-I18N-001、`platform-display.md`。完整平台范围不变。

## 目标与边界

用户名称、节点名称、版本及标识符作为显示数据逐字保留，不作为翻译模板重新解释。显示处理不修改记录、工作流、权限、选项值、命令正文、操作回执或网络请求。沿用已安装 i18next 的公开资源读取接口及现有语言选择，不改变全局 `tr`、i18next 配置、复数、富文本或后端来源证明逻辑。

新增一个可复用且有封闭键目录的纯显示模块，接入下列已复现入口，并复用现有 Handle 修复。只能读取这些自有前端静态模板；不能成为任意后端诊断键或 `display.tool.*` 的旁路。

| 键 | 必需参数 |
|---|---|
| `automation.fixed_rule_basis` | `name, revision` |
| `development.candidate.source_option` | `name, id, type` |
| `development.candidate.build_option` | `name, id, command` |
| `collaboration.capacity_resource_week` | `name, week` |
| `release.prepare.environment_option` | `name, archived, pending` |
| `development.dispatch.tool_option` | `name, worker, version, status` |
| `development.workflow.graph_node_aria` | `name, id, state` |
| `development.workflow.upstream_node` | `name, id` |
| `flow.handle.input`、`flow.handle.output` | 各自 `name, id` |

## 确定语义

- 参数键及类型由同一静态合同约束；调用端保留原有日期、数字、状态格式化，空后缀 `archived`／`pending` 是合法字符串。
- 从当前语言的受控静态资源取得模板，只对模板本身进行一次占位符替换；回调返回的原始值不得再次被扫描、嵌套翻译、解释为替换元字符或 HTML。
- 名称含 `{{id}}`、其他参数名、`$t(...)`、`$&`、`$$`、引号、标签或 Unicode 时，全部原字符保留；末尾真正的模板参数仍必须正确插入。
- 资源缺失、类型错误或模板参数不符合目录时，诚实显示原始值，不丢弃名称／ID，不渲染任意服务器成功说明。Handle 的通用回退保持已有合同；资源恢复后同一组件可重显完整用途。
- 不执行结果缓存、网络、状态更新或输入对象修改。语言变化不丢失现有焦点、选择、未提交文本、脏草稿或原件。

## 必须提供的证据

1. 保留已有精确失败、修复及完整测试切面；本次以相同 16 个双语数据样例验证新 API，并通过真实消费者及批准键调用点门禁证明 8 个入口迁移后的 RED→GREEN。原直接调用全局 `tr` 的独立校准测试保持原字节及 RED，不能用测试别名／facade 把它替换成新 API 后冒称通过。
2. 两语言的参数形状、顺序、空后缀、缺失／恢复资源、非法键／参数，以及恶意形状原文均有测试；通过真实 Workflow、Candidate／Artifact、Capacity 等组件核对消费者。
3. 与库存、授权、候选范围、表单值、命令及派发权限相关代码保持原语义。编译器限制 ID 字符的 3 个候选和原有单参数／顺序负例不冒称已确认缺陷。
4. 最终完整网页测试、类型、目录门禁、构建、独立审查和真实图界面分别验收。组件测试、直接测试数据、真实服务、厂商执行和生产验收分别说明。

这是已确认 raw-name 家族的修复，不证明全部字符串字段、富文本、第三方控件、完整国际化或 P0–P7 已完成；其他内容继续按完整实施计划核验。
