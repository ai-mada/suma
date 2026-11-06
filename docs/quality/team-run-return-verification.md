# T-TEAM-RUN-RETURN 阶段验证

2026-11-06，Windows amd64，Go 1.26.10、Node 22.16.0、Vitest 3.0.5、Chrome 156.0.8078.80 与隔离 SQLite。操作见[从运行返回团队计划](../development/team-execution.md#从运行返回团队计划)。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts ./internal/cli -run '^(TestExpertTeamPlanHTTP|TestExpertTeamCurrentModelExcludesOriginalOnlyHistory|TestExpertTeamExecutionContract|TestExpertTeamPlanListContract|TestExpertPlanCLI|TestTaskDeliveryCLI)' -count=1 -v` 核对实际派发的保护关系、同项目 viewer、不同项目、团队交集、准确大小写、游标与不一致关系。通过 14 个顶层 Go 检查。生成合同核对 1,087 个模型、491 条 API 路径和 TypeScript 类型。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/RunTeamOrigin.test.tsx src/TeamExecutionWorkspace.test.tsx --maxWorkers=1 --minWorkers=1`，13 项通过。TypeScript 检查通过，10,890 个中英语言键通过检查。重点覆盖按需读取、空结果、权限拒绝、读取失败及重试、不同运行的迟到响应和准确计划跳转。
- 使用 `E:\tmp\cx\trun01` 的隔离源码与输出，`go test -ldflags '-s -w' -overlay <stage-overlay> ./internal/platform -run '^TestTeamRunReturnActual$' -count=1 -timeout=120s -v` 验证 CLI、真实 Chrome、HTTP 和 SQLite。英文亮色桌面通过键盘从原运行打开准确计划并返回。中文暗色 390px 核对普通运行无关联、无横向溢出。真实项目 API 凭证读取准确归属及普通运行空结果。展开前无归属查询，每次展开只发出一次准确查询。
- 联调在 `9ce3bda0` 基线上复验，共 43 次 GET，零写请求。操作、运行、事件、缺陷、计划和计划绑定的持久记录均无增量。截图和回执位于本次任务的本地证据目录。
- 交互参考本地 Multica `e8b6a976` 的事项详情与执行记录组织方式。本阶段使用速码现有领域、视觉组件和授权。 [Multica 事项详情源码](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/packages/views/issues/components/issue-detail.tsx)

本阶段使用普通 Runner 流程的协议夹具与工作台共用入口。WorkBuddy 厂商实际任务、跨数据库、跨物理机器和生产规模仍分别验收。
