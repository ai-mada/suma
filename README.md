# 速码 suma

**速码一栈式智能研发平台**：连接需求、设计、任务、智能体执行、测试验收与问题回流。支持浏览器和本地桌面。

当前是可运行的开发版本。功能与验证范围见 [交付状态](docs/delivery/status.md)，完整目标见 [实施计划](docs/delivery/implementation-plan.md)。

## 当前阶段：专家团队模型选择到真实执行

T-P4-040／1.0.0 已打通团队每任务模型、参数、Token／费用预算到真实原生执行及用量账本。按职责配置，可单独覆盖；待办来源可明确修订，原报价、已开始依据与历史保留。中英文、CLI和原操作恢复共用同一合同。

实际自有协议运行结算40 Token／57 CNY微元估算，预留归零；另一GUI任务只预览／修订，未启动或正式验收。当前本地工作台已包含036–040实现，原188表数据、会话和业务历史保持。外部CLI独立模型、完整IDE与全平台其余范围继续实施。

见[合同](contracts/expert-team-model-execution.md)、[架构与AI SOP](docs/architecture/expert-team-model-execution.md)、[限定验证](docs/quality/expert-team-model-execution-verification.md)、[English](docs/en/expert-team-model-execution.md)。

## 历史阶段：执行文件编辑、冲突比较与原保存恢复（T-P4-038）

T-P4-038／1.0.0 将当前原生执行工作区贯通到浏览器和CLI实际编辑、CAS保存与回执恢复。完整文件按原摘要保存；模型或另一位编辑者先修改时，不静默覆盖，保留草稿并通过Monaco差异视图明确比较和合并。

一次执行许可、持久原回执和显式inspect核对保证未知结果不会被自动重写；保存递增工作区版本并使旧检查失效，正式测试和验收仍独立。支持中英文、快捷键、刷新和按需加载的最近保存元数据，界面模型和ARIA生命周期问题已通过真实验证修复。

真实Control／Worker／原生进程和两个隔离测试标签页已完成保存、冲突及合并，主035预览尚未升级。当前仅已存在文本、新旧均≤48KiB；完整远程IDE、PTY／LSP／DAP、实体多机与生产验收等总范围继续实施。

见[产品交互](docs/product/remote-workspace-write.md)、[合同](contracts/workspace-write.md)、[架构](docs/architecture/remote-workspace-write.md)、[AI SOP](docs/operations/remote-workspace-write.md)、[English](docs/en/remote-workspace-write.md)、[限定实际证据](docs/quality/workspace-write-verification.md)。

## 历史阶段：执行工作区只读查看与出站恢复（T-P4-037）

T-P4-037／1.0.0 新增从浏览器或CLI查看当前原生执行工作树的目录和文本。通过控制面与Runner现有出站连接返回，严格绑定当前项目、身份、尝试、派发和工作区策略；使用准确读取租约和原回执恢复，不暴露Runner文件端口或凭据。

IDE采用“本地／执行”来源切换、只读Monaco、折叠摘要、中英文和键盘操作。真实本机进程的隔离界面已验证文件正文、时分秒、刷新和运行定位；最终联合流程通过。空轮询不会持续新增操作回执。主035预览尚未重启，完整远程编辑／PTY／LSP／DAP、物理多机与生产验收继续实施。

见[产品与交互](docs/product/remote-workspace.md)、[合同](contracts/workspace-observation.md)、[架构](docs/architecture/remote-workspace-broker.md)、[AI SOP](docs/operations/remote-workspace.md)、[English](docs/en/remote-workspace.md)、[限定验证与证据](docs/quality/workspace-observation-verification.md)。完整P0–P7及需求1–22继续保留。

## 历史阶段：固定模型选择、执行预算与真实原生调用（T-P4-036）

T-P4-036／1.0.0 已贯通限定本地的原生模型固定选择、封闭执行参数、attempt 预算、真实 native CLI 请求和同一用量账本；原任务界面的固定预览、中英、键盘和刷新已在036实际验证；该界面未启动执行。支持的原生 Suma 工具从当前授权执行节点的公开允许清单选择准确部署，固定计划保存模型／参数／预算／匹配费率，启动与领取分别复核当前资格。

参数只开放 `max_steps`、`max_output_tokens`。Token 和费用预算使用保守入场预留及冻结运营方费率；当前每工具仅一份最新匹配费率供新选择使用，没有完整多模型报价目录。缺失用量、模型别名／身份不符和未知结果保留原 attempt 及待核对余额，不能记成零费用或自动换模型重跑。

新增 `suma ... developer check`，通过官方 MCP SDK 实际完成当前项目初始化、工具目录和一次只读概览调用。该诊断不等于 Codex 客户端、远程 Runner 工作区或远程 IDE 已连接。外部 CLI 与专家团队的独立模型选择、多模型报价、[远程工作区 broker](docs/architecture/remote-workspace-broker.md)、真实厂商及生产验收继续待办。

见[产品范围](docs/product/model-execution.md)、[架构与实现检查点](docs/architecture/model-execution.md)、[AI SOP](docs/operations/model-execution.md)、[English](docs/en/model-execution.md)、[实际证据与UI固定预览已验证状态](docs/quality/model-execution-verification.md)及[开发工具诊断说明](docs/integrations/developer-connection-check.md)。主预览和最终阶段状态以[交付状态](docs/delivery/status.md)为准。

## 历史阶段：只读Git审阅与原生执行限额（T-P4-035）

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
