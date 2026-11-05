# Codex 候选交付验证

T-CODEX-DELIVERY-227，速码 1.0.0，2026-11-05。 [接入与使用](../integrations/codex-candidate-delivery.md) · [机器摘要](codex-candidate-delivery-summary.json)

## Windows 真实 CLI 旅程

一次 `TestCodexCandidateDeliveryActualInstalledCLI` 实跑通过，耗时 24.93 秒。程序为本机 `codex-cli 0.166.0-alpha.6`，SHA-256 为 `925c7d0665ffa49b0dcf748e034a805a04da984164ac92301432073b47789b99`。测试通过原 HTTP、SQLite、两个普通 Worker 和独立 Git 仓库运行，使用受控回环 Responses 服务和隔离复核账号。

| 阶段 | 已验证的事实 |
| --- | --- |
| 派发 | CLI 目录返回 `gpt-5.5` 和 `medium`，原派发计划冻结该选择和 `1` Token 提醒阈值 |
| 开发与进度 | 真实 CLI 执行文件修改，第一条 Responses 回复前已持久化会话与轮次进度，原尝试共 7 条工具事件 |
| 用量 | 观察到 88 Token，覆盖为 `partial`，提醒触发，商业模型调用 0，预留与定价均未产生 |
| 封存与独测 | 父进程封存候选 `af0a3bec70121020502c05014e5f7dc1908037ac`，第二个 Worker 导入并测试同一提交，JUnit 通过，两处管理员检出保持原基线 |
| 验收 | 原 `Service.Review` 在测试前拒绝验收，测试通过后以独立隔离账号调用，得到 `accepted` |

回环服务提供 2 条确定性模型响应。实际程序、工具改动、进度入库、Git、JUnit 和产品验收逻辑均在本机执行。派发、进度、Worker 回执和独测结果使用原 HTTP 入口，验收使用原服务权限入口。后续增加的探测预检、忽略规则和停止等待保护以定向回归分别验证。供应商推理能力、商业账单和客户签收仍需各自证据。原始回执保存在 `.cache/codex-delivery-227/actual/20261105T092847.228759900Z/receipt.local.json`。

## 定向回归

- 14 项不同的 Go 定向检查通过，覆盖可选缓存写入计数、已报告小计、候选封存的停止门禁、保护路径、Git 忽略规则、探测生命周期、取消等待上限、丢失进度 ACK 后重放，以及 Git 引用写入后状态不明时保留候选。最后一组按当前提交基线隔离并行的 WorkBuddy 源码。命令和退出码保存在 `.cache/codex-delivery-227/go/` 与 `.cache/t227-runner/go/`。
- Web 7 个相关测试文件共 33 项通过。最终全量类型检查通过，国际化检查覆盖 10019 对词条，生产硬编码为 0。测试覆盖准确工作流版本、待执行时的固定配置、回执关联、尝试切换及原派发恢复。
- Linux amd64 的最终 Runner 测试二进制交叉编译通过，Go 1.26.10、`CGO_ENABLED=0`。二进制摘要为 `544635dbff94593235357f3f52c0709d58cddfabc4f6b162c7563c11291f0d68`。当前主机缺少可运行的 Linux 环境。下列命令供满足部署条件的 Linux 节点验收，包含脱离进程组的后代、自然退出、强制停止和探测预检。

```sh
SUMA_RUNNER_LINUX_PID_NAMESPACE=1 go test ./internal/runner -run 'TestLinuxPIDNamespaceStopsDetachedDescendants|TestProbeLifecyclePersistsEveryExitAndTimeoutWithoutInventedStop' -count=1
```

## 代表性界面

使用生产运行事实组件和只读合成数据，检查中文 1366px 固定配置、英文 390px 键盘操作后显示匹配回执、英文 390px 抑制不匹配回执。三组断言均通过，无横向溢出。界面夹具构建一次，保留首次 DLL 启动失败和后续 CSP 警告的记录。追加内容安全策略后复用同一构建，三组断言通过，Electron 包装脚本仍因启动阶段一条安全警告返回 1。该验证覆盖组件与交互，完整应用导航另有 Web 集成测试。

Linux 实机、macOS 整树隔离、Cursor ACP 独立收尾路径、商业模型、物理多机与原 31 项全平台验收继续。缓存、二进制、截图和隔离数据库保留在忽略目录中。
