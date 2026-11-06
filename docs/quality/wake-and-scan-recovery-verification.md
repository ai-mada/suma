# 调度唤醒与工具检测恢复验收

状态：T-WAKE-AND-SCAN-RECOVERY-275 已通过本机定向验证。版本：1.0.0。范围：控制面到期唤醒、远程节点检测面板。

## 调度恢复

按运行隔离唤醒事务。失败后重新核对同一唤醒，保存有上限的数据库时钟退避。待处理队列和退避队列分别按索引读取，合并时使用实际可处理时间。原到期时间、代次、attempt 和 fence 保留。退避到期后仍走原状态机，成功或替换时清理退避记录。调度器的执行失败退避与业务节点重试分别记录。 [内核设计](../scheduler/incremental-kernel.md) · [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/scheduler/db_ops.go#L112-L138)

Windows 本机 SQLite 共通过 19 个 Go 顶层用例。新增回归使用单条批次和两个真实计时器，模拟首条处理异常，关闭连接并重新打开原库，验证正常运行先推进。之后修复异常，推进到重试时间并处理原唤醒，确认退避记录清除。既有租约、回执与故障回流用例通过。

```powershell
go test -p 2 ./internal/dag ./internal/platform -run '^(TestTick|TestPausedTimer|TestRetry|Test.*Lease|TestFailureProjectionIncludesTickTriggeredSubworkflowDeadline)' -count=1 -v
```

## 工具检测读取

选择一条历史后立即核对该记录。确认期间隐藏准备和配置动作。手动重新读取可接管旧明细。较早的响应不能改写当前结果、重新启动轮询或恢复已失效的准备资格。 [交互设计](../design/remote-tool-installation-scan.md)

组件回归先复现旧版没有发出新明细请求，再验证修复。该组件共 9 个用例通过，覆盖原操作恢复、权限和停止证明。实际 Chrome 使用隔离组件与合成 HTTP 响应，验证中文 390px、英文 1280px、键盘选择与刷新、无横向溢出、零写请求。未触发真实远程节点检测。

```powershell
# 在 apps/web 下运行
pnpm exec vitest run src/RemoteToolScanPanel.test.tsx
```

PostgreSQL、OceanBase、物理多节点竞争、持续负载以及真实供应方仍按完整方案验收。浏览器验证限定为上述组件与窗口尺寸。
