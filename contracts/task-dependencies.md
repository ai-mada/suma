# 业务任务依赖图与原子编辑合同

状态：当前 1.0.0 合同，T-INTEGRATION-111。责任域：控制面任务域；关联来源：[《完善速码平台架构》整合记录](../docs/integrations/referenced-conversation-integration.md)。本合同描述当前实现要求；实际验证结果由本阶段交付记录单独列出。

[中文操作说明](../docs/development/task-dependencies.md) · [English guide](../docs/en/task-dependencies.md)

## 领域与操作边界

`Task.depends_on` 保存同项目业务前置任务。图上箭头从前置任务指向后续任务。它表达交付顺序与验收前提，独立于固定流程版本中的执行 DAG。编辑业务依赖不修改已发布流程、启动工具、合并代码、执行测试或批准验收。调度仍由 Go 控制面依据当前权限、版本和真实前置验收决定。

修改采用任务版本 CAS、当前项目全图校验和单次事务。当前 1.0.0 不引入旧 Python/FastAPI/Alembic 服务、升级链、旧依赖字段或接口兼容层。

## 读取完整图

`GET /api/v1/task-dependencies?project_id=PROJECT`

只接受当前项目查询；不提供分页、客户端 `limit`、状态筛选或跨项目拼图。响应为 `TaskDependencyGraph`：

| 字段 | 含义 |
| --- | --- |
| `project_id` | 已授权的当前项目 |
| `total` | 当前项目任务总数 |
| `limit` | 固定为 1000 |
| `complete` | 本次是否返回完整任务图 |
| `tasks` | `TaskDependencyNode[]`；每项含 `task_id, version, title, status, depends_on` |

总数不超过 1000 时返回完整图，空项目返回 `complete:true, tasks:[]`。超过 1000 时返回真实总数、`complete:false, tasks:[]`。界面明确说明范围限制，返回普通任务列表；不得将部分数据绘制成完整依赖图或发布基于部分图的修改。

## 提交脏任务

`POST /api/v1/task-dependencies` 的唯一作用域来自类型化 JSON 请求体。浏览器沿用原始账户、组织上下文头及当前身份权限检查。

```json
{
  "project_id": "PROJECT",
  "changes": [
    {"task_id": "TASK_B", "expected_version": 3, "depends_on": ["TASK_A"]},
    {"task_id": "TASK_C", "expected_version": 8, "depends_on": []}
  ],
  "idempotency_key": "task-dependencies-operation-unique"
}
```

`changes` 必须包含 1～100 个实际修改的任务，每个 `task_id` 只出现一次。`expected_version` 为原始图中该任务版本，必须是 1～2147483647 的精确整数。达到版本上限的任务不能继续修改。`depends_on` 必须显式提供字符串数组，空数组表示清除全部前置任务；空标识、重复前置、自引用、跨项目或不存在的前置、循环均拒绝。依赖集合相同即为无修改，单纯调整数组顺序也拒绝，不产生空操作版本。

接口不额外创造任务 ID 格式或每个任务前置数量限制；沿用请求体大小、真实记录和项目范围校验。未知输入字段拒绝，客户端保留原请求顺序和完整请求体用于恢复。

服务端在当前授权和项目锁内：

1. 核对原操作标识与请求摘要，已完成的同一操作恢复原回执。
2. 读取当前项目全图；核对每个脏任务原版本及实际变更，替换本次前置关系后校验完整图。
3. 每个被修改任务准确提升一个版本，置为 `in_progress`，清除 `verified_run_id`；按现有任务域规则使已验收下游结果失效。原执行快照、日志、测试和验收记录保留。
4. 任务变更、下游失效、审计和操作回执原子提交；任何一项失败则全部回滚。业务依赖编辑沿用任务域活动记录，不伪造执行事件或引入第二份执行 outbox。

该编辑入口不额外禁止有活动执行的任务；修改后的当前任务版本与旧运行快照分离。旧运行回写与验收仍受原有版本、fence 和证据规则约束。界面不可把旧执行成功自动显示为新依赖版本已验收。

## 原始回执与刷新

成功只返回 HTTP 200 `TaskDependencyChangeResult`：

```json
{
  "project_id": "PROJECT",
  "operation_id": "task-dependencies-operation-unique",
  "changes": [
    {"task_id": "TASK_B", "previous_version": 3, "version": 4, "depends_on": ["TASK_A"]},
    {"task_id": "TASK_C", "previous_version": 8, "version": 9, "depends_on": []}
  ]
}
```

`operation_id` 为原 `idempotency_key`。回执是原提交的不可变确认，不包含或证明当前全图。重复同 key、同请求恢复原回执；同 key 不同请求沿用 `idempotency_conflict`。当前授权先于回放，不因拥有旧 key 恢复已失去权限的数据。

收到或恢复回执后必须独立 GET 当前图。仅当脏任务当前版本及依赖仍与回执一致时，界面才可自动采用新图并显示保存完成；若随后又有编辑，需要显式复核。刷新失败保留草稿和“原操作已确认”事实，不能把旧回执冒充当前状态。

网络中断或响应不确定时，锁定原请求字段、项目、账户和操作 key，只恢复该操作。禁止改 key 重发、自动用最新版本覆盖或自动启动任务。

## 冲突、诊断与交互

HTTP 400 `invalid_input` 的稳定诊断键：`task_dependencies.request`、`.query`、`.duplicate_task`、`.duplicate_dependency`、`.unchanged`、`.self`、`.reference`、`.cycle`。HTTP 409 `version_conflict` 的诊断键：`task_dependencies.changed`、`.version_exhausted`。现有权限、原始账户上下文和幂等错误沿用统一错误合同。所有产品诊断提供中文和英文；任务标题和用户内容不做自动翻译。

图和表格共享一份草稿，提供等价编辑入口；脏任务只在显式保存后生效。409 时保留草稿，读取最新图仅作比较。用户明确选择“采用最新版本并保留本地修改”时，只覆盖本地脏任务的关系，保留其他人的未改项；也可明确选择使用最新图并放弃草稿。离开、切换项目、打开任务详情沿用未保存修改保护；待恢复操作不能被新项目或新身份接管。

## 最小必要验证

重点验证真实 HTTP/SQLite 事务、每节点 CAS、并发项目图循环、部分失败全回滚、原 key 回执、过时回执刷新、权限和账户切换、中英诊断、图与表格等价入口。复用未变动的执行、合并、独立测试与验收证据。OceanBase/跨节点竞争、1000 节点可视化性能及生产权限验收必须单列实际环境证据，不由本地通过推断。
