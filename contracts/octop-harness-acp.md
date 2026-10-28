# Octop Harness ACP 执行合同

| 元数据 | 内容 |
| --- | --- |
| 平台与当前阶段 | suma 1.0.0／T-MODEL-118 |
| 合同 | `adapter=octop`、`protocol=octop-harness-acp-v1`、`input=octop-acp-jsonl` |
| 维护责任 | 控制面、Runner、模型合同与 Octop runtime |
| 关联需求 | 统一模型／参数／预算、独立工作区编码、可靠停止、同 SHA 测试与人工验收 |
| 实现 | `internal/toolcontract/octop.go`、`internal/platform/octop_execution.go`、`internal/runner/octop_config.go`、`internal/runner/tool_octop.go`、`internal/modelgateway/`、`packages/octop-runtime/` |
| 当前架构与证据 | [统一网关](../docs/architecture/model-gateway.md)、[Octop 架构](../docs/architecture/octop-execution.md)、[本轮验证报告](../docs/quality/model-gateway-verification.md) |

本文件定义当前合同，不代表真实供应商、跨平台或生产验收通过。旧阶段报告保留自己的源码和环境来源。

## 1. 运行身份与统一模型

受控 runtime 使用 `octop-harness 1.0.5` 与 Python `agent-client-protocol 0.12.1`。Go DAG 保有唯一派发权，Go Runner 保有 attempt 工作区和进程树执行权。ACP 处理真实 session 与生命周期；模型请求必须经过 Go attempt 模型网关，供应商密钥、HTTP 和请求账本不下放给 SDK。

可用档案必须同时具有 `read_only=false`、`workspace_tools_version=attempt-workspace-v1`、`model_execution_protocol=suma-model-execution-v1`，并通过实际离线能力探测。未安装、未探测或探测失败可以报告不可用配置；不能携带已验证工作区能力、取得派发资格或退回普通 command。

## 2. 公开能力与配置

`Profile.octop_execution` 为闭合对象，仅包含：

| 字段 | 约束 |
| --- | --- |
| `harness_version` | 固定 `1.0.5` |
| `acp_sdk_version` | 固定 `0.12.1` |

这两个版本描述 runtime 协议能力，不证明模型授权、厂商身份、模型服务健康或账单。模型、提供方端点、预算和费率由[统一模型合同](model-execution.md)负责，不放入 Octop 能力对象。

本地 CommandSpec 固定管理员绝对 Python 路径、唯一绝对 `.pyz` 参数、同目录 runtime manifest 和允许检查；显式声明共同模型协议。`env`、`env_from`、任务供应商配置、任意启动参数和任意用途混用均拒绝。配置不包含 Octop 独立 provider/model/base_url/API key／参数对象；真实模型由 Runner `model_deployments` 目录提供。

Octop 不能同时取得 OpenCode、Codex、原生冲突修复、独立测试报告、规划、知识提炼、失败诊断、智能运维或候选合成用途。SDK 内置文件／终端／网络／委派能力不透传为模型工具。

## 3. 计划、Binding 与预算

用户在同一模型面板选择当前真实部署、参数和预算，既有任务预演生成 `ModelBinding`。已保存 `TaskDispatchPlan.model_execution` 与唯一 agent 节点的 `config.model_execution` 必须完全一致，含部署、参数、原费率、预算及预留；工具摘要固定 runtime 身份；Plan／Node 不复制单独的 Octop 模型或 runtime 能力对象。

Start、私有／Pool Claim 与 Runner 再次核对权限、工具／命令摘要、实际观察、原计划和本地部署。节点 `max_retries=0`；不接受 session、cwd、个人专家、工具目录、供应商环境或模型覆盖。模型变更须生成可审阅的新计划，不能修改已开始节点。

`max_steps` 和整个 attempt 的累计 `max_output_tokens` 使用统一合同与选项服务的动态边界。输入按 `context_window × max_steps` 保守预留，费用按准确冻结运营方 Rate 整数估算。组织／项目／attempt 预算沿原领取事务检查，不再以 Octop 适配器单独旁路预算。无精确 tokenizer 或供应商最终账单保证；未定价、partial／unknown 继续依现有门禁与 hold 处理。

## 4. 私有载体、SSE 与请求账本

Go 与 runtime 使用有界 JSONL 生命周期载体，runtime 与受控 Agent 使用官方 ACP stdio JSON-RPC。`execute` 绑定原 `request_id`、目标、短期 workspace bridge 与共同 `model_execution`；不含真实供应商密钥。`ready` 确认 Harness／ACP 版本与绑定摘要，`started` 记录真实 session，progress 只提供受控状态。取消通知仅记录原操作意图。

模型 SDK 只得到本 attempt 的 Go loopback 端点和短期 token。Go 先持久保存 started 才发上游请求，核对连续 0-based step、原正文摘要、固定模型与累计输出许可。相同 completed step 同正文且原响应缓存可用时仅返缓存，无第二次供应商请求；缺缓存、不同正文、乱序或未知结果拒绝。SDK 自动重试不能绕过原操作身份。

SSE token 内容仍实时转发；终态／usage／DONE 后缀必须在 Go 原始响应事实及 journal 检查点持久化后才能释放。模型身份、摘要和计数以 Go 为准，SDK 汇总和配置确认不能生成权威账务证据。

## 5. 工作区、代码与交付

模型仅见 `suma_workspace_list/read/write/edit/remove/check` 六工具。runtime 检查 MCP 身份、精确目录与参数，操作身份来自原 request、真实 tool_call_id 和工具名，经元数据送入 Go；模型不能挑选别人的操作 ID。Go 核对路径／权限／摘要并持久保存操作与检查事实，重传不能造成第二次代码效果。

Runner 创建独立 attempt 工作树与新鲜私有 Octop 状态目录；ACP cwd 或个人 workspace 不构成隔离证明。代码修改经过固定检查、操作日志及封存门禁后形成不可变候选。同 SHA 在独立 Runner／测试节点生成正式证据，再经人工验收及基线交接；模型回复和自报测试不能替代门禁。

## 6. 公开证据与停止

`ToolEvidence.octop_execution` 为闭合六字段对象；尚未发生的 session／停止／时间使用空字符串，布尔值按实际事实记录，不虚构 ACK。

| 字段 | 来源与含义 |
| --- | --- |
| `binding_sha256` | 原 ModelBinding 摘要，必须与 `ToolEvidence.model_execution.binding_sha256` 一致 |
| `session_id` | 官方 ACP 的真实 session；启动前可为空 |
| `stop_reason` | 原始 ACP／受控执行停止原因 |
| `completed_at` | 真实完成时刻，无完成事实为空 |
| `process_stopped` | runtime 父进程事实与 Go 受控进程树停止核对共同成立 |
| `cancel_requested` | 原取消意图，不从 end_turn 或退出码推断 |

模型回执同时必须具有 `gateway_protocol=suma-model-gateway-v1`；请求身份、用量和 gateway_closed 由 Go 核对。`end_turn`、取消请求、父进程退出、完整进程树停止、模型请求完整和代码正确分别记录。有效结束仍需绑定一致、真实 session、允许停止原因、Go 停止事实与非未知的宿主结果，不能自动批准业务验收。

## 7. 异常、恢复与费用

上游请求可能已经发生效果时，不盲目重发、不恢复个人 session、不复制原修改。Runner 重启保留原 journal 中已知 usage 和 Binding，未完成请求为 unknown，不伪造 session、进程停止或网关关闭；原操作核对前保留未解额度。

取消不清零消耗，代码失败不撤销已知费用。模型响应、用量覆盖、账本结算／hold、候选和独立测试是独立事实。界面未读账本时仅显示模型请求核对状态，并链接既有用量页，不能从网关关闭推断已结算。

## 8. 分发与明确范围

`.pyz` 与 manifest 由 runtime 锁定依赖构建。manifest 提供 runtime 字节及依赖版本来源，不证明整个第三方环境、解释器签名、网络或操作系统沙箱。管理员保护 Python、依赖、入口和配置，跨平台安装与实际运行分别验收。

完整 Octop CLI／控制台、个人 Agent 数据库、IM、长期记忆、办公技能与桌面能力不因 Harness 接通自动继承。项目 MCP 客户端接入与 attempt workspace/model loopback 使用不同凭据和权限。[本轮验证](../docs/quality/model-gateway-verification.md)分别记录本地协议、真实 SDK、供应商账户及生产范围。

## English contract summary

Octop Harness/ACP declares only pinned runtime versions and uses the same platform model binding, rate and budget as native coding. Provider credentials and HTTP remain Go-owned. The SDK receives only short-lived attempt loopback access; provider settings are never embedded in the Octop runtime spec.

Real SSE content stays streamed; terminal/usage suffixes wait for durable host receipts. Same-step same-body replay can return an existing completed cache, never a second provider call. Unknown outcomes and restarts retain known usage and unresolved holds. ACP end, cancellation intent, process stop, model coverage, ledger settlement and source acceptance are separate facts. The full Octop product and other fixed-purpose/model tool contracts remain out of this adapter’s claims.
