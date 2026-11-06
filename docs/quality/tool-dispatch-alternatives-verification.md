# 派发替代工具验证

T-TOOL-DISPATCH-ALTERNATIVES · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157.0.8103.38。验证以 99961ea 为基线。此阶段复用既有派发选项接口和精确工具预检。

- 两组 Web 定向检查共 13 项通过，覆盖候选页焦点、重复打开、分页保留、授权丢失、不可用原因展开和原操作待恢复。完整 Web TypeScript 与 11058 对中英文词条检查通过。
- 390px 中文深色 Chrome 读取真实临时 SQLite。原档案在交接后被暂停，预检显示版本变化。用键盘打开候选区，确认焦点转到面板入口。展开可见原档案及不可用原因，明确采用另一个兼容档案后焦点进入实际工具选择栏。界面无横向溢出或页面错误。
- 浏览器完成 1 次精确 GET 与 1 次候选页 GET，0 次写入，未创建计划、运行或用量记录。第一次启动测试程序因 Windows 参数引用失败，改用带引号的完整参数后运行通过。

    vitest run src/TaskDispatchOptionsPanel.test.tsx
    vitest run src/TaskDispatchToolHandoff.test.tsx --maxWorkers 1 --minWorkers 1
    tsc --noEmit -p apps/web/tsconfig.json
    node apps/web/scripts/check-i18n.mjs
    go test -c -ldflags '-s -w' -overlay <阶段夹具> -o <阶段目录>/platform.test.exe ./internal/platform
    platform.test.exe '-test.run=^TestDispatchAlternativesBrowserActual$' '-test.timeout=95s' '-test.v'
    node scripts/check-docs.mjs

临时库存由测试夹具上报。真实工具执行、物理节点及供应方模型调用需分别验收。

[交互](../design/task-dispatch-options.md#从已带入工具继续) · [接口合同](../../contracts/task-dispatch-options.md)
