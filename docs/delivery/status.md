# 当前交付状态

当前开发版本 **1.0.0**；新设计采用当前最佳合理结构，不增加历史兼容层。[版本与扩展需求基线](../product/v1-baseline.md)。

## 当前 T-P4-038：执行文件编辑、单次CAS保存与准确恢复（限定本地验证）

在037真实只读工作区上补齐完整小文件编辑与保存。当前原调用人写权限、task当前run／依据及Pool授权核验，固定原SHA／新内容SHA／scope／策略；只签发一次10秒execute许可，超时后不重新派发。Runner复用活动os.Root和原CAS账本，未知／prepared不补rename，只有准确原回执或持久未发生屏障才能定论。

浏览器与CLI均贯通。两个真实隔离标签页完成先保存、旧SHA冲突、保留草稿、人工比较合并、显式采用基线后再保存：提交SHA49918b20…，generation3。最近保存按原凭据准确加载3项元数据；中英、刷新、左右编辑区名称与模型退出生命周期已实际验证。旧ACK不覆盖新草稿，原POST／inspect超时保持原key和正文／版本。

唯一Control→Worker→原生Node实例联合PASS，1197.21秒含10秒许可故障与GUI等待；服务正常关闭、源HEAD未变、0任务模型／厂商调用、0正式人工验收。实际发现的ReactMonaco模型销毁和初始ARIA重置已修复，最终关闭前相关console为0。新依赖0；缓存、凭据、截图、日志和构建产物不提交。

仅当前原生attempt现有UTF-8文本、新旧均≤48KiB；创建／删除／force／Git写、PTY／LSP／DAP、完整远程IDE、实体多机／TLS、分布式数据库／容量和生产继续实施。主035预览尚未升级；下一步先核对实际进程／数据再交付当前已验证版本，继续完整P0–P7与需求1–22，不将此局部阶段当作完整平台。

[产品交互](../product/remote-workspace-write.md)、[合同](../../contracts/workspace-write.md)、[架构](../architecture/remote-workspace-write.md)、[AI SOP](../operations/remote-workspace-write.md)、[English](../en/remote-workspace-write.md)、[实际验证](../quality/workspace-write-verification.md)、[摘要](../quality/workspace-write-summary.json)。

## 历史 T-P4-037：执行工作区只读观察与出站回执（限定本地验证）

在036固定模型／参数／预算→真实原生执行的基础上，本阶段增加当前原生attempt的只读工作区链路。浏览器／CLI通过同一控制面申请目录或文本，Runner复用活动os.Root与现有出站连接读取；当前原调用凭据、项目、attempt／fence／worker、真实工作区策略摘要以及Pool当前双边授权分别核验。独立读取租约、准确原回执与迟到拒绝不干预DAG、模型或正式验收。

工作空间提供“本地／执行”两个来源，复用只读Monaco，完整中英文、日期时分秒、文件摘要、方向键、刷新和准确运行链接已在真实本机进程的隔离界面验证。Control没有本地文件绑定。空队列轮询不再创建回执记录；实际lease／ACK保持持久幂等。

最终Go联合执行5.72秒通过；长GUI等待最早读取正确expired／无正文被旧cancelled-only测试断言报FAIL，原日志保留，断言修正与最终结果分别记录。0厂商／模型调用、0正式测试／人工验收操作，无新增依赖。主035预览未重启或更新；缓存、数据库、凭据、截图不提交Git。

本阶段只读list/read；远程写入、Git写、PTY／LSP／DAP、完整远程IDE、物理多机／TLS、分布式数据库与规模、厂商和生产验收继续实施。Control逻辑到期清除与Runner journal后续checkpoint裁剪不等于所有磁盘副本同步擦除。

[产品与交互](../product/remote-workspace.md)、[合同](../../contracts/workspace-observation.md)、[架构](../architecture/remote-workspace-broker.md)、[AI SOP](../operations/remote-workspace.md)、[English](../en/remote-workspace.md)、[限定验证](../quality/workspace-observation-verification.md)、[摘要](../quality/workspace-observation-summary.json)。

## 历史 T-P4-036：模型配置、预算与真实原生执行（限定本地验证）

当前授权库存中的模型版本、封闭参数和本次预算进入同一固定Plan／节点／attempt，实际原生CLI请求所选模型，再按原准确运营方费率回写用量与账本。自有协议联合执行40 Token／57 CNY微元，预留归零；错误身份、缺证和超界保留核对，新绑定不自动重试。原任务界面实际保存2次调用／100输出／20000 Token／CNY0.02的固定预览，中英、键盘和刷新保持；未Start或代替正式验收。

CLI新增models options及基于官方SDK的developer check真实MCP初始化／工具列表／项目只读调用。费用未定价保持未知；每工具当前仅一份匹配报价。外部CLI独立模型选择、专家团队／修复入口、远程工作区broker与完整IDE、真实厂商及生产／规模验收继续实施。主035预览未修改或重启。

[合同](../../contracts/model-execution.md)、[PRD](../product/model-execution.md)、[架构](../architecture/model-execution.md)、[AI SOP](../operations/model-execution.md)、[限定验证](../quality/model-execution-verification.md)、[摘要](../quality/model-execution-summary.json)、[English](../en/model-execution.md)。

## 历史 T-P4-035：实时Git工作区与原生执行限额（1.0.0，限定本地已验证）

新增当前项目的Git状态和工作树／暂存区准确差异；复用Monaco只读DiffEditor，切换变更保留原草稿，CtrlCmd+S沿用准确回执与原操作恢复。Git读取核验身份、目录、配置与当前元数据，不修改索引或提交；暂存删除后同路径重建等真实边界已修正。原生执行的步数和输出限额在创建工作区或进程前严格校验，并准确传入SDK；可写模型选择仍为设计。

专属隔离Git实际验证磁盘2→3、暂存1→2、未保存草稿9及一次Ctrl+S后的2→9；中英文／键盘／刷新与14项界面检查通过。真实使用发现未启用配置提示错误后，仅修正该分支，最终4419对语言键及必要定向检查通过，未重复Git旅程、模型或正式任务测试。最终135资产已进入主预览；原186表完整事实在切换前后保持，界面读取后182张非锁表完整行保持，34个内部锁sequence各增加23。当前原任务v3、18authority／0Grant／18session与1failed／2succeeded保持；无新凭证。

见[PRD](../product/workspace-git.md)、[架构](../architecture/workspace-git.md)、[交互](../design/workspace-git.md)、[AI SOP](../operations/workspace-git.md)、[验证](../quality/workspace-git-verification.md)、[摘要](../quality/workspace-git-summary.json)及[English](../en/workspace-git.md)。主预览准确显示本地目录未启用，不绑定用户目录；正向Git仅在隔离夹具验收。完整PTY／LSP／DAP、远程工作区、生产模型／费率绑定、真实多机／厂商和P0–P7全部范围继续实施。

## 历史 T-P4-034：当前工具推荐与IDE保存恢复（限定本地已验证）

任务派发现在按当前权限、任务／流程版本、工具真实库存与路由显示兼容选项；明确采用只填写原选择，保留准确档案版本与摘要，再进入既有固定预览和启动门禁。模型仍由工具配置固定，费用未知，不制造最优／零费用评分。IDE保存只确认准确路径、内容和UTF-8摘要；错误200保留原操作，恢复后不覆盖后来输入的新草稿。

隔离副本实际完成中英文推荐→采用→准确计划预览／刷新，以及真实Monaco错误回执→原key恢复→新草稿新key保存。3次HTTP对应2项持久保存；任务仍待办、未Start，无新运行／模型／厂商调用／正式候选测试／凭证。15项最终冷态核对通过；原始失败与纠正依据完整保留，未重跑历史开发或完整套件。

主预览034正常运行；033旧服务正常退出，186表完整schema与行在升级前后准确保持，无新schema。原会话中英／键盘／刷新核对后，182张非锁表保持，4张内部锁表34行仅sequence各增加28；18authority／0Grant／18session及原1failed／2succeeded不变。135本地资产摘要准确，实际抽查页面入口与JS两项，未重复抓取全部资产。

见[PRD](../product/task-dispatch-options.md)、[架构](../architecture/intelligent-capability-selection-notes.md)、[交互](../design/task-dispatch-options.md)、[AI SOP](../operations/task-dispatch-options.md)、[合同](../../contracts/task-dispatch-options.md)、[验证](../quality/task-dispatch-options-verification.md)、[摘要](../quality/task-dispatch-options-summary.json)、[English](../en/task-dispatch-options.md)及[逐平台差距矩阵](../product/platform-capability-delivery-matrix.md)。完整P0–P7、模型选择／成本、完整IDE、真实厂商与分布式验收继续实施。

## 历史 T-P4-033：修复计划修订与运行溯源（限定本地已验证）

T-P4-033／1.0.0 已完成限定本地验证。修复任务支持新计划版本重选工具与流程；原冲突、来源顺序、失败运行及回执保留。只有宿主类型化证明的执行前失败可准备替代计划，未知或已发生效果的运行继续沿原操作核对。新计划仍须单独启动、独立测试和明确验收。

实际复用已验收 A／X，完成待启动修订→真实启动前失败→明确切换至 B→A 独立测试准确候选 `cd9c611ac90ea3531247ec2340b74b083bdc1ef4`→经用户许可的隔离测试账号 Review→绑定仍未开始的 D。共3项新运行、4项正式检查、6次自有loopback模型协议响应和6次真实工具调用，无来源重复开发／复测，无厂商调用。启动证明准确记零Token且费用未定价；健康模型执行用量未报告，不推断为零。

最终中英文、键盘历史、准确计划深链／刷新、Review打开时刷新及正常冷回读通过。主预览033已正常恢复：原184表结构和完整行经WAL感知备份、前向迁移保持，仅新增2空表和1显式索引至186。授权浏览器读取只令4张内部锁表34行sequence各增加24，身份字段和其余182表完整行保持；18authority／0Grant／18session和原1failed／2succeeded不变。

见[PRD](../product/candidate-repair-revisions.md)、[架构](../architecture/candidate-repair-revisions.md)、[AI SOP](../operations/candidate-repair-revisions.md)、[交互](../design/candidate-repair-revisions.md)、[CLI](../../contracts/candidate-repair-revisions-cli.md)、[限定验证](../quality/candidate-repair-revisions-verification.md)、[摘要](../quality/candidate-repair-revisions-summary.json)、[English](../en/candidate-repair-revisions.md)。真实厂商、多物理机器、分布式数据库／规模、完整IDE、智能工具／模型选择和完整P0–P7继续实施。

## 历史 T-P4-032：冲突修复与受控恢复（限定本地已验证）

固定已验收来源发生真实冲突后，明确创建独立修复任务；原生工具完成有序合并与受控编辑，独立节点测试准确候选，再通过显式 Review 交接到未开始任务。已知失败可在原运行中受控重试，保留原尝试并生成新尝试与租约；结果未知、来源变化和非幂等节点保持门禁。

本地实际形成3项新运行（来源X、原冲突、修复）、8项正式检查和2次测试账号明确评审；修复首次在工作区和模型启动前失败，修正连接后由界面重试同一次运行，第二次尝试及独立测试通过。准确提交 `21cc51598692c6121e7324f382757f45cd0a1e9e` 已绑定仍未开始的D。仅使用6次自有loopback模型协议响应与6次真实工具调用，用量未报告，不推断零Token；未调用厂商。

中英文、键盘详情、刷新、原失败与唯一修复回链及最终Control只读冷回读均通过，不重复模型或正式测试。主预览032／1.0.0正常：保留183旧表结构及迁移前全部行，新增1空表至184，无新显式索引。浏览器授权读取仅令4张内部锁表34行的sequence各增加22，身份列和其余179表完整行保持；原18authority／0Grant、会话及1failed2succeeded不变，未还原计数。[PRD](../product/candidate-conflict-repair.md)、[架构与AI SOP](../architecture/candidate-conflict-repair.md)、[交互](../design/candidate-conflict-repair.md)、[限定验证](../quality/candidate-conflict-repair-verification.md)、[精简摘要](../quality/candidate-conflict-repair-summary.json)。

## 历史 T-P4-031：多源候选合成与固定基线交接（限定本地已验证）

有序固定两份已验收候选，Runner以Git原生三方合成精确M；另一执行节点对M独立测试后，测试账号明确Review，再将M显式绑定未开始任务C。C从M开发出新提交，另一节点再独立测试并单独Review。实际同机两Runner、2项新运行、8项正式检查；原来源、管理员锚点与protected baseline全链路可追溯。

冲突返回真实路径且不封存成功候选；不确定回执沿用原操作核对，来源变化阻止新的批准或代码使用，既有执行事实保留。中英文紧凑面板、刷新及冷回读通过；主预览031／1.0.0前向升级保留180旧表完整schema及全部行，仅增加3空表和1索引至183。[PRD](../product/candidate-composition.md)、[架构与AI SOP](../architecture/candidate-composition.md)、[交互](../design/candidate-composition.md)、[限定验证](../quality/candidate-composition-verification.md)、[机器摘要](../quality/candidate-composition-summary.json)。仅本地自有协议夹具验证；未调用厂商或模型，未报告用量不推断为零Token。真实远端、多机、生产验收、完整P0–P7及全部扩展需求继续实施。

## 下一步：分布式恢复与真实集成

继续跨物理机器、未知回执故障注入、厂商账户与生产隔离、分布式数据库及规模验证。当前修复保持明确创建、启动、重试和验收，不自动更换工具、重建任务或重试未知结果。完整P0–P7与扩展需求继续实施。

## 历史 T-P4-030：固定失败证据的智能分诊与修复需求（限定本地已验证）

准确持久失败 run／node／attempt 的固定证据，经专属只读模型形成观察、假设和修复建议；人工仅修改 repair，明确采用为原问题的需求草稿，并保留原运行／原件／来源回链。未知执行先核对原运行，不自动诊断、修复、重试、验收或合并。

必要领域、Runtime／原生 SDK、CLI、私有／Pool、HTTP、权限／来源、费用、原操作恢复及中英组件检查通过。真实同机 Control／Runner／DAG／账本配合一次自有 loopback 固定模型响应，40 Token、估算 57 CNY micros；分析 adopted v4→需求 draft v1。实际进程正常闭合，最终 web-release 冷回读确认中英刷新、原分诊深链和类型化需求来源，未再调用模型、启动 Runner 或登录。主预览030／1.0.0正常，原177表schema及全部行保持，仅新增3空表、1索引至180；18authority、0Grant、原1failed／2succeeded运行及登录保持。见[PRD](../product/failure-diagnosis.md)、[架构与 AI SOP](../architecture/failure-diagnosis.md)、[交互](../design/failure-diagnosis.md)、[限定验证](../quality/failure-diagnosis-verification.md)、[机器摘要](../quality/failure-diagnosis-summary.json)。本地协议不代表真实厂商质量／账单、跨机器或生产验收。

## 历史 T-P4-029：专家团队依赖编排与交付进度（限定本地已验证）

原团队派发入口支持固定任务依赖、逐任务真实进度、暂停／恢复／停止、未开始项修订、只读版本历史及同合同 CLI。服务端持久推进器在本计划准确前置运行获得正式验收后才派发后继，每次重新核验原实际凭据、当前权限和固定来源；运行或测试成功不自动验收。沿用原 Go DAG、资源池、工作树和质量门禁。

实际同机 Control／两独立 Runner 完成两项依赖开发、交叉导入准确候选并正式测试 8 项，两次本地测试 owner 明确验收后 completed。暂停期间验收前置没有派发后继；只修订未开始项、409 草稿恢复、明确继续、代表性中英文和刷新／历史贯通。必要检查与最终构建通过，主预览029／1.0.0保留175旧表完整schema及全部行，仅加2空表和2索引至177，原登录／任务／运行／权限保持。见[PRD](../product/expert-team-orchestration.md)、[架构与AI SOP](../architecture/expert-team-orchestration.md)、[交互](../design/expert-team-orchestration.md)、[CLI](../../contracts/expert-team-orchestration-cli.md)、[验证与边界](../quality/expert-team-orchestration-verification.md)。业务依赖不自动合并代码；真实厂商、跨物理机器、生产及完整 P0–P7 继续实施。

## 历史 T-P4-028：已验收候选的受管代码集成（限定本地已验证）

任务详情支持管理员预置目标绑定、固定已验收／独立测试候选、真实完整差异、浏览器批准与独立授权执行。Go控制面、Runner、独立Git提供方分工，目标CAS／回执／fence原子提交；失联保留屏障，核对原操作并防止孤儿迟到重执行。私有和独立Pool沿用当前授权，无历史兼容层或通用写权限扩大。

真实界面在隔离裸仓库完成2文件快进集成，结果与既有已测试候选完全相同；17项实际Git／固定事实核对，中英刷新定位及10控件明确标签通过。必要领域／Pool／Provider／Runner／Core／HTTP／合同／类型及构建通过，模型／厂商调用0。主预览028／1.0.0，168原表schema及全部行相同，仅7新空表与3索引至175。 [PRD](../product/repository-integration.md)、[架构／AI SOP](../architecture/repository-integration.md)、[交互](../design/repository-integration.md)、[部署](../deployment/repository-integration.md)、[验证及边界](../quality/repository-integration-verification.md)。完整自主专家交接、远端／厂商、实库容量与P0–P7继续。

## 历史 T-P4-027：项目专家配置与原子协同派发（限定本地已验证）

真实团队／项目资源与版本化专业职责、固定流程／工具／知识绑定、独立发布与退役、不可变历史已实现。任务列表选择1–16任务，逐项匹配职责后固定预览并原子启动，各任务独立运行；当前API写入者可准备及启动，正式配置发布仍需浏览器批准。既有依赖验收、机器／Pool授权、资源及质量门禁保持。

必要领域／Core／HTTP／恢复与中英文界面通过；两个同主机本地独立Runner真实并发编码、交叉bundle导入固定候选，各4项测试通过。一次测试owner界面验收只改变一项任务；未调用模型或厂商。主预览027／1.0.0，原164表结构及全部行保持，4新空表与2索引至168。[PRD](../product/expert-team-dispatch.md)、[架构／AI SOP](../architecture/expert-team-dispatch.md)、[合同](../../contracts/expert-team-dispatch.md)、[限定验证](../quality/expert-team-dispatch-verification.md)。完整自主专家交接、目标分支集成、真实远端／厂商及完整P0–P7继续。

## 历史 T-P4-026：模型辅助知识、规范与技能提炼（限定本地已验证）

固定版本来源、专用只读模型能力、单次调用与 Token／费用额度，形成可编辑提案；原始输出不可改写。界面默认字段编辑、高级 JSON 折叠，显式采用为独立项目草稿，另行审核发布，并保留固定来源、模型与运行溯源。当前有效租约的同一只读 attempt 可以核对迟到完成回执；停止、过期及撤权均不自动放行。

必要领域／HTTP／Pool／账本／恢复及中英界面已验证，一次实际本地协议夹具调用为40 Token、57 CNY微元估算，非厂商推理或账单。主预览026／1.0.0，原161表结构与全部行保持，3新空表及1索引至164。完整向量服务、专家组、厂商、分布式容量、移动与SaaS继续。[PRD](../product/knowledge-intelligence.md)、[架构／AI SOP](../architecture/knowledge-intelligence.md)、[合同](../../contracts/knowledge-intelligence.md)、[验证](../quality/knowledge-intelligence-verification.md)。

## 历史 T-P4-025：组织共享技能与跨项目采用（限定本地已验证）

统一目录复用源项目已批准正文，组织明确共享、元数据检索、不可变版本、退役、当前权限与独立目标草稿采用已实现；项目审核发布后进入原派发固定上下文。CLI读写与MCP只读检索共用服务，API15项／Agent4项范围保持清晰。简洁双栏、中英、恢复和源项目管理入口已实际检查。

必要后端／事务／SDK／CLI及跨项目HTTP派发检查、真实界面共享→独立项目采用→审核发布→刷新通过。主预览025／1.0.0，原158表schema和全部行保持，仅3新空表及3索引至161；原登录和任务保持。向量服务、模型提炼与专家组策略及完整P0–P7继续。[PRD](../product/skill-catalog.md)、[架构／AI SOP](../architecture/skill-catalog.md)、[合同](../../contracts/skill-catalog.md)、[验证](../quality/skill-catalog-verification.md)。

## 历史 T-P4-024：Token、费用估算与组织／项目额度（限定本地已验证）

独立用量账本、固定模型费率版本、UTC月额度、私有／Pool同事务预留和幂等结算、未知／停止保留、人工确认及中英简洁管理已实现。费用估算与厂商账单、旧admission_units分别保留；未定价／未知不当0，旧历史不回填。

必要领域／权限／事务检查、跨组织Pool HTTP及真实本地模型协议链路通过。实际界面验证精确micros、严格0额度、离线改价、双窗口CAS冲突／草稿恢复及旧费率不重算。主预览024／1.0.0，152旧表schema／行保持，6新空表及4索引至158。完整多层级预算、网关硬许可与厂商商业结算继续实施。见[设计／AI SOP](../architecture/usage-accounting.md)、[合同](../../contracts/usage-accounting.md)、[限定验证](../quality/usage-accounting-verification.md)及[摘要](../quality/usage-accounting-summary.json)。

## 历史 T-P4-023：模型规划、受控采纳与持久风险预警（限定本地已验证）

默认折叠中英规划面板、只读模型工具配置、固定任务／需求／容量来源、真实DAG／Runner模型执行、原件／结构化方案、人工修订与同事务明确采纳、持久站内风险确认／关闭已实现。复用现有权限、调度、用量和事件，不因模型文字自动改变负责人或验收。

必要检查及实际Control→Runner→原生bundle→一次本地模型协议夹具→回写→界面采纳／风险确认／刷新已通过。主预览023／1.0.0正常（健康接口使用实际构建版本），原149表schema／行保持，3新空表及索引至152。厂商模型、长期自主授权、完整费用配额与专家组／分布式／移动端继续实施。见[限定验证](../quality/intelligent-planning-verification.md)、[机器摘要](../quality/intelligent-planning-summary.json)、[架构／AI SOP](../architecture/intelligent-planning.md)。

## 历史 T-P4-022：团队任务交接与 CLI（限定本地已验证）

任务详情新增默认折叠的中英交接面板，支持发起、接收、退回、撤回、固定依据与分页历史；接收按原 authority transaction／项目 guard 原子更新负责人／任务版本，并失效旧资格，不自动创建运行或验收。真实人员、账号、可选团队与当前权限重核，结果未知复用原操作。CLI 使用同一合同并要求显式影响确认。

后端六组必要检查、HTTP 边界、前端 4 项、CLI 1 组／8 子例、类型／3279 双语目录／合同／相关 vet 和隔离构建通过；原夹具失败及定向修复证据保留。实际界面完成新任务 pending→accepted 交接／owner 与 v2 更新，原已验收任务保持。主预览 022／1.0.0 正常，147 旧表保持、只加 2 空表与索引至 149，原账号／任务／运行／18 authority／0 Grant 保持。

见[限定验证](../quality/task-handoff-verification.md)、[机器摘要](../quality/task-handoff-summary.json)、[PRD](../product/task-handoff.md)、[架构与 AI SOP](../architecture/task-handoff.md)和[CLI](../api/cli-task-handoff.md)。仅 Windows／SQLite 本地和代表性中英界面；完整平台、厂商、分布式容量、专家组自动交接与生产仍继续实施。

## 历史 T-P4-021：团队多工具与 Node 入口（限定本地已验证）

Harness/dsh 和 CodeBuddy 复用现有任务／池派发、进度、私有工作树及质量门禁；固定参数、整体结果和实际进程停止分别核对。新增 Node.js 绝对入口方式，适配 npm 安装；入口内容变更会阻止执行。工具接入以默认折叠的中英四步指南呈现，按需显示运行方式及入口路径，不增加独立设置页面。

必要 decoder／配置／派发检查、真实原生与 Node 进程／候选联调、类型与 3229 双语目录、隔离构建通过；最终中英 Node 界面已验证，主预览 runtime-revision2 已更新，147 表与原登录保持。仅本地协议夹具，未宣称厂商模型、WorkBuddy 客户端或完整平台交付；完整 P0–P7 继续实施。见[限定验证与剩余范围](../quality/team-cli-verification.md)、[机器摘要](../quality/team-cli-summary.json)、[PRD](../product/team-cli-runtime.md)、[集成与 AI SOP](../integrations/team-cli-runtime.md)。

## 历史 T-P4-020：原生隔离编码与独立候选测试（限定本地已验证）

原生智能体使用官方Go／TS MCP六项attempt工具实际读取、CAS编辑、新建、删除和固定开发检查；宿主在进程停止、当前检查与操作回执核对后封存Git候选，再交另一Runner导入同SHA正式测试。紧凑中英开发事实默认折叠原始依据，保留未知、未封存、自测、正式测试与人工评审的独立语义。SPEC、PRD、架构、交互规范及AI SOP同步交付。

真实本地协议fixture5轮／9工具调用完成4项文件效果，开发检查失败后修复通过；独立Runner正式4项测试通过，Codex测试owner界面明确评审后accepted v4并刷新保持。原SDK schema失败、助手中断、断线截图和修复证据保留。当前预览020正常运行，147表结构／业务和18 authority／0 Grant保持，原登录可用；仅4协调表计数正常递增。

见[验证与限制](../quality/native-coding-verification.md)、[精简摘要](../quality/native-coding-summary.json)、[PRD](../product/native-coding.md)、[架构与AI SOP](../agents/native-coding.md)和[交互规范](../design/native-coding.md)。这不代表目标分支合并、真实厂商模型或生产验收；完整P0–P7继续实施。

## 历史 T-P4-019：官方SDK项目MCP与共享应用工具（限定本地已验证）

统一13项应用工具提供当前授权读取和受控需求／任务／设计草稿，原生Agent保持四项只读；官方Go SDK v1.8.0与Go1.26.10固定，非自写协议。紧凑连接页提供可信地址、无秘密Codex模板和默认折叠中英目录，不把签发或复制当作已连接。

实际独立控制面官方SDK客户端124项、代表性中英与390界面、生成合同及必要集成检查完成。原全Go切面发现1处漏译和复核发现分页兼容回归，修复后只复验相关35顶层／122事件；UI28项／3165词条通过。原预览已安全更新，147表结构和维护时行完全保持，现有登录保持；新Grant仅在隔离环境签发并撤销。详见[验证与边界](../quality/project-mcp-verification.md)、[PRD](../product/project-mcp.md)、[架构与AI SOP](../integrations/project-mcp.md)。完整P0–P7继续实施。


## 历史 T-P4-018：跨节点候选代码交接（限定本地已验证）

普通项目Runner与资源池独立机器支持封存Git bundle、当前授权读取、严格导入及独立测试；断线保留原成功结果并续传，不重跑开发。界面区分封存与实际导入，原始来源、SHA和字节按需展开，中英文保持原值。Root复核修复了提交ACK未知误删原件、并发同PUT回执恢复，以及Start回放旧状态造成重复传包三处问题。

最终平台定向11顶层／11子场景、Runner相关37顶层／68事件及随后重叠恢复4事件、UI27项、类型／3135双语目录／合同／本地化／隔离构建通过；各切面不累加。真实两个独立pool Runner完成A修复提交→B启动前缺对象→导入同一候选→四断言；测试owner界面明确评审后task accepted v3并刷新保持。该评审由Codex驱动测试账号，不是生产或业务负责人签收。

主预览已更新至018恢复修订2，地址仍为 `http://pool.localhost:4324/`。146旧表升级仅新增空candidate表；修订2同schema前向更新保留147旧表。独立最终核对旧业务与凭据保持，仅正常登录及精确协调计数变化。旧程序／网页／备份保留；当前控制面及holder正常运行，测试服务和两Runner正常退出。

见[PRD](../product/candidate-transfer.md)、[架构与SOP](../architecture/candidate-transfer.md)、[合同](../../contracts/candidate-transfer.md)、[验证与完整边界](../quality/candidate-transfer-verification.md)。多级无新代码来源继承、动态聚合、合并授权、厂商会话恢复、MCP／A2A、SSO、执行插件、外部记忆、安装、SaaS与分布式数据库／规模等完整P0–P7继续推进。

## 历史 T-P4-017：任务设计依据、项目CLI与安全预览升级（限定本地已验证）

任务详情直接显示正式采用依据、原批准快照与当前资格，支持同项目设计定位、明确空态及中英文切换保留原文。排除后的历史通过可重建索引定位实际create/update，读取仍核验原采用和SHA。CLI补齐项目知识与需求设计读写、精确JSON和只读预览分类；版本、权限及浏览器批准门禁沿用正式服务。

采用必要范围验证：依据12顶层／19事件、接线后迁移单项、共享维护与当前来源8顶层／33事件（与设计集成有重叠），CLI36顶层／55事件及vet，UI三文件19项、类型、双语3119目录、隔离构建、合同／启动／本地化通过；250Schema／165路径。隔离实际GUI与CLI完成，145旧表升级新增关系1表4行，闭合服务正常退出0、业务保持。数字按各次切面记录，不累加重叠用例。

旧预览先做只读WAL感知克隆演练，再实际维护升级：129旧表旧列0差异、新增17表至146，旧14凭据不提升；原owner正常重登录，旧运行和业务保持。当前 `http://pool.localhost:4324/` 使用017控制面及网页，正常模式已恢复；旧程序／网页与一致备份保留。旧API／Runner／Agent凭据需明确重签发，本轮未自动发行。

见[PRD](../product/task-design-provenance.md)、[架构与SOP](../architecture/task-design-provenance.md)、[CLI](../api/cli-project-context.md)、[升级手册](../deployment/preview-upgrade.md)、[实际验证与边界](../quality/task-design-context-verification.md)。完整厂商工具／IDE、模型设计、SSO、MCP／A2A、执行插件、外部记忆、跨平台安装、分布式数据库及完整P0–P7继续实施。

## 历史 T-P4-016：需求设计基线与可迭代任务采用（限定本地已验证）

需求详情提供单一“设计与拆解”，产品／体验／技术和任务提案共用版本化工作区。保存、发布、变更预览和明确采用分别记录，稳定key复用正式task ID，移出归档保留历史；受保护绑定和正式派发计划核对固定设计，旧run不能通过新设计验收。新需求来源显式核对，未知恢复不误贴新正文。

领域2组、独立集成5组、共享旧行为27个顶层分别通过；前端必要9项及随后受影响消费者定向通过，未重复全套。最终类型、3076中英目录、隔离构建、合同/localization和生成检查通过；249 Schema／164路径。真实SQLite升级保留141旧表、只加4表，保留原WAL；实际浏览器两轮设计发布采用，需求v2、设计version6／发布4、四修订／两采用，任务3→5且第二轮仍同ID／v2，运行0。隔离服务正常停止，原预览未改动。

见[PRD](../product/requirement-design.md)、[架构与SOP](../architecture/requirement-design.md)、[验证与限制](../quality/requirement-design-verification.md)。当前是人工设计与正式任务闭环；模型设计、完整UED原型、手动任务导入、SSO／插件／MCP／A2A、安装、厂商研发、分布式数据库及完整P0–P7继续实施。

## 历史 T-P4-015：项目知识、规范、技能与固定上下文（限定本地已验证）

已交付同一工作区内的项目记忆／SPEC／文本技能、来源提取、人工审阅发布、不可变历史、退役和来源更新；任务与 Agent 显式消费批准修订。新草稿保持旧发布，启动前核验来源和发布指针，正文不重复传递、不放宽32,000字节完整上下文预算。页面沿用紧凑列表／详情，修订标题跟随所选版本，目录中英文2963键；未知结果和晚到响应保留原操作恢复。

定向Go集成39个顶层通过，另有元数据投影RED→GREEN；新UI四文件24项与共享五文件37项分别通过，后续仅复验受影响用例，不累加重叠计数。最终类型、目录、隔离构建与生成合同检查通过；233 Schema／158路径／1372引用。实际SQLite升级保留139旧表、只加两表；真实浏览器完成提取／发布／来源变化／重新采用／草稿分离及任务选择，最终知识2／修订8、任务3／运行0，隔离服务正常停止。

见[PRD](../product/project-knowledge.md)、[架构与SOP](../architecture/project-knowledge.md)、[验收及边界](../quality/project-knowledge-verification.md)。仅验证当前Windows控制面和代表性桌面浏览器；模型总结、执行插件、新资产语义索引、外部记忆同步、完整PRD/UED编辑、厂商开发、其他系统安装、分布式数据库及规模继续实施。既有用户预览和数据库未迁移。

## 历史 T-P4-014：当前凭证权威与原页面上下文

封闭实际来源Actor、当前事务授权证明、浏览器family与独立Grant/slot、领域命令／读取／SSE及Web/CLI原页面身份已整合，本地提交d11eb7c。源码与定向验证之外，补充了打包控制面34项真实HTTP检查和实际中英任务保存／刷新；隔离服务正常停止。旧凭证真实重登录／重签发及用户服务迁移仍须明确处理，不能把SQLite夹具升级扩大到生产。详见[专项验证](../quality/current-authority-verification.md)。

## 历史 T-P4-013：工具资格显示与图控件（限定范围本地已验证）

16类当前工具资格带完整原文证明的只读显示身份；Go定义同源生成客户端合同，原始reason、Observation、历史回执及授权规则保持。固定官方React Flow12.12.0／system0.0.83，使用公开本地化接口、节点／连线说明及真实事件身份。输入／输出连接点明确用途和原始节点；移动播报明确是公开回调的移动前坐标。已确认的八个原始名称入口和两个Handle模板共用封闭字面值格式化，变量标记不被重新解释。通用错误入口不能绕过工具来源证明。

最终证据：Go1250个通过事件／359输入、0失败／测试跳过、vet通过；Web784项／55文件／225输入、类型／语言／生产构建通过。非Web47项为此前通过结果，38个相关输入保持，未虚称重跑。网页两语言各2850键、后端801键；217领域模型加Error（218个Schema）／153路径／1242引用。独立生成检查362输入稳定，六目标四程序24文件的来源／字节独立核对。各次721／723／730网页切面独立留存，不混为最终784。

实际独立工具旅程完成paused→enable→inventory_missing→真实Node版本库存→pause／enable→真实库存过期→pause v5；4次历史命令回执逐份核对。1240条严格空领取回执不是实际派发，0实际分配／运行。最终图草稿在390／768／1366／1920、深浅主题和中英文16组状态检查；真实多选、键盘移动、原始名称、用途说明及草稿保持。主预览既有协议运行的attempt1读取6条持久工具事实，原文展开／语言／焦点及刷新精确定位保持。不是新厂商任务或研发验收。

隔离UI克隆关闭后129表／1353行完整；只新增一个正常登录会话，业务／执行保持为空。持有原生句柄的自有子进程空闲后强制停止已确认，子进程exit1与宿主exit0分开，不称优雅关闭。主预览129表／7690行完整，128表保持；授权协调计数单独记录，不能写成129表全不变。两个服务的最终HTML／JS／CSS字节均匹配。自动审批用量中断、服务恢复、旧失败和被排除的过渡截图完整保留，未替换旧证据。

见[专项验证](../quality/platform-display-verification.md)、[精简摘要](../quality/platform-display-summary.json)、[显示合同](../../contracts/platform-display.md)、[原始值合同](../../contracts/ui-literal-values.md)、[交互设计](../design/platform-display.md)及[专项计划](platform-display-plan.md)。本阶段不代表所有语言表面、真实厂商开发／会话恢复、完整WCAG、安装、OceanBase／PostgreSQL、分布式或规模通过；完整P0–P7继续实施。下一阶段先完善统一当前凭证权威，再推进全部CLI／IDE／智能体／外部平台、持久原因与生成记录来源、Monaco等纵向能力。

## 历史 T-P4-012：精确尝试工具历史（本地已验证）

已实现按 project／run／node／attempt 读取持久工具事实，当前权限与选择在同一事务复核。首次读取固定watermark，位置0游标也能保存空窗口；每条返回原始`raw_data`，保留SQL正文的转义、内层重复键及数值字面量。单请求最多扫描1000个候选，单事件2MiB、事件数组4MiB；不截断或跳过未返回的有效事实。界面默认50条，可选5／25／50／100，历史页不受实时40行限制；翻页、原文、语言、焦点与刷新保持精确attempt。未知内层Unicode仅做临时兼容比较，无法证明的typed显示回退原文，不弱化来源证明。

现有检查证据：Go1179通过事件／350输入、0失败／测试跳过及vet通过；Web652项／50文件／214输入、非Web47、类型与构建通过；目录为网页两语言各2829键、后端785键，合同为216领域模型加Error（共217个Schema）／153路径／1241引用。当前24运行文件批次`20261024T164256020Z-ba85fe20`对应197个生产输入，源码SHA `80d309cb0ad83b56234af5d3187c981b88a3abae84a845d2b268591cf4a49e4d`；Windows控制面SHA `3a60573edac3cfa1b21de1ce3b1f1f73da8ed68451a0f519a5f2f10679328624`。这些结果保留各自原始切面，不覆盖旧批次。

实际原生进程连接真实本地HTTP429，形成两个不同fence／session的尝试，各5条工具事实；独立的165组合成协议活动形成334条事实，浏览器实际读取7页（50×6＋34）并返回6个旧位置。语言切换保持原文展开与焦点，刷新恢复精确attempt；390／768／1366／1920部分页面未见横向溢出，所查浏览器控制台0问题。Runner已排空3条结果／344条进展ACK，两个档案paused v3。

**本阶段通过限定范围的本地验证。** 两条旅程的全部分页与SQL原文一致；纯读取期间仅授权协调计数变化，运行、业务、模型及回执保持。独立验证服务在排空后核对身份强制停止，关闭快照129表／1805行完整；主预览更新前后原129表／7690行保持。结果由[专项验证](../quality/attempt-tool-history-verification.md)和[精简摘要](../quality/attempt-tool-history-summary.json)最终汇总。设计与边界见[架构及SOP](../architecture/attempt-tool-history.md)。合成协议内容不代表原生SDK真实工具或厂商模型执行；本地429不代表开发成果、测试或人工验收。全部语言表面、其他系统安装、分布式数据库、规模与完整P0–P7继续实施。

## 历史 T-P4-011：原生诊断贯通与命令编辑交互

46条原生诊断现由Go权威目录生成SDK目录，纳入后端778键；网页两语言各2804键。版本1的`ToolDiagnostic`将已证明的键／类型参数从实际原生进程，经Runner、持久Journal、当前授权HTTP、DAG及Outbox传至界面；上游译文不作为证据，原始消息及执行／用量事实保持。命令／智能体／测试节点明确填写单个许可命令ID，自定义task／script保留独立worker语义；滚动工具事实的展开与焦点绑定原事件，避免窗口更新后显示另一条原文。

最终Go1131通过事件／342输入、Web544项／46文件／206输入、非Web47项，失败／测试跳过均为0，vet／类型／构建／合同检查通过。实际独立本地HTTP429夹具完成1次原生失败执行、1个attempt、10条事件（5条工具事实）；中英文、原文、刷新与精确attempt保持，已上报和未上报用量不混淆。一个问题两次界面回流仍为一个需求草稿，任务保持todo，未进行研发验收。5项P2独立审查问题已修复并复核。见[设计与SOP](../agents/native-diagnostics.md)、[验收](../quality/native-diagnostics-verification.md)、[精简摘要](../quality/native-diagnostics-summary.json)。新24程序为六目标交叉编译；正式厂商模型、其他语言表面、分布式及完整P0–P7继续实施，生产未验收。

## 历史 T-P4-010：DAG、运行命令与原生智能体国际化

新增DAG151键／150注解，后端731键；原Error及sentinel身份保持。流程校验保留原始证据并返回强类型诊断，可即时切语言且保留草稿。三个运行命令中英入口、原生46键及SDK非法语言访问前拒绝已实现；独立审查两个P2修复并复核。最终Go1038／328输入、Web469／42文件／198输入、非Web45、类型／构建／合同检查通过；24程序与实际双语校验、12只读命令、1280／390局部UI及129表7690行保持已验。见[架构](../architecture/runtime-localization.md)、[验收](../quality/runtime-localization-verification.md)、[精简摘要](../quality/runtime-localization-summary.json)。该阶段未含Go Runner的原生诊断传播及命令提示合同修正，后续实现见T-P4-011；库内NLS及完整P0–P7仍分别验收。

## 历史 T-P4-009：受控平台诊断国际化

新增447条领域静态诊断及10个参数化键，后端目录580键；原Error、状态／code／details、权限、幂等正文和历史证据保持。AST门禁覆盖83文件的546个完整静态中文原文与12处参数化生产者，合成解析边界及真实并发HTTP已有RED→GREEN。最终Go904／315输入、Web461／193输入、非Web30、类型／构建／合同检查通过；24运行文件已独立审计，实际控制面拒绝接口及中英文登录错误通过，辅助入口分行44px、1280／390部分界面及129表保持实测完成。精确边界见[验收](../quality/backend-diagnostics-verification.md)与[精简摘要](../quality/backend-diagnostics-summary.json)。门禁外的非中文／原始动态错误、其他包／展示字段、原生／通知／安装器／渠道和完整P0–P7继续实施。

## 历史 T-P4-008：剩余自有前端与工作区诊断国际化

本阶段完成剩余18个旧前端模块、项目读取视图拆分及结构化错误消费；两语言各2798键，后端123消息。完整Go811个通过事件、Web461项／41文件和非Web30项通过，类型、构建及212模型／152路径／1232引用检查通过；最终输入分别302／193项且前后一致。实际中英文草稿、候选门禁、检索和Monaco TypeScript诊断／保存／刷新，以及390／768／1366／1920部分页面验证完成；原129表全部摘要不变。Go首轮Windows清理失败的占用根因尚未证实，原记录及50次定向、最终全量复验分开保留。详见[专项报告](../quality/remaining-localization-verification.md)与[精简摘要](../quality/remaining-localization-summary.json)。409个受限盘点的后端唯一原文候选、库内NLS、通知／安装器／渠道和完整P0–P7继续实施。

## 历史 T-P4-007：领域国际化与持久语言偏好

账户与组织偏好已接通版本化保存、继承、当前权限和原视图身份保护；双标签页组织Cookie漂移被事务内拦住，旧回执不能替代当前设置。开发流程、工具、任务派发、调度事实、团队和排期接入成对领域目录，含38种事件标题及公开图ARIA标签。最终1819网页键、107后端消息；18旧模块1011字面量候选、第三方内部控件、通知和安装器等仍待迁移。

完整Go787事件及vet、Web399、非Web30、类型与构建通过；最终24运行文件独立核验，Windows控制面实际激活。迁移125张旧表内容不变，新增4表；实际四次偏好操作、跨组织继承、双标签页拒写、四宽度和部分主题／键盘验证完成，原62张执行／交付表摘要保持。精确范围见[验收](../quality/locale-preferences-verification.md)与[精简摘要](../quality/locale-preferences-summary.json)。完整P0–P7继续实施，原始运行数据和构建文件不进入Git。

## 历史 T-P4-006：共享执行资源与多语言基础

从本地提交`be093ca`实现多项目共享池、双边授权、独立机器身份、固定任务及运行路由、分层加权准入份额与可视化治理。本机两组织／两项目实际并行开发、独立测试、测试账号界面验收及故障去重回流已贯通，范围见[阶段验收](../quality/execution-pool-verification.md)和[精简机器摘要](../quality/execution-pool-summary.json)。完整平台继续实施，专业SDK、SSO、MCP／A2A、插件／技能与AI开发SOP要求已进入[基础PRD](../product/platform-foundation-prd.md)和[工程架构](../architecture/platform-engineering.md)。

全平台国际化要求见[规范](../design/internationalization.md)：后端93键；网页671双语键，覆盖登录、主壳、通用记录／表单／详情／拆解、共享池域与公共展示；旧45模块2171处字面量候选仍明确列为未完成。完整Go714通过事件、0失败／跳过、vet0，294输入不变；之后仅3个CLI文件修复显式false保真并独立重验CLI33，其余291输入保持。完整网页289通过，最后仅2输入微调再验2消费者；其余125输入不变。非网页Node30、工作区类型、最终生产构建及合同检查通过。各切面不合并成同次全套计数。

当前六目标24程序源码摘要 `bdafacdab242d3a991581492f63ecd5478a1cc86492378881bb995d302f7f462`，已核对文件、实际架构及构建信息并激活Windows控制面。实际3运行／5尝试／38事件，开发进程重叠33.7秒，4项固定测试通过，两任务accepted v3，故障生成1问题及1需求草稿；验收由Codex操作测试账号，不代表业务负责人签收。四宽度／主题／键盘和切语言的部分旅程已实测，窄屏英文顶栏重叠已修正。原始日志、凭证、DB、截图和二进制不进入Git；运行已排空并停止本任务Runner。回执长期保留、分布式数据库、完整厂商模型及完整P0–P7仍需后续验收。

## 历史 T-P4-005：增量调度与可解释执行准入

在`1e9d77b`基线上完成持久增量推进、就绪/唤醒索引、丢ACK领取恢复、项目准入策略预览与确认、队列阻塞和持久attempt历史，以及用量已报告零/未知的区分。最终Go606通过事件、0失败/跳过、vet通过；前端、合同和当前交付资产见[阶段验收](../quality/scheduling-verification.md)及[机器摘要](../quality/scheduling-summary.json)。实际界面槽位0→2、双Runner真实并行、独立测试、人工批准后5节点/3尝试/22事件，累计额度3；旧78表逻辑摘要保持，新24程序当前源码一致。Git只交付必要源码/设计及精简验证，原始档案留本地。跨组织共享池、多项目执行身份、分层加权公平、模型网关及完整P0–P7继续实施。

## T-P4-004 历史：固定工具派发与完整任务依据

工具档案、实时观察、精确命令资格、固定任务/需求/流程/工具预览和原子启动、typed执行事实、独立Agent授权已交付。实际协议夹具完成隔离开发、独立2项测试、人工验收、失败去重回流和两次真实停止；它不代表厂商模型效果。最终Go512个通过事件、0失败/跳过，vet通过；JS212项、类型和生产构建通过，合同147模型/117路径/344引用。新24程序批次`20261024T051829203Z-df0ca9b5`已构建与独立核对，Windows实际激活复验；4runs/8nodes/5attempts，原三次运行/35事件及验收回流保持。工具最终paused v5。响应式完整节点列表、桌面依赖图、精确需求导航与焦点/主题部分验证完成。[阶段验收](../quality/tool-execution-verification.md)、[当前运行包](../deployment/tool-stop-runtime-bundles.md)。完整P0–P7、厂商真实账户、集群、安装和规模目标继续实施。


日期：2026-10-27。状态：可运行开发版本1.0.0，T-P4-030限定本地已验证，完整产品实施进行中。

## 当前可运行

| 能力 | 源码入口 | 已验证范围 |
|---|---|---|
| 初始化/会话/项目权限 | internal/platform/auth.go、http.go | 本机SQLite API与浏览器初始化 |
| 组织/团队/真实账号治理 | internal/platform/governance*.go | 单次邀请、最后所有者保护、授权墓碑、真实双连接竞争、浏览器组织切换与隔离 |
| 迭代/里程碑/计划/容量 | internal/platform/planning*.go、baselines.go | 双版本原子排期、全项目不可变快照、自然周估算分摊、撤权排除；浏览器真实创建/比较/完成 |
| 需求/任务/问题闭环 | internal/platform/records.go、delivery.go | 幂等、批次回滚、依赖、验收失效、回流 |
| 持久DAG | internal/dag | SQLite编译/条件/等待/恢复/子流程/补偿，以及增量前沿/索引/资源CAS/重试窗口/真实历史迁移；见当前阶段摘要 |
| 持久自动化与事件续传 | internal/automation、internal/platform/automation*.go、run_stream.go、AutomationWorkspace.tsx | 真实人工/cron双触发、固定修订与流程、时区/错过/重叠、撤权与事务竞争；Chrome完整终态、正常轮换与重启上下文保持 |
| 签名事件入口与交付追踪 | internal/platform/automation_ingress*.go、AutomationIngress.tsx、internal/cli/webhook.go | GitHub原生/Suma v1协议、本机真实Git标签/CI/监控事件触发、固定来源修订、当前授权、回执去重；重启不重复执行，来源元数据不注入执行命令 |
| Runner与候选交接 | internal/runner、cmd/suma-runner、internal/dag/candidate.go | Windows实际独立Git工作树、进展持久ACK、固定候选交接、原失败保留及真实测试 |
| 精确尝试工具历史 | internal/dag、internal/platform、apps/web | 当前授权与持久归属、固定窗口和有界分页、原始正文及历史页；T-P4-012原生双尝试／独立合成334条、部分四宽度、只读保持／停止／预览检查为历史限定验证 |
| 真实制品与不可变发布范围 | internal/platform/artifact_*.go、release_candidate_service.go | 真实上传字节、封存、build/package-test证明、固定C与当前已验收任务范围 |
| 发布、核对与独立回退 | internal/deployer、release_deploy*.go、ReleaseWorkspace.tsx | 真实浏览器5次操作A/B/A/B/A；健康404与实际活跃内容分开，独立批准、单调代数、问题去重回流 |
| 原生智能体 | packages/agent-runtime | 有界循环/工具协议；未连接生产模型 |
| 外部工具 | 工具目录与许可命令、HTTP桥接 | 实机只读探测/本地协议测试；未调用收费模型 |
| 知识服务 | packages/knowledge | 真实本地Lance全文/向量、版本隔离、墓碑 |
| 失败分诊与修复需求 | internal/platform/failure_diagnosis*.go、FailureDiagnosisPanel.tsx | 固定真实失败／一次专属只读模型／人工修复稿／明确采用草稿／原件及来源回链；同机本地协议、中英冷回读与主预览保持见030专项证据，非厂商质量验收 |
| 界面与编辑 | apps/web | 当前032受影响组件、类型、4269对中英键及最终构建通过，代表性中英／来源回链／刷新已观察；历史Web652项／50文件等原检查分别保留各自源码切面，不相加为当前全套。完整体验矩阵继续实施 |
| Windows桌面 | apps/desktop、dist/desktop | 未签名NSIS/portable及目录构建；安装/GUI/升级仍分别验收 |
| CLI与部署配置 | cmd/suma、deploy、Dockerfile | Bubble Tea/游标分页与签名交付测试；部署8项测试及Compose组合/配置校验，镜像/Helm未实跑 |
| Go四程序交付 | scripts/build-runtime.ps1、dist/runtime/builds | 六目标×控制面/Runner/CLI/Deployer，真实编译、构建信息与逐文件摘要；目标系统运行分别验收 |

## 当前完整性边界

完整目标与未完成项逐项列在implementation-plan.md。031多源合成和032受控修复、准确结果独立测试及明确代码基线交接已取得限定本地证据。需要继续完成：分片增量调度/完整跨租户公平、完整合并策略与终端语言调试、工作制容量日历/实际工时/连续进度历史、组合规划、完整设计编辑、厂商Git/CI/监控账户与公网交付验收、完整OAuth及设备治理、生产CI/签名/远端和容器部署/灰度、授权智能故障修复与独立复测、离线协作、办公/媒体/渠道、支付计费和跨平台正式交付。签名接收、本机协议事件与真实下游执行不等于真实厂商接入或外部CI身份证明。制品与Deployer首轮只支持本机静态站点；本地多源合成与基线继承不等于生产分支集成、多机或完整自动恢复验收。

OceanBase/PostgreSQL驱动与DDL代码不等于真实集群验证；本地Lance测试不等于多模态OCR/转录全部完成；单元测试不等于目标1000执行槽位已通过。

## 使用入口

浏览器默认127.0.0.1:4310。首次创建管理员和项目；执行需要另建项目范围Runner凭证并配置本地命令许可。代码编辑需要服务端设置允许根目录，再绑定项目工作区。

验证使用独立`.cache/e2e`数据，保留用户正式`.suma`数据目录。交付产物使用dist，专业文档使用docs；秘密、运行数据库和临时日志均不提交。T-P4-003历史汇总：Go385个通过事件（含子测试和helper），0失败、0跳过，9个测试包及4个只编译命令包，vet通过；JS/TS包测试173项、部署8项、类型检查与网页构建通过，最终文案后另复验12项前端消费者；合同131模型/110路由/315个Schema引用。18次本机签名/协议请求形成7条交付、3次真实成功运行、9个节点、6份派生证明和3个实际制品；Windows交付控制面与CLI重启后回执去重，原始执行仓库保持干净。该历史阶段六目标四程序批次为 `20261024T033010763Z-94b39f3b`；[事件入口专项验收](../quality/ingress-verification.md)披露具体范围。此前T-P4-002的329个Go通过事件、1个权限不足跳过、138项JS测试及原始证据仍保存在[自动化专项验收](../quality/automation-verification.md)，不作为本轮跳过项。阶段功能完成后按用户要求commit；P5及P4-002历史证据保持原切面，完整P0–P7尚未签收。
