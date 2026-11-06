# 团队执行计划筛选验证

T-TEAM-PLAN-FILTER 为团队执行增加精确团队查询、已加载计划筛选和分享链接。 [操作说明](../development/team-execution.md) · [工作台合同](../../contracts/team-execution-workbench.md)

## 定向检查

- Windows amd64、Go 1.26.10。定向运行平台与 `suma-contracts` 的 6 项检查，覆盖先筛选后计数、团队大小写、原游标隔离、其他项目访问、只读回执和生成合同。`go run ./cmd/suma-contracts --check` 验证工件一致。
- `go test ./internal/cli -run '^TestExpertPlanCLI' -count=1 -v` 通过 4 组和 14 个子用例，覆盖双语帮助、精确团队、跨范围响应拒绝和真实长度的分页游标。
- Node 22.16.0。`node ./node_modules/vitest/vitest.mjs run src/TeamExecutionWorkspace.test.tsx src/management-navigation.test.tsx --maxWorkers 2 --minWorkers 2` 通过 16 项。类型检查及 10,742 个双语词条检查通过。源码、命令与日志保存在 `.cache/team-plan-filter/` 和 `.cache/team-plan-filter-web/`。

## 实际联调

通过 `go build -o …/suma.exe ./cmd/suma` 构建终端。使用 `go test -overlay … ./internal/platform -run '^TestTeamPlanFocusActual$' -count=1 -timeout=120s -v` 运行隔离 SQLite 控制面。用现有领域接口建立两个团队与三个计划，其中一个通过独立测试后待人工验收，一个因任务来源版本变化而阻塞。执行证据来自隔离 Runner 协议夹具。

- 编译后的 CLI 实际执行 3 次，读取准确团队首尾两页并拒绝将原游标用于另一团队。
- Chrome 在同一个控制面上验证英文浅色 1366px 和中文深色 390px。覆盖键盘筛选、焦点移交、准确计划详情、返回、刷新、清除、团队深链、空结果与 44px 触达。窄屏无横向溢出。

两端共发出 24 次本地 HTTP 读取，操作回执、运行和运行事件增量均为零。请求摘要、截图及清理回执位于 `.cache/team-plan-focus/`。本次实测数据库为 SQLite，OceanBase、PostgreSQL、物理 Runner 与真实工具执行沿原矩阵验收。12ui 包仓库访问受限，界面沿用速码现有组件与令牌。
