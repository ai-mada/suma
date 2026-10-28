# Octop Harness 受控执行运行时 / Controlled runtime

状态：1.0.0 阶段实现；实际验收以 `docs/quality` 对应交付证据为准。此包复用官方 `octop-harness 1.0.5`、Python `agent-client-protocol 0.12.1` 与 `mcp 1.30.0`，提供 `octop-harness-acp-v1` 执行合同，不安装或冒充完整 Octop 桌面端、控制面或办公功能。

## 运行边界

- Go DAG 是唯一派发者。每个 attempt 使用全新私有运行目录；不读取个人 Octop 数据库、历史、专家或密钥文件。
- 外层 JSONL 生命周期管理进程通过官方 ACP SDK 启动同一个 zipapp 的智能体子进程。模型只有六个 Go MCP 工作区工具；SDK内置文件、终端、联网、委派工具在公开 middleware 的模型层和实际调用层均受限制。
- 代码只通过 Go 工作区宿主修改。`_meta["suma/operation_id"]` 由冻结请求编号、真实 tool_call_id、工具名计算，模型不能自行指定。接口目录、所有参数类型和约束、服务身份都精确验证。
- 模型、参数和预算使用平台冻结的通用 `ModelBinding`。SDK只连接 Go 为该 attempt 建立的本机模型入口，不接收供应商密钥或真实请求地址。每轮由公开 middleware 生成 `X-Suma-Model-Step`，Go负责累计输出许可、真实供应商请求与用量记录；SDK自报用量不作为平台账目依据。
- `max_output_tokens` 通过实际 `ChatRequest.configurable.model_settings.max_tokens` 传入；它在平台中是整次执行的累计额度，Go入口会按剩余额度限制实际请求。公开模型factory固定唯一模型并设置 `max_retries=0`，同时禁止Harness重试及API fallback，保留真实流式响应。输入仍需保守预留，不能将此实现宣称为厂商最终账单或无条件金额保证。
- ACP `end_turn` 只表示智能体回复结束；Go 仍核对真实操作日志、固定检查和代码候选封存。取消通知与子进程退出分开记录，平台再确认外层进程实际停止。网络中断可能已产生效果，不自动重新开始模型。

## 安装与构建

使用项目外层提供的受保护 Python 3.12–3.14 环境，按 `requirements.lock` 安装。不要全局安装。当前锁基于 Windows/Python3.13；`pywin32` 仅Windows需要。其他操作系统须在对应环境重新解析并保存验收证据。

```powershell
uv venv .cache/e2e/octop-117/venv --python C:\Python313\python.exe
uv pip sync --python .cache/e2e/octop-117/venv/Scripts/python.exe packages/octop-runtime/requirements.lock
.cache/e2e/octop-117/venv/Scripts/python.exe packages/octop-runtime/scripts/build.py
```

生成 `dist/suma-octop.pyz` 与 `suma-octop.manifest.json`。manifest 固定 zipapp字节及已安装依赖版本；第三方分发包没有打入zipapp，不能将版本清单宣称为全部依赖字节证明或操作系统隔离。管理员必须保护 Python、venv、zipapp及manifest，Go固定这些入口的摘要。probe仅导入公开API并检查版本，不创建专家、ACP会话或调用模型。

运行目录由 Go 生成 `SUMA_OCTOP_CONFIG`；不接受其他供应商、模型或参数字段：

```json
{"home":"<fresh absolute private directory>"}
```

`SUMA_MODEL_EXECUTION` 由 Go 注入，包含完整 `binding` 对象与以下 `gateway` 能力。它不是让用户手工设置供应商配置的入口：

```json
{"protocol":"suma-model-gateway-v1","base_url":"http://127.0.0.1:4311/v1","token":"<attempt capability>","request_id":"<host request id>","binding_sha256":"<approved binding digest>"}
```

`binding` 及其部署、参数、预算、费率的规范摘要都必须有效，并与stdin的 `model_execution` 完整一致；gateway必须绑定原请求编号与同一绑定摘要。缺失或不同即在模型请求前拒绝。只接受固定本机 OpenAI兼容 Chat Completions入口；没有供应商环境密钥引用或旧配置回退。

执行stdin：`{v:1,type:"execute",request_id,goal,model_execution:<完整ModelBinding>,workspace_bridge:{version:1,endpoint,token,policy_sha256,request_id}}`。后续仅接收同一编号的 `cancel`。stdout为 `ready`、`started`、有限 `progress` 和一次 `completed/failed/cancelled`；不传原始日志、推理、代码或秘密。ready与completed仅确认 `binding_sha256`，不会把SDK配置ACK当作供应商身份或账目证据。

`--probe` 仅检查已安装SDK公开API、版本、工作区策略和模型网关合同支持，不创建模型、会话或运行目录，也不需要模型绑定。它包含 `model_execution_protocol` 与 `gateway_protocol`；Runner一次检查即可复用结果，避免重复启动Python。`--model-execution-probe` 保留与原生智能体相同的三字段能力声明，供外部合同读取。取消及父进程异常均需由Go结合真实退出事实和原始请求记录处理。

## Focused local checks

```powershell
$env:PYTHONPATH='packages/octop-runtime/src'
.cache/e2e/octop-117/venv/Scripts/python.exe -m unittest discover -s packages/octop-runtime/tests
```

No user account, provider key or external model is needed for these checks. End-to-end verification uses the existing Go workspace host and a deterministic loopback model endpoint. Vendor-account execution remains a separate acceptance dependency.

The SDK now receives only a frozen model binding and an attempt-local Go gateway capability. Supplier credentials stay in Go. Both native and Octop runtimes use the same model contract; only host-observed requests and provider usage may establish accounting evidence. Streaming remains enabled, SDK retries are disabled, and cancellation is separate from provider billing reconciliation.

## Third-party notices

Octop Harness: [MIT, fixed public source](https://github.com/TencentCloud/octop-harness/tree/68b5423bb80ecef7fca57ba5a37d9a2bdcbf12d2). Python ACP SDK: [official package](https://pypi.org/project/agent-client-protocol/0.12.1/), [source](https://github.com/agentclientprotocol/python-sdk). No vendor implementation is copied into this package. Preserve all installed distributions' licenses; bundled office skills are not enabled or copied.
