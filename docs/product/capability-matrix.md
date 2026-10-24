# 能力与验收矩阵

状态：源码与本地证据审查，平台整体尚未完成。日期：2026-10-24。Owner：产品、架构与集成负责人。

计划：[P0–P7 实施台账](../delivery/implementation-plan.md)。验收依据：[验证报告](../quality/verification.md)、[验收要求](../quality/acceptance.md)、[浏览器旅程记录](../quality/evidence/browser-journey.json)。本页反映当前实现边界；最终测试日志和外部验收签字以交付报告为准。

“实现”只表示所列代码路径存在；“本地验证”限定到列出的环境和测试。协议模拟、驱动可加载、打包配置存在，都不能替代真实外部系统、安装环境或生产容量验收。以下各项均未获得生产环境验收结论。

## P0–P1：基础与数据

| 能力 | 源码实现及范围 | 本地验证与证据 | 外部验收及剩余工作 |
| --- | --- | --- | --- |
| 产品、领域、架构与交付台账 | 已形成文档；参见[领域模型](../architecture/domain-model.md)、[架构图](../architecture/diagrams.md) | 文档关联源码与本地证据 | 完整目标继续按实施台账追踪 |
| 身份、项目权限与凭据范围 | [auth.go](../../internal/platform/auth.go)、[http.go](../../internal/platform/http.go)、[治理说明](governance.md)：多组织会话、真实账号、项目授权、执行凭据范围与撤权 | [治理回归](../../internal/platform/governance_test.go)、[权限与证据回归](../../internal/platform/security_evidence_test.go)、[HTTP 回归](../../internal/platform/http_test.go)；本轮结果见统一验证报告 | 企业身份联合、目录同步、密码重置、邮箱验证及付费席位仍未完成 |
| 需求、任务、资源、问题、文档、版本、测试登记 | [records.go](../../internal/platform/records.go)：正式 SQL 记录、版本和操作回执；人类资源绑定真实有权组织账号 | 对应SQLite/API回归；组织账号与项目资源身份分别检查 | Agent/工具资源登记不证明真实外部接入；登记数据不等于真实测试或真实发布 |
| API 与端间合同 | [合同生成器](../../cmd/suma-contracts/main.go)、[OpenAPI](../api/openapi.json)：域模型反射、嵌入字段展开、nullable工时、治理/计划/进度请求字段与路由元数据 | 生成、非写入漂移检查和引用闭包回归；见[API 说明](../api/README.md) | 旧请求部分仍为通用信封；map DTO和路由需人工核对，权限/跨字段/状态校验仍由服务端执行，尚无完整端间等价证明 |
| 本机事务存储 | [store](../../internal/store/store.go)、[事务说明](../data/transactional-store.md)：SQLite 持久化和迁移 | 实际 SQLite 重启、竞争、回执、状态迁移测试 | 长期备份恢复与升级演练未完成 |
| OceanBase、PostgreSQL | 驱动、方言和配置路径已实现 | 本地 SQLite 不能证明两种集群兼容；未连接真实集群 | 迁移、隔离级别、锁行为、备份恢复、并发矩阵待验收 |
| 来源与索引 outbox | [records.go](../../internal/platform/records.go)、[projector.go](../../internal/platform/projector.go)：来源与事件同事务；逐项发送；永久错误入失败台账 | [投影回归](../../internal/platform/projection_test.go)：坏事件不阻塞、重放、删除与归档 | 运行监控、人工重驱与大规模积压治理需完善 |
| 索引查询授权 | [search.go](../../internal/platform/search.go)：返回片段前复查 SQL 来源权限、当前版本和可见状态；关键词路径过滤归档/删除 | SQLite 权限与投影回归 | 生产访问策略与性能待验收 |
| 本地全文与向量索引 | [KnowledgeIndex](../../packages/knowledge/src/index.ts)、[Lance 后端](../../packages/knowledge/src/lance.ts)：实际原生索引、版本墓碑、模型配置隔离 | [原生测试](../../packages/knowledge/tests/index.test.ts)：实际 FTS/向量；向量模型响应使用本地模拟 | 未验收生产 embedding 账户与真实数据质量；原生 OceanBase 检索适配未做集群验收 |
| 原件、多模态与对象制品 | 文本文档来源、切块及定位已实现 | 文本索引测试 | OCR、音视频解析、完整原件 API、S3、迁移与分代发布闭环未实现；[目标说明](../data/multimodal.md) |

## P2：执行与开发工作区

| 能力 | 源码实现及范围 | 本地验证与证据 | 外部验收及剩余工作 |
| --- | --- | --- | --- |
| 真正 DAG、条件与汇合 | [compiler.go](../../internal/dag/compiler.go)、[transitions.go](../../internal/dag/transitions.go)：无环验证、条件跳过、all_success/all_done/any_success/quorum | [核心回归](../../internal/dag/engine_test.go)：菱形并行、提前汇合一次、失败保留 | 条件为受限表达式；不是任意脚本执行器 |
| 人审、定时与信号等待 | 控制面持久等待，不占 Worker 槽位；审批带版本和回执 | 同上：审批冲突、重放、定时和信号 | 外部通知渠道与日历自动化未接入 |
| 动态 map | 持久子节点展开、稳定身份与汇合 | 重启、重复推进、动态子节点测试 | 单次物化数量和嵌套深度受实现上限控制 |
| 固定版本子流程 | [subworkflows.go](../../internal/dag/subworkflows.go)：固定工作流版本、父 attempt 唯一 child、等待不占槽、控制传播 | [子流程测试](../../internal/dag/subworkflow_test.go)：重启、失败、暂停、取消、嵌套边界 | 并非独立厂商流程联邦 |
| 外部工作流 | [Runner 桥接](../../internal/runner/external.go)：管理员注册插件，精确能力匹配，真实配置传递，启动/轮询/取消协议 | [Runner 测试](../../internal/runner/external_test.go)：本地 HTTP 服务模拟 | 未接入真实厂商账户；没有插件能力时不能冒充执行成功 |
| 显式补偿 | [compensation.go](../../internal/dag/compensation.go)：成功 Worker 显式模板、固定 effect 引用、逆依赖独立运行；内部生成图禁止公开启动/覆盖 | [补偿测试](../../internal/dag/compensation_test.go)：重放、来源限制、嵌套引用与内部图隔离 | 不提供通用撤销保证；需为每种外部副作用配置和验收补偿动作 |
| 租约、fence 与未知结果 | [scheduler.go](../../internal/dag/scheduler.go)、[commands.go](../../internal/dag/commands.go)：节点租约、重复/迟到结果拒绝、幂等动作重试、未知结果核对 | SQLite 竞争、重启、晚回执、未知停止证据测试 | 外部副作用无法由数据库提供 exactly-once 保证 |
| 暂停、取消和停止证据 | 实际停止事实与请求分开；未知状态保留资源，等待核对 | 核心与 Runner 停止回归 | 外部系统必须提供可靠终态；取消 HTTP 确认不等于已经停止 |
| 资源、预算与调度 | 事务预留、项目资源限制、候选运行轮转 | 核心资源竞争测试 | 全局 SQL 锁串行化变更；未实现分片增量调度、租户加权公平、模型结算；未证明 1000 槽位 |
| Runner 命令执行 | 许可命令、进程生命周期、Windows 进程树、持久回执/恢复 | [Runner 源码与测试目录](../../internal/runner)、[产物摘要](../agents/evidence/runner-sha256.json) | 容器/远端设备隔离和跨平台实机矩阵待验收 |
| Runner 进展与恢复依据 | [progress.go](../../internal/dag/progress.go)：当前attempt/worker/fence校验、稳定操作回执、持久node_progress；界面展示真实有界输出快照，恢复按钮读取运行固定标记 | 对应DAG/Runner回归与[前端消费者模型](../../apps/web/src/planning-governance.test.ts)；实际执行结果见统一报告 | 输出快照不是完成百分比或任务验收；轮询不等于可靠WS/SSE日志回放 |
| 许可写命令隔离与证据 | [workspace.go](../../internal/runner/workspace.go)：逐attempt Git工作树、管理员固定repository/base、真实提交/差异/状态/路径清单摘要、持久操作元数据 | [真实Git工作树回归](../../internal/runner/workspace_test.go)；本轮环境和结果由验证报告记录 | 隔离依赖管理员仓库配置和本地Git；未知结果需核对，不自动切换/重置用户检出或合并 |
| 候选来源选择与固定测试 | [候选校验与固定](../../internal/dag/candidate.go)、[选择框](../../apps/web/src/CandidateSource.tsx)：test选择本流程单个静态command/agent祖先，固定来源成功尝试的干净提交及证据 | [调度候选回归](../../internal/dag/candidate_test.go)、[Runner候选回归](../../internal/runner/candidate_test.go)、[前端消费者模型](../../apps/web/src/workflow-candidates.test.ts) | Runner须配置repository且候选提交已在本地；不自动聚合多分支，不推断跨map/子流程候选，不自动fetch或传送远端Git制品 |
| 审阅后的合并预检 | [workspace.go](../../internal/runner/workspace.go)：核对目标HEAD、候选HEAD、测试/工作树证据及审阅身份，生成预检回执 | 对应真实Git预检回归；实际结果见统一报告 | 只表达可预检范围，未执行合并；完整合并授权、冲突恢复、CI身份和签名证明仍待实现 |
| 原生智能体 | [agent-runtime](../../packages/agent-runtime/src)：有限轮次、模型接口与受控工具协议 | 本地协议、错误、工具边界测试；见[运行说明](../agents/runtime.md) | 真实模型账户、成本质量评估、生产工具权限待验收 |
| 外部 Agent CLI | [catalog.ts](../../packages/agent-runtime/src/catalog.ts)：Codex/OpenCode/Hermes/DeepSeek 相关命令存在性、版本/help 探测 | 本地模拟进程与协议探测 | 探测不是完整登录、执行、工具授权和会话恢复集成 |
| Workflow 编辑与观察 | [Workflow.tsx](../../apps/web/src/Workflow.tsx)：ReactFlow/ELK、编辑/执行/诊断、真实依赖和动态子节点、候选来源选择、当前尝试输出与来源证据 | 对应前端模型与浏览器旅程；本轮结论见统一报告 | 事件采用轮询；高级JSON仍需服务端校验，未实现可靠WS/SSE回放 |
| 本地代码工作区 | [IDE.tsx](../../apps/web/src/IDE.tsx)、[workspace.go](../../internal/platform/workspace.go)：Monaco、允许目录文本树、摘要并发保护、持久保存日志 | [工作区回归](../../internal/platform/workspace_test.go)；见[一致性说明](../data/file-and-query-consistency.md) | 编辑工作区与Runner隔离执行范围分别管理；真实终端、LSP/DAP、完整调试预览仍未实现 |

## P3–P5：研发闭环与运维

| 能力 | 源码实现及范围 | 本地验证与证据 | 外部验收及剩余工作 |
| --- | --- | --- | --- |
| 当前需求拆解、任务依赖与分配 | [records.go](../../internal/platform/records.go)：当前版本校验、批次回滚、依赖验证与看板数据 | SQLite/API 和[浏览器验收截图](../quality/evidence/browser-task-accepted.png) | AI 需求采集/评审/采用、完整 PRD/UED/原型编辑未完成 |
| 任务与运行原子绑定 | [delivery.go](../../internal/platform/delivery.go)：创建 run 同事务固定任务和需求依据；前置验收检查 | [HTTP 与安全回归](../../internal/platform/security_evidence_test.go) | 外部 CI/制品身份证明未接入 |
| 测试证据与人工验收 | [evidence.go](../../internal/platform/evidence.go)：当前 attempt、测试节点成功、版本稳定、子流程链和补偿检查 | SQLite 回归、浏览器真实旅程 | 当前仅认可实际存储的证据范围；不能将登记/模拟视为真实外部测试 |
| 内容与补偿导致验收失效 | 同事务撤销关联任务及递归后继的 accepted 依据 | 传递依赖、补偿、回执回放回归 | 生产协作冲突和历史迁移需演练 |
| 失败问题与需求回流 | [failure_projection.go](../../internal/platform/failure_projection.go)：持久失败事件、每 run 回执、消费台账；主循环独立推进 | [失败投影测试](../../internal/platform/failure_projection_test.go)：Tick 非 Runner 失败、崩溃窗口重放、重启 | 告警渠道、授权诊断/自动修复、复测熔断和经验评测未实现 |
| 统计、活动与版本资料 | 正式数据聚合、活动和登记 API；与真实发布对象分开 | SQLite/API 验证 | 版本资料登记不表示已经发布 |
| 真实制品与证明 | [接收与封存](../../internal/platform/artifact_storage.go)、[派生证明](../../internal/platform/artifact_proofs.go)、[Runner采集](../../internal/runner)；实际字节摘要与固定C、命令配置及当前attempt绑定 | [真实发布旅程](../quality/evidence/release-browser-execution.json)、真实HTTP包测试与上传/恢复回归 | 首轮static_site_v1、本地目录；S3、多节点共享存储和签名未验收；profile配置摘要不等于工具二进制摘要 |
| 固定范围与独立发布审批 | [候选](../../internal/platform/release_candidate_service.go)、[计划](../../internal/platform/release_deploy_plans.go)、[在线授权](../../internal/platform/release_deploy_authorization.go)；批准绑定具体内容、环境、policy/generation | 精确依据/权限变更、双连接竞争、旧票据与伪造拒绝；[独立审核](../quality/release-authority-review.md) | 首轮由浏览器管理员登记运行profile；外部CI身份和生产审批组织策略仍需集成 |
| 实际部署、健康与回退 | [独立Deployer](../../internal/deployer)、[观测恢复](../../internal/platform/release_deploy_observations.go)、[工作台](../../apps/web/src/ReleaseWorkspace.tsx)；本机静态字节、HTTP内容校验、单调代数、原operation核对及新计划回退 | 浏览器真实5次操作A/B/A/B/A，B两次健康404；提供方进程崩溃/丢ACK/迟到与A/B/A测试 | 仅本机回环静态提供方；Docker/K8s/S3/生产/灰度及跨机器验收未完成 |
| 发布问题统一与来源导航 | [统一投影](../../internal/platform/release_failure_projection.go)、[来源导航](../../apps/web/src/RecordLineage.tsx)；DAG与provider关联同一operation问题，保留来源 | 最终实际操作只有1个问题，刷新重复回流只有1个需求；保留修复前重复作为历史缺陷证据 | 回退不自动关闭问题；告警诊断、授权智能修复及独立复测仍待实现 |
| 多进程恢复 | SQL 锁、租约、事务回执、持久队列 | 本地多 Engine 竞争和重启回归 | 未进行生产故障注入、长期运行和跨集群恢复验收 |
| 自动触发 | 只有已有手动 API/CLI 入口与节点定时等待 | 未验证计划中的触发服务 | Cron/Webhook/Git/CI/监控触发、签名去重、错过/重叠策略未实现 |
| 组织、邀请、团队与项目授权 | [governance.go](../../internal/platform/governance.go)、[邀请](../../internal/platform/governance_invitations.go)、[团队与授权](../../internal/platform/governance_teams.go)、[治理界面](../../apps/web/src/Governance.tsx)：多组织、所有者保护、一次性邀请、团队归属和独立项目授权 | [治理回归](../../internal/platform/governance_test.go)与浏览器旅程；最终结果见统一报告 | 邀请由用户手工交付，不发送邮件；未包含企业身份联合、席位或目录治理 |
| 人类资源与团队身份 | [资源绑定界面](../../apps/web/src/ResourceIdentityFields.tsx)：人类项目资源绑定真实组织user_id，任务owner使用项目资源ID；团队human使用组织用户、Agent/tool使用项目资源 | 对应组织权限、资源绑定和前端身份回归 | 团队归属不授予项目访问；Agent/tool登记不代表连接可用，实际接入分别验收 |
| 迭代、里程碑与原子排期 | [planning.go](../../internal/platform/planning.go)、[计划界面](../../apps/web/src/Planning.tsx)：计划独立版本、任务版本双校验、完整字段批次原子保存、关联任务验收完成门禁 | [计划回归](../../internal/platform/planning_test.go)、[前端表单消费者模型](../../apps/web/src/planning-governance.test.ts)；本轮浏览器结果见统一报告 | 不推断任务已验收；完整产品设计编辑、风险管理和组合规划仍待完成 |
| 全量不可变项目基线 | [baselines.go](../../internal/platform/baselines.go)：事务捕获项目全部任务/计划、元数据列表、分页快照、全范围当前比较 | 对应不可变范围、分页及比较回归；最终结果见统一报告 | 全量指单个项目任务范围；连续进度历史、预测和portfolio全量统计尚未实现 |
| 每周资源容量 | [planning_read.go](../../internal/platform/planning_read.go)：真实资源容量版本、nullable可用工时、已排估算按重合日历日期比例跨周分配、未完整分配提示 | 对应跨周分摊、未知/零工时和版本回归；实际结果见统一报告 | 这是人工计划估算，不是实际工时；人员工作制、工作日/节假日/请假、容量日历和工时消耗尚未实现 |
| 项目指标与已加载范围 | 全项目任务SQL指标、任务时间线/基线分页及coverage；交付accepted与执行状态分别展示 | 对应范围与状态消费者回归 | 单项目全量指标不代表组织/跨项目portfolio汇总；风险登记与预测分析仍待实现 |
| 完整产品设计与规划 | 已有需求表单、任务排期与文档记录入口 | 未形成完整产品编辑能力验收 | AI需求采集/评审/采用闭环、完整PRD/UED/架构产品编辑、可运行原型、风险闭环和组合规划仍在台账 |

## P6–P7：协作、商业化与交付

| 能力 | 源码实现及范围 | 本地验证与证据 | 外部验收及剩余工作 |
| --- | --- | --- | --- |
| CLI/TUI | [CLI](../../cmd/suma/main.go)、[BubbleTea 界面](../../internal/cli/tui.go)：共享 HTTP 身份、分页与终端转义清理 | CLI 回归和本机构建；见[集成说明](../integrations/cli.md) | 不等于各外部 Agent 的业务集成 |
| 浏览器工作台 | React/Vite/Radix、任务/流程/编辑工作区 | 生产构建、实际 HTTP 与浏览器旅程；[旅程记录](../quality/evidence/browser-journey.json) | 完整浏览器/可访问性/视觉回归矩阵尚未验收 |
| 桌面壳 | [Electron 主进程](../../apps/desktop/src/main.cjs)：本机控制面生命周期、loopback、preload 隔离与导航限制 | Windows 未签名 unpacked 目录；[产物哈希](../deployment/evidence/windows-unpacked-sha256.json) | 桌面未自动启动 Runner；正式安装/升级、签名、公证、macOS/Linux 实机均待验收 |
| 离线与实时协作 | 有本机持久化、版本/墓碑基础 | 不构成双向同步验证 | 全项目离线同步、CRDT/Yjs、设备撤权和墓碑回收未实现 |
| 通用办公与媒体 | 目标文档存在 | 无产品能力验收 | ProseMirror 文档、Univer 表格、SVG 演示及格式往返、LiveKit 会议/语音未实现 |
| 渠道与计算机操作 | 目标范围记录 | 无渠道或设备真实验收 | 渠道身份/消息映射、授权浏览器/计算机操作未实现 |
| 商业化 | 目标范围记录 | 无商户验收 | 套餐、席位、配额、账单、支付宝/微信/企业结算与回调验签未实现 |
| 正式发布和容量 | 有文档、构建、测试及部分产物摘要 | 证据范围见[验证报告](../quality/verification.md) | 目标容量、30 分钟事件负载、全链路故障注入和正式签收未完成 |
| 第三方可追溯 | 锁定参考版本和实际依赖元数据；见[第三方清单](../delivery/third-party.md) | 本地许可证文件/manifest 哈希 | 不是法律意见，也不是最终安装包完整 SBOM 或分发许可审查 |

## 使用本矩阵

新增“已验证”结论必须关联具体源码、命令/测试、环境和结果。外部能力需要独立记录账户/系统、版本、真实调用、恢复行为和验收人；不得仅凭配置项、模拟服务或成功构建勾选整个阶段。未完成项继续保留在实施台账。
