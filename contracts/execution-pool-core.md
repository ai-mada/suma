# T-P4-006 Core 共享执行池合同

状态：接口冻结，Core 已实现并完成 SQLite 目标验证；平台集成、浏览器旅程及最终全套验收另行记录。基线 `be093ca`。总约束见 [execution-pools.md](execution-pools.md)。本文件定义跨代理 Go 接口及确定语义，不代表 OceanBase／PostgreSQL 或完整平台已经验收。

## 所有权与类型

Root保存 `suma_execution_pools` 元数据／状态、双边grant与独立机器凭证；Core保存池策略／资源／份额、当前grant的调度镜像、项目默认路由和不可变run路由。Core不导入platform。

```go
type PoolPolicyInput struct {
    ConcurrencyLimit int64
    ResourceLimits map[string]*int64
    BudgetLimit *int64
    TenantWeights map[string]int // omitted = 1, explicit 1..100
}
type PoolPolicy struct {
    PoolID string
    Version int
    PoolPolicyInput
    BudgetSpent int64
    Resources []ResourceSnapshot
    AsOf string
}
type PoolScope struct {
    PoolID, TenantID, ProjectID, GrantID string
    GrantVersion, ProjectWeight int // weight 1..100
    Status string // active | inactive; approved grant maps to active
}
type ProjectExecutionRouteInput struct { PoolID, GrantID string; GrantVersion int }
type ProjectExecutionRoute struct {
    TenantID, ProjectID string
    Version int // zero = no stored route
    ProjectExecutionRouteInput // empty PoolID explicitly selects local execution
}
type PoolCapabilityScope struct { TenantID, ProjectID string; Capabilities []string }
type PoolClaimRequest struct {
    PoolID, RunnerID, WorkerID, SlotID, OperationID string
    Scopes []PoolCapabilityScope
}
type PoolClaimAuthority struct {
    CredentialID string
    CredentialVersion, MaxSlots int
    PoolActive bool
    Scopes []PoolScope
}
```

JSON字段为snake_case。`Assignment`追加 `tenant_id/project_id/pool_id/pool_runner_id/pool_slot_id/pool_grant_id/pool_grant_version/pool_credential_id/pool_credential_version`，旧项目派发不强造pool字段。`Run`追加 `execution_pool_id/execution_route_version/execution_pool_grant_id/execution_pool_grant_version`；旧run无字段保持本地路由。池资源单位、safe integer、process/effect/cumulative分类沿用项目合同。

## 冻结入口

下列方法均在`internal/dag`，首参为context；Tx方法不提交事务：

```go
ReadPoolPolicyTx(ctx context.Context, tx *sql.Tx, poolID string) (PoolPolicy,error)
WritePoolPolicyTx(ctx context.Context, tx *sql.Tx, poolID string, expectedVersion int, in PoolPolicyInput) (PoolPolicy,error)
WritePoolScopeTx(ctx context.Context, tx *sql.Tx, scope PoolScope) error
ReadProjectExecutionRouteTx(ctx context.Context, tx *sql.Tx, tenant,project string) (ProjectExecutionRoute,error)
WriteProjectExecutionRouteTx(ctx context.Context, tx *sql.Tx, tenant,project string, expectedVersion int, in ProjectExecutionRouteInput) (ProjectExecutionRoute,error)
ClaimPoolWithOperation(ctx context.Context, request PoolClaimRequest,
    before func(*sql.Tx)(PoolClaimAuthority,error),
    eligible func(*sql.Tx,PoolScope,string,string)(bool,error)) (*Assignment,error)
GuardExecutionScopeTx(ctx context.Context, tx *sql.Tx, dialect,tenant,project,runID string) error
ReadProjectExecutionRouteTx(ctx context.Context, tx *sql.Tx, dialect,tenant,project string) (ProjectExecutionRoute,error)
ReadRunExecutionRouteTx(ctx context.Context, tx *sql.Tx, dialect,tenant,project,runID string) (ProjectExecutionRoute,error)
ValidatePoolAssignmentTx(ctx context.Context, tx *sql.Tx, dialect string, assignment Assignment) error
```

除末四个包级入口外，以上均为 `*Engine` 方法。`eligible`后两参数为runID、nodeID。Root在`before`中校验当前机器凭证、登记worker／slot上限、池状态和明确双边grant，并按稳定组织顺序取得本次需要的当前授权锁；不得在`eligible`中才取得新的组织锁。请求Scopes仅描述本地能力，不授予项目权限；未知、重复、跨授权集合或改变原请求的scope均拒绝。授权交集及资格检查与Core同事务。暂停池返回空派发但仍固定操作回执；失效凭证／grant不得恢复活跃执行。登记并发槽位为 1..128，slot 为规范十进制 0..N-1；请求最多 256 个逐项目能力范围。

包级 `ReadProjectExecutionRouteTx` 供没有 Engine 的服务只读取得默认路由，不新建服务、迁移、锁定或写入；未设置时返回 version0 的本地默认。`ReadRunExecutionRouteTx` 对不存在或跨范围运行返回 `ErrNotFound`，已知旧运行返回明确的空池路由。`ValidatePoolAssignmentTx` 只核对持久派发与完整机器／租户／项目／grant／credential／attempt／fence 身份，不取得新锁，不代替当前权限、运行状态或租约 CAS；原历史关闭回执仍可核对原绑定。旧项目 Runner 路径必须根据正式运行路由拒绝共享池运行，不能信任调用方是否提供 pool 字段。

Root同步grant时在当前组织授权后同事务调用`WritePoolScopeTx`，不允许池拥有方单方插入任意项目。非active scope不能产生新派发；排空的当前attempt仍允许经专用机器身份回写，不借撤销grant捏造停止。

## 路由和锁顺序

新run固定当前项目默认池、route version和grant身份。子流程与补偿继承父run，不跟随后续默认route；旧unpooled run不自动加入池。旧project claim不能取得pooled run，避免绕过共享份额。改变默认route只影响未来run。

所有写入口在取项目锁前只读定位不可变run路由（新建时定位项目默认route），先授权组织锁，再pool，再project，再family／领域，最后event-tail。定位后必须在项目锁下核对相同固定路由／version；变化则冲突重试，不能在已取project后补pool锁。`GuardExecutionScopeTx`不创建Engine、不迁移、不提交、不取event尾锁。涉及共享资源的完成、取消、核对、retry、Tick、子流程、补偿及受控提供方结果均遵守同顺序。仅文档／策略等明确不触碰pool资源的项目操作可以使用旧project guard。

## 公平、优先级与份额

池内先按租户权重、再按该租户中项目权重进行持久分层加权轮转。度量为**成功准入次数，quantum=1**，不是CPU、资源时长、token或金额。候选能力、当前授权、代码／工具资格、项目／pool容量不足时均不消耗成功份额；一次成功同时提交两层份额、项目及pool资源、attempt/fence、slot和claim回执。虚拟份额使用可检查的整数／定点及epoch，禁止浮点漂移或溢出；新建、重新活跃和权重变更不重置成零抢占全部历史份额。

项目内保留最少已服务运行优先的轮转。**优先级与等待排序只在被选运行内生效**：新增`Node.Priority`整数0..9，默认0；零值省略序列化以保持已有固定流程摘要及操作回执，非零值正式保存。ready排序键为固定数据库ready epoch减`Priority*60`秒，再以稳定sequence/nodeID破同分。新高优先级最多获得540秒的同运行排队优势，不能无限覆盖已等待低优先级。优先级不让一个运行绕过其它运行份额，也不改变租户／项目权重。

候选扫描最多检查 16 个范围、每范围返回 4 个匹配候选、合计 64 个，并持久化位置；nil响应也保存相同操作回执。请求指纹固定pool／runner／worker／slot及规范化逐scope能力集合，不采用客户端指定权重或项目权限。已有active回执先当前授权及原scope资格核验，closed／reconciling绝不重新执行；不得再次扣份额或预算。旧fence不能释放新占用。

同一范围内最多检查 16 个运行头；节点按 `(rank_at,ready_sequence,node_id)` 的持久覆盖索引排序并使用元组 keyset 续扫。离开 ready 同步删除派生键，迁移只为正式 ready 行补键，不更改历史时间、版本或事件。SQLite 实际 EXPLAIN 验证首屏无临时排序、续页按组合键直接定位。以上界限约束返回候选和运行头，不把 `database/sql` 返回行数当作数据库内部执行步数；大量能力不匹配仍可能让数据库过滤更多索引条目，OceanBase／PostgreSQL 执行计划需真实环境复核。

## 持久表与迁移方向

跨池路由变更按旧／新池标识排序取得池锁；一次领取涉及的项目锁按租户／项目字典序取得，不能使用会随准入变化的公平排名作为锁序。平台公开任务预览的 `execution_route` 是原审阅快照，旧缺失值不补造；实际授权仍由私有预览依据及正式运行路由核验。

Core新增`dag_execution_pool_policies`、`dag_execution_pool_scopes`、`dag_execution_pool_accounts`、`dag_execution_pool_ledger`、`dag_execution_pool_fairness`、`dag_execution_pool_dispatches`、`dag_execution_pool_slots`、`dag_project_execution_routes`、`dag_run_execution_routes`、`dag_ready_admission_order`及有界扫描状态。锁表、派生队列与索引不改变已冻结身份和事务语义。

迁移只加派生状态和显式路由；不改原run／attempt／事件／known consumed，不把历史local占用算成新池消费。route迁移默认unpooled。池并发资源先核验后CAS，正式ledger绑定pool、原grant、tenant/project、run/node/attempt/fence及process/effect类别。确认停止且效果未知只释放process；known terminal释放全部concurrent，累计预算与公平份额均不退款。

只读 `NodeAdmissionSnapshot` 及队列中的同类快照使用运行固定池，返回 `execution_pool_id/pool_policy_version/pool_resources`，同时检查当前 Core scope 代际和所有所需共享额度。`pool_scope_inactive`、`pool_scope_version_mismatch`、`pool_resource_capacity`、`pool_budget_exhausted` 等以 `source=pool` 区分；只展示所需资源的聚合额度，不展示其它租户／运行。平台另补池元数据暂停及当前双边权限；配置额度不代表真实硬件检测。

RED及基准至少包含：跨组织范围伪造、逐scope能力不同、稳定nil/active/closed/reconciling回执、1:3租户与1:2项目成功准入分布、多个连接最后槽位／budget竞争、无资格／无资源不扣份额、重新活跃不吃历史红利、权重变化、优先级与超过540秒的低优先级、不同run仍轮转、暂停排空、旧fence释放、unknown保留effect、路由切换与子流程／补偿继承、所有写路径pool-before-project及event-tail最后。SQLite实跑与OceanBase／PostgreSQL真实验收分别记录。
