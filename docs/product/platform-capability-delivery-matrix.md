# 参考平台能力与交付差距矩阵

状态：1.0.0／T-P4-034 实施输入，2026-10-27。负责人：平台集成。完整P0–P7及需求1–22继续实施；本页不是“具备所有参考平台能力”的声明。

依据：[当前交付](../delivery/status.md)、[完整能力矩阵](capability-matrix.md)、[1.0.0基线](v1-baseline.md)、[固定参考提交](../integrations/references.json)、[新增参考与许可](../integrations/development-platform-reference-extensions.md)。本轮复用已记录基线，没有重新克隆参考仓库、安装工具或调用厂商模型；部分固定README网页读取返回缓存缺失，未据此推断新的接口或运行能力。

## 判定规则

“源码实现”要求有明确模块与合同；“本地／协议验证”只证明列出的夹具或本机行为；“真实工具”要求具体版本、实际账户／CLI及完整执行回执；“生产验收”要求真实部署、隔离、恢复与容量证据。配置示例、界面相似、可执行文件存在和HTTP200均不能代替后三类证据。当前下表没有任何整个平台获得生产验收结论。

## 逐平台对齐

“对齐方向”是速码要交付的能力分类，不是对参考平台全部功能的穷举。外部原始接口变化应再固定提交／版本，不能从名称猜测参数。

| 参考与对齐方向 | 速码源码实现 | 本地／协议证据 | 真实工具与生产差距 |
| --- | --- | --- | --- |
| [Multica](https://github.com/multica-ai/multica/tree/a47a1ced0c71bdb113a43facce4675442acb9001)：团队、机器、任务与工作区协同 | Go DAG、共享池与双向Grant、专家团队计划、候选合成／修订和固定基线 | [团队依赖编排](../quality/expert-team-orchestration-verification.md)、[033实际闭环](../quality/candidate-repair-revisions-verification.md)：同机两Runner、独立工作树与独立测试 | 未完成Multica外部服务互操作、跨物理机器与规模／生产验收；不能将同机进程称为真实远端集群 |
| [Hermes](https://github.com/NousResearch/hermes-agent/commit/cb757fac59c92b49e9b8f2910e5cd7ab75efffdd)：工具、技能、记忆与长期智能执行 | 原生Suma内核、受限项目工具、技能目录与知识提案 | 原生工具／技能和提案有各自本地证据，不是Hermes运行证据 | 历史Hermes版本探测退出1，完整协议／会话／取消／恢复接入未验证；不标记可用 |
| [OpenCode](https://github.com/anomalyco/opencode/commit/02671e66454260d77fe04d63cf5eb94e76921dee)：编码CLI与交互执行 | 工具配置与Runner能力注册基础存在 | 当前profile明确不可用；局部step完成不证明整个任务完成 | 需固定官方整体终态、输入等待、取消和恢复合同，并验证实际CLI／模型；生产未验收 |
| [Zed](https://github.com/zed-industries/zed/commit/e96ffb0fd3cc43ec56dcff421146f5e823b13841)：完整IDE与协作工作台 | [Monaco工作台](../../apps/web/src/IDE.tsx)、[工作区接口](../../internal/platform/workspace.go) | 文件读写与版本冲突是基础能力，完整IDE旅程未验收 | 终端PTY、LSP／DAP、调试、Git审查、预览与实时协作仍需完整实现；打开外部IDE不等于接入其开发能力 |
| [spec-kit](https://github.com/github/spec-kit/commit/27d14e20c187c1f2916540ea201ecbdc9f8ed588)：规范驱动需求、设计与拆解 | 需求／设计提案、批准快照、可审阅任务与版本化执行合同 | 既有需求设计本地验证见[能力矩阵](capability-matrix.md) | 没有完整spec-kit CLI执行互操作证据；规范正文、开发产物及质量门禁仍需持续追踪同一需求版本 |
| [Bubble Tea](https://github.com/charmbracelet/bubbletea/commit/f9df43c4f2c021d8740a2628d59cd810a862eb98)：成熟终端交互 | [go.mod](../../go.mod)直接依赖v1.3.10；[TUI](../../internal/cli/tui.go)复用正式API | CLI／TUI已有定向检查，非终端模式有明确输出语义 | 当前只读TUI不能描述为完整交互IDE或自主开发终端；跨平台安装与完整操作旅程仍待验收 |
| [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness/commit/687f282e8625c6b8d11ff58211a90524be2e4317)／DSH：编码执行协议 | [Harness／DSH适配](../integrations/team-cli-runtime.md)固定Multica JSONL与原生／Node入口 | 本地真实进程、双向输入、终态、进程停止和ACK验证 | 本地协议夹具不等于完整DeepSeek Harness产品或真实dsh／厂商运行；第三方会话恢复仍待接入 |
| Codex：编码CLI与开发工具 | 已有受限Codex JSONL适配与管理员固定命令 | [工具记录](../integrations/tools.md)包含历史安装／帮助探测；原生Suma SDK工具闭环另行验证 | 历史版本不代表当前库存；未将原生Suma的loopback模型结果算作真实Codex厂商编码任务 |
| WorkBuddy：办公／开发工具与消息入口 | 统一工具注册、执行和集成边界可复用 | [官方连接器参考与当前边界](../integrations/team-cli-runtime.md) | WorkBuddy客户端／连接器真实执行未验收；不与CodeBuddy CLI或OpenWorkBuddy混称 |
| [OpenWorkBuddy](https://github.com/CatCatUncle/openworkbuddy/tree/2e76920dabc3ac057283148ccaa5f8dbb0489530)：多形态入口、办公交付与共享技能／记忆 | 原生内核、桌面生命周期、技能与知识有基础模块 | 固定0.17.5源码／许可已核对；没有该产品实际接入证据 | 办公文件导入编辑导出、多形态一致会话、配额与真实安装需交付；该参考为PolyFormNC，当前只研究合同／交互，无源码复制 |
| [Octop](https://github.com/TencentCloud/Octop/tree/890111b7879665840414ca28ab168b05b0db0921)：专家、ACP／MCP与多入口 | 专家团队、MCP项目查询、技能及执行合同可复用 | 固定1.0.3b1源码／MIT许可已核对；未完成Octop服务互操作 | ACP双向、OAuth连接器、消息入口及跨团队隔离须逐项真实接入；Beta能力和单进程自托管不能替代速码分布式验收 |

CodeBuddy是另一个已实现受限适配合同的工具，见[团队CLI运行时](../integrations/team-cli-runtime.md)及[本地证据](../quality/team-cli-verification.md)。它既不替代WorkBuddy，也不使其他CLI自动获得整体终态或取消能力。

## 优先级与验收产物

| 优先级 | 下一项实际纵向能力 | 可复用模块与必须补齐的边界 | 完成判据 |
| --- | --- | --- | --- |
| P1 当前可用工具选择 | 从真实任务／流程／节点读取候选，解释不可用与不兼容原因；明确采用推荐后再预览派发 | [工具档案](../../internal/platform/tool_profiles.go)、[任务派发](../../internal/platform/task_dispatch.go)、[当前专家授权](../../internal/platform/expert_team_orchestration_authority.go)；分页、实时库存、路由与权限分别校验 | 通过界面选择真实候选，固定工具版本后执行；过期库存、撤权、错误路由阻止新工作；推荐不自动批准／派发 |
| P1 模型选择与成本 | 建立可授权的模型能力／参数／费率版本，按任务约束形成可解释计划，再接同一预算预留与用量结算 | [智能规划](../../internal/platform/intelligent_planning.go)、[费用合同](../../internal/platform/usage_accounting_model.go)；模型仍由工具固定的路径必须明确显示，未知费用不能排成“最便宜” | 实际执行使用准确所选模型／参数；费用未知、超配额、切换中途失败均有真实结果；不能将固定排序称为最优AI决策 |
| P2 完整IDE | 先保证编辑／保存／切换与冲突恢复可靠，再接终端、语言服务、调试、Git与预览 | 复用现有工作区与独立attempt，客户端不自行执行命令或突破路径授权 | 用户在同一项目／工作树完成编辑、运行、调试、差异审阅、独立测试及证据定位；断线／冲突不丢内容 |
| P2 技能、知识与记忆 | 统一版本与当前权限，授权检索、提案萃取、评估、采用与同步 | [技能目录](../../internal/platform/skill_catalog.go)、[项目知识](../../internal/platform/project_knowledge_service.go)及现有检索基础 | 检索结果与模型上下文再授权，过期／删除片段不回流；跨工具消费同一正式版本；私人凭据与未审查插件不进入共享内容 |
| P3 多入口与分布式交付 | 先完成真实官方CLI／平台适配，再接多物理机器、消息渠道、桌面及移动端 | 共用身份、DAG、持久事件、配额、幂等操作与停止／核对合同 | 外部账户和机器真实执行、断线重传、取消确认、未知结果核对及独立测试；Windows/macOS/Linux安装、SaaS／私有部署分别留证 |

每项均须中文／英文完整标签、简单折叠交互、加载／空态／失败／无权限状态及可恢复深链。权限、租户／组织／项目隔离、预算和人工门禁是统一控制面能力，不能由某个CLI或前端局部配置绕过。真实多模态存储、分布式数据库、SSO、渠道、跨平台安装与生产容量仍按[完整台账](../delivery/implementation-plan.md)推进。

*English: This is an implementation and evidence gap matrix, not feature parity certification. Prefer a governed real-tool selection-to-dispatch slice, then complete model policy, IDE and distributed/multi-entry journeys. Current local protocol evidence does not establish vendor or production acceptance.*
