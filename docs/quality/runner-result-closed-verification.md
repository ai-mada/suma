# 已关闭尝试的迟到结果恢复

速码 1.0.0，`T-RUNNER-RESULT-RECOVERY-272`。关联需求为分布式节点失联、原回执恢复和执行队列连续性。 [执行合同](../../contracts/tool-execution.md#shared-types-internaltoolcontract) · [运行说明](../agents/tool-execution-runtime.md)

参考 Multica 对执行终态使用独立的持久待发记录。速码沿既有 Runner Journal 保留原结果和操作标识。若控制面确认准确尝试已经关闭，节点记录拒收并继续重放。已提交但丢失响应的结果按原回执确认。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/daemon/terminal_report_queue.go#L84-L108)

## 阶段验证

- Windows、Go 1.26.10。5 项 Runner 定向测试通过。回环 HTTP 覆盖普通和共享池两类路由，核对服务故障与普通冲突仍待重试、明确关闭后本地持久保留、重启后可确认后续结果。测试另外覆盖旧进度、桌面恢复和原回执重放。
- 3 项 SQLite 控制面 HTTP 检查和 3 项 DAG 检查通过。实际调度器触发租约失效与人工核对。原尝试仍在核对时保持普通冲突；已关闭的原尝试返回明确拒收。伪造围栏、停止回执和工具证据仍需独立核验。共享池后续尝试成功入库，重复原操作仍返回确认。
- 实机多节点和 OceanBase 仍待验证。结构化工具证据、停止确认与外部副作用走各自的核对路径。定向证据在本机忽略目录 `.cache/q272/`。
