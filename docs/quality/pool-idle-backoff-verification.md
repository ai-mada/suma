# 共享池空闲领取专项验证

T-POOL-IDLE-BACKOFF · Windows amd64 · Go 1.26.10 · SQLite · 源码基线 `2a001551fdd0`，叠加本阶段两个 Runner 文件。

- 只在当前槽位将成功空回执持久标为完成后拉长下一次领取间隔。默认基础间隔 2 秒，连续空闲最多 8 秒。派发、未知领取或回写故障清除空闲累计。未知领取仍以原操作恢复。
- 本机 HTTP 控制面及真实 `PoolWorker.Run` 使用新登记凭证、批准范围和固定池路由。配置基础间隔 200 毫秒，首批四次空领取间隔实测 213、383、807 毫秒。随后同一槽领取正式入队的命令，启动本机测试子进程，回写唯一一条成功尝试。完成后再次空闲的首段间隔为 211 毫秒。
- 数据库核对空领取持久回执、零尝试／槽位占用／资源预留和公平份额未推进。任务完成后同一空操作仍返回空，原派发操作返回已关闭的同一尝试与围栏，重放未新增尝试。停止时按原槽位正常收尾。

## 执行命令

```text
go test -ldflags '-s -w' ./internal/runner -run '^(TestPoolClaimPollBacksOffOnlyConsecutiveEmptyClaims|TestPoolWorkerRunsTwoRealProjectProcessesInIndependentSlots|TestPoolWorkerActuallyStopsProcessAfterScopedHeartbeatCancel|TestPoolClaimCheckpointReusesOriginalScopeAndOperation|TestPoolShutdownJoinsOriginalSlotsAndPreservesOutbox)' -count=1 -timeout=90s -v
go test -ldflags '-s -w' -overlay <fixture-overlay.json> ./internal/platform -run '^TestPoolIdleBackoffActual$' -count=1 -timeout=90s -v
node scripts/check-docs.mjs
```

Runner 五项检查和 SQLite 联调通过。联调用例、回执与来源校验摘要保存在阶段证据目录。子进程为测试程序，使用本机临时工作区。本阶段没有改变页面及供应商适配。物理多机、生产规模、数据库长期增长与回执保留仍单列验收。

[共享池合同](../../contracts/execution-pools.md#空闲领取退避) · [Runner 手册](../agents/execution-pool-runner.md#本地配置与运行)
