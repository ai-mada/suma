# Suma native agent runtime

版本：1.0.0。研发执行使用平台冻结的 `model_execution` 绑定；运行时仅持有 attempt 本机模型入口，不接收真实供应商地址或密钥。

`SUMA_MODEL_EXECUTION` 是 Go 宿主注入的封闭对象：

```text
{
  binding: <完整 ModelBinding>,
  gateway: {
    protocol: "suma-model-gateway-v1",
    base_url: "http://127.0.0.1:<port>/v1",
    token: <本次执行临时能力>,
    request_id: <宿主原请求编号>,
    binding_sha256: <冻结绑定摘要>
  }
}
```

绑定与stdin `model_execution`、原请求编号完全一致才允许调用；拒绝供应商地址、旧 `base_url/api_key` 对象、其他请求编号及未知字段。每轮HTTP请求带上运行时生成的 `X-Suma-Model-Step`。Go负责真实供应商调用、累计输出许可及用量证据；运行时进度不是账单依据。工作区仍通过官方MCP客户端连接同一Go宿主，不增加调度器。

`--model-execution-probe` 只输出固定合同能力，不调用模型。文档规划、知识萃取等独立用途的专用配置与研发执行绑定不同；不能用它们绕过已有工具及用途准入。

构建与本阶段定向检查：

```text
pnpm --filter @suma/agent-runtime build
node --test packages/agent-runtime/dist/tests/model-execution.test.js
```

构建生成 `dist/suma-agent.bundle.mjs` 与 `dist/suma-agent.manifest.json`，由管理员固定入口及依赖摘要；生成产物与缓存不提交Git。

Development runs receive only the frozen common model binding and a loopback capability from Go. Every model request carries a monotonic step header. Real endpoints, supplier credentials, cumulative output allowance and accounting evidence remain host-owned. Unknown usage and cancellation retain their original operation for reconciliation; the runtime must not invent usage or automatically restart the model.
