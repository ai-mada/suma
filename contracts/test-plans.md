# 测试计划与逐用例证据合同

适用：suma 1.0.0，T-QUALITY-115。负责人：质量领域、Go 调度与 Runner；关联：任务交付、独立测试、正式 Review 和问题回流。本合同描述当前源码行为，不表示真实环境或产品全范围已经验收。实现入口为 `internal/platform/test_plan_*.go`、`internal/testreportcontract`、`internal/runner/test_report.go`；公开类型与路由由 `cmd/suma-contracts/test_plans.go` 生成。参见[架构](../docs/architecture/quality-evidence.md)、[中文操作](../docs/development/test-plans.md)和[English](../docs/en/test-plans.md)。

状态与实际检查范围统一见[阶段验收报告](../docs/quality/test-plans-verification.md)和[精简摘要](../docs/quality/test-plans-summary.json)；本合同不重复维护运行结果。

## 计划、修订与依据

一个租户／项目内，一个正式任务只对应一个 TestPlan 聚合；聚合拥有递增 `version`，创建、保存、发布或停用均保存新的不可变修订。`latest_revision` 与 `published_revision` 分开：保存新草稿不会自动替换已发布修订。发布与停用通过领域命令执行，不能编辑修订原件或直接改发布指针。

`TestPlanSource` 保存任务快照、`task_basis_sha256`、可选需求快照及来源摘要。当前性比较任务的业务依据摘要，而不是仅比较任务记录版本；绑定运行、Review 等引起的任务版本递增不会单独使计划失效。任务内容或验收依据发生变化则失效。关联需求单独使用完整固定快照和精确版本比较；需求版本变化仍需重新审阅并保存草稿。创建／保存请求继续提交当前 `expected_task_version` 和 `expected_requirement_version`，无关联需求时后者明确为 `0`。

`QualityTestCase.key` 是计划内唯一、稳定的业务标识；`selector={class_name,name}` 是与报告精确匹配的二元组，不按标题、文件名、模糊名称或数组位置推断。`covers` 是 **Task.AcceptanceCriteria 的从 1 开始的索引**，不是 Requirement.AcceptanceCriteria 的索引。

| 项目 | 草稿保存 | 发布要求 |
| --- | --- | --- |
| 用例 | 显式数组，0–100 条；正文可不完整 | 至少 1 条；标题、预期结果、selector.name 和每个执行步骤非空 |
| 步骤 | 显式数组，0–20 条，每条最多 1000 UTF-8 字节 | 至少 1 条 |
| 覆盖 | 显式数组，最多 100 个当前任务验收索引；正数、无重复、不可越界 | 所有任务验收条目均被覆盖；任务有条目时每例至少覆盖一个 |
| Selector | class/name 各最多 500 字节；name 可空；非空二元组不可重复 | name 必填；与实际报告精确一致 |
| 优先级 | `low/medium/high/urgent` | 同左 |

计划标题 1–200 字节，说明最多 4000；用例标题最多 200，说明／前置条件／预期结果各最多 4000。用例 key 使用 `[A-Za-z0-9_-]{1,64}`。`cases` 规范 JSON 最多 128 KiB；整个规范化请求 JSON 最多 256 KiB。所有未标为可选的成员必须显式提供；禁止 `null`、未知字段和省略嵌套 selector 字段。空字符串和空数组只在草稿规则允许处使用。

## 运行绑定与唯一执行路径

沿用平台原有 Go DAG、派发预演、任务绑定、Runner 和执行池，不新增测试调度器或 `POST executions`。任务绑定运行时，同一事务冻结当时的已发布计划修订；当时没有已发布计划也保存明确的空绑定。事后发布不能给旧运行补签，停用或改发新修订不能取消旧运行的质量门禁。需要用新修订取证时，应按原任务流程创建并绑定新运行。

有已发布计划时，绑定要求至少一个固定 JUnit `test` 节点，当前工具档案声明 `test_report_protocol=suma-junit-report-v1`。每个测试节点使用单一许可命令和固定 `tool_profile_id`／`tool_profile_sha256`，并通过当前 Runner 或执行池的派发校验。同一运行可额外包含以 `artifact_from` 指向构建节点的独立包测试。当前拒绝包含 `map`、`subworkflow` 或 `external_workflow` 的此类运行。

共享池的包测试绑定固定运行路由、构建与包测试各自的已批准档案身份、版本及命令摘要，以及两者不同的 Runner 身份。构建固定已登记应用版本的当前源码范围，包测试使用原构建封存清单。领取、传输和成功回执分别核对这些依据及现有租约和授权边界。直接绑定新运行如果会替换应用的原验收任务，创建事务整体回滚。参见[执行池制品合同](execution-pools.md#应用构建制品与独立包测试)和[本阶段验收](../docs/quality/pool-mixed-quality-verification.md)。

`TestPlanExecution.package_evidence` 始终返回数组，每个条目对应一个独立包测试门禁。状态独立区分待执行、已验证、未验证、失败、结果未知、跳过和取消。只有当前通过核对的证明提供制品、封存清单摘要和源码提交。JUnit 用例状态、计数和覆盖仍来自原报告。正式验收重核两类门禁，源码用例通过不足以批准仍缺包测试证明的任务。

开发→独立测试复用同一运行中的 `config.candidate_from` 和显式依赖。服务端固定成功代码节点的干净提交及来源 attempt／fence／摘要，测试在新的独立工作树读取该提交。跨机器对象传递继续沿[候选交接](../docs/scheduler/candidate-handoff.md)与[传输合同](candidate-transfer.md)；测试计划没有另建跨运行基线或跳过现有候选校验。

## Runner 报告合同

管理员在本地命令配置启用 `test_evidence:true`、`test_report:true`、`read_only:false`、`adapter:command`、`protocol:text-v1`。该能力不与智能规划、模型、原生工作区、代码合成或制品模式混用。工具 profile 对其完整原始配置计算摘要；远端节点不能指定报告路径或覆盖本地命令参数。

Runner 为当前 attempt 在源码目录旁创建全新的 `test-report/report.xml`，注入受控 `SUMA_TEST_REPORT_PATH`。本地 Args 可包含字面占位符 `{{test_report_path}}`，包括 `--junitxml={{test_report_path}}`；执行前只替换参数副本，直接启动程序，不经过 shell。未启用报告能力时禁止该占位符；管理员 Env／EnvFrom 不得提供保留环境变量。

进程树确认终止后才采集：校验目录与文件身份、普通单文件、无符号链接／硬链接替换、读取前后大小与时间及上限，拒绝旧文件复用。结果中 `TestReport={protocol,xml,sha256}` 保存原始 UTF-8 XML 和原始字节 SHA-256，不把解析后的数据重新序列化成“原件”。已配置秘密出现在原文或解码内容时拒绝上传，原文件留在本地；这不是通用敏感信息识别保证。

支持的是受限 JUnit 子集：根为 `testsuite/testsuites`，可嵌套 testsuite，含 testcase、failure/error/skipped 及有限属性／元数据；最多 256 KiB、1000 个 testcase、16 层 XML 深度。拒绝 DTD、命名空间、未知元素／属性、重复 selector、计数矛盾、相互冲突的状态及超限。完整规则由 `internal/testreportcontract` 负责，不声称兼容全部 JUnit 厂商。派生状态是 `passed/failed/error/skipped`，消息投影最多 3000 字节；原 XML 不截断。

支持实际 Node.js 22.16.0 JUnit 原件所用的 `testsuites` 直接包含 `testcase` 布局；根与嵌套 suite 共用全局 selector 去重及计数限制。Node 的 testcase `failure` 属性只允许重复真实 `<failure message="…">` 的解码后完全相同消息，失败子节点仍是状态依据。仅有属性、没有明确子节点 message、属性消息矛盾或同时伪造 `status` 等未知属性均拒绝，不能把属性漏读成 passed。

真实进程退出码与报告判断分开保存。已知非零退出仍可保留有效原报告；退出码 0 但报告含 failed/error，会产生报告失败诊断且保留真实退出码 0。报告缺失、非法或不安全也不会改写真实退出码或凭空产生 passed。停止未确认／执行结果未知不提供有效用例证明。`ToolEvidence` 的实际协议终态不能用报告判断伪造。

## API、权限与恢复

所有路由位于 `/api/v1`。租户来自当前身份，项目显式指定。GET 必须携带 `project_id`；写请求体必须含相同 `project_id`。浏览器和限定项目 API 可按当前 ProjectRead／ProjectWrite 读写；发布、停用只允许有当前项目批准权限的浏览器身份。Runner／Agent 不能通过此入口自批准。

| 方法与路径 | 请求／响应 |
| --- | --- |
| GET `/test-plans?project_id&limit&before&task_id&state` | `TestPlanList`，200 |
| POST `/test-plans` | `TestPlanCreateRequest` → `TestPlanView`，201 |
| GET `/test-plans/{id}?project_id` | `TestPlanView`，200 |
| PUT `/test-plans/{id}` | `TestPlanSaveRequest` → `TestPlanView`，200 |
| POST `/test-plans/{id}/commands` | `TestPlanCommandRequest{action:publish/retire,...}` → `TestPlanView`，200 |
| GET `/test-plans/{id}/executions?project_id&limit&before` | `TestPlanExecutionList`，200 |
| GET `/test-plans/{id}/executions/{run}?project_id` | `TestPlanExecution`，200 |
| GET `/test-plans/{id}/reports/{report}?project_id` | `TestPlanReport` JSON，含原 `xml` 和派生 `cases`，200 |
| POST `/test-plans/{id}/executions/{run}/defects` | `TestPlanDefectRequest` → 问题 `Record`，201 |
| POST `/test-plans/{id}/executions/{run}/package-defects` | `TestPlanPackageDefectRequest` → 问题 `Record`，201 |

两个列表默认 20、最多 100 条，以当前作用域 keyset 游标分页。计划列表可用 `task_id` 精确筛选，并按 `state=draft|published|retired` 筛选生命周期。两项取交集，先筛选再计数及分页。省略状态沿用原 plan ID 游标，状态筛选的 `next_cursor` 绑定租户、项目、状态和任务，保留原排序位置。执行列表游标仍为 run ID。写入带 `idempotency_key`（8–128 字节）及适用的预期版本；发布／停用／创建问题带 1–2000 字节 reason。请求超时先恢复原身份、原作用域、原正文和原 key 的操作；同 key 不同正文冲突，不通过换 key 绕过版本与权限。原操作回执保留当时结果，响应中的可用动作和依据当前性仍重新授权／计算。

## 逐例门禁与问题回流

服务端将结果绑定租户、项目、运行、节点、attempt、fence 和 Worker，并核对固定 profile、工作树、候选与原报告摘要；迟到／跨 attempt 的证据不能替代当前证据。派生读取再次核验当前任务运行、计划修订、任务依据、需求版本和补偿事实。用例必须在每个被选代码提交上精确匹配唯一当前报告；缺失为 `not_run`，重复匹配为 `ambiguous`，失联为 `unknown`。这些状态、skipped、未映射的失败／错误、非零进程退出及非法报告都不能通过验收。

执行状态为 `pending/passed/failed/incomplete/unknown/source_changed/retired/run_changed`。正式人工 Review 沿现有入口重核所有质量依据；API 没有人工填写用例 passed 的操作，测试通过也不等于人已批准交付。

创建问题要求当前失败／错误用例、有效当前证据、当前版本、明确 reason 和操作确认。受保护的 `(plan,run,case_key)` 关系保证重复操作返回同一问题，不依赖可编辑 `issues.data`。任务交付将此类问题与原 IssueIntake 问题合并呈现，并沿原问题→需求入口回流。此阶段没有完整缺陷关闭／重开工作流，也未交付全功能手工测试、应用测试或发布测试管理；这些仍属原平台范围。

包测试问题使用独立命令。请求必须携带 `project_id`、`expected_version`、`node_id`、`expected_attempt`、`expected_fence`、`reason` 与 `idempotency_key`。服务端重核当前项目写权限、计划与任务依据、准确测试尝试、封存制品来源和确定的进程失败。只有证据完整的当前失败可转成问题。失联、未执行、过期或仅有基础设施故障的尝试继续通过运行诊断处理。

受保护的 `(plan,run,node,attempt,fence)` 关系单独去重，并保存原制品证据。问题进入原任务交付的问题列表。包测试摘要返回当前尝试的 `fence`、有权创建时的 `can_create_issue` 及已关联的 `issue_id`。原失败记录、修复和后续人工验收分别保存。

执行摘要不返回原 XML，原件按需在报告接口读取；内部当前 attempt 证明查询仍会选择完整私有 `dag_attempts.result` JSON 后提取工作树证明，不能理解为数据库完全不读取 XML。报告只证明受信任 Runner 所报告的执行事实，不证明测试实现正确、被攻陷 Runner 可信、供应链安全或人工／生产验收已完成。

发布与退役推进私有治理世代，保存草稿不推进；每次绑定运行固定当时世代（包括无发布计划）。无计划旧运行经历发布再退役后仍失效，不能借退役恢复旧验收资格；未来新运行固定退役后的当前策略。
