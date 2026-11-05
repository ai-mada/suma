# 共享执行池仓库候选交接验证

T-REPOSITORY-SOURCE-POOL-247 · 速码 1.0.0 · 2026-11-06。 [部署配置](../deployment/repository-integration.md#在共享执行池交接仓库候选) · [候选派发合同](../../contracts/workbuddy-task-dispatch.md#派发已批准仓库来源的候选和独立测试)

| 验证 | 实际结果 |
| --- | --- |
| 项目与 Pool HTTP | 两条实际 Git 链路通过。项目路线切换后，留存来源按当前批准的 Pool 路线派发，原 WorkBuddy 尝试仍在待交付。两个 Pool 身份在不同槽位完成封存和同提交 JUnit，期间没有再次查询外部提供方。 |
| 授权与回执 | 错误 Runner、槽位、授权、授权版本、执行代次以及当前授权版本漂移均拒绝返回包。错误摘要无法建立传递，缺少封存凭据无法完成节点。已有项目路线回归通过。 |
| 两个实际 Windows Runner 进程 | 新构建的 `suma-runner 1.0.0` 运行两个隔离配置。候选和独立测试分别产生持久派发记录，导入证明与封存包、提交相符，两份结果已确认，活动槽位为零。进程排空后终止，两个测试 PID 均确认退出。 |

本次实际进程日志为 `.cache/repository-pool-247/cli-process.log`，SHA-256 为 `c8aabe44b879fb3e0262b5af29966564841359dbf72503aaff18575ed3c89753`。候选提交为 `7ad34b630ca3ac18385def9adaa7038c8cf9b065`，包摘要为 `b3ca258966dcbe5afd46323b2362ae1f874cbd0d86a8004a8470a3f108bb0cad`。

验证使用同一 Windows 主机、SQLite、实际 Git 和回环控制面。WorkBuddy 状态由隔离夹具建立。物理跨主机、Linux、TLS、生产部署与信号退出验收仍需单独验证。进程区分参考 Multica 的前台 daemon 运行模式。 [Multica 前台 Daemon](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/CLI_AND_DAEMON.md#L112-L141)
