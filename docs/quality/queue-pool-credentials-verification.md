# 共享执行池凭证阻塞验证

T-QUEUE-POOL-CREDENTIALS 于 2026-11-06 在 Windows、Node 22.16.0、Go 1.26.10、SQLite 和 Chrome 157.0.8103.38 完成定向验证。服务端基于 `6a13700` 隔离快照和本阶段代码。

- `go test -ldflags '-s -w' ./internal/platform -run '^(TestScheduling(Queue|Pool|ToolBlockers|ReadScope)|TestExecutionPoolCredential)' -count=1 -timeout=120s -v` 通过。新用例覆盖无登记、有效凭证仍待核对、过期、撤销、池暂停、固定授权变化和等待依赖的执行节点。审批节点和项目专用路由不受凭证阻塞影响。队列与节点读取得到相同阻塞，节点身份和凭证材料保持隔离。
- 在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run src/scheduling-dom.test.tsx src/scheduling-scope.test.tsx --maxWorkers=1 --minWorkers=1`，23 项通过。TypeScript 和 10,987 个语言键检查通过。用例覆盖两类提示、准确阻塞导航、原离页保护和读取失败恢复。
- `go test -ldflags '-s -w' -overlay <阶段验证夹具> ./internal/platform -run '^TestQueuePoolCredentialsActual$' -count=1 -timeout=120s -v` 通过。英文桌面键盘与中文 390px 暗色触屏从队列打开只读项目执行资源，返回后恢复全部范围或共享池加准确运行的筛选。两种路径共 50 次只读请求，零领取、零写入，SQLite 持久记录摘要保持一致。

截图与结构化回执保存在阶段证据目录。本次验证的是项目侧调度读取和恢复导航。真实物理节点在线、能力匹配、凭证换新后的跨机领取与生产调度分别验收。
