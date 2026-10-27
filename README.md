# 速码 suma

**速码一栈式智能研发平台**：连接需求、设计、任务、智能体执行、测试验收与问题回流。支持浏览器和本地桌面。

当前是可运行的开发版本。功能与验证范围见 [交付状态](docs/delivery/status.md)，完整目标见 [实施计划](docs/delivery/implementation-plan.md)。

## 阶段进展：只读Git审阅与原生执行限额

T-P4-035／1.0.0 新增当前项目的只读Git状态，以及暂存区、工作树的准确文本比较。工作空间以“文件／变更”切换，复用Monaco DiffEditor；切回文件保留编辑模型与草稿，Ctrl/Cmd+S使用原保存与恢复合同。未保存草稿和磁盘变更分开表达；新增、删除、重命名、同路径暂存删除后重建及受限文件均保留明确边界。

原生执行的 `max_steps`、`max_output_tokens` 现按普通／受管修复用途校验硬限额，并准确传递到SDK；非法配置在工作区、MCP和主进程启动前拒绝。此修正不代表可写模型已可独立选择或计价，[035模型选择方案](docs/architecture/035-model-selection-design.md)仅为后续设计输入。

本阶段取得本地隔离Git／Monaco用户旅程和原生限额的定向证据；只读Git不会暂存、提交或派发开发任务。远程IDE、链接工作树的独立仓库授权、PTY／LSP／DAP、正式可写模型选择、真实厂商与分布式验收仍待实施。主预览与最终交付状态以[交付状态](docs/delivery/status.md)为准，完整平台继续开发。

见[产品范围](docs/product/workspace-git.md)、[架构](docs/architecture/workspace-git.md)、[交互](docs/design/workspace-git.md)、[接口与限额合同](contracts/workspace-git.md)、[限定验证](docs/quality/workspace-git-verification.md)及[逐平台差距](docs/product/platform-capability-delivery-matrix.md)。

## 历史阶段：当前工具推荐与IDE保存恢复（T-P4-034）

任务派发现在按当前权限、任务／流程版本、工具真实库存与路由显示兼容选项；明确采用只填写原选择，保留准确档案版本与摘要，再进入既有固定预览和启动门禁。模型仍由工具配置固定，费用未知，不制造最优／零费用评分。IDE保存只确认准确路径、内容和UTF-8摘要；错误200保留原操作，恢复后不覆盖后来输入的新草稿。

隔离副本实际完成中英文推荐→采用→准确计划预览／刷新，以及真实Monaco错误回执→原key恢复→新草稿新key保存。3次HTTP对应2项持久保存；任务仍待办、未Start，无新运行／模型／厂商调用／正式候选测试／凭证。15项最终冷态核对通过；原始失败与纠正依据完整保留，未重跑历史开发或完整套件。

主预览已更新，原账号、任务及186表业务记录保持；完整平台继续开发。[本阶段验证](docs/quality/task-dispatch-options-verification.md)、[产品与交互](docs/product/task-dispatch-options.md)、[参考能力差距](docs/product/platform-capability-delivery-matrix.md)。

## 历史阶段：修复计划修订与运行溯源

T-P4-033／1.0.0 已完成限定本地验证。修复任务支持新计划版本重选工具与流程；原冲突、来源顺序、失败运行及回执保留。只有宿主类型化证明的执行前失败可准备替代计划，未知或已发生效果的运行继续沿原操作核对。新计划仍须单独启动、独立测试和明确验收。

实际复用已验收 A／X，完成待启动修订→真实启动前失败→明确切换至 B→A 独立测试准确候选 `cd9c611ac90ea3531247ec2340b74b083bdc1ef4`→经用户许可的隔离测试账号 Review→绑定仍未开始的 D。共3项新运行、4项正式检查、6次自有loopback模型协议响应和6次真实工具调用，无来源重复开发／复测，无厂商调用。启动证明准确记零Token且费用未定价；健康模型执行用量未报告，不推断为零。

见[PRD](docs/product/candidate-repair-revisions.md)、[架构](docs/architecture/candidate-repair-revisions.md)、[AI SOP](docs/operations/candidate-repair-revisions.md)、[交互](docs/design/candidate-repair-revisions.md)、[CLI](contracts/candidate-repair-revisions-cli.md)、[限定验证](docs/quality/candidate-repair-revisions-verification.md)、[摘要](docs/quality/candidate-repair-revisions-summary.json)、[English](docs/en/candidate-repair-revisions.md)。真实厂商、多物理机器、分布式数据库／规模、完整IDE、智能工具／模型选择和完整P0–P7继续实施。

## 历史进展：冲突修复与受控恢复

T-P4-032／1.0.0 已完成限定本地验证。固定已验收来源发生真实冲突后，明确创建独立修复任务；原生工具完成有序合并与受控编辑，独立节点测试准确候选，再通过显式 Review 交接到未开始任务。已知失败可在原运行中受控重试，保留原尝试并生成新尝试与租约；结果未知、来源变化和非幂等节点保持门禁。

本地实际形成3项新运行（来源X、原冲突、修复）、8项正式检查和2次测试账号明确评审；修复首次在工作区和模型启动前失败，修正连接后由界面重试同一次运行，第二次尝试及独立测试通过。准确提交 `21cc51598692c6121e7324f382757f45cd0a1e9e` 已绑定仍未开始的D。仅使用6次自有loopback模型协议响应与6次真实工具调用，用量未报告，不推断零Token；未调用厂商。 中英文、键盘详情、刷新与冷回读通过；主预览已前向更新并保留原业务、权限与会话。见[设计与AI SOP](docs/architecture/candidate-conflict-repair.md)、[限定验证](docs/quality/candidate-conflict-repair-verification.md)、[English](docs/en/candidate-conflict-repair.md)。真实厂商、多物理机器、生产验收和完整P0–P7继续实施。

## 历史进展：专家团队依赖编排

T-P4-029 团队任务支持固定依赖、逐项进度、暂停／恢复／停止、未开始项修订及只读历史。服务端在准确前置运行正式验收后自动派发后继，CLI 与中英文界面共用合同。实际同机双 Runner 完成两项开发、8 项独立正式测试与两次明确验收，029／1.0.0 主预览更新作为历史证据保留。见[功能与实际边界](docs/quality/expert-team-orchestration-verification.md)、[设计与AI SOP](docs/architecture/expert-team-orchestration.md)、[CLI](contracts/expert-team-orchestration-cli.md)。完整平台持续开发。

## 历史进展：精确尝试的工具历史

T-P4-012 已实现按项目、运行、节点和 attempt 读取持久工具事实。运行详情中的“历史尝试”可分页查看原始正文和当前语言说明；刷新保持选中的尝试，翻页保持同一个读取窗口。历史页默认 50 条，可选 5／25／50／100 条，不受当前实时工具窗口 40 行限制。读取逐次核对当前权限，原文和执行依据不因显示语言变化。

本阶段已完成本地验证：分页原文与数据库一致，纯读取没有改变执行或业务状态，验证服务已关闭，主预览已更新且原数据保持。本机原生 HTTP 429 重试与独立合成协议大页验证分开记录，不代表厂商模型效果或任务验收。见[架构与操作边界](docs/architecture/attempt-tool-history.md)、[专项验证](docs/quality/attempt-tool-history-verification.md)和[精简摘要](docs/quality/attempt-tool-history-summary.json)。

## 本地运行

要求 Go 1.26+（构建固定 1.26.10）、Node.js 22+、pnpm 11。可通过 `SUMA_GO_EXE` 指向私有工具链；构建脚本不修改系统环境。

```powershell
pnpm install
pnpm build
go run ./cmd/suma-control
```

打开 http://127.0.0.1:4310 ，首次创建组织、项目和管理员。数据保存到 `.suma/`。默认只监听本机，密码至少 12 个字符。

开发前端：`pnpm dev`；控制面继续监听 4310，前端 5173 代理 API。编辑项目文件前，设置 `SUMA_WORKSPACE_ROOT` 为明确授权的代码根目录，在 IDE 中绑定项目工作区。

执行节点需要从设置中创建凭证，并明确配置可执行命令。参见 [Runner 与工具](docs/integrations/tools.md)。原生智能体参见 [运行时](docs/agents/runtime.md)。

```powershell
go test ./...
pnpm typecheck
pnpm test
pnpm build
pnpm check:docs
```

浏览器、桌面、数据库、模型账户和规模验证分别记录；未配置的服务在界面中显示不可用。

## 文档与交付

- [设计文档总览](docs/README.md)
- [系统架构](docs/architecture/overview.md)
- [DAG 调度](docs/scheduler/engine.md)
- [执行尝试与持久工具历史](docs/architecture/attempt-tool-history.md)
- [产品与用户旅程](docs/product/prd.md)
- [部署与恢复](docs/deployment/server.md)
- [真实制品与本机发布](docs/deployment/release-control.md)
- [四程序跨平台运行包](docs/deployment/release-runtime-bundles.md)
- [当前验证报告](docs/quality/verification.md)

代码在本仓库开发；运行数据、凭证、依赖和测试缓存均不进入 Git。
