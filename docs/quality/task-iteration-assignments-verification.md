# 批量调整任务迭代验证

T-MULTICA-ITERATION-BATCH。Windows、本机隔离 SQLite、Go 1.26.10、Vite 6.1.0、Chrome。构建、临时数据库和浏览器证据位于任务的忽略目录 `.cache/task-iteration-assignments/`，活动预览独立。

- `go test ./internal/platform -run '^(TestTaskIterationAssignment|TestPlanningScheduleBatchVersionReplayAndScope|TestTaskIterationQueryUsesPlansAndCurrentPlanVersions)' -count=1 -v` 通过 5 个顶层和 19 个子用例。覆盖移动与移出、原排期保留、无变化项、版本冲突、完成范围、跨项目、撤权后重放、请求边界及同回执。
- `go test ./cmd/suma-contracts -run '^(TestTaskIteration|TestTaskOwnerChangesPublicContract)' -count=1` 通过 4 个用例。`go run ./cmd/suma-contracts -check` 核对 1018 个模型、459 条 API 路径和 TypeScript。
- 在 `apps/web` 运行 `npm.cmd run test -- src/TaskIterationAssignments.test.tsx` 通过 6 个用例，覆盖逐项差异、过时依据、冲突刷新、精确重放、撤权隐藏和权限恢复。`npm.cmd run typecheck` 与 `npm.cmd run check:i18n` 通过，10073 对词条、0 项生产硬编码。
- Web 生产构建通过，Vite 报告现有大资源包警告。首次重新构建控制面时，工作区中另一个进行中的 Runner 变更缺少配套方法。浏览器验证用的控制面以已提交的 `283df50` Runner 相关文件建立 Go overlay，结合本阶段的当前业务代码构建。原工作区内容完整保留。
- 实际浏览器联调通过 6 项断言。任务列表用键盘打开三项差异。隔离控制面提交成功后，浏览器拦截器丢弃第一条 200 响应，用户点击核对后向同一服务重发完全相同的请求。SQLite 的正式读取验证排期版本仅推进一次，保留任务版本、日期、里程碑和工时。两次 POST 均为同一操作。390px 英文深色移出预览可见，Escape 关闭不产生写入。中文桌面和英文移动截图均目视复核，记录的页面与控制台错误为 0。
- 浏览器证据 `.cache/task-iteration-assignments/browser-owR3jO/result.json`，SHA256 `dfcd844eddc240251e5afcd6853d4d00a10fe6935a276364c3cf289d8d3e4d7a`。移出操作的数据库验证由 Go HTTP 用例覆盖，浏览器验证范围为移出预览和取消。其他数据库、真实跨设备和平台生产验收各有独立范围。
