# 晚到 Runner 回执与人工用量确认验收

状态：T-USAGE-CONFIRMED-RECEIPTS-277 已通过本机定向验证。版本：1.0.0。范围：执行用量、额度账户与原回执。

## 处理规则

成功结算的人工确认保留原计数、费用、额度、决定人和依据。后续来自同一尝试的有效回执进入不可变结果事实，并沿原执行流程处理。重复回执按原内容摘要去重。人工确认早于首条有效结果时，补记首条结果摘要和账本修订，已确认金额与额度保持稳定。

控制面继续校验项目、节点、尝试、fence、执行者及证据。原账本锁、版本检查、结果事实和执行状态提交在同一事务。后续观测比人工确认更高时保留观测原值，供审计比对。差额复核与调整需要单独的授权流程。 [用量合同](../../contracts/usage-accounting.md) · [架构](../architecture/usage-accounting.md)

Multica 的任务用量将同一任务、供应方和模型的累计快照用于查询。速码继续区分 Runner 原始观察和人工结算。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/pkg/db/queries/task_usage.sql#L1-L23)

## 定向证据

- 修复前通过真实 Runner HTTP 在同一 fence 提交晚到停止回执，复现人工确认退回部分上报，且重新冻结 Token 与估算费用。
- 修复后通过真实 SQLite、HTTP 与 DAG 验证两条路径。已保存未知结果后进行人工确认，再接收停止回执，原账本保持一致。租约到期且尚无结果时确认用量，首条停止回执只补原结果标识。两条路径均验证组织与项目余额、完整结果原件和幂等重放。
- 同轮通过 11 个 Go 顶层用例及 7 个子场景。覆盖普通和共享池回执、权限、并发领取、账本回滚、溢出及费用估算。
- 实测范围为 Windows 本机与 SQLite。OceanBase、PostgreSQL、多节点与供应方账单仍待实测，差额调整入口待交付。

```powershell
go test -p 2 ./internal/platform ./internal/dag -run '^(TestUsageAccounting|TestCloudExpiredLeaseParksAndOriginalMachineMayRefine|TestRunnerResult.*Closed)' -count=1 -v
```
