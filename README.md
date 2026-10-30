# 速码 suma

**速码一栈式智能研发平台**：连接想法、项目、需求、设计、任务、智能体执行、独立测试、人工验收、版本发布与问题回流，提供浏览器和本地桌面入口。

当前统一开发基线为 **1.0.0**，完整平台仍在实现与验收中。产品范围与初始化策略见 [1.0.0 基线](docs/product/v1-baseline.md)，实际能力与限制见 [交付状态](docs/delivery/status.md)，完整 P0–P7 和扩展需求 1–26 见 [实施计划](docs/delivery/implementation-plan.md)。当前58项有限实现、34项待办，共92项；新增全平台智能输入／配置进入待办。

最近交付：[交接请求收件与准确定位](docs/product/task-handoff-inbox.md)沿原事务通知接收人，从收件箱直达准确交接，责任接收仍独立于正式Review。16项实际本机GUI／HTTP断言通过，视觉图片仍待验；见[报告](docs/quality/task-handoff-inbox-verification.md)及[摘要](docs/quality/task-handoff-inbox-summary.json)，完整58／34／92不变。

当前[WorkBuddy账号授权](docs/product/workbuddy-connections.md)沿成熟OAuth SDK管理当前项目本人的连接；浏览器与桌面同会话授权分别核验，桌面返回仅在无草稿／未确认写入时GET原连接，不reload或自动标为已连接。工具页默认收起；[连接证据](docs/quality/workbuddy-connections-verification.md)与[原生窗口证据](docs/quality/workbuddy-desktop-oauth-verification.md)分开记录，最终候选18项实际本机GUI／HTTP通过；视觉图片、真实供应方账号和云派发仍待验。

## 当前能力

- 组织、项目、团队、需求、版本化设计、任务与质量门禁共用当前身份、权限和操作回执。
- [需求验收追踪](docs/development/requirement-traceability.md)按当前正式版本连接发布、采用任务及确切运行，区分测试节点事实与人工验收；完整用例／应用版本矩阵继续待办。
- [项目风险](docs/development/project-risks.md)支持正式登记、建议采用、责任／评估、任务处置、固定复核和关闭／重开；代码缓解复用当前正式验收与独立候选测试。
- Go 持久 DAG、独立 Runner 和共享执行池支持固定计划、并发执行、进度回写、停止与未知结果核对。
- [交互终端与开发交接](docs/development/interactive-development.md)在T130增加原执行按失败核对、停止修改的明确候选预览／单独启动、同SHA独立测试及正式验收后的后续基线交接。启动Shell、交付未知、进程树停止、候选与人工Review各自独立；不重放输入或自动启动后续任务。[合同](contracts/terminal-recovery-candidates.md)、[当前报告](docs/quality/terminal-recovery-candidates-verification.md)／[摘要](docs/quality/terminal-recovery-candidates-summary.json)记录实际范围；[T129历史证据](docs/quality/interactive-development-verification.md)保留，Unix、多机和生产仍待验。
- [任务讨论与智能体回复](docs/development/task-conversation-replies.md)接入明确模型、单次参数、近期上下文及 Token／费用预算，复用原 Go 网关、DAG 与准确回复回执；[架构](docs/architecture/task-conversation-replies.md)、[合同](contracts/task-conversation-replies.md)及[当前验证](docs/quality/task-conversation-replies-verification.md)分别记录实现和验收范围。本地受控提供方的三轮／六条持久消息旅程及中英文1366／390实际界面已按报告限定验证。回复只生成文本，不执行终端命令、修改代码或验收成果；受控[PTY终端及明确开发交接](docs/development/interactive-development.md)的当前限定实现见T130入口，完整跨平台范围继续交付。
- 原生智能体、受限外部 CLI 和 Codex app-server 复用隔离工作树、固定候选、独立测试及明确验收；工具或模型的可用范围以实际库存和专项证据为准。
- 原生／OpenCode／Octop的Go宿主操作通过[工作区操作](docs/development/workspace-operations.md)关联真实调用、效果回执和确切尝试历史；SDK活动、本地采集、平台确认及独立测试分别表达。
- 项目知识、记忆、规范和技能支持版本、审阅发布、授权共享及模型提案；[讨论固定资料](docs/product/task-conversation-knowledge.md)明确选择最多16个已发布的准确ID／修订／SHA，历史按需展开，真实回复沿原开发预览与Start重验来源。该阶段实际GUI仅5项通过、4组待验，范围见[专项证据](docs/quality/task-conversation-knowledge-verification.md)；专家团队支持逐任务配置、依赖编排和受控交接。
- Monaco 工作区支持本地编辑、只读 Git 差异，以及当前原生执行工作区的小文本读取、CAS 保存和手工冲突比较。
- 自动化、签名事件、用量账本、本机制品和静态站点发布已有各自限定实现与验证。

完整 IDE、广泛厂商接入、跨物理机器及分布式规模、企业 SSO、移动端、支付、正式多平台安装和生产验收仍在范围内。局部运行、协议夹具或构建结果不代表完整平台完成。各领域设计、合同与验证报告统一由 [文档索引](docs/README.md)进入。

## 本地运行

要求 Go 1.26+（构建固定 1.26.10）、Node.js 22+、pnpm 11。可通过 `SUMA_GO_EXE` 指向私有工具链；构建脚本不修改系统环境。

```powershell
pnpm install
pnpm build
go run ./cmd/suma-control
```

打开 `http://127.0.0.1:4310`，首次创建组织、项目和管理员。默认数据目录为 `.suma/`，只监听本机，密码至少 12 个字符。1.0.0 采用单一新库初始化；既有运行数据、凭据和审计原件保留，不自动升级旧数据库，也不因版本统一清空数据。新实例须使用明确的新数据目录，具体启动配置见 [部署说明](docs/deployment/server.md)。

开发前端：`pnpm dev`；控制面继续监听 4310，前端 5173 代理 API。编辑项目文件前，设置 `SUMA_WORKSPACE_ROOT` 为明确授权的代码根目录，在 IDE 中绑定项目工作区。

执行节点需要从设置中创建凭证，并明确配置可执行命令。参见 [Runner 与工具](docs/integrations/tools.md)、[原生运行时](docs/agents/runtime.md)和 [Codex 接入](docs/integrations/codex-app-server.md)。未配置或未验证的服务须明确显示不可用。

## 开发与交付

```powershell
go test ./...
pnpm typecheck
pnpm test
pnpm build
pnpm check:docs
```

按变更选择必要检查，复用未变输入的有效证据。浏览器、桌面、数据库、模型账户与规模分别验收，历史报告保留其原环境与结论。

- [文档索引](docs/README.md)
- [系统架构](docs/architecture/overview.md)
- [DAG 调度](docs/scheduler/engine.md)
- [产品与用户旅程](docs/product/prd.md)
- [当前验证入口](docs/quality/verification.md)
- [部署与恢复](docs/deployment/server.md)
- [真实制品与本机发布](docs/deployment/release-control.md)
- [四程序跨平台运行包](docs/deployment/release-runtime-bundles.md)

代码和正式文档进入 Git；运行数据、凭据、依赖缓存、原始日志及临时构建文件不进入 Git。临时工作使用项目忽略的 `.cache/`，正式产物按项目交付目录管理。

跨平台桌面：`pnpm desktop:plan`查看当前目标，`pnpm desktop:package`打包；系统／架构选择、已生成Windows产物与签名边界见[安装指南](docs/deployment/desktop-installation.md)及[阶段验证](docs/quality/desktop-packaging-verification.md)。
