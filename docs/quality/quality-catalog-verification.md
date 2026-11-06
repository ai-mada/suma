# T-QUALITY-CATALOG 阶段验证

2026-11-06，Windows amd64，Go 1.26.10，Node 22.16.0，Vitest 3.0.5，Chrome 156.0.8078.80 与隔离 SQLite。操作见[定位测试计划](../development/test-plans.md#定位测试计划)。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts -run '^TestTestPlan' -count=1 -v` 通过 10 个顶层测试和 10 个子场景。覆盖准确状态与任务、先筛选再计数、原未筛选游标、筛选游标隔离、游标锚点状态变化和当前项目授权。
- `go test -ldflags '-s -w' ./internal/cli -run '^(TestTestPlanCatalogCLI|TestCLILanguageHelpAndExplicitPreference|TestTaskDeliveryCLI)' -count=1 -v` 通过 7 个顶层测试和 3 个子场景。覆盖准确路径、原游标、原 JSON、拒绝异常参数及中英文帮助。
- 在 `apps/web` 运行 `node ./node_modules/vitest/vitest.mjs run src/TestPlansWorkspace.test.tsx`，17 项通过。冲突恢复后续调整另复验 1 项。TypeScript 与 10,861 个语言键检查通过。
- `& .cache/quality-catalog/run.ps1 -TaskRoot 'E:\tmp\cx\qcat01'` 完成真实 CLI、Chrome、HTTP 与 SQLite 联调。英文亮色桌面验证 26 条计划的列表过滤、准确任务交集，以及真实 409 后在筛选范围之外找到原计划并保留未保存标题。中文暗色 390px 验证状态与任务深链、键盘清除、44px 主要操作及无横向溢出。实际 CLI 成功续读已发布范围两页并打开准确计划。
- 联调 24 次 GET 和一次预期 409 的 POST。持久操作、计划、修订、运行和事件增量均为 0。D 盘构建因空间不足中断，项目约定的 E 盘隔离短路径复验通过。该次合同生成与检查核对 1,086 个模型、491 条 API 路径及 TypeScript 类型。截图和回执位于本机忽略目录 `.cache/quality-catalog`。

本阶段核验计划查询及草稿恢复。实际测试厂商、跨数据库与生产环境继续按原质量交付矩阵分别验收。
