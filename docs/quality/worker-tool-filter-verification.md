# 执行节点工具配置筛选验证

T-WORKER-TOOL-FILTER 覆盖原工具列表的节点筛选、项目授权、续页及网页定位。 [交互设计](../design/tool-management.md#从节点查看工具配置) · [列表合同](../../contracts/tool-management.md#当前列表接口)

## 定向检查

- Windows amd64、Go 1.26.10。`go test ./internal/platform ./cmd/suma-contracts -run '^(TestToolProfileList|TestToolManagementContract)' -count=1 -v` 通过 4 项，覆盖完整 ID、大小写、状态与文本组合、项目授权、游标隔离及原无节点游标。`go run ./cmd/suma-contracts --check` 核对生成文件一致。
- `go test ./internal/cli -run '^TestToolProfileListCLI' -count=1 -v` 通过 3 组及 3 个子用例，覆盖精确参数、错误节点响应、游标透传及双语帮助。
- Node 22.16.0。定向运行 `ToolWorkerIdentityList`、`ToolWorkspace`、`ToolWorkspaceManagement`、`ToolWorkspace-experience`、`tool-management-list-model` 共 46 项通过，布局调整后相关 15 项复验通过。Web 类型检查及 10,703 个双语词条检查通过。共享和撤销节点、迟到响应与越界响应由组件测试覆盖。命令及输入摘要保留在 `.cache/worker-filter/` 和 `.cache/tool-worker-filter-web/`。

## 实际联调

在 `6c3dc26` 上用 `go build -o …/suma.exe ./cmd/suma` 编译终端，通过 `go test -overlay … ./internal/platform -run '^TestWorkerToolFilterActual$' -count=1 -timeout=120s -v` 启动隔离 SQLite 控制面。终端 5 次调用覆盖节点精确查找、原游标续页、跨节点游标拒绝、已撤销档案和大小写变体空结果。

Chrome 在同一控制面上运行实际 Web 应用。英文 1366px 浅色核对键盘入口、列表焦点、详情、刷新、清除与相似节点排除。中文 390px 深色验证节点深链、历史配置、44px 清除操作和无横向溢出。两条旅程通过，控制面共收到 35 次读取，操作回执数增量为零。浏览器网络限于回环地址。

请求摘要、截图和临时目录清理回执位于 `.cache/worker-tool-filter/`。本阶段实测存储为 SQLite。OceanBase、PostgreSQL 和真实共享节点执行沿原环境矩阵验收。12ui 包仓库访问受限，界面沿用现有速码组件与设计令牌。
