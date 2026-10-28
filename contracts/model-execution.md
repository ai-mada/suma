# 统一模型执行合同

状态：suma 1.0.0／T-MODEL-118 当前合同。责任：模型合同、控制面、DAG、Runner 与工作台。权威类型：[modelcontract](../internal/modelcontract/model.go)；公开接口：[OpenAPI](../docs/api/openapi.json)；架构：[统一模型网关](../docs/architecture/model-gateway.md)。实际证据另见[本轮报告](../docs/quality/model-gateway-verification.md)，本文不预先声明验收通过。

## 配置与能力发现

Runner 使用现有 `--model-deployments` 允许列表，Pool 按各项目执行范围配置。公开不可变部署包含 ID、修订、摘要、协议、模型、`endpoint_origin`、完整端点配置摘要与 `context_window`。完整 `base_url`、`api_key_env_ref` 和真实密钥留在 Go 管理范围，不进入库存、任务、模型上下文或 SDK 环境。

当前接入共同协议的工具是启用 `attempt-workspace-v1` 的原生 Suma 编码与 Octop Harness／ACP，均显式声明 `model_execution_protocol=suma-model-execution-v1`。离线能力探测通过后，观察携带公开模型目录。安装、配置、最近可用性、模型认证和实际验收是不同事实；不可用工具不因目录或协议字段存在而取得派发资格。

```text
GET /api/v1/model-execution/options?project_id=PROJECT&tool_profile_id=TOOL&expected_tool_version=VERSION
suma --control http://127.0.0.1:4310 --project PROJECT models options --tool TOOL --expected-tool-version VERSION
```

响应绑定准确项目、工具 ID／版本／摘要和观察时间；支持共同选择时必须有 `parameter_limits`。当前合同最高为 `max_steps:128`、`max_output_tokens:131072`，界面遵守本次响应中的上限。`items` 提供真实部署、默认参数、当前精确匹配费率或未定价事实。不存在 Octop 专用固定模型选项，工具模型选择沿同一入口。

## 选择与不可变计划

现有任务预演接受 `model_execution_selection`：

```json
{
  "deployment_id": "team-model",
  "deployment_revision": 1,
  "deployment_sha256": "来自当前模型选项的实际64位摘要",
  "parameters": {"max_steps": 8, "max_output_tokens": 4096},
  "budget": {"token_limit": 100000, "cost_limit_micros": 20000, "currency": "CNY"}
}
```

示例仅说明字段，摘要和预算须来自真实目录与预留计算。预算可省略，沿用当前组织／项目政策；费用是整数微货币。`max_steps` 限制一次 attempt 的请求次数，`max_output_tokens` 限制**整个 attempt 累计输出**。未开放的模型、URL、凭据、环境、温度或 SDK 设置覆盖均拒绝。

服务端从可信选择生成 `ModelBinding`：部署、参数及摘要、预算、准确费率快照、保守预留与完整绑定摘要。同一完整绑定写入 Plan 与唯一 agent 生产节点。普通、专家逐项计划和原生受管修复分别沿既有授权与修订入口使用它；Octop 不自动获得原生修复用途。派发节点 `max_retries=0`，不允许供应商异常触发无条件自动重跑。

预览、Start、Claim、Runner 均核对原计划与当前权限、工具／路由、模型版本和费率依据。客户端也核验原操作的项目、任务、工具和生产节点绑定；不一致的成功响应不能清除待核对操作。网络未知结果恢复原 `idempotency_key`，不换模型、正文或 key 再创计划。

## 额度、费率与唯一请求权威

```text
I = context_window × max_steps
O = max_output_tokens
reservation_tokens = I + O
reservation_cost_micros = ceil((I × input_rate + O × output_rate) / 1_000_000)
```

领取时按原费率、组织／项目额度及 attempt 预算原子预留，溢出或不足拒绝。上下文窗口是管理员声明的保守上界，不是精确 tokenizer。当前费率为冻结运营方输入／输出两档估算，不代表完整供应商报价或绝对账单保证。未定价不显示免费，也不伪造费用通过门禁；后续当前报价不替换原计划的准确费率 ID／版本。

Go 网关唯一拥有供应商密钥、真实上游 HTTP 与持久请求账本。SDK 仅获本 attempt 的短期回环连接。发送上游前持久保存 started；同 step 同原正文仅在存在 completed 原响应缓存时返缓存，不再发上游。未知、缺缓存、不同正文或乱序请求拒绝，不能退回直连供应商。

真实 SSE 内容持续转发；终态、usage 和 DONE 后缀待权威响应及 journal 检查点持久化后再释放。完整用量才回收未用输出许可，缺失／partial／unknown 继续占用保守许可。SDK 汇总不能覆盖 Go 原始请求事实。

## 回执、恢复与账务

`ModelEvidence` 使用 `gateway_protocol=suma-model-gateway-v1`，包含 `gateway_closed`、绑定／部署／参数摘要、所选模型和连续请求数组。每次请求索引从 0 开始，包含 `state`（started／completed／rejected／unknown）、`step`、请求／可选响应摘要、真实返回模型／请求 ID、输出许可、usage_state 与可选 input／output／cached_input／cache_write_input／reasoning_output 五类计数。

缺失不是零，取消不是零。原始返回模型或证据不一致阻止代码成功；已知计数仍保留。Runner journal 重启从原 assignment／attempt／fence／工具／绑定检查点恢复，保留已知 usage，把未完成请求作为 unknown，不自动再请求供应商或伪造进程停止。原操作核对前保留未解 hold。

网关关闭、模型响应覆盖、用量完整、账本 `settlement_state`／hold、代码封存、独立测试和人工验收分别记录。真正关闭且零请求可证明零模型消耗，不证明编码已完成。代码失败仍可能产生已知费用；模型响应成功但用量不全仍需账务核对。使用既有用量账本查看结算，不由 UI 或 SDK 自行推断冻结释放。

## 接入范围

只读规划、知识提炼、故障分诊和智能运维的专用 purpose、OpenCode 固定模型与 Codex app-server 合同保持其明确范围。本合同不声明所有 SDK、CLI 或外部平台都已统一，也不扩大项目权限、生产授权或人工验收能力。真实厂商账户、账单、物理多机及生产验收须独立证据。

## English contract summary

Native coding and Octop ACP share `suma-model-execution-v1`, model selection, one immutable binding/rate snapshot and the existing budget ledger. Supported options carry dynamic parameter limits. Output tokens are a **total attempt allowance**; input reservation is a conservative declared context bound times maximum requests.

Go alone owns provider secrets, HTTP and durable request admission. SDKs use short-lived attempt loopback access. An identical completed step can return its original cache without another upstream call; unknown outcomes are not retried. SSE remains streamed, with its terminal/usage suffix released only after durable host evidence. Restart preserves known usage and unresolved holds. Model response, usage, settlement, source correctness and human acceptance remain separate facts.
