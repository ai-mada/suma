# 专家团队依赖编排架构与 AI SOP

状态：T-P4-029 限定本地已验证；版本：1.0.0；日期：2026-10-27。未据此声明真实厂商、跨物理主机或生产验收。
依据：[合同](../../contracts/expert-team-orchestration.md)、[PRD](../product/expert-team-orchestration.md)、[交互设计](../design/expert-team-orchestration.md)、[实际验证](../quality/expert-team-orchestration-verification.md)、[精简摘要](../quality/expert-team-orchestration-summary.json)。

## 现有领域的增强

继续使用 `ExpertTeamConfiguration`、`ExpertTeamPlan`、既有 item 及 `expert-team-plans` API。
任务依赖图属于业务交付层，原 Go DAG、出站 Runner／Pool、配额和运行恢复继续承担执行。
不复制交付计划聚合、HTTP 后台派发器或新工作流引擎。

`ExpertTeamPlan` 新增来源 `revision`、`graph_sha256`、`activation_generation`、当前 `allowed_actions` 及明确计数 `progress`。
item 新增任务身份／版本、`task_basis_sha256`、`source_revision`、实际依赖及其验收事实、状态、阻断理由，
以及实际 `dispatch_plan_id`／`run_id`。保留原 `Plan` 作为固定源码快照；未派发时快照 ID／run ID 为空。

`suma_expert_team_plan_revisions` 保存不可变来源历史；私有激活记录保存推进所需的凭据引用、代数与到期调度事实。
不将 token、序列化 Actor、凭据摘要或私有调度控制数据放进公开计划 JSON。

## 预览、启动与晚派发

1. 从当前已发布配置构造非持久 task dispatch 源快照，固定任务／需求／设计／知识、职责资源、工具、流程与路由。
2. 编译所选 1–16 任务的真实依赖：任务唯一、无环／自引用／跨项目；外部依赖已有当前正式验收，内部依赖不自动扩选。
3. 固定来源修订及图摘要，提交预览而不创建 run。不能预先创建一个 30 分钟有效 task preview 并将其当成未来执行授权。
4. 明确 start 激活该准确修订；当前就绪根项通过既有 managed-run 事务绑定，其余项保持无实际 run。
5. `Service.TickExpertTeamPlans(ctx, engine)` 持久、有界、按到期顺序推进，限制每轮计划和新 run 数，单个计划不能占满队列。
6. 就绪时在事务外准备固定工作流候选，在 current-authority before hook 中重读完整来源并比较，再同事务插入 task plan、绑定 task/run/item。
7. `CreateManagedRunsBound` 在 before hook 前冻结工作流；不得在 hook 中改变工作流、嵌套 Engine 事务或后台依次调用公开 Create／Start。

推进器与 DAG 租约／ready tick 分开运行，并受维护模式控制。SQL 事务内不执行网络、模型或进程工作。
无变化检查只更新私有推进时间，不制造新的业务版本或来源历史；真正变化使用预期版本／代数和稳定运行标识提交。
多个推进器争用同一计划时，以事务与 CAS 防止重复派发。

## 验收门禁与事实投影

所选前置必须针对本计划实际创建的准确 run，通过现有 `Review` 与当前任务／需求／设计／测试证据校验。
`BindRunTx` 保持原绑定约束。run succeeded、测试通过、exit 0 或 Agent 总结都不形成正式验收。
曾经针对另一个 run 的 accepted 不能被当前后继借用。

任务正文／负责人、需求、设计和依赖语义来源与合法执行／评审状态分开固定。
未开始项发生实质漂移即阻断，不能把正常从排队到待评审的状态迁移误判为用户改了计划来源。
已有运行及收到的正式事实独立投影；后来注销、模板更新或凭据过期不能抹除历史，也不能赋予投影器派发或验收权。

item 状态表达等待依赖、就绪、已排队、执行中、待正式验收、已验收、阻止或停止。
未知、失败和取消保持原 run／attempt 的真实状态及恢复入口，不由编排器创建替代运行。
业务依赖通过不意味着自动继承前置 Git 候选；原候选传输、同提交测试和目标集成继续分别授权。

## 当前凭据与管理命令

start／resume 私下固定实际浏览器或范围 API 凭据引用及原主体。每次新派发从当前凭据行恢复这一原始引用，
保留原 kind／scope，并按数据库时间核对过期、撤销、账号、组织／项目成员、角色、路由／grant 和完整来源。
不能从 Principal 合成浏览器，不能借泛化 automation proof 放大权限，也不能新增公开 token/hash 恢复端点。
凭据失效阻止晚派发，直到当前有权主体明确 resume；resume 不替代来源修订。

`POST /expert-team-plans/{id}/commands` 复用当前权限、版本、理由、明确确认及稳定幂等键：

| 动作 | 效果边界 |
|---|---|
| pause | 停止新派发，已有运行继续并继续投影事实 |
| resume | 明确更新激活授权，重核所有待派发依据 |
| stop | 未开始项终止，不声称已有进程停止 |
| revise | 仅更新明确指定的未开始项，新增不可变来源修订与图摘要 |

revise 的 `items` 使用实际任务当前版本及职责；配置版本改变时必须显式覆盖全部未开始项，不能传入或替换已开始项。
当前可用动作由服务端 `allowed_actions` 返回。原操作重放仍检查当前权限；未知响应先恢复同一 key／body，不能另起效果。
历史分页及单修订读取清空写动作，不能靠读取旧修订恢复旧授权。

## UI 与 AI 开发 SOP

原 `TeamTaskDispatchDialog` 承担选择和明确启动，计划详情组件显示拓扑列表、状态计数、原任务／运行和当前命令。
沿用 `expert-team:` URL 定位与现有身份／组织／项目隔离，固定来源和历史默认折叠。
只对当前打开的活动计划做有界读取刷新；浏览器不会派发子任务或轮询全项目。

排障先核对计划／来源修订／激活代数，再看 item 的原 task plan、run/node/attempt、正式 review 和阻断理由。
待验收时打开准确任务和运行，由有权人员走原正式评审；不能在计划里手动勾选 accepted。
来源漂移先暂停／明确修订，凭据失效由当前主体 resume，运行未知回到原 run 核对。
必要检查已覆盖权限、图、事务、当前验收、漂移和推进公平性；准确 run 的 reconciling 节点投影为 blocked，保留原 running 状态与引用而不重派。
实际同机 Control／双 Runner 以固定本地 CLI 完成两任务依赖、暂停／正式验收、未开始项修订与明确恢复，R2 保留原根来源和 run。
两次运行、8 项独立正式断言、2 次界面明确验收及 owned 进程优雅退出 0 均已记录；无模型或厂商调用。
失败收据、最终配对制品与原主预览数据保持见上述质量记录，多主机／厂商／生产及完整平台范围仍单独推进。
