# T-QUALITY-CASE-ORIGIN 阶段验证

2026-11-06，Windows amd64，Go 1.26.10、Node 22.16.0、Vitest 3.0.5、Chrome 156.0.8078.80 与隔离 SQLite。操作见[查看原始失败用例](../development/issue-repair.md#查看原始失败用例)。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts -run '^(TestTestPlanDefect|TestTestPlanNewPublication|TestIssueRepair)' -count=1 -v` 通过 10 个顶层测试与 6 个子场景。覆盖真实缺陷、伪造来源、计划重新发布、项目授权、历史回执、独立回归和重开。
- `go test ./internal/cli -run '^Test(IssueRepairCLI|TestPlanCatalogCLI|TaskDeliveryCLI|ExpertPlanCLI|TeamPlanCLI)' -count=1 -v` 通过 12 个顶层测试，核对项目范围、权限拒绝、中英文帮助及相邻命令。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/IssueRepairPanel.test.tsx src/TestPlansWorkspace.test.tsx src/test-plans-model.test.ts --maxWorkers=1 --minWorkers=1`，34 项通过。TypeScript 通过。语言目录核对 10,870 项，无生产文案硬编码。合同生成与检查通过 1,087 个模型、491 条 API 路径及 TypeScript 类型。
- `go test -ldflags '-s -w' -overlay <stage-overlay> ./internal/platform -run '^TestQualityCaseOriginActual$' -count=1 -timeout=120s -v` 在 `E:\tmp\cx\qorigin01` 验证真实 Web、CLI、HTTP 与 SQLite。夹具通过测试报告协议提交失败结果，创建对应正式缺陷，并签发项目 API 凭证。英文亮色桌面以键盘进入固定用例，再展开同运行全部结果。中文暗色 390px 核对错误摘要的提示、同运行恢复及无横向溢出。
- 联调 31 次 GET，其中一次为缺陷尚未回流需求的预期 404。零写请求，操作、修复、缺陷关系、报告、运行和事件增量均为 0。截图和回执保存在本次任务的本地证据目录。共享目录的首次编译碰到另一任务在途 Runner 改动，Go 回归和联调改用提交 `db073235` 与本阶段文件组成的隔离副本完成。在最新 dev 基线 `622f230b` 上重新核对生成合同，并复验同一 CLI 与 Chrome 流程。

来源固定到原执行和用例定义。打开后显示该次执行中当前可核对的尝试证据，旧尝试被替换时按当前证据重新解释。保留原始尝试报告的独立归档、其他数据库和生产规模仍按原质量交付矩阵验收。
