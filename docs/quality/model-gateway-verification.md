# 统一模型网关验证记录

状态：suma 1.0.0／T-MODEL-118，**本地限定链路已验证，完整平台继续实施**。更新：2026-10-29。责任：主集成代理汇总真实命令、源码摘要、界面及最终检查；模块代理仅维护已实际取得的限定事实。关联：[模型合同](../../contracts/model-execution.md)、[统一网关架构](../architecture/model-gateway.md)、[Octop 合同](../../contracts/octop-harness-acp.md)、[交互设计](../design/octop-execution.md)。

本报告不把历史 T-P4-036／T-OCTOP-117 通过结果继承为当前代码通过，也不把同机协议样本写成供应商或生产验收。下表均限定到对应命令记录；磁盘响应缓存、当前数据合同与最终浏览器验证均按下表记录，不继承旧阶段通过状态。

## 1. 当前实现与数据边界

原生编码与 Octop Harness／ACP 统一使用真实模型部署选择、ModelBinding、冻结 Rate 和 attempt 预算，Go 是供应商密钥、HTTP 与请求账本权威。真实 SSE 内容继续流式转发，终态／usage 后缀持久化后释放；同 step 同正文只返可用原响应缓存，不再发供应商。未知、取消、代码失败或 Runner 重启保留已知用量与未解 hold，不伪装为零。

产品版本仍 1.0.0，当前数据合同为 `suma-initial-gateway-v1`。旧目录和不匹配声明拒绝进入当前写路径，不迁移／清空；实际新实例必须用独立空目录／数据库。当前合同的新库／重启检查和最终数据库矩阵在下文分别记录，不能用源码存在代替真实验证。

## 2. 已取得的限定证据

| 证据组 | 实际范围与结果 | 来源与限制 |
| --- | --- | --- |
| Web 统一模型与事实 | 13 项独特行为通过，包含动态参数上限、runtime-only 原生／Octop 配置、共享 Binding／原操作回执、网关核对／缺失计数、停止意图分离、共同模型费率配置和双语组件；后续 0-based 与删除重复快照仅复跑受影响项，没有增加独特计数 | 本地 `.cache/e2e/model-118/ui-targeted-summary.json` 原12项，另 `.cache/e2e/model-118/usage-rate-ui.txt` 记录1项新增费率回归；组件／helper 不代替真实浏览器 |
| Runtime 专用检查 | Python 共同模型／step 合同、原生 token／step／漂移／无重试、无绑定 workspace 编码／修复提前拒绝、扩大参数请求构造实际通过；原生类型／bundle 与 Octop 离线 SDK／共同协议 probe 有真实记录 | `.cache/e2e/model-118/runtime-gateway-checks.json`；fake transport 构造和离线 probe 不代表供应商调用或 Go 宿主全链路 |
| Go 网关与原生真实闭环 | 持久准入、输出 clamp、同正文不二发、缺 usage 保留额度、真实 SSE 后缀持久化顺序；真实当前用户 Preview／Start、HTTP Claim、原生 bundle／MCP／Go 计量；记录 40 Token、按冻结运营方费率估算 57 CNY 微货币、账本 settled 与 hold 为零 | `.cache/mcp019-deps/20261028T155659216Z.command.json`／`.stdout.txt`，exit 0；本地自有模型协议端点，独立正式任务测试／人工验收未由该原生用例覆盖 |
| Octop 官方 SDK 真实工作区与独立测试 | `TestOctopSDKActualHostCandidateAndIndependentJUnit` 实际通过；四轮累计输出许可依次 512／509／506／503，宿主 MCP 修改、固定 check、封存；另一 Runner 对同 SHA 生成独立 JUnit 证据 | `.cache/mcp019-deps/20261028T154913597Z.command.json`／`.stdout.txt`，exit 0；新增磁盘缓存后 positive 用例最终复验待主集成补入 |
| Octop 真实取消／进程树 | `TestOctopSDKActualCancellationUsesGoTree` 实际通过，使用官方 SDK 与 Go 受控进程停止路径 | 同上命令；取消不证明供应商未生效，也不代替账务核对 |
| Runner journal 恢复 | `TestModelGatewayJournalRecoversKnownBillingWithoutInventingACPStop` 的 open-unknown／closed-known／closed-zero 实际通过；保留已知计数，不凭重启伪造 ACP session 或停止 | `.cache/mcp019-deps/20261028T153702960Z.command.json`／`.stdout.txt`，exit 0；本地 journal 场景，不代表物理断电或多主机验收 |

原先广批检查存在失败后已定向修正的场景，不能将整次广批写成全过；实际命令 `20261028T154340515Z` 保留其原始结果，后续 `155659216Z` 仅证明该命令选择的受影响项通过。子用例、重复复跑与 offline probe 不重复累加为新的独特功能检查。

## 3. 实际命令与环境

Go 命令由仓库管理的 Windows Go 1.26.10 执行，CGO 关闭，模块／构建／临时目录位于项目 `.cache`，未变更机器或用户环境。模型端点为本次自有、受控的本地协议服务；不宣称真实商业供应商账户或账单已验证。官方 SDK 与固定版本是实际安装／运行范围，不代表完整 Octop CLI／GUI 或整个 Python 环境已受沙箱证明。

```text
# 20261028T154913597Z：官方 Octop SDK 正常编码与取消
 go test ./internal/runner -run ^TestOctopSDKActual -count=1 -json
# 20261028T155659216Z：网关、原生真实闭环与参数边界
 go test ./internal/modelgateway ./internal/runner ./internal/platform -run "^(TestGateway|TestModelExecutionCurrentPreviewActualRuntimeAndUsageLedger|TestModelExecutionRequiresLocalDeploymentBeforeWorkspace|TestNativeExecutionLimits)" -count=1 -json
# 20261028T153702960Z：journal 已知费用与未知事实恢复
 go test ./internal/runner ./internal/platform ./cmd/suma-contracts -run ^TestModelGatewayJournal -count=1 -json
```

Web 费率回归 `pnpm exec vitest run src/UsageWorkspace.test.tsx -t "T118 prices" --maxWorkers=1 --minWorkers=1` 实际1项通过、5项无关跳过：Octop准确部署可进入报价，写入保留部署 ID／revision／SHA；不可用 Octop 和外部伪只读 purpose 不因此开放，原 Suma 只读用途保留。

Web 仅选择 T118 相关行为执行；初次 worker 参数冲突未执行任何用例，校正 min/max 为 1 后通过。native preset、0-based 索引与共享回执后续仅复跑相应项，详见本地摘要，不重复整个旧套件。所有本地原始命令／stdout、缓存、模型正文、数据库及秘密留在忽略目录，不作为 Git 交付物。

## 4. 最终集成检查

源码变更范围摘要：`5291a891e7daa288ee29b7d99ac7485ac611c61399f6a0589d00f7cec00a605b`，共 117 个源码／合同文件；文档正文和 memory 不参与该摘要。详细文件清单、原始命令摘要及本地制品 SHA 见[紧凑证据](model-gateway-summary.json)，全部原始日志／数据库／截图仅保留在忽略目录。

| 检查 | 本轮实际结果 |
| --- | --- |
| 聚焦 Go 检查 | 35 个不同顶层用例通过；早期批次4个失败均在修复当前夹具／配置校验后定向通过，没有将失败批次说成全过；未重复完整套件 |
| 磁盘响应缓存后真实 Octop SDK | `20261028T162233994Z`：正常链路通过，4轮许可512／509／506／503、Go六工具、封存、另一Runner同SHA独立JUnit；取消路径在`20261028T154913597Z`通过，未因不变路径重复执行 |
| 当前持久数据合同 | `20261028T162206806Z`：5项顶层初始化检查通过，含旧声明／混合组件／未知组件写前拒绝、原数据保持不变、当前新库关闭重开；只验真实SQLite，其他数据库另验 |
| 当前原生真实 HTTP／计量／浏览器 | `20261028T162910279Z`：当前用户Preview/Start、真实HTTP领取、真实Node bundle/MCP连接、1个自有模型响应、精确摘要与40tokens／57CNY微单位平台估算结算通过；无净代码变更，独立正式测试及人工验收没有执行 |
| Web与运行时聚焦验证 | Web13项、Python／Native8项独特行为通过；只复跑受影响项。最终TypeScript、5259项中英文目录／占位符／标记与硬编码0检查通过 |
| 生成与产品身份 | 当前生成器`--check`通过：532 schemas／269 API paths及TypeScript；产品与workspace／Go／desktop／Helm均1.0.0 |
| Web制品 | 4次构建分别对应集成、Octop报价准入、诊断生成一致性检查、实际窄屏修复；最后构建成功。保留现有大chunk提示，不把它宣称为性能验收；未单独重建控制面／Runner二进制 |
| 实际界面 | 当前真实原生运行：CN/EN热切、请求折叠、预算账本下钻、40tokens／CNY0.000057、未知可选计数；Octop runtime-only4项草稿、完整／缺失反馈、跨语言保留；390窄屏改为节点列表，无横向溢出；修复数值／“未上报”拆字，最终全部nowrap且无局部溢出 |
| 浏览器结束与证据保留 | 最后控制台warn/error为0；浏览器没有新发起执行、审批或费用确认。运行／attempt／review／usage计数前后相同，自有服务与标签关闭，SQLite唯一快照保留；用户预览和用户数据库未改 |
| Git交付 | 用户授权此阶段commit及正常推送dev。只提交源码、当前合同／设计和紧凑证据；最终commit与远端SHA由Git和交付消息定位，不把当前提交SHA写入自身形成循环 |

浏览器实际没有从任务表单启动Octop或完成模型选择→执行的全GUI旅程；模型选择、参数上限、共同报价与回执为组件验证，真实SDK与Go平台API链路分别留证。正式质量、人工签收、生产部署均保持独立。

## 5. 限制与后续范围

- 本阶段真实请求使用自有本地模型协议；没有证明商业模型质量、供应商最终账单、精确 tokenizer、绝对费用保证或长期额度表现。
- 流程响应覆盖、usage 覆盖、账本 settled／hold、代码成功、同 SHA 独立测试和人工验收分别记录。原生计量闭环用例没有自动补齐其业务测试与签收。
- 只读规划／知识／分诊／运维 purpose、OpenCode 固定模型与 Codex app-server 保持各自合同；不称全部 CLI 已统一预算或模型执行。
- 私有缓存当前默认 request 2 MiB、response 8 MiB、总缓存 32 MiB，需真实容量检查与准入；不是无限轮数、任意输入或任意网络请求能力。SSE 摘要针对 LF 规范转发表示。
- 物理多机、真实 OceanBase／PostgreSQL、Windows／macOS／Linux 安装、完整 Octop 控制台／渠道／记忆、供应商账户及生产规模各有独立验收依赖，本报告不代替。

## English verification status

This is scoped local evidence for T-MODEL-118; full-platform and production acceptance remain open. Actual recorded runs cover shared model UI behavior, runtime guards, Go gateway admission/SSE/usage, an owned local native model/HTTP claim flow, official Octop SDK workspace execution with independent same-SHA JUnit, cancellation and journal recovery. Earlier broad runs with failures are not described as fully passed, and repeated cases are not counted twice.

The native ledger fixture records 40 tokens and 57 CNY micro-units under the frozen operator rate; this is not a vendor invoice. Provider quality/billing, production, physical multi-host and cross-platform installation remain outside these local results. Final disk-cache SDK replay, SQLite data-contract initialization, generated contracts, Web builds and representative Chinese/English browser journeys passed within the scopes in section4. Model selection is component-verified; this does not claim an Octop launch through the entire GUI. The product is still 1.0.0 and new instances use a separate empty directory declaring `suma-initial-gateway-v1`; existing incompatible user data is preserved and rejected, never migrated or erased.
