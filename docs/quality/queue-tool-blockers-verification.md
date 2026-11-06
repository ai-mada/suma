# 队列固定工具阻塞验证

T-QUEUE-TOOL-BLOCKERS 于 2026-11-06 在 Windows、Node 22.16.0、Go 1.26.10、SQLite 与 Chrome 157.0.8103.38 完成定向验证。服务端基于 `2c8b7e3` 隔离快照和本阶段代码。

- `go test -ldflags '-s -w' ./cmd/suma-contracts ./internal/platform ./internal/cli -run '^(TestScheduling(Contracts|Queue|ToolBinding|ToolBlockers)|TestRunQueue|TestRunWatchCommandSchedulingReads)' -count=1 -v` 通过。覆盖队列和节点同一诊断、固定摘要或命令变化、暂停、撤销、审批失效、跨项目档案、缺失、未绑定节点和原有游标。CLI 保留固定工具引用与阻塞代码。`go run -ldflags '-s -w' ./cmd/suma-contracts --check` 核对生成合同。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/scheduling-dom.test.tsx src/scheduling-scope.test.tsx --maxWorkers=1 --minWorkers=1`，22 项通过。TypeScript 与 10,977 个语言键检查通过。入口保留原离页保护，并在旧读、缺失或无法核对固定依据时隐藏。
- `go test -ldflags '-s -w' -overlay <阶段验证夹具> ./internal/platform -run '^TestQueueToolBlockersActual$' -count=1 -timeout=120s -v` 通过。只读项目成员使用英文桌面键盘及中文 390px 暗色触屏，看到暂停的固定工具阻塞与未绑定节点待核对。检查链接进入准确配置体检，再返回原待派发节点。两种界面共发起 64 次只读请求，运行尝试数为零，SQLite 持久记录摘要保持一致。

截图和结构化回执保存在阶段证据目录。验证使用受控本机服务。厂商登录、实际工具领取、跨机竞争和生产调度分别验收。
