# Octop、WorkBuddy、Multica完整产品能力对齐

状态：1.0.0产品范围与缺口基线，2026-10-29公开资料审计；速码代码基线`9696bf9`。负责人：产品、工作台、智能体与平台领域。关联需求1–26，尤其9、10、11、12、21、23、25、26。本文是完整工作台与产品能力的交付要求，不把“有适配器”“装有CLI”当作功能完成。现有58项限定实现／34项待办／92项总账不因新增审计行而改变。

当前T-WORKBENCH-126先贯通原生IDE中的正式任务、需求／设计、负责人、准确执行、计划用例和正式验收同页读取，见[冻结合同](../../contracts/workspace-task-context.md)、[限定验收报告](../quality/workspace-task-context-verification.md)及[摘要](../quality/workspace-task-context-summary.json)。它不变更当前文件目录，不绑定任务工作树或授予派发／验收权；完整会话、PTY、办公和以下全部目标继续推进，本切片当前授权HTTP、真实历史复读及中英文1366／390已按报告限定验证；不构成全工作台或平台全部能力完成。

关联：[工作台交互设计](../design/reference-workbench.md)、[跨参考平台交付矩阵](platform-capability-delivery-matrix.md)、[完整本地工作台架构](../architecture/local-development-workbench.md)、[实施计划](../delivery/implementation-plan.md)。

## 范围、来源与判定

下表逐项覆盖本次读到的官方README、功能文档及其公开页面入口。隐藏功能、未公开API和未读到的细节保持未知，不能据此推断厂商全部内部能力；公开新增项需要持续纳入。本表的范围边界不是缩减用户要求：全部公开能力、对应界面及端到端操作仍需分别实现、验证和交付。

**状态**：“局部”指速码有对应领域基础及所列有限证据，剩余操作仍未完成；“待交付”指无该完整产品操作的实际证据；“参考规划”指上游自己仍标Beta、内测或规划，速码可纳入目标但不能伪装上游已发布。没有一行表示三个平台的全部功能已达到一致，更没有平台级生产验收结论。

### 官方来源版本

| 来源ID | 读取依据 | 固定性与边界 |
| --- | --- | --- |
| M0 | [Multica README](https://github.com/multica-ai/multica/blob/b2e58d0bf8a8e113b9c7bc895f4213f08442833a/README.md)、[许可](https://github.com/multica-ai/multica/blob/b2e58d0bf8a8e113b9c7bc895f4213f08442833a/LICENSE) | 本次GitHub连接器解析`main`为`b2e58d0bf8a8e113b9c7bc895f4213f08442833a`。本地`D:\codes\github\multica`为`18fa9f728a9e25f2465df2789631edb2a75442ad`，只读，未拉取、重置或覆盖。旧`a47a1ce…`仍是历史采用依据，不冒充本次最新。 |
| M1 | 固定提交的[事项](https://github.com/multica-ai/multica/blob/b2e58d0bf8a8e113b9c7bc895f4213f08442833a/apps/docs/content/docs/issues.mdx)、[运行](https://github.com/multica-ai/multica/blob/b2e58d0bf8a8e113b9c7bc895f4213f08442833a/apps/docs/content/docs/tasks.mdx)、[对话](https://github.com/multica-ai/multica/blob/b2e58d0bf8a8e113b9c7bc895f4213f08442833a/apps/docs/content/docs/chat.mdx) | 本次已读官方正文；新事项批量确认、重复事项和实时引导细节以该版本为准。 |
| M2 | 本地固定提交的[Agents](https://github.com/multica-ai/multica/blob/18fa9f728a9e25f2465df2789631edb2a75442ad/apps/docs/content/docs/agents.mdx)、[Squads](https://github.com/multica-ai/multica/blob/18fa9f728a9e25f2465df2789631edb2a75442ad/apps/docs/content/docs/squads.mdx)、[Skills](https://github.com/multica-ai/multica/blob/18fa9f728a9e25f2465df2789631edb2a75442ad/apps/docs/content/docs/skills.mdx)、[Autopilots](https://github.com/multica-ai/multica/blob/18fa9f728a9e25f2465df2789631edb2a75442ad/apps/docs/content/docs/autopilots.mdx) | 补充确切交互；与最新README来源分别标记，未假定两个提交的所有细节一致。Runtime、Inbox、成员和Git集成同目录文档已读。 |
| O0 | [Octop README_CN](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/README_CN.md)、[许可](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/LICENSE) | 本次`main`仍为`890111b7879665840414ca28ab168b05b0db0921`，产品`1.0.3b1`。速码使用的公开Harness1.0.5／ACP0.12.1是另外的SDK版本，不能混称完整Octop产品版本。 |
| O1 | 同提交的[团队](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/docs/expert-teams.md)、[配置](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/docs/configuration.md)、[连接器目录](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/docs/connectors.md) | 团队Beta与停止主持人不停止成员等语义明确保留；目录列名不证明每个第三方账号已可用。 |
| W0 | WorkBuddy官方[新建任务栏](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Task-Bar)、[任务对话](https://www.codebuddy.cn/docs/workbuddy/Conversation)、[结果](https://www.codebuddy.cn/docs/workbuddy/Results)、[任务管理](https://www.codebuddy.cn/docs/workbuddy/Task-Management) | 当前页面构建标识`main@19c7044b63c4`，构建2026-10-29。任务栏资源链接含完整文档提交`19c7044b63c472703fba6bea1c92e528aa59faa6`。官网页面可变，不把它当作可安装客户端版本。 |
| W1 | 官方功能目录下[项目](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Project)、[专家](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Expert-Center)、[技能](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Skills-Market)、[插件](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Plug-In)、[连接器](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Connector)、[记忆](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Memory)、[模型](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Model) | 已读正文。缓存页面另见`b043a5d6b529e`（10-26）与`6f318a8464d`（10-27）构建标识，逐页保留差异，不把旧页面缓存说成统一最新版本。 |
| W2 | 官方[Git与并行开发](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Worktree-Task)、[右侧栏](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Right-Sidebar)、[文件浏览器](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/File-Browser)、[人机双写](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Knowledge-Base/Cowriting) | 已读正文，保存、选区编辑、标签、浏览器登录态和本地工作树限制分别进入交互要求。 |
| W3 | 官方[资料内容](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Library/Content-Management)、[协作](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Library/Collaboration)、[轻量发布](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Library/Lightweight-Publish) | 已读正文；部分页面为`c5e9babaf44`（10-28）构建。网页发布、共享资料与正式软件部署不混同。 |
| W4 | 官方[网站](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/App-Publishing/Web-App)、[小程序](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/App-Publishing/Mini-Program)、[云服务](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/App-Publishing/Cloud-Services)、[设计](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Design-Idea)、[Buddy应用](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Buddy-App) | 已读正文；厂商托管额度、账号与上架要求不是速码已拥有的能力。 |
| W5 | 官方[定时任务](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Automation-Guide)、[邮箱](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Mailbox)、[系统设置](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Setting)、[数据](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Data)、[权限与沙箱](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Permission-Modes) | 已读正文。[助理](https://www.codebuddy.cn/docs/workbuddy/From-Beginner-to-Expert-Guide/Function-Description/Assistant)及ima授权本次仅获得官方搜索片段／导航；具体服务接入仍须补完整协议验收。套餐、账单、发票与注销只有本次可见导航，未审核其全部政策或价格。 |

**名称和许可边界。** WorkBuddy是腾讯官方办公／开发工作台；CodeBuddy CLI是独立工具，官方WorkBuddy复用`.codebuddy`配置不意味着两个产品等同；OpenWorkBuddy是另一个第三方项目，不能拿它的功能或许可证替代官方WorkBuddy。WorkBuddy公开文档不授予源码复制或品牌资源再分发权。Octop为MIT，实际复用SDK仍保留许可及依赖清单。当前Multica为完整Apache2文本加附加条件的Multica License，包含第三方托管、商业嵌入和品牌条件，不能作为纯Apache2随意合入速码SaaS；本轮只研究公开行为、独立实现，不复制受限源码／界面素材。OpenWorkBuddy既有PolyForm Noncommercial边界不变。

### 速码实现与证据索引

表内E编号只证明链接报告明确的范围。没有对应证据的操作一律待交付；官方页面截图不是速码实际交互证据。

| 编号 | 速码正式模块／有限证据 |
| --- | --- |
| E1 | [本地工作台](../architecture/local-development-workbench.md)、[桌面目录授权](../quality/desktop-workspace-verification.md)、[安装打包](../quality/desktop-packaging-verification.md)：原生目录与限定Windows产物，不是完整IDE及三系统实机验收。 |
| E2 | [文本编辑与冲突](../quality/workspace-file-context-verification.md)、[目录](../quality/workspace-directory-verification.md)、[Git读取](../quality/workspace-git-verification.md)：不包含完整PTY、LSP、DAP、Office协作。 |
| E3 | [模型网关与Harness](../quality/model-gateway-verification.md)、[Codex](../quality/codex-app-server-verification.md)、[OpenCode](../quality/opencode-actual-execution-verification.md)：固定模型／工具／用量及限定真实SDK流程，不是全部厂商账户和全部CLI。 |
| E4 | [团队编排](../quality/expert-team-orchestration-verification.md)、[智能派发推荐](../quality/dispatch-recommendations-verification.md)、[候选合成](../quality/candidate-composition-verification.md)、[交付链路](../quality/task-delivery-chain-verification.md)：任务计划与候选不等于持久聊天团队。 |
| E5 | [项目知识](../quality/project-knowledge-verification.md)、[知识提炼](../quality/knowledge-intelligence-verification.md)、[组织技能](../quality/skill-catalog-verification.md)：版本化固定上下文，不是个人长期记忆、全部连接器或技能市场。 |
| E6 | [定时与事件](../quality/automation-verification.md)、[签名入口](../quality/ingress-verification.md)：原触发与恢复，不是每个IM服务或个人邮箱实际连接。 |
| E7 | [测试计划](../quality/test-plans-verification.md)、[问题修复](../quality/issue-repair-verification.md)：独立测试、正式Review与问题回流。 |
| E8 | [应用源码版本](../quality/project-applications-verification.md)、[发布集合](../quality/application-release-sets-verification.md)、[有序发布](../quality/application-release-rollouts-verification.md)：固定源码与静态提供方范围，不等于通用云应用／小程序／容器发布。 |
| E9 | [工作区操作事实](../quality/workspace-operations-verification.md)、[当前权限](../../contracts/current-credential-authority.md)、[Octop项目MCP](../quality/octop-integration-verification.md)：限定操作、凭证及协议；不是通用OS隔离或所有第三方OAuth。 |
| E10 | [完整能力台账](../delivery/status.md)、[国际化](../design/internationalization.md)、[组织治理](governance.md)：已有基础，未配置或未实际验收部分仍待交付。 |

## Multica：事项、团队和运行工作台

| ID／来源 | 公开功能及页面交互 | 速码对应与证据 | 必须补齐的功能与验收 |
| --- | --- | --- | --- |
| M01 M0/M2 | 工作空间建立、切换、背景上下文、独立事项编号 | 局部：组织／项目，E10 | 明确组织、项目与机器工作区的不同，完整切换／离开／归档操作。 |
| M02 M0/M2 | 项目归集事项、仓库和资料上下文 | 局部：项目、来源与资料，E5/E8 | 任务会话中直接选择准确项目资料，来源可回查。 |
| M03 M1 | 手工／Agent创建事项，预览原意和原字段 | 局部：需求草稿及采用，E4 | 简单任务输入到可编辑正式任务，不能模型自行添验收条件。 |
| M04 M1 | 优先级、日期、标签、自定义属性 | 局部：正式任务字段，E10 | 可视化属性定义、筛选、表单个性化及类型校验。 |
| M05 M1 | 自定义状态、生命周期分类、退役状态 | 局部：固定业务状态，E10 | 可配置状态／工作流和历史保留；不能把显示标签变更当执行成功。 |
| M06 M1 | 列表／看板／表格／泳道及个人／共享保存视图 | 局部：任务多视图，E4 | 保存筛选、列、分组、可见性和刷新恢复的完整闭环。 |
| M07 M1 | 侧边预览、菜单排序、快捷键与批量编辑 | 局部：详情面板，E4 | 批量派发前显示影响和启动数量，保留每项失败及可访问性。 |
| M08 M1 | 子事项及重复事项关联／撤销 | 局部：依赖、问题回流，E7 | 真实父子范围、重复标记、原事项回链和撤销状态。 |
| M09 M1/M2 | 评论、附件版本、@成员／Agent | 局部：交付与原件，E4/E8 | 持久线程、精确主体引用、附件版本及通知来源。 |
| M10 M1 | 指派人、Agent或Squad，不同触发行为 | 局部：团队及工具预览，E4 | 清楚区分责任分配与执行派发，避免改负责人即偷偷开跑。 |
| M11 M0/M2 | Agent命名、模型／运行时／指令及AI配置生成 | 局部：资源、工具、模型，E3/E4 | 原生Agent详情／创建向导、配置修订和可审阅AI生成。 |
| M12 M2 | Agent可见／可运行范围、在线与负载、归档恢复 | 局部：当前权限／库存，E9 | 显示可见性、准入及健康原因；单独归档，不丢历史。 |
| M13 M2 | 混合人和Agent的Squad、Leader路由及成员角色 | 局部：专家团队计划，E4 | 团队长期编制、领导策略、成员能力与可视化变更。 |
| M14 M2 | Leader收到成员结果后再次评估并协调 | 局部：DAG与质量回执，E4/E7 | 真实事件触发再规划，预算及人工门禁不被内部聊天绕过。 |
| M15 M2 | 技能导入、文件树编辑、更新及附加Agent | 局部：版本化技能，E5 | 包来源、更新差异、支持文件及跨Agent消费确切版本。 |
| M16 M0/M2 | 添加电脑、命令引导、发现已安装CLI | 局部：Runner及接入向导，E3 | 本机一键发现／认证诊断，真实不同机器出站配对。 |
| M17 M2 | 私有／公共Runtime、访问范围、并发与目录锁 | 局部：资源池、授权及独立工作树，E4/E9 | 可理解的设备详情、占用队列及物理多机能力验收。 |
| M18 M1/M2 | 排队、离线、目录等待、执行、失败分类及有限重试 | 局部：原持久DAG，E3/E4 | 面向用户的原因链和修复入口，未知效果不重复执行。 |
| M19 M1 | 实时Transcript、工具／命令／错误和顺序切换 | 局部：宿主事实／attempt历史，E9 | 单一会话时间线、下钻原证据、虚拟列表与滚动位置。 |
| M20 M0/M1 | 事项运行内追加引导，保留原目标 | 待交付：原固定计划不等于动态消息 | 稳定消息收件箱、当前轮送达／未送达／拒绝的真实回执。 |
| M21 M1 | 独立私密Agent对话、多轮上下文 | 待交付：已有计划输入并非长期聊天 | 会话归属、隐私、上下文版本和刷新恢复。 |
| M22 M1 | 排队消息编辑／移除／清空与Steer | 待交付 | 区分下一轮、当前步骤后接收及中断重开，不能统一成“已发送”。 |
| M23 M1 | 任意页浮动对话、缩放／最小化及快捷键 | 待交付 | 共用当前会话，焦点恢复与窄屏等价入口。 |
| M24 M0/M2 | Autopilot创建事项或仅运行两模式 | 局部：定时触发，E6 | 明确离线时排队／跳过和是否创建业务对象。 |
| M25 M2 | Cron、Webhook、事件过滤及投递历史 | 局部：自动化／签名入口，E6 | 同入口下简单规则编辑和可解释触发／拒绝历史。 |
| M26 M1 | 事项事件／时间／条件唤醒 | 局部：DAG Timer/Signal，E6 | 对业务事项的可见唤醒规则与去重投递，不另建调度器。 |
| M27 M0/M2 | Inbox订阅、未读、处理／归档和通知偏好 | 待交付：已有告警并非统一收件箱 | 审批、阻塞、完成统一路由至准确任务和attempt。 |
| M28 M0 | 按Agent、事项、运行查看用量与成本 | 局部：Go真实账本，E3 | 分层可下钻汇总、缺usage／未知账单明确表达。 |
| M29 M0/M2 | GitHub、GitLab、Gitea／Forgejo的PR及CI镜像 | 局部：受控仓库集成，E8 | 每提供方真实OAuth／Webhook／PR／CI状态，不能仅靠本地Git。 |
| M30 M0 | 桌面自动注册本机Runtime，Web与移动访问 | 局部：桌面／Web，E1 | Windows/macOS/Linux实机安装；移动端及同会话跨端体验。 |
| M31 M0 | CLI／API、认证令牌与Agent可脚本化驱动 | 局部：正式CLI／MCP，E9/E10 | 完整操作合同、权限、分页、终态及SDK例子逐项验收。 |
| M32 M0 | Compose／Helm自托管与遥测选择 | 局部：部署配置，E10 | 独立／SaaS／分布式实际安装、升级、备份、容量及可视诊断。 |

Multica本次README列出26类Runtime：Claude Code、Codex、Cursor Agent、Copilot、OpenCode、OpenClaw、Hermes、Pi、Antigravity、CodeBuddy、DevEco、Grok、Kimi、Kiro、Qoder／Qoder CN、Qwen Code／QwenPaw、Reasonix、Trae、DSH、Oh-My-Pi、MiniMax Code、Dim、CodeArts、ZeroClaw。速码不得因统一命令档案可配置就声称这26项均接入；每项仍须能力发现、版本、认证、会话、进度、等待输入、停止、恢复及独立质量证据。

## WorkBuddy：本地／云端任务与可编辑交付

| ID／来源 | 公开功能及页面交互 | 速码对应与证据 | 必须补齐的功能与验收 |
| --- | --- | --- | --- |
| W01 W0 | 新建独立任务、选择或自动建立工作目录 | 局部：正式任务／独立attempt，E1/E4 | 面向用户的一步建立任务工作台与目录身份。 |
| W02 W0 | Agent／Plan／Ask三种工作模式 | 局部：固定只读规划和编码purpose，E3/E4 | 清晰模式切换、能力差异和进入执行的明确确认。 |
| W03 W0 | 追问澄清、完整计划侧览、确认／修改 | 局部：规划草稿，E4 | 持久澄清问题、可编辑步骤和原版本采用。 |
| W04 W1 | 内置、自定义及本地模型列表与管理弹窗 | 局部：冻结部署／模型，E3 | 真实能力目录、配置诊断、变更后新计划和离线可用说明。 |
| W05 W1 | 快速／均衡／极致、思考强度、上下文与Max | 局部：显式参数／预算，E3 | 可解释策略到实际Binding，不照抄厂商模型推荐或伪造最优。 |
| W06 W0/W1 | 任务输入框挑选、导入及启停技能 | 局部：组织技能版本，E5 | 当前任务的技能选择、依赖和生效范围。 |
| W07 W1 | 专家／专家团目录、最近使用和召唤 | 局部：专家团队资源，E4 | 完整角色入口、搜索、能力说明及会话身份。 |
| W08 W1 | 对话创建／修改专家、分享副本和删除 | 局部：资源配置，E4 | 可审阅创建向导、修订、分享权限及撤销行为。 |
| W09 W1 | 专家团拆解、并行协作及按需连接服务 | 局部：正式团队DAG，E4 | 多角色会话、派工事实、缺依赖等待与授权后原步骤恢复。 |
| W10 W1 | 连接器授权、启停、解绑、失效后重新连接并继续 | 待交付：项目MCP只覆盖Suma端点，E9 | 每服务真实凭据范围、原调用去重、写入前确认。 |
| W11 W1 | 自定义MCP、工具级控制与应用界面 | 局部：自有MCP服务，E9 | 外部MCP客户端、交互应用、隔离与工具权限的完整界面。 |
| W12 W0 | 文件拖入、图片粘贴、附件卡片和长文本折叠 | 局部：资料／原件，E5 | 输入草稿保留、上传错误、来源标记，不自动发送附件。 |
| W13 W0 | 办公／开发／设计模板、斜杠命令及项目补全 | 局部：SPEC／技能配置，E5 | 搜索、参数提示、当前项目命令隔离与热刷新。 |
| W14 W0 | 语音输入、音频转写及上传前确认 | 待交付 | 本地／服务端来源、时段引用、多语言及删除／重试。 |
| W15 W0 | 同一对话直接呈现接手、步骤、耗时和结果 | 局部：真实事件／日志，E3/E9 | 单一原生任务时间线，不能依赖动画制造进度。 |
| W16 W0 | 运行中追加消息、后续建议填入但不自动发送 | 待交付 | 追加上下文的固定来源、送达状态和预算／权限重核。 |
| W17 W0 | 中断后继续、循环检测与终止单次请求 | 局部：取消／核对，E3 | 真实停止证据、重复调用边界、保留已写结果。 |
| W18 W0 | 引用选中文本、表格及Widget并附批注 | 待交付 | 精确片段版本引用，安全预览，失效后重新选择。 |
| W19 W0 | 任务／空间／产物／资料全局搜索，状态／日期筛选 | 局部：任务和知识检索，E5 | 分组命中、高亮、下一页及准确原件定位。 |
| W20 W0 | 未读／待确认状态、置顶、重命名、归档和批量管理 | 局部：正式任务列表，E4 | 完整个人工作列表，归档与业务完成分别建模。 |
| W21 W0 | 工作空间分组、显示名称、打开及从列表移除 | 局部：目录授权，E1 | 最近项、重启保留、移除不误删实际目录。 |
| W22 W0 | 任务分享、独立窗口、系统／外部IDE打开 | 局部：桌面基础，E1 | 当前权限下的链接、窗口生命周期与正确工作区身份。 |
| W23 W2 | 分支搜索／切换及未提交改动的Commit／Stash／放弃 | 局部：只读Git和受控候选，E2/E4 | 真实Git写操作、文件选择、二次确认和失败回滚。 |
| W24 W2 | Worktree并行任务、基础分支固定、创建进度及清理 | 局部：attempt工作树，E4 | 用户可管理的任务工作树、快照盘点及安全归档；不能照搬自动清理阈值。 |
| W25 W0/W2 | 产物卡片、数量、预览／另存为／默认应用／加入上下文 | 局部：原件及制品，E5/E8 | 同任务真实产物清单与文件操作状态，不暴露不必要本机路径。 |
| W26 W2 | Word预览、手工编辑与AI修改 | 待交付 | 成熟文档引擎、格式保真、选区依据和导入导出。 |
| W27 W2 | Excel表格、手工／AI编辑与保存 | 待交付 | 公式、单元格范围、计算结果与导入导出实际验证。 |
| W28 W2 | PPT预览、编辑及AI协作 | 待交付 | 页面／对象选择、版式、字体与导出保真。 |
| W29 W2/W3 | Markdown所见即所得、源码／可视切换和保存 | 局部：文本编辑，E2 | 文档级可视编辑、引用、冲突及保存状态。 |
| W30 W3 | HTML预览、选元素改样式、AI编辑及动态元素保护 | 待交付：静态部署不是可视化编辑，E8 | 隔离预览、版本化选区、资源边界和真实保存。 |
| W31 W2 | 人机双写、最新内容同步、选区修改、高亮、撤销重做 | 待交付 | 协作引擎／CAS、覆盖冲突、修改来源及撤销完整性。 |
| W32 W2 | 多浏览器标签、账号隔离登录态、清Cookie、查找／缩放／截图 | 待交付 | 受控浏览器会话、真实授权、网站权限与安全预览。 |
| W33 W2 | 多终端标签、实际命令与工作区输出 | 待交付：固定命令节点不是交互PTY | 可控PTY、进程／目录身份、断线恢复和停止语义。 |
| W34 W2 | 独立文件窗口、系统文件关联、最近项和随文件对话 | 待交付 | 文件生命周期、独立窗口与主任务同步，未保存退出保护。 |
| W35 W1 | 项目模板、成员邀请审批、共享指令／技能／专家 | 局部：组织／团队，E4/E10 | 项目配置会话注入与完整邀请／申请收件箱。 |
| W36 W1 | 任务流转：产物、摘要、处理人／截止日期及附件 | 局部：代码基线交接，E4 | 通用任务包交接、权限重核、原上下文版本与新会话。 |
| W37 W3 | 我的文档／团队资料、成员权限和评论 | 局部：项目资料，E5 | 完整共享目录、评论、协作权限与内容更新反馈。 |
| W38 W1/W3 | 文件分类、上传、原产物入库、压缩包网页转换与更新人 | 局部：原件版本，E5/E8 | 多格式预览／解析、原件保留、转换失败和容量管理。 |
| W39 W3 | 轻量网页发布、关闭公开、HTML与CSV双向轻应用 | 局部：正式静态发布，E8 | 文档分享与软件部署分开；数据写回授权、撤销及一致性。 |
| W40 W4 | 网站需求→预览／修改→绑定云服务→更新／下线 | 局部：应用及静态发布，E8 | 前后端开发预览、通用运行环境、域名与真实远端部署。 |
| W41 W4 | 微信小程序试用／体验／正式发布及管理 | 待交付 | 官方账号授权、预览码、审核状态与取消／下线实际协议。 |
| W42 W4 | 应用数据库、身份认证、文件存储、统计和额度管理 | 待交付：平台账号不等于生成应用的身份服务 | 正式资源提供方、应用数据隔离、环境及用量事实。 |
| W43 W4 | 设计创意场景、示例及生成内容继续修改 | 待交付 | UED素材与需求版本绑定、可编辑交付和引用许可。 |
| W44 W4 | Buddy应用创建／安装／管理 | 待交付 | 应用生命周期、权限、运行时和原项目关联。 |
| W45 W5 | 本地／项目／云定时任务、手动／模板／对话创建、运行历史 | 局部：正式自动化，E6 | 场景化频率配置、有效期、产物通知及各来源可见性。 |
| W46 W5 | 微信、企微、QQ、飞书、钉钉助理远程任务 | 待交付：签名入口不等于厂商渠道，E6 | 每服务真实账号、手机触发／电脑执行／结果回传与在线依赖。 |
| W47 W5 | 独立助手邮箱、收发列表／附件、引用、邮件触发和发送确认 | 待交付 | 邮箱提供方、真实身份、幂等投递、审核和统一收件箱。 |
| W48 W1 | 每日记忆整理、查看／更正／删除／导入及关闭 | 局部：知识提案，E5 | 私人长期记忆、范围、再授权、撤销后不再进入上下文。 |
| W49 W5 | 设置搜索、语言／主题／字体／快捷键、代理诊断、自启及更新 | 局部：中英／桌面配置，E1/E10 | 完整可视配置、即时生效范围、系统通知和三系统体验。 |
| W50 W5 | 工作区信任／受限模式、文件／命令／网络三态、备份和审计 | 局部：固定工具／当前权限，E9 | 专用策略编辑／解释／恢复；不能称六工具受控边界为通用OS沙箱。 |
| W51 W5 | 套餐、用量、积分、账单发票与注销入口 | 待交付；本次仅官方导航可见 | 商业合同、真实计费／支付／开票及账号数据生命周期逐项交付。 |
| W52 W1 | ima、乐享及腾讯文档检索／保存与共同编辑 | 待交付：项目知识不等于这些服务 | 按用户授权的真实资料范围、精确引用、写回与撤权。 |

WorkBuddy可见交互阈值（附件数量、转写时长、Worktree清理数量、套餐资源点等）只作为来源记录，不直接硬编码为速码默认。速码按资源能力、项目配置及真实安全边界设定；厂商内置模型清单也不冻结为速码“所有模型”。

## Octop：专家、多入口与专业助手控制台

| ID／来源 | 公开功能及页面交互 | 速码对应与证据 | 必须补齐的功能与验收 |
| --- | --- | --- | --- |
| O01 O0/O1 | 初始化向导、用户／管理员、JWT、用户组及页签权限 | 局部：当前身份／组织治理，E9/E10 | 完整账号向导、组可见性、登录诊断及第三方SSO。 |
| O02 O0 | 每用户多个独立专家、库／市场、头像和系统提示 | 局部：专家资源，E4 | 用户可用的专家创建／搜索／选择／停用全流程。 |
| O03 O0/O1 | 16类MBTI与自定义性格 | 待交付 | 可编辑人格模板与作用范围；人格不能改变权限或门禁。 |
| O04 O0 | 专家共享／发布和同实例资源范围 | 局部：团队／技能范围，E4/E5 | 明确配置所有权、共享消费、撤销及历史版本。 |
| O05 O0/O1 | 每专家供应商／模型、协议及工具配置 | 局部：统一Go网关，E3 | 专家界面到真实Binding，保持Go唯一凭据／额度权威。 |
| O06 O1 | AgentTeams主持人＋至少两专家，成员时间线气泡 | 局部：正式专家任务DAG，E4 | 长期团队聊天／角色、真实派工消息和成员结果。 |
| O07 O1 | 主持人异步并行派工、同步问同事和回叫 | 局部：DAG与独立执行，E4 | 严格收件箱／预算、在途改编制限制和跨团队来源。 |
| O08 O1 | 停主持人与成员排空分开，团队及成员侧会话 | 局部：真实停止／恢复，E3/E9 | 明确单轮、子任务及整个团队的停止对象与事实。 |
| O09 O0 | 连接另一实例、浏览对端专家和跨实例对话 | 待交付 | 真实远端身份／授权、记录归属、网络中断与撤销。 |
| O10 O0 | 专家／团队实时对话，SSE／WS与刷新保留进行中消息 | 局部：运行事实通道，E3/E9 | 原生持久会话、附件及明确当前轮／后续轮。 |
| O11 O0 | 浏览器内交互Shell和AI辅助排障 | 待交付：命令节点非PTY | 真实终端、命令风险控制、会话重连。 |
| O12 O0 | Chromium/CDP会话、网页自动化、截图与远程控制 | 待交付 | 账号隔离Profile、当前授权、真实网站操作与证据。 |
| O13 O0 | Web内远程桌面、键鼠及隔离Linux桌面 | 待交付 | 设备会话、可见授权、录屏／输入范围与实际停止。 |
| O14 O0 | 自然语言／斜杠及可视Cron主动任务 | 局部：自动化，E6 | 在当前专家／项目直接创建、查看原运行和产物。 |
| O15 O0 | 飞书、钉钉、QQ、微信、企微及网关扩展 | 待交付：通用入口基础，E6 | 五类渠道分别认证、关联会话、主动上报、重放与撤权。 |
| O16 O0/O1 | OAuth应用、MCP网关、自定义MCP与探测 | 局部：自有项目MCP，E9 | 外部客户端、凭据保险库及真实服务授权全流程。 |
| O17 O1 | 内置连接器目录的鉴权、启用及工具使用 | 待交付；29项见下文 | 每连接器独立权限、查询／写入、秘密及异常恢复。 |
| O18 O0 | 文档知识库、RAG、语义检索和同实例共享 | 局部：正式项目知识，E5 | 文档导入、解析、引用、索引水位及共享权限全链路。 |
| O19 O0/O1 | 可迁移分层记忆、全文召回及独立后端 | 局部：知识／记忆条目，E5 | 真实私人／专家长期记忆、迁移、来源和失效机制。 |
| O20 O0/O1 | 技能包共享、支持文件树与子Agent资源 | 局部：组织技能，E5 | 安全包导入／编辑／版本／更新与执行读取。 |
| O21 O0/O1 | 内置／第三方插件安装、启停及配置 | 待交付：扩展合同并非完整宿主 | 隔离宿主、签名／来源、权限、升级及卸载清理。 |
| O22 O0 | `octop acp`供IDE／终端调用专家 | 局部：公开Harness ACP，E3 | 完整Octop CLI专家／会话语义独立接入，不能混称已完成。 |
| O23 O0 | ACP出站委派OpenCode、CodeBuddy、Claude Code、Codex | 局部：各工具独立合同，E3 | 四工具分别完整双向会话、等待、取消、恢复及权限。 |
| O24 O0/O1 | 专家文件本地／Docker／PostgreSQL／COS/S3后端 | 局部：工作区与原件接口，E1/E9 | 各后端真实读写、隔离、迁移及错误恢复；区别控制面数据库。 |
| O25 O0/O1 | SQLite或PostgreSQL控制面、重启重建与日志保留 | 局部：SQLite／数据库适配，E10 | 实际OceanBase／PostgreSQL事务、备份恢复与分布式验收。 |
| O26 O0 | 搜索供应商与模型工具开关 | 局部：模型／工具配置，E3 | 每种搜索、图片／视频工具的真实合同与UI，不能从开关推出已支持生成。 |
| O27 O0/O1 | Shell防护、敏感内容处理与高风险工具审批 | 局部：当前权限／固定工具，E9 | 可视规则、范围和审批原依据，未知不自动放行。 |
| O28 O0/O1 | 密钥加密保存、隔离工作区与上传限额 | 局部：Go独占模型秘密，E3/E9 | 全连接器秘密保险库、轮换、导出与部署边界验证。 |
| O29 O1 | 用户、安全、TLS、时区、模型和存储可视设置 | 局部：中英／组织设置，E10 | 配置验证、生效时机、权限、回退和诊断。 |
| O30 O0 | CLI管理、HTTP／SSE／WS接口及按需API文档 | 局部：正式API／CLI，E9 | 全域可操作客户端、SDK和MCP／A2A一致权限。 |
| O31 O0 | Windows/macOS/Linux桌面及FnOS安装 | 局部：六目标打包合同，E1 | 各系统安装、升级、签名和本地恢复；FnOS未交付。 |
| O32 O0 | 单命令运行、系统服务、Docker及私有云安装 | 局部：本机服务／部署配置，E1/E10 | 安装向导、服务健康、独立／分布式／SaaS实机完整交付。 |

Octop固定目录列出29个连接器：腾讯文档、IMA、腾讯会议、腾讯新闻、微信读书、乐享、微云、个人邮箱、QQ音乐、飞猪、百度地图、携程问道、美团旅游、滴滴、元典、企查查、腾讯Ardot、有道云笔记、Notion、OpenAlex、滴答清单、飞书CLI、企业微信CLI、钉钉CLI、Agent Mail、Obsidian、语雀官方MCP、WeKnora、Dify。速码目前不能将任何一项标为因“存在通用MCP”而完成；每项要登记官方协议／动作／版本、可用权限、UI、实际调用和撤权证据。自定义连接器仍需相同合同。

上游明确未完成的范围单独保留：AgentTeams为Beta，移动客户端内测；浏览器操作录制、自进化沉淀技能、托管Agent生命周期、项目边界、云边协同、插件市场及对话式全控制面仍为规划。速码需要这些能力时按自己的正式领域实现，不能把Octop的规划打勾当作速码实现。

## 实施顺序与可验收结果

1. **原生开发工作台。** 先把W01/W03/W12/W15/W16/W18/W25与M19/M20/M21连接成一个真实任务会话：固定上下文、可审阅计划、真实执行／消息回执、代码和产物、独立测试、原Review；进度、等待和失败不分散到多个不相关页面。
2. **完整开发工具面。** 在同一工作台补M16/M17/W23/W24/W32/W33：设备发现、分支／工作树、真实终端、预览、目录／模型／账号状态，现有Go DAG仍唯一派发。
3. **能力与知识资产。** 补W06–W11/W35–W38/W48和O02–O09/O16–O21，形成资源配置、当前授权、固定版本、会话消费、成果提炼及可撤销共享。
4. **办公和多入口。** 补W14/W26–W34/W39–W47及O11–O15/O31，复用成熟编辑器、媒体、浏览器、渠道和安装框架，逐系统／服务留证。
5. **商业和规模交付。** 补W51及全部跨物理机器、SSO、分布式数据库、对象存储、安装／升级、账务与生产容量，完整P0–P7继续推进。

每个纵向切片的验收最少包含：真实当前身份和权限；界面创建／编辑；实际后台操作；断线或冲突后的原操作恢复；刷新回到原对象；中英文、键盘和窄屏；准确源码／版本／原件及证据。只验证改动的关键边界和一次代表性实际链路，复用未变证据，不重复付费模型或完整SDK构建。实现、协议夹具、真实厂商／机器运行及生产签收分别记录，剩余行不能因同类已通过而自动关闭。

*English: This is a public-capability parity backlog, including product pages and interaction requirements, not an adapter checklist or completed parity certification. Public sources are pinned or dated; unavailable details remain unknown. Suma must implement and verify each native user journey independently while preserving the full platform scope, one control plane, exact evidence and human gates.*
