# OpenAI 模型真实联调记录

2026-11-06 · T-MODEL-OPENAI-LIVE · Windows amd64、Go 1.26.10、Node 22.16.0。使用已提交的 `622f230` 相关源码快照、任务修改和单独的 SQLite、Git 工作区及 Runner。项目密钥由用户选择的 OpenAI 项目授权，只有 Runner 进程加载本次厂商凭据。

| 环节 | 实际结果 |
| --- | --- |
| 厂商认证 | 使用项目凭据只读查询固定模型 `gpt-4.1-mini-2025-04-14`，HTTP 200。该模型及费率依据见[OpenAI 模型页](https://developers.openai.com/api/docs/models/gpt-4.1-mini)。 |
| 节点与目录 | 真实 `suma-control`、`suma-runner --describe-tools` 和 `--report-tools` 完成库存探测、上报、暂停登记及启用。真实 `/model-execution/options` 返回准确的部署与工具版本，从未定价切换到匹配的 USD 费率版本。 |
| 实际请求 | 三个独立的原尝试各限制一条模型请求、96 输出 Token。05:20 与 05:24 UTC 的请求返回 HTTP 400，第二次保留诊断 `invalid_function_parameters`，字段 `tools[5].function.parameters`。修正 `workspace.write` 的厂商声明后，05:31 UTC 的请求未收到任何 HTTP 状态。单独的只读连通性检查随后出现 TLS EOF。 |
| 原尝试与用量 | 最后一次运行 `8948f570ee034567c5b1beba32825087`，节点 `develop`，尝试／fence 均为 1。网关闭合，原请求状态为 `unknown`，账本为 `reconciling`，Token 与实际费用估算字段为空，保留 1,047,672 Token 和 419,184 USD 微单位的预留。 |
| 厂商账单 | 本次浏览器访问跳转到厂商登录。需该项目的授权用量页或 Activity / Cost 导出核对 UTC 05:31 左右的请求、Token 及实际费用。现阶段保存账本待核对状态。 [OpenAI 用量导出](https://help.openai.com/en/articles/20001072) |

## 本机回归

- `node packages/agent-runtime/node_modules/typescript/bin/tsc --noEmit -p packages/agent-runtime/tsconfig.json` 通过。使用 `SUMA_NATIVE_RUNTIME_OUT_DIR` 指向隔离目录，运行 `node packages/agent-runtime/scripts/build-native-runtime.mjs` 构建实际 Node 运行包。
- `go test ./internal/runner -run '^Test(ModelExecutionTwoDeploymentsReachActualNativeBundle|NativeWorkspaceCancellationCannotReportUnsealedSuccess)$' -count=1` 通过。真实本机 Node 与 Go 网关对接受控 HTTP，对比两次成功、HTTP 400 拒绝和无响应。拒绝与无响应均保留一次原请求，诊断区分状态，厂商错误正文无公开输出。
- `go test ./internal/platform -run '^TestModelExecution(CurrentPreviewActualRuntimeAndUsageLedger|FrozenRateClaimReservationAndSingleSettlement|InventoryDriftAndMissingSelectionPreventStart|UncertainProviderUsageKeepsKnownCountersAndHold)$' -count=1` 四个顶层检查通过，另有三个子例。受控成功响应的 40 Token／57 CNY 微单位按冻结费率结算。
- 模型网关包测试通过。本地化检查在排除 `TestDisplayCatalogExplicitSourceAndNoReverseAlias` 与 `TestPlatformDiagnosticCoverage` 后通过。上述两个本地化用例和 `TestNativeWorkspaceHostProbesMJSAndSealsNoChange` 在未改基线上复现失败。

实现修复见[网关架构](../architecture/model-gateway.md#2-模块及数据流)和[核对操作](../operations/model-execution.md#核对厂商用量与账单)。真实响应拒绝和未知尝试保存在本机隔离项目。原生协议、工具登记与本地账本验证范围独立于厂商账单及物理多机。

脱敏联调回执 SHA-256 为 `b69dfa27f6e9bee126a41f1cceaa88f21d49cfd679782d6ba3c24114ded9ead1`。
