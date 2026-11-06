# 任务工具派发预检验证

T-TOOL-TASK-PREFLIGHT · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157.0.8103.38。控制面联调及合同生成以 417f762 的隔离源码和本阶段 Go 文件为输入。

- 控制面与生成器 12 项定向检查通过。普通分页与准确配置共用当前权限、工具观察、执行路径和流程构建。覆盖暂停、撤销、缺失、过期任务版本、项目凭据撤权、工作流和共享池路径不匹配、无执行写入。隔离生成与复核得到 1097 个模型 Schema 和 495 条 API 路径。
- CLI 2 项顶层定向检查通过。准确读取发送工具 ID，显式分页参数与该模式互斥，控制面原错误保留。
- Web 16 项定向检查、整站 TypeScript 和 11054 对中英文词条检查通过。覆盖精确版本、越界或替代档案拒绝、刷新当前任务、原范围重试、迟到响应及原预览写入恢复。
- 390px 中文深色 Chrome 使用正式派发组件、真实 HTTP 与临时 SQLite。选中缺少独立测试的流程时，准确 GET 返回流程不兼容，工具保持未选中且预览不可用。切换兼容流程后再次核对，原档案被准确选中。两次准确 GET，零写请求、零计划、零运行、零用量记录，代表界面无横向溢出或页面异常。
- 首次浏览器夹具选择了对话框的语言选择器，后续排查发现临时会话省略项目列表。使用准确流程控件和真实认证会话响应后复验通过。

验证命令与范围：

    go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts -run '^TestTaskDispatch(Options|TestOption)' -count=1 -timeout=100s -v
    go test ./internal/cli -run '^TestTaskDispatchOptionsCLI' -count=1 -v
    go run -ldflags '-s -w' ./cmd/suma-contracts --check
    vitest run src/TaskDispatchToolHandoff.test.tsx src/TaskDispatchOptionsPanel.test.tsx src/ToolTaskHandoffDialog.test.tsx src/tool-task-handoff-model.test.ts
    tsc --noEmit -p apps/web/tsconfig.json
    node apps/web/scripts/check-i18n.mjs
    go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestExactDispatchBrowserActual$' -count=1 -timeout=95s -v
    node scripts/check-docs.mjs

浏览器工具观察来自测试夹具的正式上报。真实供应方调用、物理多节点领取和生产验收分别保留待验。

[派发合同](../../contracts/task-dispatch-options.md#exact-tool-handoff) · [交互设计](../design/tool-task-handoff.md) · [命令行操作](../operations/task-dispatch-options.md#工具选择)
