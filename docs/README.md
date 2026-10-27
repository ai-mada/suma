# 速码项目设计文档

状态：当前T-P4-041／1.0.0，Codex模型配置、真实独立工作树编码、原进度／结果与partial未定价账本取得限定本地证据；中英固定预览与刷新已验证。完整P0–P7、需求1–22继续实施。负责人：平台集成；最终范围见[交付状态](delivery/status.md)。

| 主题 | 入口 |
|---|---|
| Codex模型到真实编码（当前T-P4-041） | [PRD](product/codex-model-execution.md)／[合同](../contracts/codex-app-server.md)／[架构与AI SOP](integrations/codex-app-server.md)／[交互](design/codex-model-execution.md)／[English](en/codex-app-server.md)／[限定验证](quality/codex-app-server-verification.md)／[摘要](quality/codex-app-server-summary.json) |
| 专家团队逐任务模型执行（历史T-P4-040） | [合同](../contracts/expert-team-model-execution.md)／[架构与AI SOP](architecture/expert-team-model-execution.md)／[English](en/expert-team-model-execution.md)／[限定验证](quality/expert-team-model-execution-verification.md)／[摘要](quality/expert-team-model-execution-summary.json) |
| 历史执行器能力与预览交付（T-P4-039） | [合同](../contracts/workspace-transport-capabilities.md)／[架构与AI SOP](integrations/workspace-transport-capabilities.md)／[English](en/workspace-transport-capabilities.md)／[限定验证](quality/workspace-capability-verification.md)／[摘要](quality/workspace-capability-summary.json) |
| 历史执行文件编辑、CAS保存与原操作恢复（T-P4-038） | [产品交互](product/remote-workspace-write.md)／[合同](../contracts/workspace-write.md)／[架构](architecture/remote-workspace-write.md)／[AI SOP](operations/remote-workspace-write.md)／[English](en/remote-workspace-write.md)／[实际验证](quality/workspace-write-verification.md)／[摘要](quality/workspace-write-summary.json) |
| 执行工作区只读观察与出站恢复（历史T-P4-037） | [产品与交互](product/remote-workspace.md)／[合同](../contracts/workspace-observation.md)／[架构](architecture/remote-workspace-broker.md)／[AI SOP](operations/remote-workspace.md)／[English](en/remote-workspace.md)／[实际证据](quality/workspace-observation-verification.md)／[摘要](quality/workspace-observation-summary.json) |
| 固定原生模型选择、attempt预算与真实执行（历史 T-P4-036，UI固定预览已验证） | [PRD](product/model-execution.md)／[架构与实现检查点](architecture/model-execution.md)／[AI SOP](operations/model-execution.md)／[English](en/model-execution.md)／[实际验证及剩余边界](quality/model-execution-verification.md)；团队逐任务原生选择见040，外部CLI、修复入口和完整报价／模型授权仍待实施 |
| 开发工具真实只读MCP诊断（T-P4-036） | [developer check操作与边界](integrations/developer-connection-check.md)／[限定证据](quality/developer-connection036-verification.md)；[远程工作区出站broker](architecture/remote-workspace-broker.md)已在037/038限定实现，完整远程IDE继续实施 |
| 只读Git审阅、Monaco保存快捷键与原生执行限额（历史 T-P4-035） | [PRD](product/workspace-git.md)／[架构](architecture/workspace-git.md)／[交互](design/workspace-git.md)／[接口与限额合同](../contracts/workspace-git.md)／[限定本地验证](quality/workspace-git-verification.md) |
| 独立模型选择与计价的历史设计输入（T-P4-035） | [035模型部署、版本绑定与费率设计](architecture/035-model-selection-design.md)；当前限定原生实现见036，独立模型授权／完整报价目录等历史设计中的后续项不据此标记完成 |
| 当前工具推荐与IDE保存恢复（历史 T-P4-034） | [PRD](product/task-dispatch-options.md)／[架构](architecture/intelligent-capability-selection-notes.md)／[AI SOP](operations/task-dispatch-options.md)／[交互](design/task-dispatch-options.md)／[合同](../contracts/task-dispatch-options.md)／[验证](quality/task-dispatch-options-verification.md)／[摘要](quality/task-dispatch-options-summary.json)／[English](en/task-dispatch-options.md)／[逐平台差距](product/platform-capability-delivery-matrix.md) |
| 修复计划修订与运行溯源（历史 T-P4-033） | [PRD](product/candidate-repair-revisions.md)／[架构](architecture/candidate-repair-revisions.md)／[AI SOP](operations/candidate-repair-revisions.md)／[交互](design/candidate-repair-revisions.md)／[合同](../contracts/candidate-repair-revisions.md)／[CLI](../contracts/candidate-repair-revisions-cli.md)／[限定验证](quality/candidate-repair-revisions-verification.md)／[摘要](quality/candidate-repair-revisions-summary.json)／[English](en/candidate-repair-revisions.md) |
| 新增平台参考与智能选择／IDE缺口 | [OpenWorkBuddy、Octop固定官方基线](integrations/development-platform-reference-extensions.md)；许可、来源能力、当前实现与未完成项分别记录 |
| 冲突修复与受控恢复（历史 T-P4-032） | [PRD](product/candidate-conflict-repair.md)／[架构与AI SOP](architecture/candidate-conflict-repair.md)／[交互](design/candidate-conflict-repair.md)／[合同](../contracts/candidate-conflict-repair.md)／[限定验证](quality/candidate-conflict-repair-verification.md)／[摘要](quality/candidate-conflict-repair-summary.json)／[English](en/candidate-conflict-repair.md) |
| 固定失败证据的智能分诊（历史 T-P4-030） | [PRD](product/failure-diagnosis.md)、[架构与 AI SOP](architecture/failure-diagnosis.md)、[交互](design/failure-diagnosis.md)、[合同](../contracts/failure-diagnosis.md)、[CLI](../contracts/failure-diagnosis-cli.md)、[限定验证](quality/failure-diagnosis-verification.md)、[摘要](quality/failure-diagnosis-summary.json)、[English](en/failure-diagnosis.md) |
| 多源候选合成与下游代码基线（历史 T-P4-031） | [PRD](product/candidate-composition.md)、[架构与AI SOP](architecture/candidate-composition.md)、[交互](design/candidate-composition.md)、[合同](../contracts/candidate-composition.md)、[CLI](../contracts/candidate-composition-cli.md)、[限定验证](quality/candidate-composition-verification.md)、[摘要](quality/candidate-composition-summary.json)、[English](en/candidate-composition.md) |
| 专家团队依赖编排（T-P4-029 历史） | [PRD](product/expert-team-orchestration.md)、[架构与AI SOP](architecture/expert-team-orchestration.md)、[交互](design/expert-team-orchestration.md)、[合同](../contracts/expert-team-orchestration.md)、[CLI](../contracts/expert-team-orchestration-cli.md)、[限定验证](quality/expert-team-orchestration-verification.md)、[English](en/expert-team-orchestration.md) |
| 受管代码集成（T-P4-028） | [PRD](product/repository-integration.md)、[架构／AI SOP](architecture/repository-integration.md)、[交互](design/repository-integration.md)、[部署](deployment/repository-integration.md)、[合同](../contracts/repository-integration.md)、[限定验证](quality/repository-integration-verification.md) |
| 项目专家配置与团队派发（T-P4-027） | [PRD／交互](product/expert-team-dispatch.md)、[架构与AI SOP](architecture/expert-team-dispatch.md)、[合同](../contracts/expert-team-dispatch.md)、[限定验证](quality/expert-team-dispatch-verification.md) |
| 模型辅助知识提炼（T-P4-026） | [PRD／交互](product/knowledge-intelligence.md)、[架构与AI SOP](architecture/knowledge-intelligence.md)、[合同](../contracts/knowledge-intelligence.md)、[限定验证](quality/knowledge-intelligence-verification.md) |
| 组织共享技能目录（T-P4-025） | [PRD／交互](product/skill-catalog.md)、[架构与AI SOP](architecture/skill-catalog.md)、[正式合同](../contracts/skill-catalog.md)、[限定验证](quality/skill-catalog-verification.md) |
| Token、费用估算与执行额度（T-P4-024） | [架构／交互与AI SOP](architecture/usage-accounting.md)、[正式合同](../contracts/usage-accounting.md) |
| 模型规划与站内风险建议（T-P4-023） | [架构／交互与AI SOP](architecture/intelligent-planning.md)、[接口合同](../contracts/intelligent-planning.md) |
| 跨节点候选代码交接（T-P4-018，限定本地验证） | [PRD](product/candidate-transfer.md)、[领域架构与SOP](architecture/candidate-transfer.md)、[正式合同](../contracts/candidate-transfer.md)、[实际验收](quality/candidate-transfer-verification.md) |
| 任务设计依据、项目CLI与安全升级（T-P4-017） | [PRD](product/task-design-provenance.md)、[架构与SOP](architecture/task-design-provenance.md)、[读取合同](../contracts/task-design-context.md)、[CLI](api/cli-project-context.md)、[升级手册](deployment/preview-upgrade.md)、[验证](quality/task-design-context-verification.md) |
| 需求设计与任务采用（T-P4-016，限定本地验证） | [PRD](product/requirement-design.md)、[架构与SOP](architecture/requirement-design.md)、[正式合同](../contracts/requirement-design.md)、[阶段验证](quality/requirement-design-verification.md) |
| 项目知识／记忆／规范／技能（T-P4-015，限定本地验证） | [产品需求](product/project-knowledge.md)、[领域架构](architecture/project-knowledge.md)、[固定上下文合同](../contracts/project-knowledge.md)、[阶段验证](quality/project-knowledge-verification.md) |
| 当前凭证与原页面身份（T-P4-014，限定本地验证） | [统一合同](../contracts/current-credential-authority.md)、[领域架构](architecture/current-credential-authority.md)、[并行实施计划](delivery/current-authority-plan.md)、[验证边界](quality/current-authority-verification.md) |
| 当前工具资格显示与图控件（T-P4-013，限定范围本地已验证） | [显示合同](../contracts/platform-display.md)、[交互设计](design/platform-display.md)、[专项计划](delivery/platform-display-plan.md)、[验证边界](quality/platform-display-verification.md)、[精简摘要](quality/platform-display-summary.json) |
| 精确尝试工具历史（T-P4-012，历史本地已验证） | [架构与操作SOP](architecture/attempt-tool-history.md)、[读取合同](../contracts/attempt-tool-history.md)、[专项验证](quality/attempt-tool-history-verification.md)、[精简摘要](quality/attempt-tool-history-summary.json) |
| 原生诊断与命令编辑（T-P4-011历史） | [链路设计与SOP](agents/native-diagnostics.md)、[专项验收](quality/native-diagnostics-verification.md)、[精简摘要](quality/native-diagnostics-summary.json) |
| 执行诊断与运行命令国际化（T-P4-010历史） | [架构与边界](architecture/runtime-localization.md)、[专项验收](quality/runtime-localization-verification.md)、[精简摘要](quality/runtime-localization-summary.json) |
| 产品范围、角色和原则 | [产品需求](product/prd.md) |
| 可复用平台与专业工程 | [基础PRD](product/platform-foundation-prd.md)、[领域／SDK／扩展架构](architecture/platform-engineering.md)、[AI与人员开发SOP](delivery/ai-engineering-sop.md) |
| 完整能力与验收范围 | [能力矩阵](product/capability-matrix.md) |
| 可操作的完整旅程 | [用户旅程](product/user-journeys.md) |
| 实现与待办 | [实施计划](delivery/implementation-plan.md)、[交付状态](delivery/status.md) |
| 系统、部署与信任边界 | [架构总览](architecture/overview.md) |
| C4 与关键流程时序 | [可维护 Mermaid 源图](architecture/diagrams.md) |
| 领域归属与数据结构 | [领域模型](architecture/domain-model.md)、[事务存储](data/transactional-store.md) |
| 调度、恢复、资源和回执 | [DAG 内核](scheduler/engine.md)、[可靠性](scheduler/reliability.md)、[候选代码交接](scheduler/candidate-handoff.md) |
| 增量调度与项目准入 | [增量内核与迁移](scheduler/incremental-kernel.md)、[交互规范](design/scheduling-workspace.md)、[用量与领取](agents/scheduling-usage.md)、[阶段验收](quality/scheduling-verification.md)、[平台门禁验证](quality/scheduling-platform-guards.md)、[当前运行包与旧数据复核](deployment/scheduling-runtime-bundles.md) |
| 共享资源与跨项目执行 | [领域架构](architecture/execution-pools.md)、[治理与路由交互](design/execution-pools.md)、[独立Runner](agents/execution-pool-runner.md)、[阶段验收](quality/execution-pool-verification.md)、[当前运行程序](deployment/execution-pool-runtime-bundles.md) |
| 原件、多模态、索引与同步 | [数据平面](data/multimodal.md)、[同步协议](data/synchronization.md) |
| 真实制品、发布授权与恢复 | [制品存储](data/release-artifacts.md)、[发布证据架构](architecture/release-evidence.md)、[发布工作台](design/release-workspace.md) |
| 本机发布与执行提供方 | [发布控制面](deployment/release-control.md)、[提供方协议](integrations/release-provider.md)、[四程序运行包](deployment/release-runtime-bundles.md) |
| 原生智能体与开发工具 | [智能体运行时](agents/runtime.md)、[工具接入](integrations/tools.md) |
| 原生实际编码与机器封存 | [PRD](product/native-coding.md)、[架构与AI SOP](agents/native-coding.md)、[交互规范](design/native-coding.md)、[实现合同](../contracts/native-coding.md)、[阶段验收](quality/native-coding-verification.md) |
| 团队多工具接入 | [PRD](product/team-cli-runtime.md)、[集成与AI SOP](integrations/team-cli-runtime.md)、[合同](../contracts/team-cli-runtime.md)、[English](en/team-cli-runtime.md) |
| 项目 MCP 与受控草稿 | [PRD](product/project-mcp.md)、[统一合同](../contracts/application-tools-mcp.md)、[接入与AI SOP](integrations/project-mcp.md)、[交互与国际化](design/project-mcp.md)、[阶段验证](quality/project-mcp-verification.md)、[生成工具Schema](api/mcp-tools.json) |
| 工具配置与固定任务派发 | [执行架构](architecture/tool-execution.md)、[工具工作台](design/tool-workspace.md)、[本机工具运行时](agents/tool-execution-runtime.md)、[实现合同](../contracts/tool-execution.md)、[本机验收](quality/tool-execution-verification.md)、[当前四程序](deployment/tool-stop-runtime-bundles.md) |
| 定时自动化与事件续传 | [自动化与事件架构](architecture/automation-events.md)、[自动化工作台](design/automation-workspace.md)、[本机验收](quality/automation-verification.md)、[当前四程序交付](deployment/automation-runtime-bundles.md) |
| 签名来源与交付 | [信任与事务](architecture/automation-ingress.md)、[入口工作台](design/event-ingress-workspace.md)、[GitHub/CI/监控接入](integrations/automation-ingress.md)、[精确合同](../contracts/automation-ingress.md)、[本机验收](quality/ingress-verification.md)、[当前运行包](deployment/ingress-runtime-bundles.md) |
| 产品交互与流程图 | [工作台](design/workbench.md)、[DAG 三视图](design/dag-interactions.md) |
| 全平台多语言与本地化 | [国际化规范](design/internationalization.md)、[账户与组织偏好](design/locale-preferences.md)、[偏好接口合同](../contracts/locale-preferences.md)、[偏好验收](quality/locale-preferences-verification.md)、[自有工作台迁移验收](quality/remaining-localization-verification.md)、[自有工作台摘要](quality/remaining-localization-summary.json)、[受控诊断架构](architecture/localized-diagnostics.md)、[后端诊断验收](quality/backend-diagnostics-verification.md)、[当前工具历史摘要](quality/attempt-tool-history-summary.json) |
| English documentation | [English entry and translation status](en/README.md) |
| 组织治理、项目计划与候选测试 | [组织、团队与权限](product/governance.md)、[规划与治理工作台](design/planning-governance.md) |
| HTTP 合同 | [API 说明](api/README.md)、[OpenAPI](api/openapi.json) |
| 桌面、服务器与安装 | [桌面](deployment/desktop.md)、[服务器](deployment/server.md)、[容器与集群模板](deployment/containers.md) |
| 可复现构建与本地缓存 | [构建说明](deployment/build-reproducibility.md)、[六平台 Go 运行时](deployment/runtime-bundles.md) |
| 命令行与终端 | [CLI 与 Bubble Tea](integrations/cli.md) |
| 文件保存、分页与检索一致性 | [实现合同](data/file-and-query-consistency.md) |
| 验证与运维 | [验证报告](quality/verification.md)、[验收矩阵](quality/acceptance.md)、[运维手册](operations/runbook.md) |
| 实际运行与交付摘要 | [证据索引](quality/evidence/README.md) |
| 架构取舍、参考依据 | [ADR](adr/README.md)、[参考锁定](integrations/references.json) |
| 第三方依赖与许可盘点 | [第三方清单](delivery/third-party.md) |

## 文档状态

“目标”表达完整产品合同；“已实现”必须有源码入口；“已验证”必须有当前环境证据。外部账户、OceanBase 集群、支付、媒体服务与跨平台包分别验收。历史结果不自动继承为新代码的通过结论。

035当前范围为本地只读Git状态、准确索引／工作树文本、Monaco保存快捷键与原生执行硬限额；远程IDE、PTY／LSP／DAP、正式可写模型选择及计价仍待实施。独立模型选择文档仅记录设计，不作为运行或生产验收证据。

历史031已完成多源候选合成、精确结果独立测试、明确下游代码基线与后续开发闭环，原来源和准确测试分别记录。主预览保留180原表schema及全部行→183。仅本地自有协议夹具验证；未调用厂商或模型，未报告用量不推断为零Token。真实远端、多机、生产验收、完整P0–P7及全部扩展需求继续实施。此前030固定失败分诊、029依赖编排和更早发布／运维保留各自历史证据；完整设计、工作日历、生产CI、S3、分布式数据库、容量与跨平台验收仍以实施台账为准。

## 维护方式

T-P4-012 已完成限定范围本地验证并提交：精确 attempt 的持久工具事实分页保留原始 SQL 正文、固定读取窗口和当前权限边界；实际原生 HTTP429 与334条合成协议事实分别记录，数据库保持和停止已验证。T-P4-013 已在此基础上完成当前工具资格的来源绑定显示、官方图控件本地化及确认的原始名称入口修正：最终代码检查、独立复核、实际工具生命周期和最终图四宽度／主题已记录限定本地验证。精确切面及剩余范围见[专项验证](quality/platform-display-verification.md)。两轮均不代表厂商模型效果、全表面国际化、分布式规模或完整平台完成。

T-P4-011 原生诊断贯通、此前自动化、制品与发布验证保留各自历史源码和证据切面。目录键数、一次成功构建或局部响应式检查都不代表所有语言、平台、规模与完整 P0–P7 已完成，范围以各专项报告和实施台账为准。

接口修改先更新共享合同和对应验证；业务门禁变更同步状态机、用户旅程、错误恢复文案和测试。运行截图、HTTP 记录和大规模性能报告统一进入项目交付证据目录；日常临时文件使用 `.cache/`。

本轮团队 CLI 的[限定验证](quality/team-cli-verification.md)与[机器摘要](quality/team-cli-summary.json)记录实际覆盖和后续完整范围。

团队任务交接：[PRD](product/task-handoff.md)、[架构与 AI SOP](architecture/task-handoff.md)、[冻结合同](../contracts/task-handoff.md)。

团队交接：[English guide](en/task-handoff.md)、[限定验证](quality/task-handoff-verification.md)、[机器摘要](quality/task-handoff-summary.json)。

当前产品版本与最新完整需求：[1.0.0 设计基线](product/v1-baseline.md)。
