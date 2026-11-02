# 开发工具与 Harness 接入矩阵

T-CURSOR-SETUP-191、T-CURSOR-INPUTS-190及T-CURSOR-ACP-189更新 Cursor 行及[Multica采用记录](multica-cursor-acp.md)；T-TURN-STEERING-188更新 Codex 行及[采用记录](multica-turn-steering.md)。其余产品行保留下面明确的T134检查基线，不能用旧的“未发现”描述当前全部源码；后续八类开发协议见[当前开发交接合同](../../contracts/developer-tool-handoff.md)，实际范围仍逐项以专项证据为准。

阶段：T-PLATFORM-DESIGN-134；产品：速码 suma 1.0.0；核查日期：2026-10-29。

本文是增强设计与源码差异说明，不是新增适配器的实现或验收报告。
源码核查基线为工作树 `unified-experience/suma` 的 `3e09a51`；后续实现须重新核对对应文件。
官方网页仅记录核查时可见内容；不依据搜索抓取时间推测发布版本，不把网页最新功能套用到本地固定版本。
本轮没有安装工具、登录厂商账户、调用商业模型、启动第三方任务或改变现有工具合同。
完整交付范围仍由[实施计划](../delivery/implementation-plan.md)管理；本文不改变能力完成计数。

集成补充：已保留远端 `e8f441e`（T-EXPERIENCE-133）的[开发客户端接入](developer-clients.md)：Cursor、Claude Code、Codex、OpenCode四类无密钥MCP配置导出，以及Qoder、Zed、WorkBuddy、Harness手动指南。该方向为客户端调用速码，不等于新增速码反向派发执行适配；配置导出也不证明客户端已连接或厂商实测通过。以下“未发现执行适配”保留此区分。

## 1. 接入对象与方向

一个产品可以有多个入口，每个入口需要独立协议、配置、权限和验证记录。
工具品牌、安装记录、模型名称、协议能力与运行证据不能互相代替。

| 入口类型 | 方向与职责 | 不能据此推断 |
| --- | --- | --- |
| CLI | suma Runner 启动受控子进程，处理标准输入、输出与退出 | 退出成功不代表候选正确、测试通过或任务验收 |
| SDK | suma 托管库及其运行时，控制配置、工具、会话与事件 | SDK 本身不提供操作系统隔离或统一账单 |
| MCP client | suma 调用外部工具与资源 | 能列出工具不代表有权执行任意动作 |
| MCP server | 外部开发工具调用 suma 项目能力 | 外部客户端接通不代表 suma 能反向派发该客户端 |
| ACP client | suma 向 Agent 发起会话、提问及权限交互 | 会话加载不是业务幂等或安全重跑证明 |
| ACP server | 编辑器通过 ACP 使用 suma Agent | 编辑器宿主不因此成为 suma 的执行器 |
| A2A client/server | 通过实际 Agent Card、任务端点与产物交换协作 | 名称包含 Agent 不代表实现 A2A |
| 编辑器定位 | 打开已授权文件、位置、工作区或差异 | 打开窗口不授予自动编码、终端或后台执行权限 |

ACP 按握手能力使用，`loadSession` 不是所有实现的必选能力；见[官方会话规范](https://agentclientprotocol.com/protocol/v1/session-setup)。
A2A 的任务、取消、消息与产物按实际协议版本登记；见[官方规范](https://a2a-protocol.org/latest/specification/)。
MCP、ACP、A2A 各自版本与产品 1.0.0 独立，不增加产品历史兼容分支。

## 2. 产品与适配矩阵

T-CLAUDE-SDK-192更新Claude行；原T158/159 CLI正式执行与新SDK原生交互、反向MCP客户端分别记录，不能以本表旧未发现结论覆盖当前源码。

“源码支持”只说明当前仓库存在对应入口；实际结果以关联专项报告为准。
“拟接入”是本轮设计，不代表已经注册、配置、探测或完成厂商实测。

| 产品与入口 | 官方协议方向 | 当前 suma 源码支持 | 拟接入路径 | 验证缺口与限制 | 官方直达来源 |
| --- | --- | --- | --- | --- | --- |
| Cursor Agent CLI | 固定包 Node + index.js；官方 ACP stdio | T189/T190/T191：`cursor-cli` / `cursor` / `acp-stdio-v1`，复用 acp-go 0.13.5；完整包摘要、明确 API key 环境引用、准确模型配置 ACK、原一次权限、普通／Pool 派发和代码候选 | 桌面原ACP连接与托管Worker；高级／浏览器／独立节点可逐字Shell准备配置并交给原节点指南；讨论／专家团队沿原固定模型和预算门；同提交独立测试及验收仍独立 | 真实 2026.10.22-27dcf5f 仅版本／help／初始化及认证拒绝已验；无账号 Prompt、真实模型或商业费用证明。受控 SDK／HTTP／SQLite／Git 质量旅程见限定报告，不等于厂商验收；原问题／计划typed决定沿190已接入；成功RPC后受控停止与实际host exit分记，cancel写入不称ACK；其他扩展及旧会话恢复不冒充支持 | [执行合同](../../contracts/cursor-acp-execution.md)、[原执行交互](../../contracts/cursor-inputs.md)、[节点配置合同](../../contracts/cursor-setup.md)、[本阶段报告](../quality/cursor-setup-verification.md)、[原输入报告](../quality/cursor-inputs-verification.md)、[官方ACP](https://cursor.com/docs/cli/acp) |
| Claude Code CLI／Claude Agent SDK | 独立CLI print及官方SDK管理原生CLI | T158/159原claude-jsonl-v1已限定交付；T192新增claude-sdk-v1／官方SDK0.3.314及固定CLI2.1.314，原一次许可、结构化提问、候选及同提交独测／验收 | 七字段真实本机配置准备→原节点库存／暂停登记／明确启用；原任务、讨论、专家团队与Pool派发 | 自有native fixture的真实SDK／Worker／HTTP链已验；真实厂商账号／模型、其他OS、多机／安装及生产继续；observe_only不借硬预算，回调不是native ACK | [新SDK合同](../../contracts/claude-sdk.md)、[限定验证](../quality/claude-sdk-verification.md)、[SDK官方说明](https://code.claude.com/docs/en/agent-sdk/overview) |
| Qoder CLI／ACP／Agent SDK | headless CLI；`qoder --acp` stdio server；官方 Python/TS SDK | 未发现 Qoder 执行适配 | 优先 ACP client 接入，必要时独立 SDK 配置；文件与终端经 suma 宿主授权 | 续接、取消与用量按实际握手核验；官方 ACP 页列 Windows arm64 暂不支持；不默认启用绕过权限模式 | [CLI](https://docs.qoder.com/cli/run-in-scripts)、[ACP](https://docs.qoder.com/cli/acp)、[SDK](https://docs.qoder.com/cli/sdk/overview) |
| Codex app-server／CLI | app-server stdio线程／轮次协议；CLI JSONL为另一入口 | T188更新：明确on-request工具权限、T187显式实验性问答、T188同活turn追加已保存本人消息；独立codex-jsonl-v1入口继续保留 | 原Runner／Journal／单writer、当前模型预算及受保护任务运行；[当前合同](../../contracts/turn-steering.md) | [真实CLI＋自有模型限定验证](../quality/turn-steering-verification.md)；未知写不重发，持久thread恢复未实现，取消ACK不等于停止；厂商账户与生产另验 | [官方app-server](https://learn.chatgpt.com/docs/app-server) |
| WorkBuddy 云任务 | OAuth Open API 创建/查询任务；ACP SSE＋JSON-RPC；会话产物 | 未发现 WorkBuddy 云任务执行适配，不能以 CodeBuddy CLI 代替 | 新增 `workbuddy-cloud`，映射供应方 task/session 与 suma run，凭据由服务端管理 | 文档未明确创建幂等和取消请求合同；需真实授权、未知创建核对、云产物导入与独立测试 | [Open API](https://open.workbuddy.cn/docs/openapi) |
| WorkBuddy 连接器 | WorkBuddy 通过 MCP＋Skill 或 CLI＋Skill 调用第三方 | suma 有项目 MCP 基础，不代表完整 WorkBuddy connector 已接通 | 发布明确权限的 suma MCP/CLI 连接器，单独登记反向入口 | 此方向不证明 suma 可派发 WorkBuddy；需要连接器安装、登录与项目授权实际旅程 | [连接器](https://open.workbuddy.cn/docs/connector) |
| CodeBuddy CLI | print／stream-json；持续输入、指定会话恢复 | 已有 `codebuddy/codebuddy-jsonl-v1` 与限定 decoder | 沿原固定 profile 增补经过验证的交互和会话能力 | 新后台子任务不能套用现有终态假设；完整续接、用量、取消与厂商任务仍需实测 | [CLI参考](https://www.codebuddy.ai/docs/cli/cli-reference)、[Headless](https://www.codebuddy.ai/docs/cli/headless) |
| Zed 编辑器 | ACP client，承载外部 Agent 线程；外部 Agent 自管运行时与账户 | 无 Zed 执行适配 | suma 提供 ACP server 与项目 MCP；文件定位作为独立桌面能力 | 不能把 Zed 客户端当无头执行器；文件控制、候选封存、会话和计费归属需明确 | [External Agents](https://zed.dev/docs/ai/external-agents) |
| OpenCode SDK/server／ACP | 官方 SDK 控制 server/session；另有 ACP 入口 | 已有 `opencode/opencode-sdk-v1`，SDK/server 固定 1.19.4 | 保留当前 SDK 路线；ACP 若引入应单列配置和能力 | 当前 observe-only、私有 server 与六个宿主工具；不等于完整插件、账户或跨平台验收 | [官方SDK](https://opencode.ai/docs/sdk/)、[ACP](https://opencode.ai/docs/acp/) |
| Octop Harness SDK／ACP | 官方 Harness SDK 与 ACP agent runner | 已有受控 Octop 协议、固定 runtime、Go 模型网关与宿主工具 | 扩展前继续固定依赖、用途、工具及模型绑定 | 限定 ACP 执行不等于完整 Octop GUI／CLI／插件与供应方账单 | [Harness](https://github.com/TencentCloud/octop-harness)、[Octop ACP](https://github.com/TencentCloud/Octop/blob/main/docs/acp.md) |
| DeepSeek Harness／DSH | 官方 Cordis 插件架构及桌面/Web；具体外部入口须逐项核验 | 已有 `harness/dsh-multica-jsonl-v1`，限定 Multica profile | 为实际插件/版本登记适配；当前固定 profile 单独保留其范围 | 本轮未从官方页面核验 `--profile multica --stdio` 是当前通用稳定接口，不能扩大为新版全能力 | [官网](https://deepseek.com/harness/)、[源码](https://github.com/deepseek-ai/deepseek-harness)、[文档](https://deepseek-harness.github.io/deepseek-harness/guide/quickstart) |

### 2.1 当前源码依据

| 范围 | 权威入口与合同 | 本轮判定 |
| --- | --- | --- |
| 协议集合 | [validation.go](../../internal/toolcontract/validation.go)、[types.go](../../internal/toolcontract/types.go) | 当前为显式 adapter/protocol 配对，尚无本文统一 manifest |
| Codex | [合同](../../contracts/codex-app-server.md)、[Runner](../../internal/runner/codex_appserver.go)、[类型](../../internal/codexcontract/codex.go) | app-server 与 JSONL 是独立协议合同，不称产品 legacy 或历史兼容分支 |
| OpenCode | [合同](../../contracts/opencode-sdk.md)、[runtime](../../packages/opencode-runtime/README.md)、[资产校验](../../internal/runner/opencode_manifest.go)、[原生新文件准备](../../contracts/opencode-tool-preparation.md) | SDK、server、插件资产与配置摘要固定；快捷准备静态核对后创建新文件，沿原准确节点指南，不安装、连接模型或自动启用 |
| Octop | [合同](../../contracts/octop-harness-acp.md)、[类型](../../internal/toolcontract/octop.go) | 官方 ACP 被受控 runtime 包装，沿唯一 Go DAG 与模型网关 |
| CodeBuddy／DSH | [合同](../../contracts/team-cli-runtime.md)、[外部CLI校验](../../internal/runner/tool_external_cli.go)、[DSH解码](../../internal/runner/tool_dsh.go) | 固定命令与严格事件校验，不接受任意 resume 或未声明协议 |

### 2.2 必须单列的厂商差异

- Cursor 结果可提供 session ID，但本轮所见输出结构不足以保证完整 Token 或费用；失败可能缺少终态帧。
- Claude CLI 的费用上限按客户端估算执行，可能超过阈值；不能映射为供应商账单绝对上限。
- Claude Agent SDK 产品嵌入使用官方允许的鉴权方式；SDK 名称不授予个人订阅转供权。
- Qoder 的 CLI 恢复参数不自动证明 ACP `loadSession` 能力；按实际初始化响应记录。
- Codex 当前 Token 阈值是提醒，配置确认不是提供方模型身份或费用凭证。
- OpenCode 当前 session abort、父进程关闭与 Go 进程范围清理分别记录，不接受单一包装层 ACK 作为停止证明。
- WorkBuddy 云任务支持任务查询、ACP 加载/追问、权限请求、usage 和产物；文档未明确的取消/幂等保持未知。
- WorkBuddy 的 cancelled 结果不证明客户端具有可调用的取消操作；官方私有产物扩展不当成 ACP 通用字段。
- WorkBuddy 文档中的 link 与 sandbox URL 示例形式有差异，实现须验证允许的 origin 与响应形态，不能任意拼接地址。
- DSH 当前限定 profile 的握手成功不证明所有 DeepSeek Harness 插件或新版本兼容。

### 2.3 WorkBuddy云任务的未来实施注意（2026-10-30只读复核）

本段不改变WorkBuddy云入口的planned状态，也不是厂商任务验收。官方Open API已提供OAuth授权码／令牌刷新，以及Bearer鉴权的`POST /openapi/v2/tasks`（JSON prompt及可选name）、任务分页列表和单任务查询；查询可取得新的ACP凭据。ACP使用SSE接收、JSON-RPC发送，连接标识关联双通道；产物提供私有通知和REST读取。任务创建未明确幂等键，取消结果未明确对应取消接口；兑换码的request_id不能套用于任务。产物说明与任务响应的link示例形式不同，不能直接推定一个通用下载根路径。来源：[官方Open API](https://open.workbuddy.cn/docs/openapi)。

本次未找到该`/openapi/v2/tasks`的官方SDK；腾讯官方确有另一企业端点的[workbuddyenterprise Python SDK](https://github.com/TencentCloud/tencentcloud-sdk-python/blob/master/tencentcloud/workbuddyenterprise/v20260709/workbuddyenterprise_client.py)，不能用名称相近推定可直接复用此任务接口。不得用社区账号代理、企业管理API或CodeBuddy CLI代替用户授权的云任务协议。

实现建议基于当前速码合同，需在开发阶段验证：

- 先实现独立强类型HTTP模块和本机协议测试。沿原DAG／Worker／PoolWorker／Journal固定准确attempt、来源、凭据版本与请求摘要，不增加外部调度器。
- 当前`internal/runner/external.go`固定`/runs`、dispatch_id及`/cancel`，不直接套用为WorkBuddy。复用事件、回执和未知效果处理机制，需要独立协议分支与持久检查点。
- 发出创建请求前持久化提交意图；响应丢失保留待核对。已知task ID查询原任务；标题、时间或列表相似性不能证明原操作身份，不能自动重建。
- ACP重连仅恢复读取，不默认重发prompt；首次创建与初始prompt的执行关系需真实验明。权限请求仍经过速码当前策略，超时不能默认批准。
- 固定可信HTTPS来源及重定向策略，凭据不随任意产物URL转发；产物需字节限制、摘要及隔离导入。外部产物不能直接成为Git候选、独立测试或正式验收事实。
- 云模型用量、费用、硬预算、取消与远端停止分别验证。本地连接结束不等于云执行停止；源码实现、协议夹具、用户授权及真实任务证据分别记录。

## 3. 四层 AdapterManifest 设计

本节是拟新增公共合同；具体字段由实现阶段统一生成到 Go、TypeScript 与 OpenAPI。
不向 manifest 写入真实密钥、用户凭据、实时健康或一次运行结果。

| 层 | 对象 | 内容与权威 |
| --- | --- | --- |
| 静态能力 | `AdapterManifest` | 发布者、修订/摘要、入口方向、协议/传输、版本与平台约束、配置/事件/结果 schema、能力与验证要求 |
| 安装观测 | `AdapterObservation` | 实际二进制/依赖摘要、探测版本、时间、设备、观测能力与失败原因；不能扩大静态声明 |
| 授权实例 | `AdapterInstallation` | 组织/项目/设备范围、固定配置、秘密引用、准入策略、撤销与有效期；当前授权来自服务端 |
| 执行绑定 | `AdapterBinding` | 原计划、工具/配置/模型修订、预算、能力快照、工作区与路由摘要；随准确 attempt 固定 |

`AdapterManifest` 最小字段分组：

| 分组 | 拟定字段 |
| --- | --- |
| 身份 | `id, revision, digest, publisher, product, documentation_refs` |
| 入口 | `entrypoint_kind, direction, protocol.name, protocol.version, transport, vendor_extensions` |
| 运行时 | `version_constraints, entry_digest, dependency_manifest, platforms` |
| 配置 | `config_schema, auth.mechanisms, auth.scopes, credential_ref_schema` |
| 输入输出 | `input_schema, event_schema, result_schema, artifact_schema` |
| 执行 | `task_start, session_resume, cancel, human_input, artifacts, usage, budget, workspace` |
| 协议角色 | `mcp_client, mcp_server, acp_client, acp_server, a2a_client, a2a_server` |
| 效果 | `idempotency, unknown_result_policy, reconciliation, verification_requirements` |

每项能力使用 `supported / unsupported / unverified` 及来源，不靠字段缺失猜测为支持。
`session_resume` 细分历史读取、会话加载、新轮次续谈和中断轮次继续；最后一项必须另有作用核对。
`cancel` 分开声明取消请求方式、供应方终态与宿主停止证明。
`workspace` 声明宿主工具、独立工作树或远端环境，不能用 cwd 配置代替隔离证明。
`usage` 标明原始来源、累计/增量语义、覆盖字段、完整性和供应方费用是否可核验。
`budget` 区分观察阈值、估算停止和网关准入约束；实际账单上限另行证明。

## 4. 统一生命周期与恢复

所有适配器复用当前项目授权、稳定操作 ID、版本控制、唯一 Go DAG、Runner 和证据链。
统一命令为 `start / resume / respond / cancel / reconcile`，由能力与当前状态决定是否提供。
统一事件至少包含 `started / progress / message / tool_call / waiting_input / usage / artifact / ended / error`。
事件保存原 session、turn、request、tool call 与来源引用；不修改原始事实来匹配统一文案。

| 场景 | 必需行为 |
| --- | --- |
| 启动 | 固定计划/工具/模型/权限/工作区；发送前持久记录原操作 |
| 创建回执丢失 | 查询原操作或供应方任务；没有核对路径则保持未知，禁止重新创建冒充恢复 |
| 流断开 | 保留已收事实与最后位置；重连读取不等于重发 prompt |
| 续谈 | 使用准确会话与新的明确输入；重新检查授权、上下文范围和预算 |
| 中断后继续 | 只有合同证明可安全继续且原副作用已核对时开放；不因提供方有 resume 参数自动启用 |
| 等待人工 | 展示原工具/问题、范围、影响与原请求身份；超时/撤权不给默认批准 |
| 取消 | 先记录意图，再读取终态和实际停止证据；保留已知消耗和未知效果 |
| 进程结束 | 本地适配核对受控进程树；云适配核对供应方状态，不能假称拥有远端 OS 停止证明 |
| 结果采集 | 工具报告与文件候选分开；代码经摘要、封存、独立测试及人工验收 |
| 权限撤销 | 在线下一动作重新授权；离线已授权窗口按明确期限约束，不能承诺断网即时撤权 |

MCP 工具调用、ACP permission 请求、CLI 命令和 A2A 任务都映射到相同项目权限，协议握手不授予新权限。
适配器不能修改正式验收状态，也不能自行引入平台外的无限重试或后台调度循环。
新版本或能力变化产生新观测与配置修订；既有运行保留原快照，未开始计划重新预览。

## 5. 限额、用量与产物

- 凭据仅存受控服务/设备的秘密引用；浏览器、manifest、日志和任务提示不包含真实密钥。
- 工具选择先检查权限、能力、路由、平台和配额，再提供有依据的成本/性能建议。
- 缺失用量记 `unreported`，部分字段记 `partial`；取消或失败不把已消耗 Token 清零。
- 累计通知按原会话快照处理，增量按原事件去重；不把两者混加。
- 估算费用保留费率来源与修订，不作为供应商账单；未定价不显示免费。
- 强制预算策略只准入能满足具体约束的适配器；不能静默降级为提醒。
- 恢复原请求复用原账本身份；开始效果未知时保留额度占用，待核对后结算。
- 外部产物 URI、下载 URL、路径与摘要均属输入；验证授权范围、允许 origin、字节上限与实际内容。
- 外部测试摘要、会话总结和退出码不能替代准确候选的独立测试原件。
- 工作树封存、测试通过、人工验收、仓库合并与发布仍为独立门禁。

## 6. 准入交互与验收矩阵

工具中心按“选择入口→连接账户/设备→检查能力→选择项目范围→保存停用配置→明确启用”组织。
连接卡显示入口方向和真实能力；将安装、配置、探测、鉴权、任务验证分别呈现。
未验证能力展示具体缺口与下一步，不用统一绿色徽章掩盖差异。
执行预览展示准确工具/模型/范围/预算模式和取消能力，低频协议信息按需展开。
恢复页面展示原任务、未知效果、最后事实和允许动作；不提供无依据的一键重跑。

| 验证层 | 必须保留的证据 |
| --- | --- |
| schema/协议 | 固定官方版本/样例、异常帧、身份漂移、终态缺失和用量语义 |
| 本地运行时 | 实际可执行摘要、依赖、平台、权限配置、工作树与进程树停止 |
| 真实厂商任务 | 明确账户与工具版本、实际任务输入/结果、会话标识和必要去敏用量 |
| 取消与未知 | 开始 ACK 丢失、断线、重启、重复通知、取消失败和核对路径 |
| 代码与产物 | 可信采集、摘要、固定候选、不同 Worker 独立测试与人工 Review |
| 界面/CLI | 中英文、键盘、深链、范围切换、过期配置、错误与未知恢复 |
| 分布式 | 物理多机、节点失联/撤权、跨机候选传输、预算与资源账本一致性 |

能力状态至少记录 `installed / configured / probed / authenticated / task_verified / cancel_verified / artifact_verified / usage_verified` 的独立事实与时间。
上述名称是拟定观测维度，不是单一线性状态机；例如取消已验证不能推导费用完整。
协议夹具、本机运行、官方客户端、商业模型、目标系统与生产环境的证据分别保存。
只运行变更影响的合同与旅程检查；集成里程碑再执行完整回归，不重复无关全套验证。
本文完成只代表设计与来源核查完成；适配实现、厂商授权和真实端到端验收仍按各阶段交付。
