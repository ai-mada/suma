# 共享执行池关闭验证

T-POOL-SHUTDOWN-231，速码 1.0.0，2026-11-05。 [领域合同](../../contracts/execution-pools.md) · [架构](../architecture/execution-pools.md) · [停止与核对](../agents/execution-pool-runner.md#停止与核对) · [证据摘要](pool-shutdown-summary.json)

共享 Runner 收到宿主关闭信号后停止新领取，等待槽位和库存管理活动结束。原槽位锁内核对执行与本机状态，释放锁后汇总退出结论。已知结果的控制面回执可由同一身份重启后续传。未知原执行保持待核对，错误按槽位或库存范围输出。终端保留原状态目录并提示到原运行核对。

| 验证层级 | 证据 |
| --- | --- |
| Windows 双槽位原执行 | 通过两种宿主取消。两个槽位结果都明确时本机可关闭。受控 ACP 的进程已停止但协议结果未知时报告准确槽位。重开原槽位日志后，HTTP 503 与成功重试提交的原回调逐字节一致。各槽位、库存和池状态锁均可重新取得。 |
| Windows 已构建 CLI | 在两个独立隐藏控制台向真实 `suma-runner` 发送 `CTRL_BREAK_EVENT`。空闲路径退出码为 0。原槽位已有未完成执行、结果回执失败时退出码为 1，中文提示含原状态核对方式，JSON 标出槽位 0，原回调保留且未请求新任务。使用隔离回环控制端。 |
| 原日志检查 | 运行 `--inspect-state` 读取失败场景的原运行、节点、attempt 3 与 fence 7，检查前后的 Journal 摘要一致。 |
| Linux amd64 | `suma-runner` 交叉构建通过。 |

Runner 4 项与命令入口 5 项定向检查通过。双槽位回归在旧实现复现未区分退出结论。定向集合也暴露两个测试前提，隔离测试只在机器已报告对应项目能力后返回派发，维护探测断言使用独立工作和 HOME 目录。更正夹具后按受影响项通过。

参考 Multica 固定版本将原进程退出与清理结果分别确认。本阶段沿用速码的槽位 Journal、原机器凭证和既有诊断接口。 [Multica 固定版本](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/server/cmd/multica/cmd_daemon_shutdown_result.go#L12-L44) · [许可证](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/LICENSE#L20-L88)

本轮 ACP 使用受控协议夹具，控制面使用回环响应。实际 Linux／macOS 停止、跨主机服务托管、真实厂商执行和完整平台验收仍待验证。 [完整实施计划](../delivery/implementation-plan.md)
