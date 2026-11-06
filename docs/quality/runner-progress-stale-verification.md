# Runner 过期进度与停止回执恢复

速码 1.0.0，`T-RUNNER-PROGRESS-RECOVERY-267`。关联需求为外部工具进度回写、节点失联恢复与共享池调度。 [执行合同](../../contracts/tool-execution.md#shared-types-internaltoolcontract) · [运行说明](../agents/tool-execution-runtime.md)

Multica 在重新连接后触发运行状态核对。速码沿现有 Go 调度与本地 Journal 分别确认已入库进度、确定失效的旧进度和仍需重试的请求，使旧进度不再挡住停止回执。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/daemon/reconcile.go#L12-L31)

## 验证记录

- Windows、Go 1.26.10。3 项 DAG 测试覆盖已入库回执重放、租约到期、旧围栏和数据读取错误。2 项 SQLite 控制面 HTTP 测试覆盖普通节点和共享池。共享池测试让真实调度器进入待核对，随后以原身份提交停止回执并核对失败终态。
- 5 项 Runner 定向测试覆盖两类 HTTP 回放路由、拒绝与确认分别落盘、重启后继续发送原停止回执、普通冲突和服务故障保留原请求，以及桌面恢复。Runner 路由测试使用回环 HTTP 测试端点。验证范围为 SQLite 与本机，物理多节点和 OceanBase 仍待验证。
- 定向输出保存在本机忽略目录 `.cache/q267/`。
