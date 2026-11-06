# 问题修复、回归绑定与人工关闭合同

状态：suma **1.0.0**／**T-QUALITY-119** 当前实现合同。负责人：质量领域。权威DTO：[issue_repair_types.go](../internal/platform/issue_repair_types.go)。关联：[架构](../docs/architecture/issue-repair.md)、[使用与AI SOP](../docs/development/issue-repair.md)、[测试计划](test-plans.md)、[任务交付](task-delivery-chain.md)。实际验证由专项交付记录提供，本文不预先写验收通过。

## HTTP与权限

| 方法及路径 | 服务方法 | 输入 | 返回／能力 |
| --- | --- | --- | --- |
| `GET /api/v1/issues/{id}/repair?project_id=…` | `GetIssueRepair` | 原问题ID、当前项目 | `IssueRepairView`；当前项目读取，不产生业务写入 |
| `POST /api/v1/issues/{id}/repair/prepare` | `PrepareIssueRepair` | `IssueRepairPrepareRequest` | 当前浏览器／项目API写；创建本轮修复任务 |
| `POST /api/v1/issues/{id}/repair/regression` | `BindIssueRegression` | `IssueRepairBindRequest` | 当前浏览器／项目API写；绑定当前发布回归计划 |
| `POST /api/v1/issues/{id}/repair/close` | `CloseIssueRepair` | `IssueRepairCloseRequest` | 当前浏览器且具备Review权限；重新核验关闭依据 |
| `POST /api/v1/issues/{id}/repair/reopen` | `ReopenIssueRepair` | `IssueRepairReopenRequest` | 当前浏览器且具备Review权限；保留历史并开启下一轮 |

POST的 `project_id` 来自请求正文，与原问题项目严格匹配；身份、组织和项目视图继续使用平台当前认证合同，不能从正文授予。Agent／Runner不是新的问题管理入口；项目API写权限也不等于Close／Reopen权限。

所有写入带稳定 `idempotency_key`；新操作按 `expected_*_version` 做CAS。先通过当前授权才能恢复原回执；原回执恢复优先于当前版本漂移判断。相同 key／同输入返回原结果，相同 key／不同输入冲突。原响应可能是历史成功快照，需要GET刷新当前视图，不能把重放视为第二次执行。

## 请求字段

字段按权威DTO的JSON名称列出。路径中的原问题ID不再在正文重复；调用方不填写tenant、cycle、修复ID、来源摘要、批准状态、关闭证明或任意执行命令。

| 请求 | 必备字段 |
| --- | --- |
| Prepare | `project_id, expected_issue_version, title, description, owner_id, acceptance_criteria, reason, idempotency_key` |
| Bind regression | `project_id, expected_issue_version, expected_repair_version, expected_plan_version, plan_id, case_keys, reason, idempotency_key` |
| Close | `project_id, expected_issue_version, expected_repair_version, run_id, reason, idempotency_key` |
| Reopen | `project_id, expected_issue_version, expected_repair_version, reason, idempotency_key` |

Prepare的 `acceptance_criteria` 必须非空，负责人经当前项目成员规则验证。它复用原唯一问题回流需求；已存在的需求不被新正文覆盖。准备只生成普通任务，不启动开发、不预留模型预算、不批准需求、不完成验收。

Bind的 `case_keys` 必须非空、唯一且属于该任务计划的当前已发布修订。`expected_plan_version` 是计划聚合CAS版本，不是发布修订号。服务端解析并固定发布修订与SHA，客户端不提供 `plan_revision/plan_sha256` 来替代当前来源。若原问题来自受保护失败用例，选择必须含原 `case_key`，且新发布定义的 `selector.class_name/name` 与原定义完全一致。

Close的 `run_id` 是当前修复任务实际绑定的运行，不是最初失败运行，也不是调用方任选的已成功运行。后端重新核验正式Review、当前受测代码及所有所选回归用例；请求中没有可以自行声明 `passed` 的字段。

每条所选用例证据必须匹配同一运行当前 test节点的attempt／fence／Worker、`candidate_from` 和 `dag_candidate_revisions` 原pin；pin继续匹配当前成功command／agent生产者、其原工作区摘要及head。生产与测试Worker必须不同，测试报告代码头必须等于pin。test-only、同Worker自测、无等价候选证明的 `test_from`／复杂来源拒绝关闭，以 `repair_evidence_stale` 表达，不自动降低门禁。

command生产者只接受正式 `command/text-v1` 档案绑定；领取和回写继续核对当前授权库存及固定指纹，不允许用agent协议伪装或取消绑定来通过。map／subworkflow等不具备等价来源证明的流程在当前readiness受阻。确定命令的真实OS／HTTP执行证据不代表已调用AI模型。

来源为独立包测试时，还需从当前修复运行获得新的已核验包测试证明。该证明固定在回归计划的同一次运行，使用原包测试的命令档案摘要，并核对不同于构建者的测试 Runner、封存清单和当前尝试。应用级包测试沿用原应用、代码范围及 Git 集成目标，可以针对同一范围的新源码版本复验。未登记应用范围的来源仅支持相同源码提交、原构建 Runner 和原构建命令档案下的复验。

## 响应及历史

`IssueRepairView`：

| 字段 | 含义 |
| --- | --- |
| `issue` | 原正式问题及当前版本、状态 |
| `cycle` | 当前修复轮次；重开增加，不重写旧轮次 |
| `current?` | 当前轮次保护修复对象；未准备时可缺省 |
| `task?` | 当前轮次关联的真实任务，不根据问题可编辑Data拼装 |
| `history, history_total, history_complete` | 最近最多10个非current旧轮次；total不含current，complete仅声明旧轮次覆盖；不是全历史分页接口 |
| `can_prepare, can_bind, can_close, can_reopen` | 当前授权及当前事实下的界面操作资格；实际命令再次核验 |
| `blockers` | `IssueRepairBlocker{code,ref_id?}`；稳定代码与可定位原事实 |
| `original_package?` | 保护记录中的原失败运行、节点、尝试、fence、制品、清单与源码；应用范围存在时附应用和 Git 目标标识 |
| `release_handoff?` | 当前有写权限、当前关闭轮次且证据仍有效时返回。固定问题与修复版本、轮次、已验收任务版本、运行、Review、制品、构建证明和包测试证明 |

`IssueRepair`：`id, tenant_id, project_id, issue_id, cycle, version, requirement_id, task_id, prepared_by, created_at, updated_at, regression?, closure?, reopened_at?, reopened_by?, reopen_reason?`。

`IssueRegressionBinding`：`plan_id, plan_revision, plan_sha256, case_keys, bound_by, bound_at`。

`IssueRepairClosure`：`run_id, task_version, review_id, plan_id, plan_revision, plan_sha256, case_keys, source_commits, closed_by, closed_at, reason, package_regression?`。包测试关单快照包含复验的节点、尝试、fence、制品、清单、源码与证明标识，运行取同一关闭快照的 `run_id`。这些字段由已复核事实生成，不接受调用方自行构造。

关闭状态为问题的 `closed`；修复过程使用 `open/in_progress/closed`，任务、回归和验收阶段从各自正式事实投影。关闭后的轮次仍为current并带closure；重开后保留旧 `closure`，记录原轮次重开人／时间／原因并移入history，新cycle在Prepare前没有current任务。原已关闭运行有保护索引，不可作为新轮次的关闭依据。

包测试关单可以交接到原发布候选表单。链接只携带原问题和轮次定位，表单重新读取当前问题修复视图。创建时随原候选正文携带 `repair_handoff={issue_id,expected_issue_version,expected_repair_version}`，交付范围必须是本轮唯一验收任务。候选创建事务重核关闭状态、人工 Review、准确制品和两份证明。候选保存原交接快照，问题重开后旧候选的后续正向发布资格失效。参见[发布控制合同](release-control.md#修复交接到发布候选)。

## 资格、阻断与失效

当前稳定blocker代码如下；`ref_id` 若存在只定位该项目有权读取的记录，不授予额外权限。

| 代码 | 操作含义 |
| --- | --- |
| `repair_not_prepared` | 尚无本轮任务；先明确准备 |
| `issue_closed` | 问题已关闭，不能继续修改本轮修复依据 |
| `regression_not_bound` | 尚未固定非空回归范围 |
| `repair_task_not_accepted` | 修复任务尚未取得当前正式验收 |
| `repair_run_changed` | 指定运行不再是当前修复任务运行 |
| `repair_evidence_stale` | 当前任务／需求／代码或测试依据已变 |
| `regression_plan_changed` | 当前发布计划不再匹配绑定修订和SHA |
| `regression_case_failed` | 所选用例未全部形成当前可接受的passed证据 |
| `original_case_required` | 缺少原失败key或原selector不一致 |
| `formal_review_missing` | 缺少相应当前正式Review |
| `package_regression_missing` | 当前回归缺少已核验的独立包测试证明 |
| `package_regression_scope_changed` | 已核验的包测试不匹配原命令档案或应用、源码范围和 Git 目标 |

未通过当前权限、版本、来源和证据校验不能写成功回执。数据库或读取错误不能伪装成无问题、无历史或已关闭。版本冲突与未知响应保留用户草稿和原操作key；需要重新读取原项目事实，不自动重试开发或改用另一运行。

## 不变量

1. 唯一问题回流需求沿用原IssueIntake保护回执，同一事务内建立本轮新任务及修复关系；不创建第二份回流需求或第二套任务系统。
2. 来源来自正式保护关系；`issues.data`、同标题任务、伪造原操作key和普通记录receipt均不是证明。
3. 回归绑定固定准确发布修订、SHA及非空用例；原测试来源的case和selector必须进入每轮回归。
4. Close按当前正式任务Review和独立测试复核；模型输出、开发检查、DAG绿色或单独退出码0都不能代替。
5. 关闭不修改原失败报告；Reopen保留历史并使下一轮通过Prepare建立新任务，不重新打开已验收任务作为可变代码容器。
6. 普通记录CRUD不能绕过修复关系及关闭门禁。所有写权限、幂等、CAS、活动和回执与当前领域事务共同执行。
