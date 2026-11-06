# 终端派发推荐验证

T-CLI-DISPATCH-RECOMMENDATIONS 为团队派发增加当前候选、任务推荐历史和单条推荐三种终端读取。候选查询要求浏览器登录视角，并固定任务、流程和 Agent 节点依据。历史读取使用当前项目角色与访问范围。 [操作说明](../operations/task-dispatch-options.md#在终端核对派发推荐) · [合同](../../contracts/dispatch-recommendations.md#终端只读核对)

## 定向检查

- Windows amd64、Go 1.26.10。运行 `go test -overlay … ./internal/cli -run '^TestDispatchRecommendationOptionsCLI' -count=1 -v`，4 组顶层测试和 19 个子用例通过。覆盖固定请求、硬预算、范围校验、空候选、异常响应和只读分类。新增用例复现了大小写别名覆盖任务 ID，修复后拒绝该响应。
- 运行 `go test -overlay … ./internal/cli -run '^TestDispatchRecommendationHistoryCLI' -count=1 -v`，3 组顶层测试和 12 个子用例通过。覆盖单条读取、分页原游标、项目和任务范围、重复项和异常响应。
- 测试参数、输入 SHA256 和日志分别保存在 `.cache/recommendation-cli/options/alias-green.json` 和 `.cache/recommendation-cli/history/green.json` 同目录。别名覆盖复现记录位于 `options/alias-red.json` 同目录。

## 实际联调

使用 Node 22.16.0 驱动隔离验证，在 `4778ee5` 上叠加本阶段源码，通过 `go build -overlay … -o …/suma.exe ./cmd/suma` 构建终端程序。通过 `go test -overlay … ./internal/platform -run '^TestDispatchRecommendationCLIActual$' -count=1 -timeout=60s -v` 启动隔离控制面，以真实回环 HTTP 调用 SQLite 数据。

9 次终端调用全部通过，共 5 次 GET 和 4 次只读 POST。确认候选查询按固定版本返回，显式硬预算为 0 时返回完整空集合，过期任务版本返回冲突，项目 API 凭证不能代替浏览器登录读取候选。历史按原游标续读，单条查询保留已保存方案与依据摘要，跨项目和撤销后的凭证均被拒绝。

每次调用前后核对 13 张业务表的完整指纹，结果一致。模型供应商调用次数为 0。构建输入摘要、结果和清理回执位于 `.cache/recommendation-cli/actual/evidence/`。

本轮验收范围为终端读取与当前授权。生成或采用推荐、真实工具运行和多节点调度沿现有验证矩阵分别验收。
