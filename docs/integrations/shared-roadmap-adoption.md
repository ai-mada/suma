# 分享规划的采用与当前能力映射

状态：1.0.0，T-DELIVERY-112 设计整合；不作为完整平台完成或新增验收证据。正式进度仍为 **58 项有限实现、33 项待实现，共 91 项**，见[实施计划](../delivery/implementation-plan.md)与[当前交付状态](../delivery/status.md)。

## 来源与可见范围

来源：[速码 suma：下一阶段需求深化与智能化增强开发规划](https://chatgpt.com/s/t_6ae19e1f3af481918d11132ccf5119fe)。本轮最初 Web、Chrome 和会话读取未取得正文；随后通过 Firecrawl 普通 basic 代理访问同一明确链接，HTTP 200，成功获取当前分享可见正文。

只读来源文件为项目忽略目录 `.cache/e2e/delivery-112/shared-reference-source/share-body.md`，13033 字节，SHA-256：`1135aeaa9519c56d5daa95d6d0ae37fda944e0eeb4ecaae7edc361277086fc30`。该摘要绑定实际获取的 Markdown，而非源码、运行结果或完整原会话。

当前可见部分覆盖需求、团队、集成、调度、质量及知识六个领域，共 64 个编号：REQ 10、TEAM 10、INT 12、SCH 12、QA 10、KM 10。页面在 KM-010 后的“知识回答…”未完句处结束，并显示 Continue generating。**已取得当前可见正文，原规划末尾仍不完整**；不推断后续章节或编号。知识编号采用来源实际的 `KM`，不另造 `KN` 别名。原文是基于旧独立 0.2 报告的规划，明确未读取当时 Windows 最新源码，因此不能作为当前 1.0.0 的实现证明。

## 采用决定

1. 采用“先闭环、再扩展”和每项具备持久化、权限、恢复、证据的完成定义。来源 P0／P1／P2 是业务优先级，与本项目 P0～P7 实施阶段分开，不重写台账或重算完成数。
2. 采用结构化 SPEC、验收条目、版本和依赖的方向，继续使用当前需求设计及固定任务上下文；完整逐条验收追踪矩阵仍待完成。
3. 规划层提出可编辑建议；Go 调度器独立核验权限、依赖、预算和资源，保持平台唯一派发权。业务任务依赖与一次执行 DAG 分开，智能体不能绕过人工门禁。
4. 采用多身份、任务委托和版本化工具合同方向，沿用当前封闭 Actor 与项目凭据，不让人、Agent、Runner 或团队关联自动扩权。优先成熟官方 SDK／CLI／MCP，不访问未授权内部接口。
5. 保留精确代码／测试／验收依据、知识与记忆与技能的领域边界；声明工具能力、CLI 退出成功、测试节点成功、业务验收与生产发布分别记录。
6. 本轮 T-DELIVERY-112 实现其中有限关键闭环和本地安全诊断。下表为可见全部编号到当前模块的映射，未覆盖能力继续进入原正式计划；没有将规划内容导入运行中的用户需求池或自动创建任务。

## 需求工程：REQ

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| REQ-001 | 项目、成员、归档及权限已有有限实现；[治理](../product/governance.md)与[应用交付基线](../product/application-delivery.md) | 子项目、产品线、微应用和依赖应用的统一全生命周期继续待办 |
| REQ-002 | 固定流程版本与发布已有基础；[调度合同](../../contracts/scheduling-kernel.md) | 项目／研发模板目录、复制、审批、回滚及完整复用旅程未完整实现 |
| REQ-003、REQ-010 | 问题、失败诊断、显式需求回流已有有限实现；[失败分诊](../../contracts/failure-diagnosis.md)、[任务交付链路](../../contracts/task-delivery-chain.md) | 独立想法池、所有告警／生产渠道自动收集与归类、持续去重策略待验；回流不能自动批准需求 |
| REQ-004、REQ-005 | 需求、PRD／体验／技术设计、发布与任务绑定已有版本化实现；[需求设计](../../contracts/requirement-design.md)、[任务设计依据](../../contracts/task-design-context.md) | 完整产品状态机、结构化非功能要求、全设计编辑与发布版本统一追溯继续完善 |
| REQ-006、REQ-009 | 可编辑任务提案、单次模型规划与人工采用已有有限实现；[智能规划](../../contracts/intelligent-planning.md) | 全自动需求拆解、业务价值／风险／成本综合排序及质量评估不能由现有单次规划推断 |
| REQ-007、REQ-008 | 依据变化失效、当前任务／运行／验收／问题下钻已有基础；[交付链路](../../contracts/task-delivery-chain.md) | 逐条验收标准到实现、用例和版本的完整矩阵，跨微应用／接口／技能／发布影响分析待办 |

## 团队与身份：TEAM

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| TEAM-001、TEAM-002、TEAM-010 | 组织、团队、项目访问、人／Agent／工具资源、独立 Runner 凭据与当前授权已有有限实现；[治理](../product/governance.md)、[当前权限](../../contracts/current-credential-authority.md) | 完整自定义角色、跨组织远程与企业 SSO 验收仍待办；团队关联不授予项目权限 |
| TEAM-003、TEAM-007 | 版本化专家职责、工具／模型／知识配置与发布已有有限实现；[专家团队派发](../../contracts/expert-team-dispatch.md) | 通用角色模板目录和按技能、语言、环境、质量进行完整能力匹配仍待完善 |
| TEAM-004、TEAM-009 | 人员责任交接、独立执行身份、候选冲突修复及原操作恢复已有有限实现；[任务交接](../../contracts/task-handoff.md)、[冲突修复](../../contracts/candidate-conflict-repair.md) | 完整协作者／审核人模型、负责人离线替代与全部协作冲突策略待办 |
| TEAM-005、TEAM-006 | 固定多任务专家计划、独立工作树、依赖推进、暂停／停止和受限子流程已有基础；[团队编排](../../contracts/expert-team-orchestration.md)、[调度合同](../../contracts/scheduling-kernel.md) | 通用 Agent 委托、预算继承、全取消传播及跨机器专家团队效果需独立实现与验收 |
| TEAM-008 | 队列、执行池容量、项目准入和团队进度已有有限实现；[执行池](../../contracts/execution-pools.md)、[团队执行](../development/team-execution.md) | 完整人员容量日历、实际利用率／等待与失败率历史、目标负载均衡规模未验收 |

## 工具、SDK 与互操作：INT

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| INT-001、INT-012 | 固定工具档案、统一事件／取消／结果合同与已选适配器检查已有有限实现；[工具合同](../../contracts/tool-execution.md)、[团队 CLI](../../contracts/team-cli-runtime.md) | 全适配器 SDK、每厂商版本统一契约矩阵和真实认证环境不能由本地协议夹具替代 |
| INT-002、INT-003 | Codex app-server 和 OpenCode 官方 SDK／CLI 已有实际限定执行；[Codex](../../contracts/codex-app-server.md)、[OpenCode](../../contracts/opencode-sdk.md) | 现有官方工具配自有模型端点证据不等于商业模型身份／效果／账单；跨平台与会话恢复仍有缺口 |
| INT-004 | CodeBuddy／WorkBuddy 参考及有限工具协议已有落点；[逐平台差距](../product/platform-capability-delivery-matrix.md) | WorkBuddy 真实账号授权、任务／进展／取消完整回写须独立接通，不以相近产品或命令名冒充 |
| INT-005 | Harness／DSH 结构化协议、运行事件与本机自有命令交付已有有限实现；[团队 CLI](../../contracts/team-cli-runtime.md) | 本轮实际提交与独立测试仍是自有协议命令，厂商会话、真实账号、工具版本覆盖及恢复待验 |
| INT-006、INT-011 | 程序／版本／协议探测、现场库存和有界有效期已有实现；[工具运行时](../agents/tool-execution-runtime.md) | 安装、授权、离线、模型可用和实际执行资格仍需分别探测；不能因可执行文件存在就标健康 |
| INT-007 | 官方 MCP SDK 项目读取、受控草稿与当前工具调用已有有限实现；[项目 MCP](../../contracts/application-tools-mcp.md) | 全量远程 MCP 注册治理、完整权限／兼容矩阵和生态验收待办 |
| INT-008、INT-009 | IDE／互操作边界已有设计；[平台工程架构](../architecture/platform-engineering.md) | 通用 ACP 会话与 A2A 发现／委托／状态回传未完整交付，不将现有 MCP 或外部流程桥接等同实现 |
| INT-010 | 工具启停、永久撤销与版本固定已有基础；[工具合同](../../contracts/tool-execution.md) | 可执行插件安装、签名、依赖／兼容检查、升级回滚与隔离宿主生命周期继续待办 |

## 确定性调度：SCH

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| SCH-001、SCH-002 | 不可变流程修订、条件及全部／任一／阈值汇合已有实现；[DAG 内核](../scheduler/engine.md)、[任务依赖](../../contracts/task-dependencies.md) | 完整流程编辑／版本差异／发布预演和所有大图交互仍需逐场景验收；业务图不替代执行 DAG |
| SCH-003、SCH-004 | 持久定时／重试、attempt 与显式补偿已有基础；[可靠性](../scheduler/reliability.md) | 不能把失联当作可安全重跑；完整外部副作用核对、补偿故障和运维接管矩阵仍待验 |
| SCH-005、SCH-006、SCH-008 | 多租户／项目加权准入、能力范围、执行池、资源与预算账本已有有限实现；[增量内核](../scheduler/incremental-kernel.md)、[执行池](../../contracts/execution-pools.md) | 自适应并发调优、真实 CPU／GPU 约束、物理多机公平与目标吞吐不能由同机多进程推断 |
| SCH-007、SCH-011 | 原操作回执、attempt／fence、事件游标及断线恢复已有实现；[调度合同](../../contracts/scheduling-kernel.md)、[本地 journal 诊断](../../contracts/runner-journal-inspection.md) | 完整交互式运行回放、持久隔离原因及跨主机故障演练仍待完善；只读检查不能改变 ACK |
| SCH-009 | 站内风险、持久自动化和事件输入已有基础；[智能规划](../architecture/intelligent-planning.md)、[自动化事件](../architecture/automation-events.md) | 全任务 SLA、持续主动预警、多渠道送达及其误报／漏报评估待办 |
| SCH-010 | 当前工具推荐、固定预览与阻塞诊断已有有限实现；[派发推荐](../../contracts/task-dispatch-options.md) | 现有排序不是全局最优决策；完整执行器选择解释、成本模拟和离散调度预演待办 |
| SCH-012 | 项目策略版本、影响预览和显式确认已有基础；[调度合同](../../contracts/scheduling-kernel.md) | 策略灰度、完整回滚、生产流量观察和长期调优验收待办 |

## 测试、验收与发布：QA

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| QA-001、QA-002、QA-003 | 精确 SHA 候选封存、另一 Runner 独立测试、持久证据及当前验收已有有限实现；[候选合成](../../contracts/candidate-composition.md)、[验收矩阵](../quality/acceptance.md) | 完整需求／版本测试计划、项目策略化静态／格式／测试门禁与用例覆盖矩阵继续完善 |
| QA-004、QA-005 | 人工审阅及受控诊断／权限基础已有实现；[工程 SOP](../delivery/ai-engineering-sop.md) | 全依赖／秘密／漏洞／危险配置扫描、独立 AI 审查质量评估及门禁证据未完整交付 |
| QA-006 | 固定 Git 合成、差异审阅、冲突任务、CAS 快进与代码交接已有有限实现；[仓库集成](../../contracts/repository-integration.md) | 完整远端 PR 身份、分支保护、厂商审核状态及多策略合并待验 |
| QA-007、QA-008、QA-010 | 固定发布范围、制品、审批、本机静态部署／健康／回退已有基础；[发布证据](../architecture/release-evidence.md)、[发布控制面](../deployment/release-control.md) | 生产 CI、签名、容器／远程环境、灰度推广、业务核心指标与真实发布后验证待办 |
| QA-009 | 任务与需求依据变化可使验收失效；[应用交付基线](../product/application-delivery.md) | 微应用／下游接口／回归用例的完整依赖影响分析仍待实现 |

## 知识、记忆与技能：KM

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| KM-001、KM-002、KM-008 | 项目资产、记忆／规范／文本技能版本、审阅发布、模型提案和授权跨项目草稿采用已有有限实现；[项目知识](../../contracts/project-knowledge.md)、[知识提炼](../../contracts/knowledge-intelligence.md) | 全类型资产、自动过期／纠错／删除投影、持续记忆提炼与完整同步待办 |
| KM-003 | 代码目录／文件读取及外部开发工具语义能力已有基础；[IDE 目录](../../contracts/workspace-tree.md) | 平台正式代码符号／调用图索引、代码知识图谱和版本化语义查询未完整实现，外部 Serena 可用不等于平台已交付 |
| KM-004 | Lance／OceanBase 检索适配与来源设计已有代码；[多模态数据](../data/multimodal.md) | 多模态解析、真实向量／全文、当前权限过滤、索引删除／重建与数据迁移矩阵需独立验收 |
| KM-005、KM-006 | 组织共享文本技能目录及固定任务采用已有有限实现；[技能目录](../../contracts/skill-catalog.md) | 可执行技能、签名／依赖、宿主隔离、工具兼容与输入输出安全合同继续待办 |
| KM-007、KM-009 | 模型提案审阅、固定来源与版本核对已有基础；[知识智能架构](../architecture/knowledge-intelligence.md) | 矛盾／过期自动发现及相关性、召回率、错误引用等持续评测未完成 |
| KM-010 | 外部记忆／知识系统扩展边界已有设计；[平台工程](../architecture/platform-engineering.md) | 专业系统连接器的授权、增量同步、冲突、删除与恢复需实际实现和验收 |

## 本轮交付与后续准入

T-DELIVERY-112 对 REQ-008／REQ-010、TEAM-004／TEAM-005、INT-005、SCH-007、QA-001／QA-002 的部分用户旅程提供当前模块支撑：依赖图选择任务 → 固定团队预览 → 实际命令提交 → 独立 Runner 精确 SHA 测试 → 人工退回 → 唯一问题 → 需求草稿；下游继续等待前置正式验收。它不是这些编号全部验收通过，详见[限定报告](../quality/task-delivery-chain-verification.md)。

下一批需求从原 91 项台账对应域细化，补明确验收条件、依赖、负责人和证据；不将此参考自动当作批准执行、发消息、付费模型调用、生产发布或数据迁移授权。完整原规划不可见的部分保持未知；取得新的可见来源后再增补映射，不补造编号。
