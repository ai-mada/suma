# 1.0.0 验证入口

更新：2026-10-28。当前只有单一 **1.0.0** 初始化基线；完整平台继续实施。本页按能力指向准确报告，不重复阶段流水、旧测试数字或旧数据库升级说明。详细过程仍可从专项档案与 Git 追溯。

当前范围为需求1–26，58项有限实现、34项待办，共92项。新增需求26／`FND-INTENT-001`尚未实现；验收追踪只推进已有范围，历史专项报告保留当时台账和证据。

| 当前范围 | 验证与限制 |
| --- | --- |
| 当前需求版本的验收追踪 | [限定报告](requirement-traceability-verification.md)、[精简摘要](requirement-traceability-summary.json)、[中文指南](../development/requirement-traceability.md)／[English](../en/requirement-traceability.md)、[正式合同](../../contracts/requirement-traceability.md)：固定需求版本／条目序号、当前发布／受保护采用及确切运行／测试节点／正式验收，实际检查结果以报告为准；不代表完整用例覆盖、应用版本／发布矩阵或需求26完成 |
| 任务交付闭环与 Runner 本地诊断 | [限定报告](task-delivery-chain-verification.md)、[精简摘要](task-delivery-chain-summary.json)、[中文指南](../development/task-delivery-chain.md)／[English](../en/task-delivery-chain.md)、[交付合同](../../contracts/task-delivery-chain.md)、[本地诊断合同](../../contracts/runner-journal-inspection.md)：当前运行摘要与正式来源、退回问题／需求回流、单文件只读诊断；本机自有 DSH 协议命令和独立 Runner 同 SHA 测试不代表厂商模型或物理多机验收，最终界面覆盖以专项报告为准 |
| 参考源码整合与业务任务依赖编辑 | [本阶段报告](task-dependencies-verification.md)、[中文指南](../development/task-dependencies.md)／[English](../en/task-dependencies.md)、[合同](../../contracts/task-dependencies.md)、[来源映射](../integrations/referenced-conversation-integration.md)：真实交付包设计适配、完整图／原子 CAS／冲突和原回执恢复；实际 API 与生产构建界面复验结果分别以专项报告为准，旧包数字不计为当前通过证据 |
| 项目团队执行入口 | [限定报告](team-execution-verification.md)、[操作指南](../development/team-execution.md)：复用正式分页与原详情，区分验收／排队／阻塞，URL恢复及权限隔离；没有新增物理多机、厂商任务或生产验收结论 |
| 本地文件来源、草稿与原操作恢复 | [限定报告](workspace-file-context-verification.md)、[操作指南](../development/local-file-editing.md)：读取／保存绑定实际目录身份，显式基线采用，账号组织隔离与原回执；完整IDE和生产验收继续待办 |
| 六目标桌面打包与安装交付 | [限定报告](desktop-packaging-verification.md)、[摘要](desktop-packaging-summary.json)：六目标控制程序／计划、实际Windows安装／便携包、资源／校验清单和归档；原生多系统安装、GUI、远端CI及生产签名未验，非off签名预检拒绝 |
| 桌面项目目录与根约束文件链路 | [限定报告](desktop-workspace-verification.md)、[摘要](desktop-workspace-summary.json)：原生桥／绑定合同、定向检查及实际HTTP保存／Git／回执／184表重启通过；Electron图形子进程启动失败，原生界面、安装器和完整IDE未验 |
| 本地IDE目录分页与CLI | [限定报告](workspace-directory-verification.md)、[摘要](workspace-directory-summary.json)：元数据入口、原范围游标、草稿保护、CNEN及640px布局；正式保存／运行0，物理多机／完整IDE和容量矩阵未验 |
| OpenCode实际CLI／SDK／宿主／独立测试与运行交互 | [实际限定报告](opencode-actual-execution-verification.md)、[摘要](opencode-actual-execution-summary.json)：真实官方Windows工具配自有模型协议、两Worker／同SHA独立测试；原数据快照CNEN／刷新／测试下钻；商业模型效果、物理多机、跨平台与生产未验 |
| Codex当前执行工作区只读 | [限定报告](codex-workspace-observation-verification.md)、[精简摘要](codex-workspace-observation-summary.json)：真实本机Control／Runner／隔离Git／自有协议子进程的出站读取、界面及取消；零厂商或模型调用，保存仍仅限原生 |
| 模型选择跨普通任务、专家与原生修复 | [当前限定报告](model-selection-workflows-verification.md)、[精简摘要](model-selection-workflows-summary.json)：受影响接口／CLI／Web、实际预览及修订／刷新、零新增模型和执行；原生修复仍不支持 Codex |
| 单一初始化结构与共享版本 | [初始化报告](initial-version-verification.md)、[摘要](initial-version-summary.json)：该阶段183表、同结构正常重启、未知既有结构拒绝；不升级旧用户库 |
| 安装版 Codex 实际编码 | [准确源码与环境的专项报告](codex-app-server-verification.md)：安装版 CLI 配合自有 Responses 端点；配置 ACK 不是提供方身份，提醒不等于硬预算；不将旧切片作为本轮厂商验收 |
| 其他领域能力 | [交付状态](../delivery/status.md)与[完整文档索引](../README.md)列出各域当前实现和独立证据 |
| 未完成范围 | [完整实施计划](../delivery/implementation-plan.md)、[产品基线](../product/v1-baseline.md)：P0–P7及扩展需求1–26全部保留；[统一智能输入／配置](../architecture/intelligent-operations.md)仍待实现 |

检查按变更影响选择，失败修正后仅复验相关路径。源码完成、协议夹具通过、真实执行、正式业务验收与生产验收分别记录；不累加重叠测试为虚构同轮全套。费用未定价、partial／缺失用量、未知执行结果都不能推断为零或成功。

当前 Windows 构建与本机浏览器证据不证明 OceanBase／PostgreSQL 实集群、目标吞吐、物理多机、跨平台安装、厂商账户、生产模型、签名、支付或完整平台验收。构建清单、日志和私有夹具留在忽略目录，Git只收必要源码、合同、设计与精简证据。
