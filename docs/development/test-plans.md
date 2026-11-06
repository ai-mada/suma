# 自动化测试计划与逐例结果使用指南

适用：速码 1.0.0，T-QUALITY-115。负责人：质量与工作台；关联：任务验收、独立测试、问题回流。当前支持正式自动化用例计划；完整手工测试、应用／发布测试及缺陷全生命周期仍待后续实现。[English](../en/test-plans.md) · [接口合同](../../contracts/test-plans.md) · [证据架构](../architecture/quality-evidence.md)。本页配置是使用示例，不表示已在你的项目完成真实验收。

具体已验范围、环境和未完成验证见[阶段报告](../quality/test-plans-verification.md)与[摘要](../quality/test-plans-summary.json)。

## 定位测试计划

在「质量验收 → 测试记录」的测试计划工作台按草稿、已发布、已退役状态，或准确任务 ID 筛选。两个条件同时使用时取交集。计数来自当前服务端范围，筛选链接可分享，已有计划的详情和未保存内容仍由原编辑器与导航保护管理。

若创建时发现同一任务已有计划，使用冲突提示中的重新核对。工作台只读取该任务的计划，保留已输入的标题、说明和用例。核对后进入原计划，继续显式保存；仅核对不会写入新修订。

```sh
suma --project PROJECT test-plan list --state published --limit 20
suma --project PROJECT test-plan list --task TASK
suma --project PROJECT test-plan show PLAN
```

续页沿用原项目及筛选，把响应的 `next_cursor` 传给 `--before`。已发布描述计划生命周期，测试结果和验收继续分别核对。

## 先准备任务与真实测试工具

任务应有明确的验收条目和当前需求版本。每个任务只维护一个计划，可以不断保存新草稿、发布新修订；运行只使用绑定时的已发布修订，不会跟随草稿自动变化。

管理员为 Runner 配置隔离仓库、固定基线、许可测试程序及正确环境，再同步／探测工具 profile。报告能力需要 `command/text-v1`、`test_evidence:true`、`test_report:true`、`read_only:false`。这是独立工作树测试，不是平台共享只读命令；能力观察也不等于测试已经运行成功。

以下是本地 `CommandSpec` 片段；`executable` 由管理员选择已安装的真实程序，不是远端任务输入：

```json
{
  "adapter": "command",
  "protocol": "text-v1",
  "executable": "node",
  "args": [
    "--test",
    "--test-reporter=junit",
    "--test-reporter-destination={{test_report_path}}",
    "test/acceptance.test.mjs"
  ],
  "read_only": false,
  "test_evidence": true,
  "test_report": true
}
```

Node 提供 JUnit reporter 和目标文件参数，具体格式可能随版本变化；上例以 [Node.js 22.23.3 官方 reporter 文档](https://nodejs.org/download/release/v22.23.3/docs/api/test.html#test-reporters)为参数依据。固定实际工具版本，用代表性输出确认其落在平台支持的受限子集，不假设所有嵌套测试与厂商格式均兼容。pytest 等程序可同样在本地 Args 使用 `--junitxml={{test_report_path}}`，也需核对其固定版本的真实报告及 selector。

当前解析支持实际 Node.js 22.16.0 原件中的 `testsuites` 直接包含 `testcase`，以及重复于真实 failure 子节点 message 的 testcase `failure` 属性。重复消息必须解码后完全一致；不要仅填写该属性、删除失败子节点或增加自定义 status 来表达结果，这些格式会被拒绝。此格式支持不替代[阶段报告](../quality/test-plans-verification.md)中的真实运行结果。

Runner 替换字面占位符后直接传递单个参数，不执行 shell，也不需要填写 `>`、PowerShell 或 shell 环境变量表达式。还会提供 `SUMA_TEST_REPORT_PATH` 给直接读取环境变量的程序。禁止在本地 Env／EnvFrom 或远端工作流指定该变量或任意报告路径。

## 创建、保存与发布

在当前项目的测试计划入口选择正式任务，填写计划标题，再添加用例。为每例提供稳定 key、标题、前置条件、步骤、预期结果、优先级、覆盖条目和报告 selector。`covers:[1]` 指任务的第一个验收条目。selector 的 class/name 必须与实际 XML 的 `classname/name` 精确一致，不能用界面显示标题代替。

草稿可以没有用例，或暂留空步骤、预期结果、selector.name；所有字段和数组仍显式保存。发布前补齐这些内容，至少一个用例，并覆盖当前任务所有验收条目。界面按当前 allowed_actions 展示保存、发布和停用；发布／停用需要当前浏览器批准权限，项目 API／Agent／Runner不能替代批准。

API 创建示例：先读取真实任务及关联需求，用它们的当前版本替换示例数值；不存在关联需求时 `expected_requirement_version` 必须是 `0`。

```json
{
  "project_id": "project_id",
  "task_id": "task_id",
  "expected_task_version": 3,
  "expected_requirement_version": 0,
  "title": "任务自动化验收",
  "description": "固定候选代码的独立测试",
  "cases": [{
    "key": "AC-1",
    "title": "满足第一条任务验收条件",
    "description": "",
    "preconditions": "测试工作树使用当前固定候选",
    "steps": ["运行项目已有的验收测试"],
    "expected_result": "实际断言满足任务第一条验收条件",
    "priority": "high",
    "covers": [1],
    "selector": {"class_name": "", "name": "acceptance case one"}
  }],
  "idempotency_key": "create-plan-001"
}
```

提交至 `POST /api/v1/test-plans`。该示例只有在真实任务存在第一条验收条件且实际报告 selector 对应时才适用。保存已有计划使用 PUT 并带当前 `expected_version`；发布使用 `/test-plans/{id}/commands` 的 `action:publish`、当前版本、明确 reason 和新的稳定操作标识。

任务或需求依据变化时，保留编辑内容，读取当前依据并重新保存草稿、审阅和发布。绑定运行或 Review 引起的任务自动增版不会单独令计划失效；需求版本是独立精确依据。新草稿不影响既有已发布修订，也不会改变已经绑定的运行。

## 启动独立测试与读取结果

沿原任务／流程入口预演并启动开发运行。开发节点与 test 节点之间建立真实依赖，测试节点设置 `candidate_from` 指向本运行中的代码来源，并固定报告工具的 ID／摘要；详见[候选交接](../scheduler/candidate-handoff.md)。每个测试使用新的独立工作树。当前有已发布计划的运行不支持 map、子流程或外部工作流。

计划须在任务绑定运行前发布。绑定时明确保存“无已发布计划”的事实，后续发布不会给旧运行补签；停用也不能使旧运行绕过质量门禁。需要新计划证据时沿原流程创建并绑定新的运行。

结果页逐项显示实际用例状态、代码提交、节点、attempt、报告引用及关联问题。普通结果不载入原 XML，打开报告时才请求原文；原文按文本查看，保持原语言。中英文切换只改变界面与受控诊断，不改测试内容或原报告。

| 状态／现象 | 处理 |
| --- | --- |
| passed | 已有满足当前逐例规则的自动化证据；继续原人工验收流程 |
| failed / error | 查对应报告与断言，可明确创建关联问题 |
| skipped / not_run | 补齐实际执行；不能手工点为通过 |
| ambiguous | 同一 selector 对同一代码 head 有多份匹配，检查重复测试与计划映射 |
| unknown / reconciling | 先沿运行诊断核对实际进程及原操作；不要重复派发猜测结果 |
| source_changed / retired / run_changed | 核对当前任务、需求、计划修订和绑定运行；旧原件仍可追溯 |
| 进程退出 0，但报告失败 | 退出事实与报告结论分别保存，查看真实失败用例 |
| 报告缺失／格式不支持／文件不安全 | 检查本地 profile、实际输出位置及有界 JUnit 规则；缺报告不能通过 |

报告最多 256 KiB／1000 个 testcase／16 层 XML；计划最多 100 例。不要通过截断 XML 或删除失败用例让报告符合限制，应调整测试分组或明确修正固定格式。有效非零退出报告仍保留；未知进程结果不能提供有效逐例证据。未映射的失败／错误也会阻止验收，避免只选择报告中的通过条目。

## 在共享执行池中组合源码与包测试

先登记并核对应用版本的源码范围，选用已经授权的共享池。为构建、JUnit 测试和包测试配置固定命令。构建和包测试须使用不同 Runner 上已批准的制品档案，包测试依赖构建并消费该次封存清单。保留至少一个带原始报告的 JUnit 节点。使用单独的交付任务承接这次运行，保留应用原源码任务的验收依据。

进入「质量验收 → 测试计划」，打开对应运行。用例区显示 JUnit 覆盖与结果，包测试区显示独立门禁、对应尝试和已核验制品。点开制品或尝试可继续定位。JUnit 先通过时，仍需等待包测试证明，再到任务执行正式验收。参见[执行池制品合同](../../contracts/execution-pools.md#应用构建制品与独立包测试)。

## 问题回流、人工验收与恢复

对当前 failed/error 用例明确确认创建问题，填写原因；服务端重核计划版本、当前依据和该用例证据，同一计划／运行／用例重复提交返回同一问题。任务交付摘要合并显示此类问题与原 IssueIntake，继续沿原问题→需求入口回流。创建问题不意味着已自动修复，关闭问题也不改写失败报告；本阶段未实现完整缺陷关闭／重开流程。

对于确认失败的独立包测试，在对应包测试行创建问题并填写原因。系统将问题绑定到准确执行尝试和原封存制品，同一次失败的重复确认返回原问题。创建后可从测试行或任务交付打开问题。失联、结果未知或缺少当前依据时，先打开执行尝试排查。

正式 Review 仍要由有当前权限的人明确决定，并再次核对需求／任务版本、候选、测试及逐例门禁。当前 API 没有人工设置用例 passed 的入口。报告证明受信任 Runner 的执行记录，不替代代码审查、测试设计判断、供应链验证或生产验收。

保存／发布／创建问题请求超时后，保留原账户、项目、路径、完整正文、预期版本及操作标识，先恢复原操作，再读取当前计划与结果。不要换 key 或改正文重放来绕过冲突。403 先恢复合法权限，409 先核对当前依据；界面编辑内容应保留，原回执与当前可用动作分别显示。

模型任务上下文仅带计划标题、修订、摘要和用例数量；不会自动加入完整用例或 XML，避免无必要的上下文开销。需要具体用例时，通过有当前项目权限的质量接口读取所需范围。

发布与退役推进私有治理世代，保存草稿不推进；每次绑定运行固定当时世代（包括无发布计划）。无计划旧运行经历发布再退役后仍失效，不能借退役恢复旧验收资格；未来新运行固定退役后的当前策略。
