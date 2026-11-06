# T-TEAM-DELIVERY 阶段验证

2026-11-06，Windows amd64，Go 1.26.10，Node 22.16.0，Vitest 3.0.5，Chrome 156.0.8078.80，隔离 SQLite。操作见[团队执行与交付进度](../development/team-execution.md#从团队计划核对交付)。

- `go test ./internal/cli -count=1 -run '^(TestTaskDeliveryCLI|TestTaskCollectionCLI|TestUnsafeInputsNeverSendRequests|TestCLILanguageHelpAndExplicitPreference)' -v` 通过 13 个顶层和 17 个子场景。覆盖一次授权 GET、精确范围、原 JSON、参数拒绝、中英文帮助及既有任务集合。
- 在 `apps/web` 运行 `node ./node_modules/vitest/vitest.mjs run src/task-delivery-navigation-model.test.ts src/TaskDeliveryPanel.test.tsx src/record-navigation-dom.test.tsx src/TeamTaskDispatchDialog.test.tsx --maxWorkers 2 --minWorkers 1`，39 项通过。覆盖原运行、不同绑定、无绑定、未知、版本变化、拒绝、操作锁和历史修订。TypeScript 及 10,850 个语言键检查通过。
- `& .cache/team-delivery/run.ps1 -TaskRoot 'E:\tmp\cx\tdel01'` 构建实际 CLI 并通过 Chrome、HTTP 与 SQLite 联调。英文亮色桌面从真实待验收团队计划以键盘打开原交付，焦点落在交付面板，浏览器返回恢复原计划和团队。中文暗色 390px 使用另一有效运行 ID 检查绑定不匹配，再明确打开当前交付；另核对未绑定任务、44px 触达和无横向溢出。
- 最终联调共 54 次 GET，其中交付读取 6 次。实际 CLI 同项目成功、跨项目拒绝。原任务仍待正式验收，持久操作、运行、事件及问题增量均为 0。首次界面通过后补充焦点与授权处理，复验因 D 盘空间不足中断一次，随后在项目约定的 E 盘短路径复验通过。截图和回执位于本机忽略目录 `.cache/team-delivery`。

阶段核验真实平台读取、授权及导航。运行由隔离协议夹具完成，真实厂商执行、物理多机与生产部署按原矩阵独立验收。
