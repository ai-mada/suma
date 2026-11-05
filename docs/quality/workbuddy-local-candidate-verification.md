# WorkBuddy 原尝试与本机候选交接验证

T-WORKBUDDY-LOCAL-CANDIDATE-239 · 速码 1.0.0 · 2026-11-05。 [终端候选合同](../../contracts/terminal-recovery-candidates.md) · [证据摘要](workbuddy-local-candidate-summary.json)

在同一任务的本机工作区准备改动并停止终端，候选预览会固定正式 WorkBuddy 原运行、节点、尝试、围栏和派发摘要。预览不改绑。明确启动后，新本机候选成为任务当前运行，原云记录仍保留等待交付状态。候选文件来自本机停止快照。

| 验证 | 结果 |
| --- | --- |
| 真实 Windows 本机交付 | 原 WorkBuddy 任务通过回环供应方和正式派发进入等待。Windows ConPTY 实际修改隔离工作树，原 Worker 停止后封存 Git 提交。另一个 Worker 从独立 Git 仓库测试同一 SHA 并提交 JUnit。提前验收被拒绝，测试账号在结果通过后完成验收。原云运行、尝试和围栏保持，供应方创建仍为一次。普通终端候选实际回归亦通过。 |
| 启动原子性 | HTTP／SQLite 回归覆盖来源标记或版本漂移、预览不改绑及同键重放。在候选插入后通过 SQLite 触发器改变原节点的等待标记，改绑前复核拒绝启动，原任务、原节点和新运行写入均回滚。 |
| Web 和窄屏 | 18 项相关 Web 测试通过。390×844 Chrome 使用真实 Suma HTTP 候选与停止会话响应，在受控接口上核对原运行链接、双语改绑说明和明确确认。无写入、横向溢出或脚本错误。浏览器只验证有效预览及目标 URL，过期预览提示由最终组件回归核对。 |
| 生成合同 | `TerminalCandidateSource` 及 `TerminalCandidateWorkBuddyOrigin` 从 Go 同源生成到 OpenAPI 与 TypeScript。生成合同、Web 类型和中英文文案检查通过，现有请求未新增字段。 |

参考 Multica 将人工发起的新运行与来源记录分开保存的做法。速码沿原本机终端、DAG、候选和人工 Review 实现。真实 WorkBuddy Git 文件交付、跨物理机器、其他操作系统和生产验收仍待独立验证。 [Multica 来源关联](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L6078-L6084)
