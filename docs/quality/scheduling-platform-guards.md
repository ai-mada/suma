# 平台项目级门禁与事件尾序验证

状态：T-P4-005 委派实现与 SQLite 验证完成；完整仓库、交付包和 GUI 验收由主集成记录。负责人：Runtime／主集成。关联需求：完整 DAG、可靠派发、质量验收、发布及制品授权。合同：[调度内核](../../contracts/scheduling-kernel.md)。验收结果：[阶段验证摘要](scheduling-verification.md)、[机器可读摘要](scheduling-summary.json)。

## 实现与源码边界

平台原有 17 条有效早期 `dag_lock` 路径已迁移：实际变更使用共享 `dag.GuardProjectTx`；展示 GET 移除写锁。当前平台生产代码只在健康探测中读取 `dag_lock`，不再自行更新它。全局事件游标由 DAG 在事务尾部统一分配；此处静态核对仅作补充，行为证明来自记录实际 SQL、包含 rollback 事务的 SQLite 测试。

| 原路径 | 当前处理 |
|---|---|
| `currentArtifactAssignmentTx` | 可信租户／项目 guard，当前执行状态、attempt、fence、worker 和租约核对 |
| `Review` 的 accepted 分支 | 项目 guard 在回执和当前验收依据之前；保持版本、补偿、依赖及需求门禁 |
| `CreateReleaseCandidate` | 项目 guard 与当前构建／测试／人工验收依据 |
| `ReleaseCandidate`、`ReleaseCandidates` | 只读快照，无组织／项目写锁 |
| `SaveReleaseEnvironment` | 当前组织／凭证授权后取项目 guard |
| `ReleaseExecutionForAssignment` | 当前项目范围内实际执行元数据读取，保留门禁保护 |
| `ProviderArtifactAccess` | 管理员配置的提供方租户／项目范围；当前发布依据 |
| 提供方 `provider/artifacts` 路径 | 实际受保护的制品提供操作，使用项目 guard 和当前部署依据 |
| `ObserveReleaseDeployment` | 配置范围内项目 guard 后投影持久观测 |
| `ReconcileReleaseDeployment` | 浏览器当前授权、项目 guard、固定原操作核对 |
| `RollbackReleasePreview` | 项目 guard 与当前历史部署／候选依据 |
| `PreviewReleasePlan`、`CreateReleasePlan`、`ApproveReleasePlan` | 保留真实计划与环境代数、策略和审批门禁 |
| `ReleasePlan` | 只读快照，无组织／项目写锁 |
| `projectManagedReleaseFailure` | 内部已验证的租户／项目范围，项目 guard 后关联原操作问题 |

列表中的组合行合计 17 条原有效路径。另将同一发布模块的环境详情／列表／动作提示、部署详情改为一致的只读展示语义；执行及 observed reconciliation 管理事务的 before hook，在回执及当前计划／部署读取之前取得项目 guard。

只读页面在入口授权后，再在其只读事务内复核项目和成员资格。这样，在入口读完、事务尚未开始时发生的真实撤权仍返回拒绝访问；不能用空动作列表掩盖无权读取。实际动作处理器继续重新授权，展示动作提示不是授权凭据。

强门禁使用显式 current 版本的任务／运行证据、制品证明、候选、计划和部署依据读取。它们在已有组织／凭证与项目保护下检查可变事实；默认 helper 保持只读，防止展示路径意外取得写锁。不可变候选 payload、封存 manifest 和 blob 不机械加写锁，原证明和失效传播规则保持一致。

OceanBase／PostgreSQL 变更事务显式使用 Read Committed，关键可变行使用当前锁定读取，避免授权预读建立的旧 Repeatable Read 快照隐藏等待项目锁期间提交的事实。展示使用 Read Only Repeatable Read 快照。这是实现边界；本阶段没有实际 OceanBase 或 PostgreSQL 实例，未宣称对应数据库通过验证。

关键回归源码：[项目门禁与双连接竞争](../../internal/platform/scheduling_project_guard_integration_test.go)、[事件尾 SQL trace](../../internal/platform/scheduling_event_tail_test.go)。完整仓库、依赖和交付包绑定以阶段验证摘要为准。

## 实际验证

最终完整平台回归为 **254 个通过事件、0 失败、0 跳过**，包耗时 **123.635 秒**；事件数含父／子测试及 helper，不能解释为 254 条独立业务旅程。`go vet ./internal/platform` 退出 0。该结果是平台包范围，不代表独立业务旅程数量。

实际测试覆盖：

- 第二真实 SQLite 连接持有无关组织写事务时，候选详情和列表仍立即完成。
- 暂停真实入口授权读取，另一连接提交项目权限移除，再恢复读取；环境详情、列表和动作入口均拒绝访问。
- 另一连接持项目 guard、提交执行失联状态后，等待中的制品准入重新核对并返回冲突，没有创建新上传会话。
- 两个实际连接、两个正式创建并授权的组织／项目并发创建 20 次管理运行，持久事件游标连续，事件范围保持在对应组织／项目。
- SQL trace 验证实际候选读取、管理发布与受保护 Runner 领取：每个事件事务只在域工作完成后分配一次全局游标，之后仅读取游标和插入事件；提交事件数与游标增量相同。

验证包含真实撤权竞争及包含 rollback 的事件尾 trace。修复前的失败和最终通过记录均保留在本地 `.cache/e2e/scheduling-stage/` 与 `docs/quality/evidence/scheduling-platform-guards/`，不随源码提交；[阶段摘要](scheduling-verification.md)记录最终范围和复现方式。

## 限制与后续

本阶段是 SQLite、本机 HTTP／进程夹具和平台门禁验证；没有厂商模型调用、OceanBase／PostgreSQL 实例、跨租户性能、生产部署或商业账单验收。SQLite 自身仍串行写入，这些并发测试不能证明分布式数据库的行锁并行吞吐。

旧独立 `AttachRun`／`BindRunTx` 合同尚待后续收拢；它不属于本次 17 条早期全局锁迁移，不宣称所有旧平台 API 已统一当前授权／事务合同。管理式创建的项目保护属于本阶段 Core 的独立实现与验证范围。

Git 仅保留必要实现、设计与精简验证摘要。原始输出、快照和旧 T-P4-004 证据继续留在本地，不删除、不覆盖；不提交数据库、凭证或私有上下文。
