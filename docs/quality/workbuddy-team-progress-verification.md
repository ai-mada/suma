# T-WORKBUDDY-TEAM-PROGRESS 阶段验证

2026-11-06，Windows amd64，Go 1.26.10、Node 22.16.0、Vitest 3.0.5、Chrome 156.0.8078.80 与隔离 SQLite。操作见[团队进度工作台](../design/workbuddy-cloud-task.md#团队进度工作台)。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts ./internal/cli -run '^(TestWorkBuddyCloudProgress|TestWorkBuddyProgress|TestRunWatchCommand|TestRunQueue)' -count=1 -v` 通过 18 个顶层检查。覆盖已准备与未知提交、分页、与单节点投影一致、只读成员、撤权、游标隔离、畸形响应与拒绝未授权身份。合同生成核对 1,089 个模型、492 条 API 路径及 TypeScript。
- `node node_modules/vitest/vitest.mjs run src/WorkBuddyProgressWorkspace.test.tsx src/TeamExecutionWorkspace.test.tsx --maxWorkers=1 --minWorkers=1` 通过 14 项。阶段源码和生成合同的完整 App TypeScript 检查通过，中英语言目录通过。浏览器首轮发现通用等待徽章会被读成待审批。已改用等待状态及对应原因，组件与实际浏览器均复验。
- `go test -ldflags '-s -w' -overlay <stage-overlay> ./internal/platform -run '^TestWorkBuddyTeamProgressActual$' -count=1 -timeout=120s -v`。受控供应方 HTTP 建立两条真实平台提交。项目 API 凭证用 `--limit 1` 续读两页。独立只读成员在英文桌面用键盘筛选已加载的待处理记录、打开原节点并返回。中文暗色 390px 核对 44px 点击区和横向溢出。
- 联调共 16 次 GET，零写请求、零新增供应方调用、零持久业务记录增量。两条供应方记录均已完成，平台一条仍在执行、一条等待交付证据。首次联调夹具重复注册节点，改用同一已登记节点领取后续运行，随后联调通过。截图和回执位于本次任务的本地证据目录。

本阶段验证项目级已保存进度的查询、工作台和终端。真实 WorkBuddy 厂商联调、跨数据库、物理多机和生产规模仍分别验收。
