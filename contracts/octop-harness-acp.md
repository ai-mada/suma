# Octop Harness ACP 执行合同

| 元数据 | 内容 |
| --- | --- |
| 平台版本 | suma 1.0.0 |
| 合同标识 | `octop-harness-acp-v1` |
| 状态 | T-OCTOP-117 阶段实现合同；验收状态单独记录，不以本文件代替证据 |
| 维护责任 | 控制面、Runner、工具合同与 Octop runtime 模块负责人 |
| 关联需求 | 固定模型配置派发、独立工作区开发、可靠取消、同代码基线测试与人工验收 |
| 实现 | `internal/toolcontract/octop.go`、`internal/platform/octop_execution.go`、`internal/runner/octop_config.go`、`internal/runner/tool_octop.go`、`packages/octop-runtime/` |
| 证据入口 | [阶段验证报告](../docs/quality/octop-execution-verification.md) |

## 1. 运行身份与责任

本合同将公开发布的 `octop-harness 1.0.5` 与 Python `agent-client-protocol 0.12.1` 接入速码现有 Go DAG。适配器为 `octop`，输入载体为 `octop-acp-jsonl`，工作区能力为 `attempt-workspace-v1`。它是受控的 Harness SDK 执行入口，不代表完整 Octop CLI、桌面客户端或厂商账号认证。

Go 控制面拥有唯一派发权，Runner 拥有本机进程树和 attempt 工作区的执行权。Python runtime 通过官方 ACP SDK 启动受控 Agent，只能调用当前 attempt 的六个 Go MCP 工作区工具。模型不得通过生成的命令、配置或工具参数改变这些权属。

## 2. 公开能力对象

`Profile.octop_execution` 为闭合 `OctopExecutionCapability`。下表字段全部必填；拒绝未知字段、重复键、别名、`null`、缺失字段、非法 Unicode 和超限对象。单个能力或证据对象最多 8 KiB。

| 字段 | 类型 | 约束与含义 |
| --- | --- | --- |
| `harness_version` | string | 固定 `1.0.5` |
| `acp_sdk_version` | string | 固定 `0.12.1` |
| `provider_id` | string | 管理员配置的稳定本地标识 |
| `model` | string | 管理员固定的模型名称 |
| `endpoint_origin` | string | 公开协议与主机来源；不含密钥、查询参数或完整供应商路径 |
| `configuration_sha256` | string | 64 位小写十六进制摘要，绑定实际本地配置及合同版本 |
| `budget_mode` | string | 固定 `observe_only` |
| `identity_evidence` | string | 固定 `configuration_ack_only` |

可用 profile 同时要求：`adapter=octop`、`protocol=octop-harness-acp-v1`、`read_only=false`、`workspace_tools_version=attempt-workspace-v1`。它不能同时声明原生模型执行、OpenCode 执行、冲突修复、测试报告、规划、知识萃取、故障诊断或智能运维用途。失败的探测可以报告配置存在，但不能声明已获得工作区执行能力。

能力声明表示受控运行时确认所配置的 provider/model，不证明厂商实际账号、实际计费模型或账户健康。只读 probe 检查固定包版本、公开 SDK API 与配置摘要，不创建 ACP session，不调用模型。

## 3. 固定配置与派发

管理员在本地 `CommandSpec.Octop` 设置 `provider_id`、`model`、`base_url`、可选 `api_key_env_ref`、`max_steps`、`max_output_tokens`。入口必须是固定绝对 Python 路径及唯一固定绝对 `.pyz` 路径；用户任务不能补充 shell、环境变量、启动参数或其它执行用途。

配置摘要绑定版本、provider/model、完整服务路径、凭据环境变量名称和固定参数，不包含凭据值。当前 Go 入口限定 `max_steps` 为 1–32，`max_output_tokens` 为 1–32768；参数由 runtime 传给模型中间件，不能解读为总费用或总 Token 硬限额。provider 支持固定 OpenAI 兼容 Chat Completions 入口，正式远端使用 HTTPS，仅明确的回环地址可使用 HTTP。

派发预览冻结 `TaskDispatchPlan.octop_execution`；目标 agent 节点的 `config.octop_execution` 必须与 profile 完全相同。启动、工作池领取和 Runner 执行再次核对当前工具观察、原计划、命令和配置摘要。配置改变时要求重新生成预览。节点固定 `MaxRetries=0`，不接受模型、预算、工具目录、cwd、专家、凭据、进程或 session 覆盖，不提供盲目 resume。

组织或项目启用强制预算时，本适配器在预览、启动、领取及原子用量检查中拒绝执行，不悄悄降低为观察模式。当前模型请求直接由 SDK 发往固定 provider，没有经过原 Go 模型网关。缺失可靠用量时保留 `unreported`，局部用量保留 `partial`；不能将通用价目表当作已核实的模型价格。

## 4. 私有传输载体

Go 与 runtime 使用本地 JSONL 生命周期载体；runtime 与 Agent 使用官方 ACP stdio JSON-RPC。外层载体不替代 ACP，也不暴露为通用 HTTP ACP 接口。

| 方向 | 消息 | 责任 |
| --- | --- | --- |
| Go → runtime | `execute` | `v=1`、原始 `request_id`、目标与短期工作区 bridge 配置；不包含任意命令 |
| Go → runtime | `cancel` | 同一 `request_id` 的取消意图；不表示停止已完成 |
| runtime → Go | `ready` | 确认冻结版本、配置摘要、provider/model |
| runtime → Go | `started` | 记录官方 ACP 创建的真实 `session_id` |
| runtime → Go | `progress` | 有限的模型或工具状态；不输出原始推理、代码、日志或秘密 |
| runtime → Go | `completed`、`failed`、`cancelled` | 唯一终态载体；Go 继续核对生命周期与真实工作区效果 |

bridge 含当前 attempt 的回环端点、短期令牌、策略摘要和相同请求编号。不得将它作为普通项目 API 凭据使用。输出中的 provider/model 确认值必须匹配冻结配置；不能由模型自由填报。错误跨载体仅使用受控诊断，不回传可能携带密钥的上游异常正文。

## 5. 工作区工具、操作日志与代码候选

模型目录只提供 `suma_workspace_list`、`suma_workspace_read`、`suma_workspace_write`、`suma_workspace_edit`、`suma_workspace_remove`、`suma_workspace_check`。模型层和实际工具调用层共同限制目录；SDK 内置文件、终端、网络和委派工具不得透传。`check` 仅执行管理员固定检查，不接受任意测试命令。

runtime 验证 MCP 服务身份、精确工具集合、参数合同及策略摘要。操作身份从冻结请求编号、真实 `tool_call_id` 和工具名生成，经 `_meta["suma/operation_id"]` 交给 Go；模型不能自选一个身份以覆盖其它操作。Go 校验路径、权限和输入，持久保存真实操作日志与检查事实。相同操作的重传不能生成第二次代码效果。

Agent 的 ACP `cwd` 和个人 Octop workspace 不作为代码隔离证明。Runner 为 attempt 创建独立工作区和新鲜私有 Octop 状态目录，源码读写走 Go 宿主。完成后依次核对操作日志、固定检查、进程停止条件和候选封存条件，再形成不可变代码候选。候选随后使用同一 SHA 进行独立 JUnit 测试；Agent 结束回复和自报测试都不能替代该证据。人工验收绑定具体候选与测试证据，不由本适配器自动批准。

## 6. 公开执行证据

`ToolEvidence.octop_execution` 为闭合 `OctopExecutionEvidence`，下列字段全部必填。未发生的事实使用合同允许的空字符串或 `false`，不使用 `null`，不生成不存在的用户消息 ID、助手消息 ID或 abort acknowledgement。

| 字段 | 类型 | 事实来源 |
| --- | --- | --- |
| `configuration_sha256` | string | 冻结配置摘要 |
| `session_id` | string | 官方 ACP session 的真实 ID；开始前可为空 |
| `requested_provider` / `requested_model` | string | 冻结请求 |
| `acknowledged_provider` / `acknowledged_model` | string | 受控 runtime 配置确认；不是厂商身份认证 |
| `stop_reason` | string | ACP/受控执行事实：空、`end_turn`、`max_tokens`、`max_turn_requests`、`refusal`、`cancelled` |
| `completed_at` | string | 规范 UTC 时间；无完成事实时为空 |
| `process_stopped` | boolean | 私有 Agent 父进程关闭事实与 Go 进程树停止核对同时成立 |
| `cancel_requested` | boolean | 原始取消意图；不能从 `end_turn` 推断 |

`completed` 必须同时满足真实 session、冻结确认一致、`end_turn`、有效完成时间、无错误或拒绝、无未知结果、`process_stopped=true`、`cancel_requested=false` 及正常进程结果。之后仍须通过代码封存、独立测试和人工验收；这不是业务任务自动完成的条件。

## 7. 取消、故障与恢复

| 事实 | 可证明内容 | 不可推断内容 |
| --- | --- | --- |
| 取消已请求 | 控制面要求停止 | 模型、子进程或工具已停止 |
| ACP `end_turn` | Agent 回复阶段结束 | 开发成功、未拒绝、未取消或代码正确 |
| Agent 父进程退出 | runtime 观察到该父进程关闭 | 所有后代进程均已停止 |
| Go 进程树停止核对 | Runner 已核对其进程作用域的停止 | 外部服务没有已经生效的请求 |
| `unknown` / 待核对 | 现有证据不足以确认结果 | 可以安全重新执行 |

断线、无有效终态、迟到/错配消息、进程停止证据不足或已发生效果未明时，保留原 attempt、操作日志与未知事实，进入现有核对流程。不得将取消通知当 abort ACK，不得因 `end_turn` 或退出码单独报成功，不得自动重启 session 或复制原修改。确认原操作后才可通过受审阅的新计划创建后续 attempt。

## 8. 分发与验收边界

runtime 源码在 `packages/octop-runtime/`，产物为 `.pyz` 与 manifest。受管理 venv 锁定 96 项依赖；当前锁基于 Windows/Python 3.13，其他系统需在对应环境形成证据。manifest 固定 runtime 字节及安装版本清单，不能证明每个第三方依赖的全部字节，也不能代表操作系统、网络或任意 Python 扩展沙箱。管理员需保护 Python、venv、zipapp 和 manifest。

本阶段不接管完整 Octop CLI、原有个人 Agent 数据库、IM 渠道、长期记忆、办公技能、桌面端或完整专家团队调度。它们的官方存在不等于速码已完成集成。实际检查、确定性本地模型验证、真实供应商账户运行和生产验收必须在[阶段验证报告](../docs/quality/octop-execution-verification.md)分开登记；本合同不预先声明任何一类验收通过。
