# 参考平台能力与当前交付边界

产品统一为 **1.0.0**。完整P0–P7与需求1–26及58项限定实现／34待办／92总账保持不变，见[交付状态](../delivery/status.md)和[实施计划](../delivery/implementation-plan.md)。[Octop、WorkBuddy、Multica逐项产品能力表](reference-platform-parity.md)列明公开功能、页面、交互、实现及证据差距，[原生工作台规格](../design/reference-workbench.md)定义完整体验；不能以适配器清单替代完整产品能力。

当前T-WORKBENCH-126推进原生IDE中的正式任务、需求／设计、负责人、准确运行、计划用例及正式验收同页关联，复用[任务上下文合同](../../contracts/workspace-task-context.md)。[限定报告](../quality/workspace-task-context-verification.md)及[摘要](../quality/workspace-task-context-summary.json)由实际验证生成，不提前声明通过。选择任务不改工作区绑定、不派发或批准；PTY、LSP／DAP、持续聊天和办公协作仍完整保留为待交付目标。

## 证据规则

源码实现须有明确领域和合同；本地／协议验证只证明报告所列源码、环境与场景；真实厂商运行需要准确CLI／SDK、实际账户和完整回执；生产验收还需部署、隔离、恢复及容量证据。旧报告保留原范围，新变更不自动继承通过。业务验收、执行成功、独立测试与发布健康分别表达。

## 逐平台采用

| 平台与固定来源 | 当前速码实现及对应证据 | 完整产品仍需交付的能力 |
| --- | --- | --- |
| 原生模型选择与预算 | [统一Go网关](../architecture/model-gateway.md)及[当前报告](../quality/model-gateway-verification.md)：原生与Octop共享冻结模型、参数、费率、attempt额度及真实请求账本；[跨普通／专家／修复入口](../quality/model-selection-workflows-verification.md)和[智能推荐](../quality/dispatch-recommendations-verification.md)分别限定 | 多供应商完整协议与报价、外部账单核对、长期调优／路由、真实账户及生产；部分usage不能补零，配置额度不代表绝对供应商账单保证。 |
| [Multica当前README](https://github.com/multica-ai/multica/blob/b2e58d0bf8a8e113b9c7bc895f4213f08442833a/README.md) | Go DAG、共享池、专家计划、独立工作树与候选／基线；[团队编排](../quality/expert-team-orchestration-verification.md)、[候选修订](../quality/candidate-repair-revisions-verification.md)、[任务交付](../quality/task-delivery-chain-verification.md)。最新／本地／历史来源及逐项差距见[专表M01–M32](reference-platform-parity.md) | 原生多轮对话与运行内引导、完整事项配置／共享视图、Squad会话、Inbox、各Git／IM服务、真实多物理机器和安装矩阵。当前许可有额外托管／商业嵌入条件，不按纯Apache2复制到速码SaaS。 |
| [Hermes](https://github.com/NousResearch/hermes-agent/commit/cb757fac59c92b49e9b8f2910e5cd7ab75efffdd) | 原生工具／技能、记忆及知识提案有对应基础，见[项目知识](../quality/project-knowledge-verification.md)；这些不是Hermes实际运行证据 | 完整Hermes协议、账号、会话、技能／记忆消费、取消与恢复；历史探测不作为当前可用证明。 |
| [OpenCode](https://github.com/anomalyco/opencode/tree/148842ef7cb1bc34192b3fff57d0f9721ab9c801) | [固定官方SDK／server1.19.4](../../contracts/opencode-sdk.md)、六工具及Go宿主封存；[实际CLI配自有模型端点](../quality/opencode-actual-execution-verification.md)完成两Worker编码／候选交接／独立测试 | 商业模型质量／身份／费用、完整会话恢复、IDE功能、物理多机／跨平台运行与生产。 |
| [Zed](https://github.com/zed-industries/zed/commit/e96ffb0fd3cc43ec56dcff421146f5e823b13841) | [Monaco工作台](../../apps/web/src/IDE.tsx)、[文本与保存](../quality/workspace-file-context-verification.md)、[目录](../quality/workspace-directory-verification.md)、[只读Git](../quality/workspace-git-verification.md)；本阶段接原正式任务上下文 | 真实PTY、LSP、DAP、调试、预览、协作、多根工作区、受管Git写操作和远端完整IDE；打开外部IDE不等于接入其能力。 |
| [spec-kit](https://github.com/github/spec-kit/commit/27d14e20c187c1f2916540ea201ecbdc9f8ed588) | [需求／设计提案](../architecture/requirement-design.md)、[追踪](../quality/requirement-traceability-verification.md)、任务版本及验收基础 | 完整spec-kit CLI互操作、SPEC到设计／代码／独立测试／发布的持续版本追踪和冲突治理。 |
| [Bubble Tea](https://github.com/charmbracelet/bubbletea/commit/f9df43c4f2c021d8740a2628d59cd810a862eb98) | [直接依赖v1.3.10](../../go.mod)、[正式API只读TUI](../../internal/cli/tui.go) | 完整可操作终端工作台、键盘旅程、跨平台安装与真实恢复；只读列表不是完整IDE。 |
| [DeepSeek Harness／DSH](https://github.com/deepseek-ai/deepseek-harness/commit/687f282e8625c6b8d11ff58211a90524be2e4317) | [既有JSONL／Node合同](../integrations/team-cli-runtime.md)；[本地真实进程、双向输入和停止证据](../quality/team-cli-verification.md) | 真实dsh厂商任务、完整模型与会话语义、恢复／质量及生产；协议夹具不代表完整上游产品。 |
| Codex | [官方app-server合同](../../contracts/codex-app-server.md)、[真实CLI＋自有Responses端点编码](../quality/codex-app-server-verification.md)；[专家与修复跨入口](../quality/model-selection-workflows-verification.md)覆盖逐任务原生／Codex选择，原生修复显式选择；[活动工作区](../quality/codex-workspace-observation-verification.md)有独立范围 | 真实供应商账户／效果／结算、完整会话及等待输入、Codex专用修复工作区、远程IDE与生产。自有端点协议不等于付费厂商运行；Codex用量及提醒不冒充原生enforce预算。 |
| [WorkBuddy官方文档](https://www.codebuddy.cn/docs/workbuddy/Overview) | 项目、执行、技能／知识、原件和自动化有基础；官方功能及交互拆为[W01–W52](reference-platform-parity.md)，不使用CodeBuddy CLI或第三方OpenWorkBuddy冒充官方产品 | 完整本地／云会话、产物与人机双写、专家／插件／连接器、任务流转、终端／浏览器、邮箱／IM、网站／小程序／云服务、个人记忆、安装及商业交付。 |
| [OpenWorkBuddy固定来源](https://github.com/CatCatUncle/openworkbuddy/tree/2e76920dabc3ac057283148ccaa5f8dbb0489530) | 研究0.17.5公开交互、桌面与办公合同；[来源和许可](../integrations/development-platform-reference-extensions.md)保留原记录 | 独立第三方项目，无实际互操作证据；PolyForm Noncommercial不按宽松许可拷入速码SaaS，完整办公／多入口仍须独立实现。 |
| [Octop固定产品1.0.3b1](https://github.com/TencentCloud/Octop/tree/890111b7879665840414ca28ab168b05b0db0921) | [项目MCP](../quality/octop-integration-verification.md)、公开Harness1.0.5／ACP0.12.1受控编码，共用Go网关、六工具、候选和质量；[当前证据](../quality/model-gateway-verification.md)。原生页面及公开功能见[O01–O32](reference-platform-parity.md) | 完整专家／Teams会话、共享资源／记忆、连接器OAuth、浏览器／终端／桌面、多渠道、插件及安装；完整Octop CLI／GUI、厂商和分布式生产须独立验收。单个SDK不能关闭其余产品能力。 |

CodeBuddy是已实现受限执行合同的另一CLI，见[团队运行时](../integrations/team-cli-runtime.md)；它不替代WorkBuddy，也不使其他CLI自动获得整体终态、取消和恢复。各工具来源与版本独立，不因为名称相似共享证据。

## 原生产品交付顺序

| 优先级 | 完整纵向能力 | 正式边界与完成判据 |
| --- | --- | --- |
| 当前T-WORKBENCH-126 | IDE中选择真实任务，读取需求／设计、负责人、准确运行和逐例质量，进入原派发／测试／Review | 原[任务上下文读取](../../contracts/workspace-task-context.md)和CAS编辑共存；刷新／中英／权限／原保存恢复留证。选择任务不自动绑定该任务工作树，不创建、派发或验收。 |
| P1 原生任务协作 | 目标、可编辑计划、固定资料、持续消息、实际步骤和产物同页 | 持久会话与运行分别建模，真实送达／未知／停止事实；独立测试和人审门禁继续权威。 |
| P1 工具和模型 | 当前库存、模型、参数／预算和人工采用到原计划 | [智能推荐](../architecture/dispatch-recommendations.md)已有限定交付；后续补完整目录、动态会话和真实厂商。未知价格不参与伪造评分。 |
| P2 完整IDE | 多标签／搜索、真实PTY、语言服务、调试、Git写入、预览和远端工作区 | 复用当前目录授权、隔离attempt及唯一Go DAG，完成真实编辑→运行→调试→独立质量／恢复旅程。 |
| P2 专家与资产 | 专家／团队、技能／插件／MCP、知识／记忆、版本／共享／授权 | 资源配置到任务真实消费，成果提炼经审阅采用；删除／撤权后不再进上下文，秘密不随共享包流转。 |
| P3 办公、多入口及部署 | 办公原件、浏览器／桌面、IM／邮箱、应用交付、跨端安装及SaaS | 各格式、服务、设备、存储和安装目标分别真实验收；SSO、数据库、支付／商业、跨物理机器与容量按完整台账交付。 |

所有页面须中文／英文完整标签、加载／空态／错误／无权限／断线／冲突与原操作恢复，支持键盘、窄屏和可恢复深链。基础设施、渠道、SSO、OceanBase、多模态、对象存储、安装升级及生产容量仍按[完整实施计划](../delivery/implementation-plan.md)推进。本矩阵不新增整项勾选、不删减目标，也不把设计规格当实现证据。

*English: Full native product workflows and interfaces remain the target. The detailed Octop/WorkBuddy/Multica parity backlog covers their public capabilities, not just adapter names. Current native/Octop gateway execution, real Codex/OpenCode protocol runs, expert model selection and reviewed dispatch recommendations have separate scoped evidence. The T-WORKBENCH-126 task context is a bounded native IDE slice; terminal, full IDE, continuous collaboration, office formats, channels, multi-device installation and production remain independent deliverables. No full parity or production certification is claimed.*
