# 1.0.0 验证入口

T-MENTIONS-140：[定向与真实接口报告](task-conversation-mentions-verification.md)／[紧凑摘要](task-conversation-mentions-summary.json)。46项独立Go、56项Web、完整类型／7191语言键／合同／构建及12项本机双账号HTTP-CLI检查通过；最终浏览器因审批容量限制待验，未提交／推送。其他阶段结果及桌面在途内容保留。

T-TEAM-EXECUTION-141：[准确执行跟进报告](expert-team-execution-verification.md)／[摘要](expert-team-execution-summary.json)。8项后端、24项前端、类型／7228双语键、生成器及44.17秒隔离构建通过；指定构建输入前后摘要一致。最终浏览器与实现提交按T-COLLAB-144记录；未重复计算139／140结果。

T-REVIEW-142：[报告](task-review-requests-verification.md)／[摘要](task-review-requests-summary.json)。93项独立Go、58项Web、类型／7292语言键／合同／构建和12项真实本机三账号HTTP-CLI-Runner检查通过，最终GUI缺口与实现提交见T-COLLAB-144。历史及他人阶段证据保留。

T-COLLAB-144：[整合报告](collaboration-integration-verification.md)／[浏览器证据](collaboration-browser-summary.json)。36项实际GUI全部通过，准确最终资产、双角色保存v4未发布和两个Review入口草稿保护已复核；源码63bcc39，最终证据独立提交。全平台及未完整后端套件不据此宣称通过。

T-ADAPTERS-139：[限定验证](adapter-connections-verification.md)／[摘要](adapter-connections-summary.json)。状态scoped_verified，覆盖此前139待验状态。复用43项独立Web（11项复验包含在内）、9项Go、类型／7156语言键和合同检查；用户重新授权后新增25项真实GUI通过，覆盖中英、桌面／390px、明暗及键盘代表性流程，修复移动标题被固定页头遮挡。53.28秒隔离构建来自当前合成前端工作区，与既有139后端核验本阶段API和交互；不表示其他在途页面、厂商执行或生产验收。审批容量拒绝、恢复和缺陷证据均保留。

T-WORKBUDDY-143：[协议与持久提交验证](workbuddy-cloud-submission-verification.md)／[摘要](workbuddy-cloud-submission-summary.json)。27项去重Go通过，覆盖准确HTTP、单次准备、并发／重启、未知不重发、保存失败及原终端恢复。仅内部底层，未开放为可派发云工具；正式OAuth、ACP、预算、候选、厂商与多机验收继续待办。本阶段没有UI变更，其他阶段分别验收，以各自当前记录为准。

T-INBOX-138：[个人收件箱限定验证](notification-inbox-verification.md)与[摘要](notification-inbox-summary.json)。37个Go顶层、完整Web1510项、类型／7079双语／合同／隔离构建，以及23项实际浏览器和7项真实Suma CLI检查通过；原始失败及修复保留。仅任务分配来源，完整通知／工具／数据库／规模仍待交付，58／34／92不变。

T-TASKS-137：[统一任务工作台限定验证](task-collection-verification.md)与[摘要](task-collection-summary.json)。当前权限、统一行查询／字面条件、四种排序、三类有界分组、独立列分页、命名偏好、纳秒排序与恢复已定向检查；实际界面证据与最终源码差异分别记录。完整平台／其他数据库／规模和厂商仍待交付，58／34／92不变。

T-MULTICA-137：[任务工作台验证](multica-task-views-verification.md)与[摘要](multica-task-views-summary.json)。限定全项目读取、当前权限、筛选游标、本机视图恢复及真实App／Go／SQLite交互；记录初次失败、修复与实际命令，不扩展为共享视图、全部工具、跨数据库或生产验收。

T-MULTICA-136：[个人协作与命令入口限定验证](multica-collaboration-verification.md)、[摘要](multica-collaboration-summary.json)。当前规范人员任务、精确SQL范围和命令导航接入既有任务／工作区；16个不同Go顶层、新功能Web34项及已完成导航14项、类型／6885语言／合同／最终隔离构建通过。初批worker异常和针对性修复单独留证，浏览器禁用、完整看板／通知／厂商及生产仍待交付；参考SHA139b13a，本轮独立实现，无参考代码或品牌复制。


T-WORKLOG-135：[人员工时限定验证](worklogs-verification.md)、[摘要](worklogs-summary.json)。当前人员明确登记／更正／作废历史、跨项目每日整数上限、同源CLI、原操作恢复和计划父子页失权清除；34个不同Go顶层、17个不同Web用例、类型／6798项语言／合同／最终隔离构建及文档引用通过。原始初次失败与针对性修复留证，未重跑全平台；浏览器禁用、真实其他数据库、HR审批及完整平台继续待验收。

T-DESIGN-135：[架构主文档与操作样稿验证](design-workflows-verification.md)、[摘要](design-workflows-summary.json)。覆盖模型供给与交接、团队分配、事件模拟、修复周期、配置冲突和离线提案；仅本地中文设计样稿与代表性浏览器场景，不是生产集成验收。

T-PLATFORM-DESIGN-134：[全平台设计与样稿验证](platform-design-verification.md)、[紧凑摘要](platform-design-summary.json)。仅核查设计、官方接入依据和独立HTML样稿的浏览器交互；不改变生产实现或全套测试结论，未重复Go／生产前端套件。

更新：2026-10-29。当前只有单一 **1.0.0** 产品基线，当前唯一数据合同为 `suma-initial-quality-v1`；完整平台继续实施。本页按能力指向准确报告，不重复阶段流水、旧测试数字或旧数据库升级说明。详细过程仍可从专项档案与 Git 追溯。

T-EXPERIENCE-131：项目全量交付事实、有权限分页、导航视觉基础和IDE渐进展开，见[限定报告](unified-experience-verification.md)与[紧凑摘要](unified-experience-summary.json)。实际隔离HTTP／SQLite页面已验证中英、明暗、桌面与390px及编辑器草稿保留；前端整合1236项通过。本阶段后端专项通过，全量Go仍有失败与环境限制，报告逐项区分，不能视为全平台或生产通过。

T-TERMINAL-129：真实Windows ConPTY终端、原输入／输出回执、受保护讨论到原生开发预览与明确启动，以及同SHA独立测试、夹具账号显式Review和问题回流，见[当前限定报告](interactive-development-verification.md)及[精简摘要](interactive-development-summary.json)。原浏览器终端夹具派生Name字段误判保留exit1，以17列持久审计和最终Web只读回看133表不变分别补证；商业调用0，Unix完整containment、物理多机、三系统安装和生产仍待验。

T-TERMINAL-130：原终端失联后持久交换先于结果、准确停止／未知输入分开审阅、明确授权候选预览与单独启动，以及真实同SHA独立JUnit／Review／后续基线／退回问题回流，见[当前报告](terminal-recovery-candidates-verification.md)和[摘要](terminal-recovery-candidates-summary.json)。最终浏览器被Work设置禁用，未进行本轮渲染／窄屏验收；只读HTTP副本134表不变不能当作界面通过。完整平台和跨系统／多机仍待交付。

当前范围为需求1–26，58项有限实现、34项待办，共92项。新增需求26／`FND-INTENT-001`尚未实现；验收追踪只推进已有范围，历史专项报告保留当时台账和证据。

| 当前范围 | 验证与限制 |
| --- | --- |
| T-CALENDAR-134 资源工作日历 | [合同](../../contracts/resource-calendars.md)、[报告](resource-calendars-verification.md)、[摘要](resource-calendars-summary.json)：七日工作制／例外、完整区间可用小时分摊、未知与0、原回执及当前资格；36项Go、103项Web证据及类型／中英／构建通过，GUI及真实数据库／HR／工时／生产未验 |
| T-EXPERIENCE-133 项目焦点与开发客户端 | [需求／架构](../product/enterprise-workbench.md)、[报告](enterprise-experience-verification.md)、[摘要](enterprise-experience-summary.json)：服务端准确分类与原依据、独立读取及任务下钻、四类无密钥项目配置导出；8项Go／67项Web及类型／语言／构建通过，GUI与真实厂商／多数据库矩阵未验 |
| T-BASELINE-131 已验收基线上的终端迭代 | [报告](terminal-baseline-development-verification.md)、[摘要](terminal-baseline-development-summary.json)、[合同](../../contracts/terminal-baseline-development.md)：B／M／N来源、受保护当前导入、明确候选授权、独立同SHA测试与验收递归；16项Go／9项Web及本机Windows真实交接通过，GUI未验，全平台仍在实施 |
| T-DISCOVERY-132 执行工具自动发现 | [合同](../../contracts/tool-execution.md)、[中文操作](../development/tool-discovery.md)／[English](../en/tool-discovery.md)、[报告](tool-discovery-verification.md)、[摘要](tool-discovery-summary.json)：授权节点真实上报自动填入、60秒／失权／未上报分态、人工草稿与重试恢复；80项Web、11项Go、合同及隔离类型／语言／构建通过，真实Node探测／Runner／SQLite／中英390暂停登记启用撤权闭环。读取不远程安装或执行，不代表模型健康或生产验收 |
| T-MGMT-131 独立管理工作区 | [设计与SVG](../design/management-and-tool-discovery.md)、[报告](management-workspaces-verification.md)、[摘要](management-workspaces-summary.json)：六领域独立入口、本域导航、实际项目目录与明确来源模型目录；38项专项、类型／语言／构建及中英1440／390实际App夹具界面通过。真实节点自动发现属于后续阶段；模型配置不代表调用健康，估算不代表供应商发票 |
| T-RELEASESET-124 应用版本集与原部署 | [报告](application-release-sets-verification.md)、[摘要](application-release-sets-summary.json)、[操作](../development/application-release-sets.md)：准确成员、独立制品及闭包，原计划逐项批准／执行／实际健康，混合包测试与JUnit分别门禁；同机真实静态演示、中英390与原计划双向追溯，完整跨应用与生产仍待办 |
| T-APPLICATION-123 应用版本与依赖影响 | [报告](project-applications-verification.md)、[摘要](project-applications-summary.json)：真实集成来源、不可变代码版本、精确人工固定、声明覆盖、轻量图与反向影响；实际双Worker／Git／JUnit／fixture Review及中英390只读界面。完整跨应用与生产能力仍待办 |
| T-TOOLUX-130 执行工具交互深化 | [设计](../design/execution-tools-workspace.md)、[当前报告](execution-tools-refinement-verification.md)、[摘要](execution-tools-refinement-summary.json)：状态与操作优先、技术依据折叠、三段表单、本地文件导入／错误恢复与原回执反馈；70项专项、隔离类型／构建与中英375／390界面通过，真实探测文件／Runner登记启用暂停撤权验证。前阶段全套既有失败保留在[T129报告](execution-tools-workspace-verification.md)，不代表模型健康或生产验收 |
| T-PROJECT-121 正式项目风险闭环 | [报告](project-risks-verification.md)、[摘要](project-risks-summary.json)、[中文操作](../development/project-risks.md)／[English](../en/project-risks.md)：正式风险与模型建议分别保存，来源去重、当前人类负责人、评估与版本关联、复核／关闭／重开；接口规范字段与子流程门禁专项修正，实际Node／Git／独立Worker同SHA测试及正式Review、概览和中英390界面分别留证。复杂来源、portfolio及生产验收仍待完成 |
| T-WORKSPACE-120 Go宿主工作区操作 | [报告](workspace-operations-verification.md)、[摘要](workspace-operations-summary.json)、[操作](../development/workspace-operations.md)／[English](../en/workspace-operations.md)、[架构](../architecture/workspace-operations.md)：真实调用与原效果、远程CAS、独立宿主Outbox及重启元数据恢复；复用私有／Pool进度和原attempt分页。15项Go、5项Web聚焦检查及一次真实原生／HTTP／Journal／封存历史旅程；中英文与390键盘操作实见，刷新恢复原attempt上下文，其后事实请求完成未观察。不代表完整Octop、厂商、多主机或生产验收 |
| T-QUALITY-119 受保护修复、回归与关单重开 | [当前报告](issue-repair-verification.md)、[精简摘要](issue-repair-summary.json)、[中文操作](../development/issue-repair.md)／[English](../en/issue-repair.md)、[合同](../../contracts/issue-repair.md)：唯一需求回流与新修复任务、原失败case／selector、固定发布计划、正式产码候选与不同Worker独立测试、当前Review后人员关单及保留历史的重开；复用固定`command/text-v1`档案和私有／Pool资格，通用记录旁路拒绝。简洁中英文问题面板、实际OS／HTTP执行、接口、权限、恢复及当前数据合同各以报告记录为准，不推断所有检查通过；完整Issue／Bug治理、复杂候选来源、应用／发布测试和生产仍待交付 |
| T-MODEL-118 原生／Octop统一模型网关 | [当前报告](model-gateway-verification.md)、[精简摘要](model-gateway-summary.json)、[架构与SOP](../architecture/model-gateway.md)：共同ModelBinding／冻结报价／预算、真实HTTP／SSE、逐次剩余总输出许可、持久原操作与原响应回放；本机原生真实Claim与费用账本、官方Harness／ACP六工具工作树和同SHA独立JUnit、取消与journal分别留证。中英文模型／报价入口与可展开事实的组件、实际界面及最终数据合同检查各以报告为准；已知费用为运营方费率估算，非供应商账单硬帽，历史通过不继承为当前全套通过 |
| Octop 项目MCP与统一交付读取 | [指南](../integrations/octop.md)、[阶段报告](octop-integration-verification.md)与[摘要](octop-integration-summary.json)：17项共享目录、2项新增API只读、分页／当前权限、项目专用配置及中英文实际界面；此报告仅证明业务MCP范围，当前Harness／ACP编码与隔离工作树证据见[T-MODEL-118报告](model-gateway-verification.md)。完整Octop CLI／GUI和厂商账户仍未验 |
| T-QUALITY-115 自动化计划与逐例证据 | [阶段报告](test-plans-verification.md)、[精简摘要](test-plans-summary.json)、[中文指南](../development/test-plans.md)／[English](../en/test-plans.md)、[合同](../../contracts/test-plans.md)、[证据架构](../architecture/quality-evidence.md)：当前代码覆盖任务计划不可变修订、固定候选 JUnit 原件采集、当前 attempt 逐例投影、正式 Review 附加门禁和受保护失败用例问题回流；实际隔离双Runner／HTTP／Node报告旅程与定向检查通过，真实界面因浏览器工具无法确认URL仍未验，以报告为准。仅自动化用例，完整测试／缺陷生命周期、应用／发布测试和平台全范围仍待实现，不改变58／34／92统计 |
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
| 当前数据合同与共享产品版本 | 产品保持1.0.0，当前唯一数据合同为 `suma-initial-quality-v1`；旧网关标记、不匹配或混合声明、未知组件及未登记领域表在初始化写入前拒绝，原数据保留，不迁移／回填，新实例另选独立空目录／数据库。当前新库／重启／拒绝写入检查见[T-QUALITY-119报告](issue-repair-verification.md)及[摘要](issue-repair-summary.json)；[T-MODEL-118报告](model-gateway-verification.md)、[原初始化报告](initial-version-verification.md)保留原合同及源码结果，不证明当前数据合同已验 |
| 安装版 Codex 实际编码 | [准确源码与环境的专项报告](codex-app-server-verification.md)：安装版 CLI 配合自有 Responses 端点；配置 ACK 不是提供方身份，提醒不等于硬预算；不将旧切片作为本轮厂商验收 |
| 其他领域能力 | [交付状态](../delivery/status.md)与[完整文档索引](../README.md)列出各域当前实现和独立证据 |
| 未完成范围 | [完整实施计划](../delivery/implementation-plan.md)、[产品基线](../product/v1-baseline.md)：P0–P7及扩展需求1–26全部保留；[统一智能输入／配置](../architecture/intelligent-operations.md)仍待实现 |

检查按变更影响选择，失败修正后仅复验相关路径。源码完成、协议夹具通过、真实执行、正式业务验收与生产验收分别记录；不累加重叠测试为虚构同轮全套。费用未定价、partial／缺失用量、未知执行结果都不能推断为零或成功。

当前 Windows 构建与本机浏览器证据不证明 OceanBase／PostgreSQL 实集群、目标吞吐、物理多机、跨平台安装、厂商账户、生产模型、签名、支付或完整平台验收。构建清单、日志和私有夹具留在忽略目录，Git只收必要源码、合同、设计与精简证据。
