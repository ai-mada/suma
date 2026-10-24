# 速码项目设计文档

状态：实施中。文档负责人：平台集成。设计依据：2026-10-23 用户确认的完整方案。本文档索引与源码、接口和验证报告共同维护。

| 主题 | 入口 |
|---|---|
| 产品范围、角色和原则 | [产品需求](product/prd.md) |
| 完整能力与验收范围 | [能力矩阵](product/capability-matrix.md) |
| 可操作的完整旅程 | [用户旅程](product/user-journeys.md) |
| 实现与待办 | [实施计划](delivery/implementation-plan.md)、[交付状态](delivery/status.md) |
| 系统、部署与信任边界 | [架构总览](architecture/overview.md) |
| C4 与关键流程时序 | [可维护 Mermaid 源图](architecture/diagrams.md) |
| 领域归属与数据结构 | [领域模型](architecture/domain-model.md)、[事务存储](data/transactional-store.md) |
| 调度、恢复、资源和回执 | [DAG 内核](scheduler/engine.md)、[可靠性](scheduler/reliability.md)、[候选代码交接](scheduler/candidate-handoff.md) |
| 增量调度与项目准入 | [增量内核与迁移](scheduler/incremental-kernel.md)、[交互规范](design/scheduling-workspace.md)、[用量与领取](agents/scheduling-usage.md)、[阶段验收](quality/scheduling-verification.md)、[平台门禁验证](quality/scheduling-platform-guards.md)、[当前运行包与旧数据复核](deployment/scheduling-runtime-bundles.md) |
| 原件、多模态、索引与同步 | [数据平面](data/multimodal.md)、[同步协议](data/synchronization.md) |
| 真实制品、发布授权与恢复 | [制品存储](data/release-artifacts.md)、[发布证据架构](architecture/release-evidence.md)、[发布工作台](design/release-workspace.md) |
| 本机发布与执行提供方 | [发布控制面](deployment/release-control.md)、[提供方协议](integrations/release-provider.md)、[四程序运行包](deployment/release-runtime-bundles.md) |
| 原生智能体与开发工具 | [智能体运行时](agents/runtime.md)、[工具接入](integrations/tools.md) |
| 工具配置与固定任务派发 | [执行架构](architecture/tool-execution.md)、[工具工作台](design/tool-workspace.md)、[本机工具运行时](agents/tool-execution-runtime.md)、[实现合同](../contracts/tool-execution.md)、[本机验收](quality/tool-execution-verification.md)、[当前四程序](deployment/tool-stop-runtime-bundles.md) |
| 定时自动化与事件续传 | [自动化与事件架构](architecture/automation-events.md)、[自动化工作台](design/automation-workspace.md)、[本机验收](quality/automation-verification.md)、[当前四程序交付](deployment/automation-runtime-bundles.md) |
| 签名来源与交付 | [信任与事务](architecture/automation-ingress.md)、[入口工作台](design/event-ingress-workspace.md)、[GitHub/CI/监控接入](integrations/automation-ingress.md)、[精确合同](../contracts/automation-ingress.md)、[本机验收](quality/ingress-verification.md)、[当前运行包](deployment/ingress-runtime-bundles.md) |
| 产品交互与流程图 | [工作台](design/workbench.md)、[DAG 三视图](design/dag-interactions.md) |
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

本轮在组织治理、规划和固定候选测试基础上，进一步实现真实字节制品、构建与包测试证明、不可变发布候选、环境与独立部署审批、受保护执行、实际本机 HTTP 健康检查、未知核对及独立新代数回退。浏览器完成 A→B健康异常→A回退，修复后再次复验问题及需求去重。容量仍是计划估算；完整设计编辑、工作制日历、风险与组合规划、生产CI、S3及分布式数据库验收继续列为待办，不能据此签收完整P0–P7。具体边界以能力矩阵、实施台账和统一验证报告为准。

## 维护方式

本阶段在持久人工/cron、固定修订派发与授权SSE基础上，继续加入GitHub及Suma签名事件、交付去重、CLI发送和事件工作台。实际3次事件运行完成开发/制品/包测试，暂停及修订保留历史，最终交付控制服务重启后恢复原回执。范围与原始证据见专项报告，完整平台仍按实施台账推进。

接口修改先更新共享合同和对应验证；业务门禁变更同步状态机、用户旅程、错误恢复文案和测试。运行截图、HTTP 记录和大规模性能报告统一进入项目交付证据目录；日常临时文件使用 `.cache/`。
