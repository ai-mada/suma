# 需求验收追踪读取合同

状态：当前 1.0.0，T-TRACE-113。实际验证见[限定报告](../docs/quality/requirement-traceability-verification.md)；完整平台仍在实施。

[中文操作](../docs/development/requirement-traceability.md) · [English](../docs/en/requirement-traceability.md) · [需求设计与采用](requirement-design.md) · [任务交付](task-delivery-chain.md)

## 作用域与来源

`GET /api/v1/requirements/{id}/traceability?project_id=PROJECT`

只接受一个非空 `project_id`，拒绝其他或重复查询参数、非法编码及超过8,192字节的原查询串；其他方法返回404。HTTP 与服务入口仅允许当前有 ProjectRead 权限的浏览器／项目 API 凭据；Runner／Agent 不可读取。返回 HTTP 200 `RequirementTraceability`，不授予编辑、发布、采用、派发或验收权限。

同一当前授权与项目协调事务读取正式需求、设计聚合、当前发布快照、受保护采用绑定、正式任务及其确切运行。沿用现有 authority／project 锁序；只读指不新增或改变业务、运行、审计成功回执和执行事件，不承诺所有 SQL 都是 SELECT。展开、刷新和折叠均不调用模型、不推进调度、不创建任务。

每条标准的身份是 **`requirement_id + requirement_version + index`**，`index` 从 1 开始。它只在该正式需求版本内有效；相同文字的两条标准仍是两个位置。排序、正文或其他正式需求版本变化后，旧 `Covers` 不得解释为新版本的映射。当前投影直接使用正式字符串验收与不可变设计快照，不新增双写或旧格式兼容层，也不宣称跨版本稳定 criterion ID。

## 固定响应

| 字段 | 语义 |
| --- | --- |
| `project_id, requirement_id, requirement_version, requirement_status` | 当前已授权正式需求及其版本、状态 |
| `design_version, published_revision, design_sha256` | 真实设计聚合版本、当前发布指针及摘要；无设计／发布时使用相应 0／空值，不伪造发布 |
| `eligibility` | `none / current / requirement_changed / retired / source_unavailable` |
| `criteria` | 当前正式需求的完整标准数组，每项为 `index, text, task_keys` |
| `tasks` | 仅在 `eligibility:current` 时返回当前发布的完整任务提案及其正式来源状态 |

`none` 表示没有当前可用发布；`requirement_changed` 表示发布固定的需求版本已变；`retired` 表示设计已退役；`source_unavailable` 表示正式来源已不可用。非 `current` 时仍显示当前标准，但每项 `task_keys:[]`，`tasks:[]`；设计元数据可用于解释失效，不能继续展示旧版本的覆盖关系。

`RequirementTraceabilityTask` 字段：

| 字段 | 语义 |
| --- | --- |
| `key, title, covers` | 当前发布提案的任务与其固定需求标准索引；稳定任务 key 不等于跨需求版本的标准身份 |
| `task_id, task_version, task_status` | 通过受保护绑定找到的当前正式任务；尚未采用为空 ID、版本 0、空状态 |
| `binding_state` | `not_adopted / current / excluded / retired / publication_changed / requirement_changed / task_changed / source_unavailable` |
| `run_basis_state` | `not_dispatched / current / changed / unknown`，沿用任务交付合同 |
| `formal_acceptance` | 当前绑定、精确运行与正式验收依据全部核验后的事实，不能作为 `can_review` 权限 |
| `run` | 该正式任务确切关联运行的 `TaskDeliveryRun`，未关联或无法确认时为 null |

`not_adopted` 不以普通 `task.requirement_id`、同名任务或可编辑 `Record.Data` 补齐。其他绑定状态分别表达移出、退役、发布替换、需求变化、任务偏离及来源不可用；只有 `current` 允许成立当前验收事实。受保护绑定存在但正式任务缺失时，使用 `source_unavailable`、空任务引用、`run_basis_state:unknown`，不能显示为从未采用。已找到的正式任务即使绑定失效，仍可保留其真实任务／运行引用用于追溯，不能把旧运行成功归入新发布的验收。

## 测试、验收和完整性

当前正式需求最多 100 项标准，发布提案最多 100 个任务；读取完整数组，不截取前一页冒作全部覆盖。每条 `task_keys` 仅来自当前发布的 `covers`，与 `tasks[].key` 一致，不读取客户端提供的覆盖证明。

`run` 复用[任务交付](task-delivery-chain.md)的确切运行版本、状态、节点及测试汇总。节点／测试计数覆盖该运行全部节点，`items` 各最多 20 项，`limit:20`、`complete` 明示下钻是否完整。`tests` 仅为当前运行 `type:test` 节点，**不是测试用例数、标准的自动测试覆盖率或跨子流程完整测试计划**。未开始节点 `attempt:0` 不能构造具体尝试引用。

`formal_acceptance:true` 要求绑定与运行依据当前、运行固定的设计上下文匹配，并通过既有正式证据及需求／依赖核验。任务执行成功、测试节点成功与人工验收分别记录；Review 保存验收前任务版本、验收推进任务版本，投影复用正式领域核验，不能简单比较两个版本号或信任任务状态文字。

界面可以展示每条标准映射的任务数与其中当前正式验收的任务数，不将一条任务通过推导成整条标准通过，不计算测试覆盖百分比。完整 TestPlan／TestCase、逐用例结果、应用版本集和发布范围追踪继续按原台账实施；本接口不新增这些事实。

## 失败与交互恢复

SQL、扫描、取消和上下文故障返回读取错误，不伪装成空设计、零覆盖或失效验收。只有已确认的业务来源缺失或依据失配才投影 `source_unavailable`／`unknown`／其他业务状态；未知不等于成功或未发生。

需求详情提供默认折叠“验收追踪”，展开读取一次，显式刷新先隐藏旧事实再读。折叠或切换账号／组织／项目／需求取消旧请求并隔离迟到响应；401／403 隐藏受保护引用，显式重试建立新读取范围。响应校验项目、需求、版本、完整标准／任务关系及安全引用后才开放下钻；需求版本与外层快照不同则标记过时并禁止事实动作，只有用户点击“刷新需求详情”才通知父详情更新。401／403与版本失配均不得自动刷新父页面形成重复读取。

此读取没有写回执；后续设计保存、发布、采用及 Review 仍用原合同的预期版本、原操作 key、当前权限及未知结果恢复。不通过刷新追踪替用户批准新依据，不改写历史设计、运行或验收证据。
