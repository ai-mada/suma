# 原生模型执行合同 v1

状态：1.0.0／T-P4-036，限定本地实现及验证通过。权威类型为 [modelcontract](../internal/modelcontract/model.go)，生成接口见 [OpenAPI](../docs/api/openapi.json)。本合同不授予模型访问权限，不代替当前项目、工具和执行节点授权。

## 配置与发现

Runner 使用独立 `--model-deployments` 文件；Pool 在各项目执行范围配置相同的允许列表。每项包含公开不可变部署定义，以及仅本地读取的完整 `base_url` 和 `api_key_env_ref`。密钥值不进入库存、计划、节点或浏览器。本地定义和完整端点摘要必须匹配，节点不能覆盖 URL、环境或模型。

工具显式声明 `model_execution_protocol: suma-model-execution-v1`，且必须是启用工作区的原生 Suma JSONL 工具。实际离线能力探测通过后，库存携带公开 `model_deployments`。配置声明、程序可执行、模型服务健康和厂商验收是不同事实。

`GET /api/v1/model-execution/options?project_id=…&tool_profile_id=…&expected_tool_version=…` 返回当前授权工具版本／摘要、观察时间及部署选项。未知、重复、空参数和非规范版本整数均拒绝。CLI 等价读取为：

```text
suma --control http://127.0.0.1:4310 --project PROJECT models options --tool TOOL --expected-tool-version 2
```

## 选择与固定计划

原 `POST /task-dispatch-plans` 接受 `model_execution_selection`：

```json
{
  "deployment_id": "team-model",
  "deployment_revision": 1,
  "deployment_sha256": "从当前选项完整复制的64位摘要",
  "parameters": {"max_steps": 2, "max_output_tokens": 100},
  "budget": {"token_limit": 20000, "cost_limit_micros": 20000, "currency": "CNY"}
}
```

上述片段是字段说明，摘要及部署须来自当前真实选项；不能直接作为现成部署使用。预算可省略；费用使用整数微货币，界面将 `CNY 0.02` 精确转换为 `20000`。只有 `max_steps` 和累计 `max_output_tokens` 可选，普通任务范围分别为1–16和1–32768。其他参数、凭证或环境输入拒绝。

服务端先清除模板可能携带的模型绑定，再从可信选项生成 `ModelBinding`：公开部署、参数及摘要、预算、准确费率快照、保守输入／输出预留和绑定摘要。相同绑定进入 Plan 与 agent Node.Config；顶层执行限额必须一致。仅复制的派发节点将 `max_retries` 固定为0，原发布模板保留。编译拒绝非零自动重试，模型异常不能授权自动重跑。

预览、Start和Claim分别检查其当前依据；未知结果恢复原操作，不能换模型重新执行。恢复存储只例外接受准确选择路径中的数值额度，凭证字段及伪装对象仍被拒绝。

## 费率、预算与实际结果

现有 `/usage/rates` 写请求增加 `model_deployment_id`、`model_deployment_revision`、`model_deployment_sha256`。可写原生工具报价必须匹配当前真实库存。首轮每工具只读取一份当前报价，其他模型显示未定价；已固定 Plan／attempt 始终读取其原准确报价ID／版本，不自动采用后续当前报价。

输入保守预留为 `context_window × max_steps`，输出预留为整个执行的总额度。本次预算必须覆盖预留，领取时同时检查当前组织／项目策略并原子预留。此处依赖提供方声明的上下文上限，尚无精确 tokenizer、厂商账单对账或逐 Token 金额网关，不能宣称无条件费用保证。

真实 Runtime 核验本地部署和绑定，按所选模型、完整端点和允许参数发请求。单请求输出额度取总剩余、用途上限与声明上下文上限的最小值；只有完整可信整数用量才回收未使用输出许可。请求／响应摘要、提供方模型／请求ID和逐次覆盖状态形成绑定回执；模型身份不符、缺证、超界或不完整用量保留已知事实和未解除的预留，进入核对。按冻结运营方输入／输出两档费率使用整数运算结算，重复回执不重复记账。

## 边界

本合同已贯通普通任务，并在040扩展到[专家团队每项原生任务](expert-team-model-execution.md)。外部CLI和受管冲突修复入口尚未接入独立模型选择；不支持时明确拒绝，不退回环境中的其他模型。模型 HTTP 层为受限 OpenAI 兼容协议适配器，工作区使用已安装的官方 MCP SDK；未新增官方模型 SDK。

`suma … developer check` 通过官方 Go MCP SDK 实际初始化、列工具并执行一次当前项目概览只读调用；这证明该次 MCP 连接，不能代表远程工作区、PTY、LSP或DAP已实现。见 [开发工具连接](../docs/integrations/developer-connection-check.md) 和 [限定验证](../docs/quality/model-execution-verification.md)。
