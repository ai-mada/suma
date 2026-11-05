# 共享执行池本机诊断验证

T-POOL-INSPECTION-232，速码 1.0.0，2026-11-05。 [诊断合同](../../contracts/runner-journal-inspection.md#资源池总览) · [值班指南](../agents/runner-journal-inspection.md) · [证据摘要](pool-state-inspection-summary.json)

使用启动时相同的 `--state` 调用 `suma-runner --inspect-pool-state`。总览只输出槽位编号、日志计数和领取检查点阶段。找到目标槽位后，用 `--inspect-state --state <原状态>/pool/slots/<编号>` 查看原运行、节点、尝试和 fence。

| 范围 | 验证结果 |
| --- | --- |
| Windows 已构建 CLI | 读取前一阶段真实控制台停止留下的原未知结果和原领取记录。两种语言的 JSON 一致，原槽位定位到 attempt 3、fence 7。诊断前后所有记录的摘要一致。 |
| Windows 部分证据 | 隔离副本同时含损坏领取文件与指向其他目录的 junction。命令返回 1，保留正常日志计数，将损坏记录标为 `invalid`，拒绝读取 junction。 |
| Runner | 3 项定向检查通过，覆盖槽位数值排序、领取阶段、原身份校验、超限、持锁读取和文件不变。目录符号链接子项因当前 Windows 权限跳过，已用真实 junction 补充验证。 |
| CLI | 5 项定向检查通过，含中英文输出、模式互斥、凭据隔离和已有单槽位读取。 |
| Linux amd64 | `suma-runner` 交叉构建通过。 |

Multica 的本机状态命令先核对原实例再报告存活。本阶段借鉴原实例绑定，速码总览的证据来自本机文件，各记录分别观察。控制面回执、机器活性和业务结论仍由对应权威确定。 [Multica 固定版本状态实现](https://github.com/multica-ai/multica/blob/f230f9f55bdfc4de68015473b07de329a5079f87/server/cmd/multica/cmd_daemon.go#L1482-L1558)

Linux 与 macOS 实机、多主机运维、真实供应商和完整平台验收仍待验证。 [完整实施计划](../delivery/implementation-plan.md)
