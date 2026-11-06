# T-WORKBUDDY-PROJECT-ATTENTION 阶段验证

2026-11-06，Windows amd64，Go 1.26.10、Node 22.16.0、Vitest 3.0.5、Chrome 156.0.8078.80 与隔离 SQLite。操作见[团队进度工作台](../design/workbuddy-cloud-task.md#团队进度工作台)。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts ./internal/cli -run '^(TestWorkBuddyCloudProgress|TestWorkBuddyProgress|TestRunWatchCommand|TestRunQueue)' -count=1 -v` 通过 19 个顶层检查。SQLite／HTTP 回归建立了 43 条已保存提交，验证首批 40 条无匹配时仍可续查，下一批返回三条待处理记录，且筛选与普通游标不可混用。CLI 检查空批续查、错用游标、错误回显和权限拒绝。生成合同核对 1,091 个模型、493 条 API 路径及 TypeScript 类型。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/WorkBuddyProgressWorkspace.test.tsx src/TeamExecutionWorkspace.test.tsx --maxWorkers=1 --minWorkers=1`，16 项通过。覆盖空批续查、查完无匹配、重复游标、过期读取、无效链接和准确运行跳转。完整 App TypeScript 检查和 10,941 个语言键检查通过。
- 隔离副本执行 `go test -ldflags '-s -w' -overlay <stage-overlay> ./internal/platform -run '^TestWorkBuddyProjectAttentionActual$' -count=1 -timeout=120s -v`。受控供应方 HTTP 建立两条平台提交。真实 CLI 使用项目凭证检查普通分页、待处理匹配和续查后的最终空结果。英文亮色桌面由独立只读成员用键盘切换筛选、打开准确尝试并返回。中文暗色 390px 核对直接打开筛选链接、44px 点击区及无横向溢出。
- 联调 18 次 GET，零写请求、零新增供应方调用、零持久业务记录增量。截图和回执位于本次任务本地证据目录。

本阶段验证项目级已保存进度的筛选和续查。真实 WorkBuddy 厂商、跨数据库、物理多机和生产规模分别验收。
