# 桌面停止核对验证

T-DESKTOP-STOP-RECOVERY-230，速码 1.0.0，2026-11-05。 [执行合同](../../contracts/desktop-managed-runner.md#停止回执) · [交互设计](../design/desktop-managed-runner.md) · [证据摘要](desktop-stop-recovery-summary.json)

用户停止时，桌面先保存原操作与进程号，Go Worker 清理原执行并关闭 Journal 后，把绑定原节点、进程和操作的证据同步写入私有目录。桌面重开后的状态核对继续检查当前会话与项目绑定。证据匹配且原 PID 确认不存在，才恢复手动连接入口。业务任务与验收仍以对应运行记录为准。

| 范围 | 验证结果 |
| --- | --- |
| Windows 进程级恢复 | 两个独立 Node 进程运行生产桌面控制器，前一个启动真实 `suma-control` 与 Worker，在其输出证据后、桌面收到退出通知前主动结束。加密记录仍为 `stopping`。第二个进程仅核对状态，匹配原节点和操作后保存 `stopped`，新启动 0 次，总登记 1 次。登录会话、控制端、密钥存储和 CodeBuddy 探测使用隔离夹具。 |
| Go 私有停止协议 | 3 项定向检查通过，停止协议含 11 个子场景。覆盖正常和最大长度启动帧、旧证据替换、重复操作、异常帧、清理未确认、保存冲突及 stdout 失败后保留证据。初始实现缺失时复现 7 个失败子场景，损坏旧证据误拦截新停止另做一次修复前后对照。 |
| 桌面控制器 | 5 组共 47 个顶层检查，含子项共 63 项通过。实际 Windows Node 子进程验证干净退出且丢失 stdout 时恢复，另覆盖仍运行的 PID、错操作、硬链接、强停和自动停止。 |
| Web | 9 项连接组件检查通过，双语检查 10,079 对词条。Chrome 390×844 使用合成只读桥接验证待核对、核对中和已确认，原生调用均为 `status`，无横向溢出和浏览器错误。 |
| Linux amd64 | `suma-control` 交叉构建通过。 |

上游固定版本按原 PID 区分退出与清理证据，本阶段复用速码的私有管道、加密记录和授权边界。 [Multica 停止回执](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/server/cmd/multica/cmd_daemon_shutdown_result.go#L12-L44) · [固定版本许可证](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/LICENSE#L20-L88)

父进程已观察到异常退出、强停或原证据缺失时，继续保留待核对。共享池停止、完整 Electron 重启、断电耐久性、Linux 与 macOS 实机和真实供应商运行仍待验证。 [完整实施计划](../delivery/implementation-plan.md)
