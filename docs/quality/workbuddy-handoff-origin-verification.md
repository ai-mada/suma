# T-WORKBUDDY-HANDOFF-ORIGIN 阶段验证

2026-11-06，Windows amd64，Go 1.26.10、Node 22.16.0、Vitest 3.0.5、Chrome 156.0.8078.80 与隔离 SQLite。操作见[任务交付](../development/task-delivery-chain.md)。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts -run '^(TestWorkspaceTerminalCandidateWorkBuddyOrigin|TestTaskDeliveryHTTP(UndispatchedReadOnlySnapshot|WorkBuddyTraceUsesExactCurrentAttemptBeyondNodeLimit)|TestTaskDeliveryRepositoryDispatchActualIndependentQuality|TestTaskDeliveryBoundedReadContract)$' -count=1 -v` 通过 5 个顶层检查与 7 个子场景。包括真实 Git 仓库留存与独立测试、本机候选交接、原状态改变后读取、伪造摘要拒绝以及交付协议边界。
- `go test -ldflags '-s -w' ./internal/cli -run '^TestTaskDeliveryCLI' -count=1 -v` 通过 3 项，覆盖精确项目查询、拒绝及双语帮助。合同生成与检查核对 1,088 个模型、491 条 API 路径及 TypeScript 类型。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/TaskDeliveryPanel.test.tsx src/task-delivery-model.test.ts src/task-delivery-navigation-model.test.ts src/requirement-traceability-model.test.ts --maxWorkers=1 --minWorkers=1`，28 项通过。TypeScript 和 10,893 个语言键检查通过。
- 在项目忽略目录的隔离副本运行 `go test -ldflags '-s -w' -overlay <stage-overlay> ./internal/platform -run '^TestWorkBuddyHandoffOriginActual$' -count=1 -timeout=120s -v`。夹具用受控供应方 HTTP、真实平台派发和本机候选交接建立两个不同的运行。以独立项目只读成员实测英文桌面键盘打开历史尝试并返回，以及中文暗色 390px 的准确深链、44px 点击区和页面无横向溢出。真实 CLI 使用项目凭证读取相同来源。
- 联调共 55 次 GET，零写请求、零新增供应方调用及零持久业务记录变化。截图和回执位于本次任务的本地证据目录。

本阶段验证已启动验证运行的交接追踪。真实 WorkBuddy 厂商、跨数据库、物理多机及生产规模继续分别验收。
