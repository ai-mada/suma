# 固定工具待派发队列阶段验证

阶段 T-TOOL-QUEUE-FOCUS，速码 1.0.0。 [交互](../design/scheduling-workspace.md#从工具体检定位待派发节点) · [接口合同](../../contracts/scheduling-kernel.md#按固定工具配置查看待派发队列)

- Go 定向 HTTP／SQLite 核验工具 ID 大小写、同名命令隔离、跨项目、执行路径与运行组合、游标范围、暂停／撤销与固定指纹变化。空值、重复或非法筛选被拒绝。无配置写入或领取。
- CLI 验证 `run queue --tool` 保留准确 ID 和原游标，错误参数在本地拒绝。OpenAPI 生成只采用队列操作改动。TypeScript 响应类型无需变更。
- 实际 Chrome 157 连接隔离 Go HTTP 与 SQLite。英文浅色 1366px 通过键盘从原工具体检进入并用浏览器返回，中文深色 390px 通过触摸进入、清除工具条件并恢复同命令的另一节点。服务端在两种布局均返回准确的 1 个节点及暂停原因。查询使用项目查看者账号，产生 53 次只读请求、0 次写入、0 次领取，持久状态保持一致。
- Web 定向 35 项及中英文词条通过。按提交范围独立运行 TypeScript，结果为零诊断；独立检查以当前提交中的文件替代并行任务交付改动，不覆盖工作区。已有工具工作台用例输出 React `act` 提示。界面验证使用当前源码及夹具，没有调用供应方、模型或原生执行工具。

```powershell
go test ./internal/platform -run '^Test(SchedulingQueueExactFrozenToolProfileFilter|SchedulingQueueFrozenRouteAndExactRunFilters|SchedulingToolBlockersScopedRead)$' -count=1
go test ./internal/cli ./cmd/suma-contracts -run '^Test(RunQueue|SchedulingQueueFilterContract)' -count=1
node node_modules/vitest/vitest.mjs run --no-cache src/scheduling-dom.test.tsx src/ToolProfileDoctor.test.tsx src/ToolWorkspaceDoctor.test.tsx
node scripts/check-i18n.mjs
node scripts/check-docs.mjs
```

本阶段验证 SQLite。PostgreSQL、OceanBase 实库和真实多机的性能与排队竞争继续独立验收。
