# 速码 1.0.0 文档索引

当前只有 **1.0.0** 产品基线，完整平台仍在实现与验收中。入口按领域组织，当前能力由交付状态统一维护；专项设计、合同、英文说明、验证报告与精简摘要保留各自范围，不把历史阶段编号当成产品版本或当前完成状态。

| 当前入口 | 内容 |
| --- | --- |
| [产品与设计基线](product/v1-baseline.md) | 单一新库初始化、版本边界、完整需求1–25及工程约束 |
| [实施计划](delivery/implementation-plan.md) | P0–P7完整范围、限定实现与未完成项 |
| [交付状态](delivery/status.md) | 当前能力、实际证据、限制与后续交付 |
| [验证入口](quality/verification.md) | 当前变更检查与各专项证据 |

## 产品、范围与体验

| 领域 | 设计、合同与验证 |
| --- | --- |
| 产品范围、角色和原则 | [产品需求](product/prd.md) |
| 可复用平台与专业工程 | [基础PRD](product/platform-foundation-prd.md)、[领域／SDK／扩展架构](architecture/platform-engineering.md)、[AI与人员开发SOP](delivery/ai-engineering-sop.md) |
| 完整能力与验收范围 | [能力矩阵](product/capability-matrix.md) |
| 可操作的完整旅程 | [用户旅程](product/user-journeys.md) |
| 实现与待办 | [实施计划](delivery/implementation-plan.md)、[交付状态](delivery/status.md) |
| 产品交互与流程图 | [工作台](design/workbench.md)、[DAG 三视图](design/dag-interactions.md) |
| 全平台多语言与本地化 | [国际化规范](design/internationalization.md)、[账户与组织偏好](design/locale-preferences.md)、[偏好接口合同](../contracts/locale-preferences.md)、[偏好验收](quality/locale-preferences-verification.md)、[自有工作台迁移验收](quality/remaining-localization-verification.md)、[自有工作台摘要](quality/remaining-localization-summary.json)、[受控诊断架构](architecture/localized-diagnostics.md)、[后端诊断验收](quality/backend-diagnostics-verification.md)、[当前工具历史摘要](quality/attempt-tool-history-summary.json) |
| English documentation | [English entry and translation status](en/README.md) |
| 应用级迭代与问题全生命周期 | [需求24／25领域及验收基线](product/application-delivery.md)，待实现范围与现有局部基础分开记录 |
| 组织治理、项目计划与候选测试 | [组织、团队与权限](product/governance.md)、[规划与治理工作台](design/planning-governance.md) |

当前外部工具执行：[OpenCode 固定 SDK、架构与 AI SOP](architecture/opencode-execution.md)、[合同](../contracts/opencode-sdk.md)、[英文指南](en/opencode-execution.md)、[限定验证](quality/opencode-actual-execution-verification.md)。本地协议与厂商运行证据分别记录。

## 架构、身份与数据

| 领域 | 设计、合同与验证 |
| --- | --- |
| 当前凭证与原页面身份 | [统一合同](../contracts/current-credential-authority.md)、[领域架构](architecture/current-credential-authority.md)、[并行实施计划](delivery/current-authority-plan.md)、[验证边界](quality/current-authority-verification.md) |
| 系统、部署与信任边界 | [架构总览](architecture/overview.md) |
| C4 与关键流程时序 | [可维护 Mermaid 源图](architecture/diagrams.md) |
| 领域归属与数据结构 | [领域模型](architecture/domain-model.md)、[事务存储](data/transactional-store.md) |
| 原件、多模态、索引与同步 | [数据平面](data/multimodal.md)、[同步协议](data/synchronization.md) |
| HTTP 合同 | [API 说明](api/README.md)、[OpenAPI](api/openapi.json) |
| 文件保存、分页与检索一致性 | [实现合同](data/file-and-query-consistency.md) |

## 需求、知识与技能

| 领域 | 设计、合同与验证 |
| --- | --- |
| 模型辅助知识提炼 | [PRD／交互](product/knowledge-intelligence.md)、[架构与AI SOP](architecture/knowledge-intelligence.md)、[合同](../contracts/knowledge-intelligence.md)、[限定验证](quality/knowledge-intelligence-verification.md) |
| 组织共享技能目录 | [PRD／交互](product/skill-catalog.md)、[架构与AI SOP](architecture/skill-catalog.md)、[正式合同](../contracts/skill-catalog.md)、[限定验证](quality/skill-catalog-verification.md) |
| 任务设计依据与项目 CLI | [PRD](product/task-design-provenance.md)、[架构与SOP](architecture/task-design-provenance.md)、[读取合同](../contracts/task-design-context.md)、[CLI](api/cli-project-context.md)、[历史预览交付记录（追溯）](deployment/preview-upgrade.md)、[验证](quality/task-design-context-verification.md) |
| 需求设计与任务采用 | [PRD](product/requirement-design.md)、[架构与SOP](architecture/requirement-design.md)、[正式合同](../contracts/requirement-design.md)、[阶段验证](quality/requirement-design-verification.md) |
| 项目知识／记忆／规范／技能 | [产品需求](product/project-knowledge.md)、[领域架构](architecture/project-knowledge.md)、[固定上下文合同](../contracts/project-knowledge.md)、[阶段验证](quality/project-knowledge-verification.md) |

## 调度、资源与事件

| 领域 | 设计、合同与验证 |
| --- | --- |
| 调度、恢复、资源和回执 | [DAG 内核](scheduler/engine.md)、[可靠性](scheduler/reliability.md)、[候选代码交接](scheduler/candidate-handoff.md) |
| 增量调度与项目准入 | [增量内核](scheduler/incremental-kernel.md)、[交互规范](design/scheduling-workspace.md)、[用量与领取](agents/scheduling-usage.md)、[阶段验收](quality/scheduling-verification.md)、[平台门禁验证](quality/scheduling-platform-guards.md)、[运行包与原数据证据](deployment/scheduling-runtime-bundles.md) |
| 共享资源与跨项目执行 | [领域架构](architecture/execution-pools.md)、[治理与路由交互](design/execution-pools.md)、[独立Runner](agents/execution-pool-runner.md)、[阶段验收](quality/execution-pool-verification.md)、[当前运行程序](deployment/execution-pool-runtime-bundles.md) |
| 定时自动化与事件续传 | [自动化与事件架构](architecture/automation-events.md)、[自动化工作台](design/automation-workspace.md)、[本机验收](quality/automation-verification.md)、[当前四程序交付](deployment/automation-runtime-bundles.md) |
| 签名来源与交付 | [信任与事务](architecture/automation-ingress.md)、[入口工作台](design/event-ingress-workspace.md)、[GitHub/CI/监控接入](integrations/automation-ingress.md)、[精确合同](../contracts/automation-ingress.md)、[本机验收](quality/ingress-verification.md)、[当前运行包](deployment/ingress-runtime-bundles.md) |

## 模型、工具与开发工作区

| 领域 | 设计、合同与验证 |
| --- | --- |
| 模型选择跨普通／专家／修复入口 | [产品操作](product/model-selection-workflows.md)／[架构与AI SOP](architecture/model-selection-workflows.md)／[修复合同](../contracts/repair-model-execution.md)／[English](en/model-selection-workflows.md)／[限定验证](quality/model-selection-workflows-verification.md)／[摘要](quality/model-selection-workflows-summary.json) |
| Codex模型到真实编码 | [PRD](product/codex-model-execution.md)／[合同](../contracts/codex-app-server.md)／[架构与AI SOP](integrations/codex-app-server.md)／[交互](design/codex-model-execution.md)／[English](en/codex-app-server.md)／[限定验证](quality/codex-app-server-verification.md)／[摘要](quality/codex-app-server-summary.json) |
| 专家团队逐任务模型执行 | [合同](../contracts/expert-team-model-execution.md)／[架构与AI SOP](architecture/expert-team-model-execution.md)／[English](en/expert-team-model-execution.md)／[限定验证](quality/expert-team-model-execution-verification.md)／[摘要](quality/expert-team-model-execution-summary.json) |
| 电脑本地开发工作台 | [完整架构与AI SOP](architecture/local-development-workbench.md)、[原生目录合同](../contracts/desktop-workspace.md)、[当前部署](deployment/desktop.md)、[English](en/local-development-workbench.md)、[限定验证](quality/desktop-workspace-verification.md) |
| 本地 IDE 目录浏览 | [接口合同](../contracts/workspace-tree.md)、[架构与 AI SOP](architecture/workspace-directory-browser.md)、[English](en/workspace-directory-browser.md)、[限定验证](quality/workspace-directory-verification.md) |
| Codex活动工作区读取 | [架构与AI SOP](architecture/codex-workspace-observation.md)／[交互](design/codex-workspace-observation.md)／[English](en/codex-workspace-observation.md)／[限定验证](quality/codex-workspace-observation-verification.md)／[摘要](quality/codex-workspace-observation-summary.json) |
| 执行器传输能力 | [合同](../contracts/workspace-transport-capabilities.md)／[架构与AI SOP](integrations/workspace-transport-capabilities.md)／[English](en/workspace-transport-capabilities.md)／[限定验证](quality/workspace-capability-verification.md)／[摘要](quality/workspace-capability-summary.json) |
| 执行文件编辑、CAS保存与原操作恢复 | [产品交互](product/remote-workspace-write.md)／[合同](../contracts/workspace-write.md)／[架构](architecture/remote-workspace-write.md)／[AI SOP](operations/remote-workspace-write.md)／[English](en/remote-workspace-write.md)／[实际验证](quality/workspace-write-verification.md)／[摘要](quality/workspace-write-summary.json) |
| 执行工作区只读观察与出站恢复 | [产品与交互](product/remote-workspace.md)／[合同](../contracts/workspace-observation.md)／[架构](architecture/remote-workspace-broker.md)／[AI SOP](operations/remote-workspace.md)／[English](en/remote-workspace.md)／[实际证据](quality/workspace-observation-verification.md)／[摘要](quality/workspace-observation-summary.json) |
| 固定原生模型选择、attempt预算与真实执行 | [PRD](product/model-execution.md)／[架构与实现检查点](architecture/model-execution.md)／[AI SOP](operations/model-execution.md)／[English](en/model-execution.md)／[实际验证及剩余边界](quality/model-execution-verification.md) |
| 开发工具真实只读MCP诊断 | [developer check操作与边界](integrations/developer-connection-check.md)／[限定证据](quality/developer-connection036-verification.md) |
| 只读Git审阅、Monaco保存快捷键与原生执行限额 | [PRD](product/workspace-git.md)／[架构](architecture/workspace-git.md)／[交互](design/workspace-git.md)／[接口与限额合同](../contracts/workspace-git.md)／[限定本地验证](quality/workspace-git-verification.md) |
| 独立模型选择与计价的设计输入（追溯） | [模型部署、版本绑定与费率设计](architecture/035-model-selection-design.md) |
| 当前工具推荐与IDE保存恢复 | [PRD](product/task-dispatch-options.md)／[架构](architecture/intelligent-capability-selection-notes.md)／[AI SOP](operations/task-dispatch-options.md)／[交互](design/task-dispatch-options.md)／[合同](../contracts/task-dispatch-options.md)／[验证](quality/task-dispatch-options-verification.md)／[摘要](quality/task-dispatch-options-summary.json)／[English](en/task-dispatch-options.md)／[逐平台差距](product/platform-capability-delivery-matrix.md) |
| 新增平台参考与智能选择／IDE缺口 | [OpenWorkBuddy、Octop固定官方基线](integrations/development-platform-reference-extensions.md) |
| Token、费用估算与执行额度 | [架构／交互与AI SOP](architecture/usage-accounting.md)、[正式合同](../contracts/usage-accounting.md) |
| 模型规划与站内风险建议 | [架构／交互与AI SOP](architecture/intelligent-planning.md)、[接口合同](../contracts/intelligent-planning.md) |
| 当前工具资格显示与图控件 | [显示合同](../contracts/platform-display.md)、[交互设计](design/platform-display.md)、[专项计划](delivery/platform-display-plan.md)、[验证边界](quality/platform-display-verification.md)、[精简摘要](quality/platform-display-summary.json) |
| 精确尝试工具历史 | [架构与操作SOP](architecture/attempt-tool-history.md)、[读取合同](../contracts/attempt-tool-history.md)、[专项验证](quality/attempt-tool-history-verification.md)、[精简摘要](quality/attempt-tool-history-summary.json) |
| 原生诊断与命令编辑 | [链路设计与SOP](agents/native-diagnostics.md)、[专项验收](quality/native-diagnostics-verification.md)、[精简摘要](quality/native-diagnostics-summary.json) |
| 执行诊断与运行命令国际化 | [架构与边界](architecture/runtime-localization.md)、[专项验收](quality/runtime-localization-verification.md)、[精简摘要](quality/runtime-localization-summary.json) |
| 原生智能体与开发工具 | [智能体运行时](agents/runtime.md)、[工具接入](integrations/tools.md) |
| 原生实际编码与机器封存 | [PRD](product/native-coding.md)、[架构与AI SOP](agents/native-coding.md)、[交互规范](design/native-coding.md)、[实现合同](../contracts/native-coding.md)、[阶段验收](quality/native-coding-verification.md) |
| 团队多工具接入 | [PRD](product/team-cli-runtime.md)、[集成与AI SOP](integrations/team-cli-runtime.md)、[合同](../contracts/team-cli-runtime.md)、[English](en/team-cli-runtime.md) |
| 项目 MCP 与受控草稿 | [PRD](product/project-mcp.md)、[统一合同](../contracts/application-tools-mcp.md)、[接入与AI SOP](integrations/project-mcp.md)、[交互与国际化](design/project-mcp.md)、[阶段验证](quality/project-mcp-verification.md)、[生成工具Schema](api/mcp-tools.json) |
| 工具配置与固定任务派发 | [执行架构](architecture/tool-execution.md)、[工具工作台](design/tool-workspace.md)、[本机工具运行时](agents/tool-execution-runtime.md)、[实现合同](../contracts/tool-execution.md)、[本机验收](quality/tool-execution-verification.md)、[当前四程序](deployment/tool-stop-runtime-bundles.md) |
| 命令行与终端 | [CLI 与 Bubble Tea](integrations/cli.md) |

## 团队、候选与受管集成

| 领域 | 设计、合同与验证 |
| --- | --- |
| 修复计划修订与运行溯源 | [PRD](product/candidate-repair-revisions.md)／[架构](architecture/candidate-repair-revisions.md)／[AI SOP](operations/candidate-repair-revisions.md)／[交互](design/candidate-repair-revisions.md)／[合同](../contracts/candidate-repair-revisions.md)／[CLI](../contracts/candidate-repair-revisions-cli.md)／[限定验证](quality/candidate-repair-revisions-verification.md)／[摘要](quality/candidate-repair-revisions-summary.json)／[English](en/candidate-repair-revisions.md) |
| 冲突修复与受控恢复 | [PRD](product/candidate-conflict-repair.md)／[架构与AI SOP](architecture/candidate-conflict-repair.md)／[交互](design/candidate-conflict-repair.md)／[合同](../contracts/candidate-conflict-repair.md)／[限定验证](quality/candidate-conflict-repair-verification.md)／[摘要](quality/candidate-conflict-repair-summary.json)／[English](en/candidate-conflict-repair.md) |
| 多源候选合成与下游代码基线 | [PRD](product/candidate-composition.md)、[架构与AI SOP](architecture/candidate-composition.md)、[交互](design/candidate-composition.md)、[合同](../contracts/candidate-composition.md)、[CLI](../contracts/candidate-composition-cli.md)、[限定验证](quality/candidate-composition-verification.md)、[摘要](quality/candidate-composition-summary.json)、[English](en/candidate-composition.md) |
| 专家团队依赖编排 | [PRD](product/expert-team-orchestration.md)、[架构与AI SOP](architecture/expert-team-orchestration.md)、[交互](design/expert-team-orchestration.md)、[合同](../contracts/expert-team-orchestration.md)、[CLI](../contracts/expert-team-orchestration-cli.md)、[限定验证](quality/expert-team-orchestration-verification.md)、[English](en/expert-team-orchestration.md) |
| 受管代码集成 | [PRD](product/repository-integration.md)、[架构／AI SOP](architecture/repository-integration.md)、[交互](design/repository-integration.md)、[部署](deployment/repository-integration.md)、[合同](../contracts/repository-integration.md)、[限定验证](quality/repository-integration-verification.md) |
| 项目专家配置与团队派发 | [PRD／交互](product/expert-team-dispatch.md)、[架构与AI SOP](architecture/expert-team-dispatch.md)、[合同](../contracts/expert-team-dispatch.md)、[限定验证](quality/expert-team-dispatch-verification.md) |
| 跨节点候选代码交接 | [PRD](product/candidate-transfer.md)、[领域架构与SOP](architecture/candidate-transfer.md)、[正式合同](../contracts/candidate-transfer.md)、[实际验收](quality/candidate-transfer-verification.md) |

## 交付、部署与运维

| 领域 | 设计、合同与验证 |
| --- | --- |
| 固定失败证据的智能分诊 | [PRD](product/failure-diagnosis.md)、[架构与 AI SOP](architecture/failure-diagnosis.md)、[交互](design/failure-diagnosis.md)、[合同](../contracts/failure-diagnosis.md)、[CLI](../contracts/failure-diagnosis-cli.md)、[限定验证](quality/failure-diagnosis-verification.md)、[摘要](quality/failure-diagnosis-summary.json)、[English](en/failure-diagnosis.md) |
| 真实制品、发布授权与恢复 | [制品存储](data/release-artifacts.md)、[发布证据架构](architecture/release-evidence.md)、[发布工作台](design/release-workspace.md) |
| 本机发布与执行提供方 | [发布控制面](deployment/release-control.md)、[提供方协议](integrations/release-provider.md)、[四程序运行包](deployment/release-runtime-bundles.md) |
| 桌面、服务器与安装 | [桌面](deployment/desktop.md)、[服务器](deployment/server.md)、[容器与集群模板](deployment/containers.md) |
| 可复现构建与本地缓存 | [构建说明](deployment/build-reproducibility.md)、[六平台 Go 运行时](deployment/runtime-bundles.md) |
| 验证与运维 | [验证报告](quality/verification.md)、[验收矩阵](quality/acceptance.md)、[运维手册](operations/runbook.md) |
| 实际运行与交付摘要 | [证据索引](quality/evidence/README.md) |
| 架构取舍、参考依据 | [ADR](adr/README.md)、[参考锁定](integrations/references.json) |
| 第三方依赖与许可盘点 | [第三方清单](delivery/third-party.md) |

## 补充专项证据与操作入口

以下入口补齐原README、状态与台账引用的领域材料；保留原报告，不将原始测试计数累加为当前全套通过。

- [团队多工具执行验证](quality/team-cli-verification.md)
- [团队多工具验证摘要](quality/team-cli-summary.json)
- [团队任务交接 PRD](product/task-handoff.md)
- [任务交接架构与 AI SOP](architecture/task-handoff.md)
- [团队任务交接合同](../contracts/task-handoff.md)
- [Task responsibility handoff](en/task-handoff.md)
- [团队任务交接验证](quality/task-handoff-verification.md)
- [任务交接验证摘要](quality/task-handoff-summary.json)
- [原生模型执行合同 v1](../contracts/model-execution.md)
- [原生模型执行验证摘要](quality/model-execution-summary.json)
- [工作区Git与执行限额 AI SOP](operations/workspace-git.md)
- [工作区 Git 验证摘要](quality/workspace-git-summary.json)
- [Workspace Git review and native execution limits](en/workspace-git.md)
- [Token、费用估算与额度：限定验证](quality/usage-accounting-verification.md)
- [用量与额度验证摘要](quality/usage-accounting-summary.json)
- [模型规划与站内风险预警：限定验证](quality/intelligent-planning-verification.md)
- [模型规划验证摘要](quality/intelligent-planning-summary.json)
- [CLI任务交接](api/cli-task-handoff.md)
- [原生编码验证摘要](quality/native-coding-summary.json)
- [UI 原始值插入合同](../contracts/ui-literal-values.md)
- [后端诊断验证摘要](quality/backend-diagnostics-summary.json)
- [语言偏好验证摘要](quality/locale-preferences-summary.json)
- [共享执行池验证摘要](quality/execution-pool-summary.json)
- [增量调度验证摘要](quality/scheduling-summary.json)
- [Multica中文README](https://github.com/multica-ai/multica/blob/main/README.zh.md)
- [Multica中文文档](https://multica.ai/docs/zh)
- [前端共享池与双语验证](quality/frontend-localization-verification.md)
- [共享执行资源池与多项目执行合同](../contracts/execution-pools.md)

## 维护规则

产品基线确定完整范围，实施计划保留待办，交付状态只记当前事实，各领域报告保存准确源码、环境、证据及限制。新库初始化变更不承诺旧库升级；原运行目录、凭据与证据不因文档整理被删除或改写。

接口变化同步共享合同、生成Schema与实际消费者；业务门禁变化同步状态机、用户旅程、错误恢复和必要测试。源码实现、本地协议、真实厂商、人工验收与生产交付分开记录；未验证能力不标可用。

历史阶段过程由Git与专项报告追溯。原始运行证据按项目交付目录保存，临时文件使用忽略的.cache；日常维护不再向当前入口追加长篇阶段流水。

## 初始化架构与验证

[1.0.0初始化设计与AI SOP](architecture/initial-version.md) · [初始化限定验证](quality/initial-version-verification.md) · [精简摘要](quality/initial-version-summary.json)。
