# 执行到工具诊断验证

T-TOOL-DIAGNOSTIC-HANDOFF 于 2026-11-06 在 Windows、Node 22.16.0、Go 1.26.10、SQLite 和 Chrome 157.0.8103.38 完成定向验证。服务端使用 `f427362` 的隔离快照。

- 在 `apps/web` 分别运行 `node node_modules/vitest/vitest.mjs run src/ToolWorkspace-experience.test.tsx src/ToolWorkspace-recovery-navigation.test.tsx --maxWorkers=1 --minWorkers=1` 和 `node node_modules/vitest/vitest.mjs run src/ToolProfileImpact.test.tsx src/ToolProfileDoctor.test.tsx src/ToolProfileDecision.test.tsx --maxWorkers=1 --minWorkers=1`。共 28 项通过，覆盖准确配置、跨项目与错误配置响应、原待核对操作、影响尝试及未领取节点的导航。
- `node node_modules/typescript/bin/tsc --noEmit` 和 `node scripts/check-i18n.mjs` 通过，核对 10,970 个语言键。
- `go test -ldflags '-s -w' -overlay <阶段验证夹具> ./internal/platform -run '^TestToolDiagnosticHandoffActual$' -count=1 -timeout=120s -v` 通过。只读成员使用英文桌面键盘及中文 390px 暗色触屏，从测试节点第 1 次尝试进入准确暂停配置。两种界面都验证了诊断区焦点、首屏暂停原因和返回链接。返回仍打开第 1 次尝试，最新执行仍为第 2 次。浏览器共发起 86 次只读请求，SQLite 持久记录摘要一致。

截图和结构化回执保存在阶段证据目录。测试尝试由受控 Runner 夹具上报。真实厂商进程和多机执行仍按各自范围验收。
