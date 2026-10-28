# 分享规划的采用与当前能力映射

状态：1.0.0，T-OCTOP-116 四份可见规划逐项采用；来源不作为完整平台完成或新增运行证据。需求26新增一个待办后，正式进度为 **58 项有限实现、34 项待实现，共 92 项**，见[实施计划](../delivery/implementation-plan.md)与[当前交付状态](../delivery/status.md)。历史T-DELIVERY-112报告保留当时范围。

## 第一份来源与可见范围

来源：[速码 suma：下一阶段需求深化与智能化增强开发规划](https://chatgpt.com/s/t_6ae19e1f3af481918d11132ccf5119fe)。本轮最初 Web、Chrome 和会话读取未取得正文；随后通过 Firecrawl 普通 basic 代理访问同一明确链接，HTTP 200，成功获取当前分享可见正文。

只读来源文件为项目忽略目录 `.cache/e2e/delivery-112/shared-reference-source/share-body.md`，13033 字节，SHA-256：`1135aeaa9519c56d5daa95d6d0ae37fda944e0eeb4ecaae7edc361277086fc30`。该摘要绑定实际获取的 Markdown，而非源码、运行结果或完整原会话。

当前可见部分覆盖需求、团队、集成、调度、质量及知识六个领域，共 64 个编号：REQ 10、TEAM 10、INT 12、SCH 12、QA 10、KM 10。页面在 KM-010 后的“知识回答…”未完句处结束，并显示 Continue generating。**已取得当前可见正文，原规划末尾仍不完整**；不推断后续章节或编号。知识编号采用来源实际的 `KM`，不另造 `KN` 别名。原文是基于旧独立 0.2 报告的规划，明确未读取当时 Windows 最新源码，因此不能作为当前 1.0.0 的实现证明。

## 第二份来源与可见范围

来源：[速码 suma研发平台：完整需求基线V1.0与智能化升级方案](https://chatgpt.com/s/t_6ae1a95216648191961a25087e713f3a)。普通抓取的Markdown最初只有空正文与Continue generating；随后从同一页面已取得HTML中的序列化正文恢复可见内容，不执行网页脚本。原件保留在忽略目录 `.cache/e2e/coverage-113/source2.html`，提取正文为 `.cache/e2e/coverage-113/source2-body.md`，10,174字节，SHA-256：`038cb2784ece5c79a710b9c8eca7d083b4c63a9a5951f6163183d9fc2efbcb9e`。

实际可见正文新增需求26：全平台智能输入、智能补全、自动配置、CLI驱动创建和低人工干预流程；给出S01–S12子系统、智能输入场景、结构化流程、五类配置职责及八项验收要求。正文止于第四节参考平台表的“腾讯云 Octop”空单元格行，**第四节余文、第五节及以后不可见**，不补造后续方案或实现证据。题中的V1.0是来源文档名，速码产品仍为当前单一1.0.0。

## 第二份规划的采用与12子系统映射

采用统一入口、结构化意图、可审阅差异、有限追问、真实后验核对及当前权限的方向。沿用现有Go应用服务、单次智能规划、配置领域、模型／工具端口、CLI与唯一Go DAG；来源提出的“统一智能操作引擎”不成为平行dispatcher。Pydantic、Rich／Typer只是来源示例，不替换现有Go合同与Bubble Tea。五类配置对象为[目标职责](../architecture/intelligent-operations.md)，不声称已新增通用配置服务。低风险自动采用必须受当前策略与实际主体权限约束，审批、秘密、预算、原操作与未知效果恢复保持。

| 来源子系统 | 当前领域／需求映射 | 当前缺口与采用边界 |
| --- | --- | --- |
| S01 统一工作台与智能输入 | 工作台、当前项目上下文；需求4／26 | `FND-INTENT-001`待办：全平台意图目录、智能补全、默认值来源及统一预览／采用；现有单场景提案不等于完成 |
| S02 项目与产品管理 | 项目、计划、里程碑；需求2／10／25 | 复用治理与计划；子项目／微应用／依赖及版本集按[应用交付基线](../product/application-delivery.md)待实现 |
| S03 需求与设计管理 | 需求、三段设计、发布／采用；需求1／2／10 | 复用[正式设计合同](../../contracts/requirement-design.md)；完整想法、PRD／UED编辑与模型设计仍待办 |
| S04 团队与组织管理 | 组织、团队、人／Agent／工具资源及当前授权；需求3／12 | 团队关联不授予项目权限；自定义角色／菜单及企业SSO仍待实现／验收 |
| S05 智能任务与DAG引擎 | 智能规划、业务依赖及唯一Go调度；需求7／9／12／13 | 规划生成可编辑建议；完整自主策略、长期恢复、物理多机与目标规模不能由当前本机证据推导 |
| S06 外部开发工具与IDE | 工具端口、原生／Codex／OpenCode、Monaco；需求1／9／15／17／22／23 | 固定版本与真实适配证据分别记录；完整厂商会话／PTY／LSP／DAP／Git写待办，不增新工具派发中心 |
| S07 质量、测试与交付 | 候选、独立测试、Review、发布；需求8／12／15／24／25 | 本轮[验收追踪](../../contracts/requirement-traceability.md)仅当前需求版本／序号到正式采用和运行；完整TestPlan／TestCase及应用版本／发布矩阵待办 |
| S08 知识、记忆与技能 | 项目资产、共享技能、模型提炼；需求10／11／16 | 保留原件／索引／技能边界及当前重授权；向量索引、自动同步、代码语义图与可执行技能待办 |
| S09 模型与智能决策 | 模型选择、工具推荐、冻结费率及账本；需求14／18／21 | 完整智能路由、报价／硬许可、评测与商业账单待办；缺失用量／费用不记零 |
| S10 DevOps与智能运维 | 持久事件、自动化、失败分诊及本机发布；需求5／7／13／24／25 | 复用原operation与实际目标核对；生产CI、告警渠道、受控自动修复及生产恢复待验 |
| S11 平台治理与商业化 | 身份、审计、预算／配额；需求3／5／14 | SaaS商业租户、SSO、支付及财务结算继续待办，现有运行用量不等于正式账单 |
| S12 多端与生态 | Web／桌面／CLI、项目MCP及扩展端口；需求5／17／19／20／23 | 移动端、消息渠道、A2A／ACP、完整插件与多系统安装逐项实施，协议名称不作为接通证明 |

需求26以一个明确新增待办纳入[产品基线](../product/v1-baseline.md)和P0 `FND-INTENT-001`；S01–S12是已有范围归并，不另外新增12个完成项或重设菜单。智能输入八项验收及配置／恢复边界见[架构与AI SOP](../architecture/intelligent-operations.md)。两份来源的优先级不替代P0–P7实施阶段。

## 采用决定

1. 采用“先闭环、再扩展”和每项具备持久化、权限、恢复、证据的完成定义。来源 P0／P1／P2 是业务优先级，与本项目 P0～P7 实施阶段分开，不重写台账或重算完成数。
2. 采用结构化 SPEC、验收条目、版本和依赖的方向，继续使用当前需求设计及固定任务上下文；本轮[当前版本验收追踪](../../contracts/requirement-traceability.md)不冒充跨版本标准身份、完整逐用例／版本／发布矩阵。
3. 规划层提出可编辑建议；Go 调度器独立核验权限、依赖、预算和资源，保持平台唯一派发权。业务任务依赖与一次执行 DAG 分开，智能体不能绕过人工门禁。
4. 采用多身份、任务委托和版本化工具合同方向，沿用当前封闭 Actor 与项目凭据，不让人、Agent、Runner 或团队关联自动扩权。优先成熟官方 SDK／CLI／MCP，不访问未授权内部接口。
5. 保留精确代码／测试／验收依据、知识与记忆与技能的领域边界；声明工具能力、CLI 退出成功、测试节点成功、业务验收与生产发布分别记录。
6. 此前 T-DELIVERY-112 推进有限关键闭环和本地安全诊断，本轮 T-TRACE-113 继续当前版本验收追踪。下表覆盖第一份来源全部可见编号；未覆盖能力仍在正式计划中，没有将规划内容导入运行中的用户需求池或自动创建任务。

## 需求工程：REQ

| 来源编号 | 当前模块／合同与状态 | 验收缺口及下一步 |
| --- | --- | --- |
| REQ-001 | 项目、成员、归档及权限已有有限实现；[治理](../product/governance.md)与[应用交付基线](../product/application-delivery.md) | 子项目、产品线、微应用和依赖应用的统一全生命周期继续待办 |
| REQ-002 | 固定流程版本与发布已有基础；[调度合同](../../contracts/scheduling-kernel.md) | 项目／研发模板目录、复制、审批、回滚及完整复用旅程未完整实现 |
| REQ-003、REQ-010 | 问题、失败诊断、显式需求回流已有有限实现；[失败分诊](../../contracts/failure-diagnosis.md)、[任务交付链路](../../contracts/task-delivery-chain.md) | 独立想法池、所有告警／生产渠道自动收集与归类、持续去重策略待验；回流不能自动批准需求 |
| REQ-004、REQ-005 | 需求、PRD／体验／技术设计、发布与任务绑定已有版本化实现；[需求设计](../../contracts/requirement-design.md)、[任务设计依据](../../contracts/task-design-context.md) | 完整产品状态机、结构化非功能要求、全设计编辑与发布版本统一追溯继续完善 |
| REQ-006、REQ-009 | 可编辑任务提案、单次模型规划与人工采用已有有限实现；[智能规划](../../contracts/intelligent-planning.md) | 全自动需求拆解、业务价值／风险／成本综合排序及质量评估不能由现有单次规划推断 |
| REQ-007、REQ-008 | 依据变化失效、当前任务／运行／验收／问题下钻及[当前需求版本追踪](../../contracts/requirement-traceability.md)；[交付链路](../../contracts/task-delivery-chain.md) | 当前标准按版本／序号连接正式发布和受保护采用，不推断新版本位置；完整逐用例／应用版本／发布矩阵、跨微应用／接口／技能影响分析待办 |

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
| QA-001、QA-002、QA-003 | 精确 SHA 候选封存、另一 Runner 独立测试、持久证据、当前验收及[需求追踪读取](../../contracts/requirement-traceability.md)；[候选合成](../../contracts/candidate-composition.md)、[验收矩阵](../quality/acceptance.md) | 仅当前run测试节点事实，不是case覆盖；完整需求／版本测试计划、项目策略化静态／格式／测试门禁与用例覆盖矩阵继续完善 |
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

## 限定交付与后续准入

T-DELIVERY-112 对 REQ-008／REQ-010、TEAM-004／TEAM-005、INT-005、SCH-007、QA-001／QA-002 的部分用户旅程提供当前模块支撑：依赖图选择任务 → 固定团队预览 → 实际命令提交 → 独立 Runner 精确 SHA 测试 → 人工退回 → 唯一问题 → 需求草稿；下游继续等待前置正式验收。它不是这些编号全部验收通过，详见[限定报告](../quality/task-delivery-chain-verification.md)。

T-TRACE-113 按当前需求版本及1起始序号读取标准 → 当前发布提案 → 受保护采用 → 精确运行／测试节点／正式验收，实际检查和界面范围由[专项报告](../quality/requirement-traceability-verification.md)记录。它仅推进REQ-008／QA-001部分基础，不完成完整用例、应用版本／发布追踪或新增需求26。

下一批需求按当前92项台账对应域细化，补明确验收条件、依赖、负责人和证据；不将此参考自动当作批准执行、发消息、付费模型调用、生产发布或数据迁移授权。两份原规划不可见部分保持未知；取得新可见来源后再增补，不补造编号。

## 2026-10-28 补充：第三、第四份分享与 Octop

状态：T-OCTOP-116 来源采用补充。保留前文各阶段的原范围与历史事实；当前仍为 **58项有限实现、34项待实现、共92项**，P0–P7及需求1–26全部保持。以下是新的设计来源与采用决定，不是新增完成项，也不表示各参考平台的全部能力已经接通。检查与真实场景以[Octop专项报告](../quality/octop-integration-verification.md)及[摘要](../quality/octop-integration-summary.json)为准。

### 第三份：专业化增强实施蓝图

来源：[速码 suma 专业化增强实施蓝图](https://chatgpt.com/s/t_6ae1ab6e8dbc8191932d54994456a29b)。网页工具未直接取得正文后，通过同一用户指定地址的普通只读HTTP获取原HTML，状态200；只对其中序列化 `fallbackMarkdown` 做JSON字符串解码，不执行页面脚本。原件在项目忽略目录 `.cache/e2e/octop-116/share-3.html`，实际可见正文为 `share-3-body.md`，10,599 UTF-8字节，SHA-256 `75cce3fa8054555776577e7a019ff80be6db7a4511d0c36d9b2f557a73643063`。

可见范围：总体原则与分层架构、统一智能工作台的UX-001…UX-010、项目／子项目／微应用、需求／PRD／UED／SPEC／ADR及组织／团队／人和Agent职责。正文止于3.4组织团队权限说明，接三个“可继续说明”提议；没有取得其后章节，不能将提议当正文或要求用户重新确认已授权范围。

### 第四份：深度设计 V2

来源：[速码 suma 深度设计 V2：从智能研发工作台升级为可扩展的智能研发操作系统](https://chatgpt.com/s/t_6ae1c83c15d481919f8406398a772b2a)。同样取得HTTP200原HTML后仅JSON解码可见正文，原件 `.cache/e2e/octop-116/share-4.html`、正文 `share-4-body.md`，11,829 UTF-8字节，SHA-256 `b6a0fa8ec712f9c68ab6a6ae287f03234be1f47967ff13d5afba8f4c95ba9f87`。

可见范围：任务驱动工作台、统一领域实体／状态／事件、智能理解与拆解、先硬约束后软评分的模型／工具路由、有预算和次数上限的重新规划、统一插件／技能／工具／MCP／A2A、五类可视配置与部分页面状态表。正文止于第六节表格第二行的项目总览“空项目”项，接三个继续提议；完整后续页面矩阵及章节不可见。来源的V2是该设计文档名，不改变速码单一1.0.0产品基线。

### 采用与尚未完成的范围

| 新来源的方向 | 当前采用与对应领域 | 保持明确的缺口 |
| --- | --- | --- |
| UX-001…UX-004：智能输入、上下文、补全、结构化解析 | 复用需求26、统一智能输入与固定项目上下文；[智能操作架构](../architecture/intelligent-operations.md) | 既有有限草稿计划不等于全平台自动配置／全部输入场景完成 |
| UX-005…UX-008：预览、差异、明确执行及快捷操作 | 原DAG、领域命令、固定计划、当前权限和正式回执；本阶段连接器只生成无秘密人工合并片段 | 模型输出不能自行授权、派发、批准、写测试结果或生产变更 |
| UX-009…UX-010：CLI与中断恢复 | 原CLI／Runner、稳定operation、凭据撤销及未知结果核对 | 完整CLI项目创建／同步、所有厂商会话恢复继续待办，不把重试变成重复资源 |
| 项目、微应用与产品设计 | 现有项目、版本化需求设计、任务依赖及[应用交付基线](../product/application-delivery.md) | 子项目／微应用完整生命周期、PRD／UED编辑、接口与发布影响矩阵仍待完整实现 |
| Profile／Runtime／成员关系分离 | 复用当前团队、专家配置、工具profile和Worker事实；角色不自动等于运行进程或项目权限 | 全部专家自主协作、SSO、跨组织与物理多机验收继续待办 |
| 十二领域及统一事件约束 | 采用Identity、Portfolio、Product、Collaboration、Orchestration、Execution、Integration、Intelligence、Knowledge、Quality、Delivery、Governance的职责方向 | 不为来源名称复制现有服务、调度状态或操作回执，实际领域能力仍逐项交付 |
| 硬约束优先、再软评分 | 现有权限、路由、工具能力、资源、预算先核验；排序只能作为候选解释 | 完整预测路由、历史效果评分和持续优化没有由本次MCP读取获得实现证据 |
| 有上限重新规划 | 采用按任务预算、次数、截止时间及未知效果边界审阅的方向；权限拒绝不能靠换工具绕过 | 来源建议初期最多2次不是全平台已生效策略，本阶段不新增自动重规划引擎 |
| 通用扩展与Octop | 同一项目MCP服务新增真实任务交付与有界逐例质量读取；复用已有草稿工具；[Octop指南](octop.md)与[集成架构](../architecture/octop-integration.md) | ACP编码、A2A、完整插件／技能／记忆／IM／桌面与厂商实际实例另行准入，不由“配置存在”推断健康 |
| 五类配置与渐进披露 | 同一DeveloperConnection面板选择Codex／Octop；保留项目、账户、组织、语言和可撤销凭据边界 | 全平台默认／组织／项目／任务继承、配置审计／差异／回滚的统一中心仍待交付；低层级不能放松安全策略 |

第三份中的FastAPI、Celery、Alembic等是参考实现示例；当前已批准的Go控制面／唯一DAG／Runner与初始库设计保持。第四份的插件版本与兼容范围属于插件协议建议，不引入速码产品历史兼容分支。采用可验证的产品、领域、交互与扩展约束，不能为追求“全部参考能力”复制互相冲突的控制面。

### Octop 官方代码采用证据

本次通过已连接GitHub只读能力核验，`TencentCloud/Octop`当前main仍为[`890111b7879665840414ca28ab168b05b0db0921`](https://github.com/TencentCloud/Octop/commit/890111b7879665840414ca28ab168b05b0db0921)，版本1.0.3b1；[README_CN](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/README_CN.md)与[MIT许可](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/LICENSE)已读。没有把未变化的提交说成一次新的代码升级。`octop-harness`相关ACP源码固定为[`68b5423bb80ecef7fca57ba5a37d9a2bdcbf12d2`](https://github.com/TencentCloud/octop-harness/commit/68b5423bb80ecef7fca57ba5a37d9a2bdcbf12d2)。原始资料、代码片段与摘要清单保留在忽略目录 `.cache/e2e/octop-116/`。

优先复用Octop已有Streamable HTTP自定义MCP：项目私有server、字面Bearer占位符、固定预期主体与语言、人工合并保留其他配置。新增读取只供当前API项目读权限，Agent仍原四读；无秘密片段不是已连接证明。官方ACP的`session/new.cwd`未直接决定实际Agent工作区，`OCTOP_HOME`／workspace_dir／backend.root_dir需要真实配置和范围核验；取消事件也不是进程树停止证明。上述差距写入[ACP后续准入设计](../architecture/octop-integration.md)，不以普通命令或MCP接通替代编码执行验收。

T-APPLICATION-123进一步采用已保留的share3微应用／代码版本／依赖方向：实现见[正式应用领域](../architecture/project-applications.md)及[限定证据](../quality/project-applications-verification.md)。本轮核对两份正文与原摘要一致，不补写未见后文；应用层级继续受原项目ACL约束，尚不代表独立子项目权限或完整版本集。
