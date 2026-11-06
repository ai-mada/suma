# 派发候选目录搜索验证

T-TOOL-DISPATCH-SEARCH · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157.0.8103.38。

- 8 项最终平台检查和 2 项生成器检查通过。核对公开字段、原查询回显、大小写与空白规范化、搜索分页和跨操作者拒绝，以及精确工具模式互斥。旧的未绑定操作者游标需要从第一页重新查询。
- CLI 3 项顶层检查及精确工具子检查通过。搜索文本可配合原分页参数使用，无效输入与互斥参数在发送前拒绝。
- Web 10 项定向检查、完整类型检查和 11072 对中英文词条检查通过。覆盖显式搜索、原搜索翻页与重试、迟到响应隔离、无匹配、清除及保留已选配置。
- 390px 中文深色 Chrome 使用正式派发组件、真实 HTTP 和临时 SQLite。项目登记 52 个配置，目标不在首批 50 项。使用大写关键字按回车找到目标，浏览器核对准确配置及匹配总数。采用后查询不存在的名称并清除搜索，已选工具仍保留。4 次候选读取、0 次写请求、0 个计划、运行或用量记录，无横向溢出或页面异常。
- 浏览器联调基于 aa4f674 与本阶段控制面代码。生成合同基于 5bdfbca 与本阶段代码，1099 个模型 Schema、496 条 API 路径通过生成复核。

    go test -ldflags '-s -w' ./internal/platform -run '^TestTaskDispatchOptions' -count=1 -timeout=100s -v
    go test ./internal/cli -run '^TestTaskDispatchOptionsCLI' -count=1 -v
    go run -ldflags '-s -w' ./cmd/suma-contracts --check
    vitest run src/TaskDispatchOptionsPanel.test.tsx
    tsc --noEmit -p apps/web/tsconfig.json
    node apps/web/scripts/check-i18n.mjs
    go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestDispatchSearchBrowserActual$' -count=1 -timeout=95s -v
    node scripts/check-docs.mjs

工具库存由测试夹具上报。真实第三方工具执行、供应方调用和物理多节点仍需分别验收。

[交互](../design/task-dispatch-options.md#在候选目录内搜索) · [接口](../../contracts/task-dispatch-options.md#filtered-candidates)
