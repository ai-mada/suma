# 桌面与 Worker 停止回执验证

T-RUNNER-STOP-229，速码 1.0.0，2026-11-05。基线 `6306b4d4b31f108bb23b8ed54c37fa19fd65afda`。 [执行合同](../../contracts/desktop-managed-runner.md#停止回执) · [交互状态](../design/desktop-managed-runner.md) · [证据摘要](runner-stop-receipt-summary.json)

## 停止和恢复

- 主进程先保存停止请求，再用私有管道通知原 Go 子进程。只有 Worker 结束原工作、关闭 Journal，且同一节点与 PID 的回执通过校验并观察到正常退出，桌面才确认停止。父进程断开也触发收尾。清理不明、无回执、强停和证据写入失败保留待核对。
- 本机停止与业务结果分开。已持久化的原领取、定案结果或待上传产物由同一 Worker 恢复。尚无结果的已接纳执行、新发现的未知结果和未收尾的工作区写入或工具维护继续待核对。停机不发起新业务操作。
- 每次后台巡检绑定启动时的子进程。旧响应在重连后失效。界面保留等待、确认和待核对三种双语状态，任务结论在对应运行详情查看。

## 实际与定向证据

| 检查 | 结果 |
|---|---|
| Windows 构建后的 `suma-control`、Go Worker、Git 仓库和 Journal | 四条路径通过。正常停止有身份匹配回执，父进程 EOF 没有回执，控制面未回应时保留原领取，同一目录重启继续原请求。使用本地回环控制端与只允许版本／帮助的 CodeBuddy 测试夹具。 |
| Windows Go 定向检查 | Worker 7 项、两个 CLI 入口共 4 项通过。独立 Node 子进程只执行一次，Journal 重开后回调字节一致。确认写入失败时原结果仍可补传。包含最大启动帧后接收停止指令的回归。 |
| 桌面主进程 | 5 个文件共 45 个顶层检查，含子项共 55 项通过。覆盖实际 Windows Node 私有管道、分片回执、缺失和格式错误、身份不匹配、强停、持久化失败及旧巡检返回。 |
| Web 连接组件 | 9 项通过。覆盖停止等待、确认、待核对和现场切换中文。双语词条检查 10,073 对通过，生产硬编码 0。 |
| Linux amd64 | Runner 交叉构建通过。 |

实际子进程夹具首轮遗漏工具巡检接口，补充回环响应后将业务领取计数限定为准确路径，复用同一最终 Go 二进制通过。测试日志与构建回执保存在 `.cache/runner-stop-229/`。

## 来源与剩余范围

核对 Multica 的固定提交 `f230f9f55bdfc4de68015473b07de329a5079f87`。借鉴其按原 PID 分开确认进程退出和任务清理的设计，沿速码自己的私有管道、Journal 和授权实现。 [Multica 停止回执](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/server/cmd/multica/cmd_daemon_shutdown_result.go#L12-L44) · [固定版本许可证](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/LICENSE#L20-L88)

主动停止后真正缺少退出证据的连接仍需核对，界面目前保留原状态并阻止重连。恢复动作需要原执行身份和进程清理证据。共享池停止协议、跨系统实机、完整 Electron 退出与重启旅程和厂商账号执行仍待验。完整平台验收继续按原实施计划推进。 [完整实施计划](../delivery/implementation-plan.md)
