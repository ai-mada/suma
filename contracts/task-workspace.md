# 项目任务统一查询与有界分组合同

状态：T-TASKS-137，初始产品1.0.0；负责人：规划领域／主集成。关联M06、M07及原需求项目进度／分布式协作；实现为 `internal/platform/task_workspace_read.go`、`task_collection.go`，合同生成器为 `cmd/suma-contracts/`。验收见[专项报告](../docs/quality/task-collection-verification.md)。

## 正式数据与权限

仅投影已有任务，不创建另一套状态、队列或执行权。浏览器人员及当前有权的用户API身份可读；Runner／Agent身份在HTTP与服务层均拒绝。每页在同一当前授权事务内重核身份、组织、项目、凭证，再计算完整匹配总数与有界结果。HTTP响应 `Cache-Control: no-store`。页面失去401／403／404范围权限时清除列表、详情、编辑器、选择和私有查询；旧身份请求不能影响新身份。

## 唯一任务行接口

`GET /api/v1/task-workspace?project_id=...` 返回 `TaskWorkspacePage {project_id,limit,items,total,next_cursor,as_of}`。不存在第二个 `/task-collection/items` 别名。

| 参数 | 确定语义 |
| --- | --- |
| q | 标题、描述、ID的字面子串；保留首尾空格；最多200个Unicode字符，拒绝控制字符；SQL `%`、`_`、`!`均转义 |
| status／priority | 最多200个Unicode字符的精确业务值，无控制字符；拒绝纯空白；省略或空字符串不限范围；`all`、`archived`是字面值 |
| owner_id／requirement_id | 最多64字节的受限标识；未知或其他项目ID只得到空结果；`__unassigned__`没有特殊含义 |
| unassigned／priority_unset／status_unset | 显式true只匹配真实空字段；与对应非空精确值互斥；只接受true／false |
| sort | updated_desc（默认）、created_desc、title_asc、priority_desc；相同排序值以ID稳定打破平局 |
| limit／cursor | 默认100、最大100；游标绑定当前人员、组织、项目、全部标准化筛选、排序及页大小；必须规范编码，最大4096字符 |

未知参数、重复参数、无效组合和跨范围游标拒绝，不以忽略参数扩大查询。归档任务包含在默认完整范围中。优先级顺序为urgent、high、medium、low，其余随后。标题／字面匹配排序遵循数据库自身比较语义，不承诺跨库相同Unicode排序。

UTC更新时间和创建时间使用固定九位小数的精确SQL排序投影；源字段保持原样，不采用浮点、截断或全库回写。返回行及一条前瞻的非法UTC源值产生409 `task_timestamp_invalid`。表达式排序的真实大规模索引性能仍需单独验收；本阶段不代表其他旧读取入口的时间排序全部修复。

## 有界分组接口

`GET /api/v1/task-collection/summary` 使用上述相同基础筛选，新增 `group_by=status|owner|priority`（默认status）、`group_limit`（默认20、最大100）、`group_cursor`。它拒绝任务行的sort／limit／cursor，避免两类分页混用。

返回 `TaskCollectionSummary {project_id,as_of,total,group_by,groups:[{key,total}],groups_total,groups_limit,next_group_cursor}`。total覆盖整个匹配任务集；groups_total是匹配分组总数；groups仅当前有界分组页。真实空字段用空key返回；客户端分别映射至status_unset／unassigned／priority_unset，不发明业务状态。分组按数据库原生key升序，游标绑定身份、完整基础条件摘要、分组及页大小；空key也是有效边界。源key超过限制或包含控制字符时明确409 `task_group_key_invalid`，不静默隐藏。

COUNT、COUNT(DISTINCT)与GROUP BY均在当前授权事务完成，不把全项目任务搬到浏览器计数。更多分组明确点击加载；分组内任务分别从唯一任务行接口分页。单客户端最多三个并行读取，队列中的已取消请求不得发出。

## 当前窗口、并发与恢复

每次请求是当前事实，不是跨列／跨页冻结快照。筛选、排序、分组、身份、组织、项目或刷新变化取消旧读取。看板按最高已加载任务版本去重；同版本矛盾显示刷新提示。每批分组保留自己的读取时间，统计变化或行计数不一致标为需要刷新；不会伪造相同时点的进度。

503只影响对应列并可单独重试；明确权限拒绝清除完整私有范围并停止队列。光标失配要求重新加载首页，不自动循环重试。人员批量派发仅使用当前已加载且版本仍匹配的任务；打开详情可按正式ID独立读取，不能将详情行偷偷加入筛选结果。

## 浏览器偏好与业务门禁

命名视图只保存本浏览器、当前账号／组织／项目的筛选和展示偏好；含需求、空字段条件、四种排序与三类分组，不保存任务正文、凭证或审批。保存／载入／删除明确操作；本地存储损坏、不可用或跨标签变动可恢复。URL变更只投影白名单字段，不恢复旧record／panel。

任务表单前置依赖用独立未筛选项目查询，保留已选ID。已有任务讨论、依赖图、团队计划、唯一Go调度、模型预算、固定候选、独立测试、人工验收及问题回流继续各自正式合同；切视图或改筛选绝不触发执行或批准。

服务端共享视图、跨端同步、所有工具真实接入、OceanBase／PostgreSQL真实矩阵和规模目标仍在原计划；本专项不改变58限定实现／34待交付／92总项。

本机视图仅识别明确的 `format: "suma-task-views-literal-v1"`。无当前标识的数据原件保留并提示显式恢复，不推断资源字段的旧含义、不迁移或自动覆盖。
