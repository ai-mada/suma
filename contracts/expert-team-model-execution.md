# 专家团队逐项模型与预算合同

状态：T-P4-040／1.0.0，限定本机实现。负责人：专家计划与模型域、工作台、CLI。关联需求9、12、14、18、21、22；扩展[团队派发](expert-team-dispatch.md)、[依赖编排](expert-team-orchestration.md)及[原生模型执行](model-execution.md)。完整平台范围保持不变。

## 输入与固定依据

`ExpertTeamPlanItemRequest` 接受可选 `model_execution_selection`（`modelcontract.Selection`）或 `codex_execution_selection`（`codexcontract.Selection`），二者互斥；显式 `null` 和同时出现两个字段均拒绝。Create.items和显式revise.items使用相同DTO。一个职责的不同任务可以选择不同模型、参数与本次额度或提醒阈值；已发布Role配置、版本和批准语义不变，没有隐藏的Role默认模型。

当前批准的原生模型工具要求显式选择。普通CLI或不支持的工具收到独立选择仍拒绝，不回退模板或环境。参数与普通任务一致：1–16次调用、累计1–32768输出Token；预算沿用封闭整数微货币、当前模型版本、部署摘要、运营方费率和组织／项目策略。不是整个团队的共享预算。

批准的 Codex app-server 工具逐项显式提交真实当前目录摘要、模型、推理强度及可选 `token_notice_limit`。服务端经同一个派发构建器冻结 `items[].plan.codex_execution` 和执行节点的相同 Binding，不接受客户端提供配置摘要、Binding、提供方、密钥、进程参数或预算模式。此绑定为 `observe_only`／`configuration_ack_only`，提醒不是原生硬额度，不承诺提供方身份或费用。组织或项目任一 enforce 策略都阻止该入口；不静默降为观察模式。

服务端固定内容通过 `items[].plan.model_execution` 及执行节点相同绑定公开，不增加重复的模型字段、表、路由或记账系统。Create只建立预览，不启动模型、预留执行槽位或接受任务。显式修订仅作用于未开始项，保留旧来源修订；已开始项的准确模型、代码和运行不改变。

## 等待、派发与结算

待依赖项在后续Start／Resume／Tick时重新核验当前部署、工具、任务、来源、路由与原激活凭证；复用原冻结Binding及准确历史费率ID／版本，不把后来报价偷偷替换进去。新预览或显式来源修订才重新选择当前报价。缺部署、权限撤回或证据失效会阻止派发，不换模型重跑。

Codex 的只读资格、Start／Resume／Tick 与真正 Claim 同样检查当前授权 worker 的目录、配置和预算模式。等待期间保持原模型、推理强度、提醒和 Binding 摘要；目录／配置变化、撤回模型或原激活凭证失效会阻断后续派发。显式修订仅替换未开始项，已启动项保持原模型与运行。Create／Command／Start 的恢复回执保留原 Codex Binding、原来源修订及原运行；当前 head 已变化时原回执没有操作资格。

模型绑定薄适配位于[模型域](../internal/platform/model_execution_source.go)，普通builder保留统一执行语义。真正Claim仍走同一原子预留，Runner及原生Runtime验证固定配置并回写真实请求、响应和用量；不完整用量或未知效果沿原操作核对。模型估算、厂商账单、代码测试、人工验收和发布是不同事实。

## 界面与CLI

工作台先按已选职责提供一份共享选择，允许单个任务明确覆盖；发请求时每项都有自己的显式Selection。准确project／task集合／role／tool版本及SHA变化即失效，不能借用旧选择。未定价不能填写费用承诺，错误与不可读选项保持阻断。来源修订用同样控件；只读历史显示当时Binding。

Create、Start和Command的成功体须先通过原计划／任务／模型选择对应检查，才确认持久请求。错误200保持完整原key、参数和预算，可恢复原操作；不因换界面或后续输入改变原请求。

CLI沿现有 `suma team-plan create|command --file … --confirm`，每项JSON可带上述选择；仍使用原版本、确认和稳定操作ID。输入拒绝私有字段、显式空选择、双选择、错类型、超界参数、重复键和歧义数值，服务端再次检查真实当前依据。不确定响应后由用户显式使用原 JSON 与操作 ID 恢复，不自动重试或替换选择。

[架构与AI SOP](../docs/architecture/expert-team-model-execution.md)、[实际验证](../docs/quality/expert-team-model-execution-verification.md)。本次专家 Codex 检查使用独立新库、授权工具目录与类型化协议证据，未调用模型、安装版 CLI 或厂商。其他外部CLI独立模型参数、模型专属权限和完整报价目录继续实施，不据此称所有平台能力完成。

原Create／Command／Start回执恢复保留原固定计划、任务、模型和配置快照；仍按当前项目写权限读取。当前head已改变时，原快照禁止Start和其他动作，界面另取当前head；不能用后续修订替代原请求结果。Start并发恢复遵循同一规则。
