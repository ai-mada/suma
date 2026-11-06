# 任务交付与质量链路合同

状态：当前 1.0.0；关联任务：T-DELIVERY-112。责任域：任务交付投影、正式验收与问题回流。

[中文操作](../docs/development/task-delivery-chain.md) · [English](../docs/en/task-delivery-chain.md) · [任务依赖编辑](task-dependencies.md) · [专家团队编排](expert-team-orchestration.md)

## 单任务读取

`GET /api/v1/tasks/{id}/delivery?project_id=PROJECT`

仅接受一个非空 `project_id` 查询，不支持自定义分页、`limit`、其他或重复查询参数。成功响应 HTTP 200 `TaskDelivery`。HTTP 入口允许当前授权浏览器或项目 API 凭据，拒绝 Runner／Agent 凭据；读取资格不授予派发、验收或诊断模型使用权限。

任务、确切关联运行、执行／测试节点计数、正式验收、专家计划和问题引用在同一当前授权与项目协调事务内读取。实现沿用 authority/project 协调锁，SQLite 等方言可能执行锁协调 SQL；“只读”指不创建或修改业务任务、运行、问题、需求、审计成功回执或执行事件，不承诺数据库只执行 SELECT。打开、刷新或折叠面板不推进团队控制器、不分配预算、不调用模型、不创建问题回流。

数据库／扫描／上下文错误返回读取错误，不能伪装成“未验收”或空记录。已确认的证据不匹配、前置失效或相关记录不存在，才按照现有证据规则显示相应业务事实。

### 团队计划来源与终端读取

`suma --project PROJECT task delivery TASK` 通过同一精确 GET 输出交付摘要 JSON。执行前检查任务 ID 为安全单段标识。当前项目和身份随原 CLI 会话或项目 API 凭据传递，服务端使用既有授权。

团队计划的当前项只有在正式派发计划与任务、运行匹配时才提供 `panel=task-delivery:<运行ID>`。普通 `panel=task-delivery` 继续读取当前交付。带运行的链接仅提供比较条件，不用于发起运行读取。只有当前授权的交付摘要与任务版本、运行均匹配时，原交付事实才在该面板显示。运行不匹配、未绑定或无法核对时，显式切换到普通交付以查看当前事实。原验收权限和版本检查继续独立生效。

## 固定响应与来源

| 字段 | 精确定义 |
| --- | --- |
| `project_id, task_id, task_version, task_status` | 当前已授权任务及其版本，不能由 URL 外的其他任务快照替代 |
| `run_basis_state` | `not_dispatched / current / changed / unknown`，见下表 |
| `formal_acceptance` | 同一确切任务／运行及当前需求、依赖和证据核对后的正式验收事实，不能用作 `can_review` 权限 |
| `run` | 当前任务 `run_id` 的 `TaskDeliveryRun`；未关联或无法确认对应运行时为 null |
| `expert_plans` | `TaskDeliveryExpertPlans`，仅 `scope:current_run` |
| `issues` | `TaskDeliveryIssues`，仅 `scope:current_run` |

| 运行依据 | 显示语义 |
| --- | --- |
| `not_dispatched` | 当前任务没有关联运行，不表示整个项目从未执行 |
| `current` | 已保存的运行任务依据与当前任务依据一致；不证明执行成功或业务验收通过 |
| `changed` | 当前任务依据已变，旧运行保留可追溯性，不能作为当前交付完成证明 |
| `unknown` | 原运行记录或依据无法确认；不推断成功或没有问题 |

`TaskDeliveryRun` 包含 `run_id, version, state, nodes, tests`。`nodes` 汇总确切运行中的全部节点；`tests` 只汇总该运行中 `type:test` 的节点，不汇总独立子流程的完整测试结论，也不是测试用例数量或覆盖率。

`run.workbuddy_delivery` 可选，含 `run_id, node_id, attempt`。服务端从当前任务绑定的运行按节点 ID 选取首个 WorkBuddy 交付待核对目标，要求节点为 `waiting`、原等待原因是 `cloud.delivery_required`，且当前尝试与持久派发身份和围栏一致。查找范围独立于通用节点列表的 20 项上限。任务依据变化时仍可追溯当前绑定的运行，任务改绑后不再返回原目标。目标只允许定位原尝试，候选、测试和验收继续各自核验。

`run.workbuddy_origin` 是当前已启动续接计划的历史来源，仅返回 `run_id, node_id, attempt, kind`。`kind` 为 `repository_source` 或 `local_candidate`。按当前项目与任务核对原派发、固定交接、云节点及原尝试凭据，任务改绑、原节点等待状态变化或旧账号失效不会抹去已经确认的历史来源。读到不一致的保护记录时返回核对错误。该字段只用于打开原尝试，当前运行的候选、独立测试和人工验收各有自己的证据。

`TaskDeliveryNodeSummary` 包含 `total, waiting, running, succeeded, failed, skipped, reconciling, other, limit, complete, items`。等待类含 pending、ready、人审／事件／定时／重试等待和 paused；running 含 running、cancelling；不认识的状态计入 `other`。各状态计数相加等于 `total`。

每个节点条目仅有 `id, name, type, state, attempt`。未开始节点 `attempt:0` 不是有效执行尝试；下钻此时定位运行和节点，已有尝试才带具体 attempt。响应不包含命令、环境、凭据、日志、输出、错误正文、完整节点 spec 或证据载荷；需要通过已有授权详情接口查看证据。

## 有界集合与当前运行范围

节点、测试、专家计划及问题的单个 `items` 列表固定最多 20 项，`limit:20`。`total` 覆盖该集合的声明范围，`complete` 仅在 `total<=20` 时为 true；不能用前 20 项代替全部记录。节点按 ID，计划按 ID，验收退回引用按创建时间与 ID 的稳定顺序读取。没有隐含的分页游标。

专家计划从正式 `suma_expert_team_plan_items` 的确切 run 关联读取，再核对当前任务与 run 绑定。只返回 `plan_id, version, revision, team_id, team_name, state, item_state`。未开始、未取得真实运行绑定的计划，以及任务的其他历史运行，不在 `current_run` 中；空列表不能表述为项目没有团队计划。

问题来源只允许 `execution_failure` 或 `review_rejected`：

- 执行失败需要同租户／项目确切运行的正式失败事件，以及 `failure-RUN` 原操作槽和请求摘要一致。
- 验收退回需要正式 Review 记录，以及 `review-rejected-REVIEW` 原操作槽和原 reviewer、task、run、reason 请求摘要一致。
- 原回执定位真实问题后，仍读取当前授权问题版本和状态。编辑 `issue.data.run_id/origin` 不能建立来源。
- 回流需求通过 `issue-intake-ISSUE` 的专用映射定位：私有 envelope 的 `kind` 固定为 `issue_requirement_intake`，绑定真实 issue 与 requirement ID，摘要同时绑定领域、租户、项目及两个记录。读取核对 shape、kind、摘要、当前授权记录和 `source_issue_id` 一致性；通用创建／更新不能预占领域保留操作前缀，旧通用 Record 回执也不能冒作该映射。只展示当前需求 ID／版本，不把可编辑元数据直接当作回流证据。

`TaskDeliveryIssue` 只返回问题 ID、标题、当前版本／状态、来源、来源 Review ID，以及已正式回流需求 ID／版本。未回流需求为空 ID、版本 0。问题描述、退回原因和诊断上下文不在此摘要中输出。

## 正式验收与问题回流

执行成功、测试节点成功和正式人工验收分别展示。验收须沿用现有 Review 命令及当前权限、任务版本、确切运行、独立测试和代码证据核验；面板不能用成功计数代替这些门禁。

明确提交 `decision:rejected` 时，在同一事务记录 Review、清除当前任务 `verified_run_id`、阻塞当前任务、按现有规则使已验收下游失效，并创建具有原 Review 来源的问题。原 Review 请求回放恢复原回执，不重复生成问题。退回质量结论不篡改执行状态，也不伪造 `run_failed` 事件。

需求回流仍是问题详情中的显式采用动作，使用原操作和当前权限；不会因打开交付面板自动创建需求、运行修复或关闭问题。团队已运行项的任务依据变化显示阻塞，不能把旧运行完成误报为当前可验收。

## 界面一致性

面板按需展开，单次读取所选任务。账户、组织、项目或任务切换重新挂载；旧请求取消并用序列隔离迟到结果。读取前隐藏旧事实；401／403 隐藏引用，显式重试重新建立读取范围。

响应必须通过项目／任务、当前版本字段、集合计数／上限、`current_run`、来源枚举和安全定位校验，再开放证据导航。任务版本与依赖图快照不一致时标记图过时；不能用交付读取自动覆盖未保存依赖草稿。摘要及哈希是服务端固定记录的标识，不是前端自授执行权限。

团队派发入口仅在完整依赖图、无未保存修改／待恢复写／冲突／过时读取时采用 1～16 个任务，携全部任务 ID 和原版本进入既有预览。模型选择绑定当前项目、任务、专家角色与工具 ID／版本／摘要；预览、启动及原请求恢复继续由现有合同校验，不从依赖图直接执行。

当前实现、协议夹具、真实执行和界面复验分别记录。该链路不扩大为历史运行总览、完整跨子流程测试覆盖、物理多机或生产验收声明。
