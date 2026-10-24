# 发布权限与恢复边界独立复核

- 状态：本轮定向审核完成；发现的读取撤权竞态已修复，聚焦回归通过。
- 日期：2026-10-24（Asia/Shanghai）。
- 负责人：独立审核代理 `release_audit_final`；业务修复由主集成代理完成。
- 关联需求：P5 发布权限、审批版本绑定、持久观测恢复、问题去重。
- 接口：环境列表/详情、环境可用操作、发布审批与执行、Provider 授权、managed 结果核对。
- 证据：[专属机器收据](evidence/release-authority-review.json)。本报告不替代全仓库测试或安全扫描。

## 复核范围

只读检查了 `CompleteManagedObservedBound`、通用记录写入的事务内授权、发布计划与审批、环境代数和屏障、Provider 配置与消费者约束，以及统一发布问题投影的锁顺序。新增永久测试仅位于 `internal/platform/release_authority_race_test.go` 和 `internal/dag/managed_test.go`。本审核代理没有修改业务实现。

## 已复现并修复的问题

环境详情和可用操作读取在事务外完成权限预检后，事务内用 `releaseActorAdminTx(...) == nil` 判断管理员，却同时吞掉了读取权限已被移除的错误。环境列表也存在通过可用操作辅助方法吞错后返回数据的路径。

回归通过真实邀请获得开发者身份，在实际 SQLite 授权读取处暂停；组织 owner 在另一连接通过正式接口移除项目授权后再继续读取。修复前三条路径均返回成功，测试要求 HTTP 403，得到真实 RED。主集成代理随后为详情和可用操作增加明确的事务内读取授权，将列表改为一次事务内授权、读取和操作提示计算。相同授权撤销窗口的永久回归现在全部通过。

历史 RED 的列表测试在当时第二次事务外预检处设置屏障；修复消除重复预检后，最终测试直接复用公共屏障辅助方法，在唯一事务外预检后移除授权。历史 RED 日志保留；未另行封存该历史测试源码，因此最终源码摘要不用于追认历史 RED 的源码版本。

## 本轮执行的聚焦验证

| 验证 | 实际结果 |
|---|---|
| 环境详情、可用操作、列表读取与项目授权移除竞争 | 三个子场景修复前 RED；修复后全通过 |
| 公开凭证接口省略 `scope` | 实际得到当前项目的 `runner` 范围，没有推断存在公开签发的无范围 Worker |
| 普通 Runner 与可信部署 Worker 读取业务接口 | 环境列表/详情、Provider 列表、发布计划、部署列表均返回 403 |
| 普通运行已有结果回执后调用 managed 恢复 | 明确拒绝；没有生成 managed 恢复记录，没有再次执行证明回调 |
| 当前 Worker 租约仍有效时调用 managed 恢复 | 明确拒绝；没有结果回执、恢复记录或后继派发 |
| managed unknown 结果后的恢复与重放 | 原结果独立保留，恢复事件和提交回调各一次，后继只领取一次 |
| 恢复成功回执之后权限回调拒绝 | 仍拒绝重放，不跳过当前权限检查 |
| 相同恢复操作标识更换内容 | 返回冲突，不重写证据 |

```text
go test ./internal/platform -run '^TestReleaseEnvironment(ReadsRecheckAuthorityAfterConcurrentGrantRemoval|WorkerCredentialsCannotReadBusinessRoutes)$' -count=1 -v
go test ./internal/dag -run '^TestManagedObservationResolution' -count=1 -v
```

实际退出均为 0，包耗时分别为 0.815 秒和 0.277 秒。环境为 Windows amd64、Go 1.24.5、真实 SQLite，缓存与临时目录在仓库 `.cache`，`GOPROXY=off`。精确日志和源码摘要见机器收据。

DAG 单元回归使用显式服务授权/证明回调边界和数据库租约故障注入。其 `ReleaseEvidence` 仅是边界测试引用，不是实际 Provider 健康证明，也不证明远端发布成功。实际 Provider 与浏览器旅程由其他独立证据记录。

## 只读核对结论

- `records.Create/Update` 在组织锁和事务内重新检查当前权限，再读取操作回执。现有 `TestReleaseDeploymentTaskBasisMutationRechecksLiveAuthority` 确实存在，测试暂停实际事务前权限读取，再通过正式接口降权并要求拒绝更新；该用例未由本代理另行执行。
- managed 恢复在读取结果回执前检查仅限服务管理运行；当前权限回调先于调度器锁及回执读取。恢复保留原 attempt 结果，不伪造 Worker 的停止回执。
- Provider 配置装载与实际核对消费者均限定固定 HTTP 字面回环地址。当前源码中已无先前 `https/localhost` 约束不一致。
- 审批及派发绑定计划摘要和版本、当前批准者/执行者权限、候选任务依据、环境策略、代数和屏障。消费授权与开始副作用会再次检查当前依据。历史事实恢复不会自动恢复已撤销的审批。
- managed 失败投影与 Provider 观测均遵循组织锁 → 调度器锁 → 发布数据顺序。两条路径重读同一部署的 `issue_id`，使用同一问题创建辅助方法。本代理未发现新的已确认锁顺序缺陷。

## 验证界限

这是 P5 选定边界的定向复核，没有运行全套测试、浏览器旅程、Provider 与投影器双连接竞争、OceanBase/PostgreSQL、跨机器或生产环境验证。统一问题投影的同时竞争由本轮主集成验证另行负责。当前结论不能用于声称全平台能力完成、性能指标达标、生产可用或完整安全审计通过。
