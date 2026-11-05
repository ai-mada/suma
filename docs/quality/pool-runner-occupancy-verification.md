# 共享资源池节点槽位占用验证

T-POOL-RUNNER-OCCUPANCY。Windows、Go 1.26.10、Vite 6.1.0、Chrome 156.0.8078.80 与隔离 SQLite。浏览器使用 `2fcf6d8` 加本阶段源码。

- `go test ./internal/platform -run '^Test(ExecutionPoolRunner(Occupancy|Exact)|PoolMachineHTTPIndependentIdentity)' -count=1 -v` 通过 5 项。通过真实 HTTP 在两个获批组织范围领取 2 个槽位，未知结果进入待核对且继续占用，正式完成和确切核对分别释放各自的槽位。覆盖缺失记录、错误 fence 或执行节点、未知分类、明确零、跨组织、管理员降级、凭证隔离及公开投影。
- `go test -overlay <阶段配置> ./cmd/suma-contracts -run '^TestExecutionPool' -count=1` 通过 3 项。生成检查核对 1071 个模型与 481 条路径，差异限定在原节点列表接口、可选槽位字段及其模型。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run <文件> --maxWorkers 2 --minWorkers 1 --no-cache`。新交互 4 项、原资源池消费者 18 项通过。更新了三处沿用旧按钮文字的定位。撤权后延迟重试的用例确认旧计数不会在请求期间重新出现。整库 `tsc --noEmit` 通过，中英 10576 对词条检查通过。
- Chrome 与真实控制面通过 10 项，覆盖中文明亮桌面、英文深色 390px、节点诊断键盘操作与焦点恢复、刷新和过时提示。隔离数据库写入三类槽位，移除其中一条后刷新只减少对应计数。使用真实 HTTP 登记节点并降级管理员，收到预期的 403 后隐藏旧占用。实际领取与核对释放由 Go 集成场景独立验证。零意外浏览器错误和外部请求。两个早期构建遇到源码并发更新被完整性检查拒绝；最终隔离构建成功，有既存 Vite 分块大小提示。
- 最终证据 `.cache/pool-runner-occupancy/browser-evidence-jvnMED/result.json`，SHA256 `c4b85c993b3db21bd766f1364320037977a7a0261a2895c79a50707027145f8b`。同目录保留两张复核截图。控制进程、Chrome 和回环端口均已释放。盘点后清理 630 个浏览器临时文件，收据 `.cache/pool-runner-occupancy/browser-cleanup.json`；生成检查收据 `.cache/pool-runner-occupancy/contract/scratch-cleanup.json`。

节点列表只展示调度器已保留槽位和可关联的尝试状态。机器就绪、物理多主机、PostgreSQL／OceanBase 实库与生产规模各自等待验证。
