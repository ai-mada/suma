# T-QUEUE-SCOPE 阶段验证

2026-11-06，Windows amd64，Go 1.26.10，Node 22.16.0，Vitest 3.0.5，Chrome 156.0.8078.80 与隔离 SQLite。设计见[执行路径和运行定位](../design/scheduling-workspace.md#按执行路径和运行定位)。

- `go test ./internal/dag ./internal/platform ./internal/cli ./cmd/suma-contracts -run '^(TestScheduling|TestPoolAdmission|TestProjectCommandClaimRemainsLocalWithPoolEnabled|TestRunQueue|TestRunWatchCommand|TestCLILanguageHelpAndExplicitPreference)' -count=1 -v` 通过 29 个顶层测试和 12 个子场景。覆盖过滤后总数、冻结路由、池暂停、精确运行、跨项目及游标隔离、参数错误、只读和 CLI 帮助。首轮发现共享分页助手的调用签名不一致，恢复原签名后复验通过。
- `go run ./cmd/suma-contracts --check` 验证 1,085 个模型、490 个 API 路径及 TypeScript 类型。
- 在 `apps/web` 执行 `node ./node_modules/vitest/vitest.mjs run src/scheduling-dom.test.tsx src/scheduling-scope.test.tsx` 的调度相关定向检查，21 项通过。行内焦点和无项目的无效链接调整后，另复验 2 项。最终 TypeScript 检查和 10,836 个语言键检查通过。
- `& .cache/queue-scope/run.ps1` 构建实际 `suma` CLI，使用隔离 SQLite、真实 HTTP 和 Web 工作台。CLI 以原游标连续读取同一共享池运行的两页。英文亮色桌面的首次读取只显示 53 项中的 50 项，池路径可直接查到后续页的两项。按运行与路径取交集后，浏览器打开准确节点，返回恢复筛选。中文暗色 390px 验证深链、限定续页、44px 主要操作、键盘清除及行内筛选后的焦点。重复条件的链接在发送队列请求前拒绝。
- 33 次实际 HTTP 请求均为 GET，其中队列读取 11 次。持久操作、尝试和事件增量均为 0。原共享池暂停和授予撤回继续显示为当前阻塞。代表截图和证据保存在本机忽略目录 `.cache/queue-scope`。

真实多机领取、跨数据库并发及生产部署按原交付矩阵独立验收。
