# 服务端个人与项目共享任务视图

阶段：T-VIEWS-138；产品1.0.0；项目规划领域。对象只保存[任务查询条件](task-workspace.md)及展示偏好，不保存任务、运行、验收或授权副本。交互与恢复见[设计](../docs/design/shared-task-views.md)，实现和验收状态由阶段记录单独确认。

## 对象与权限

`SharedTaskView` 包含 `id, project_id, owner_id, name, visibility, version, state, preferences, created_at, updated_at, can_edit, can_delete`。owner_id由当前服务端身份决定；visibility只允许private／project；state为active／archived。名称1–80个Unicode字符，禁止控制字符及纯空白。更新保持原visibility，切换保存范围需要明确另存。

每次读写及回执重放都重新验证当前身份、组织、项目、凭证与对象资格。Runner／Agent身份拒绝。private仅作者可读写，仍需项目读取权，其他人员包括管理员不可读取；project视图对当前有项目读取权的人员可见，修改／归档需项目写入权且为作者或当前组织owner／admin，不包含只有项目reviewer资格的人员。共享视图不授予任务访问权。can_edit／can_delete由当前资格计算，不能由请求填写或从旧回执继承。

`SharedTaskViewPreferences` 只包含 `q,status,status_unset,owner_id,priority,requirement_id,iteration_id,iteration_unset,unassigned,priority_unset,sort,layout,group_by,columns`。筛选沿任务查询字面语义，不引入all或__unassigned__哨兵；空字段标志与对应非空精确值互斥。q／status／priority最多200个Unicode字符；资源引用最多64字节。省略条件为不限范围；sort默认updated_desc，layout默认list，group_by默认status。其枚举分别为四种正式排序、list／board、status／owner／priority／iteration。未知、重复及null字段拒绝，不能通过未识别数据默默扩大范围。

## HTTP与分页

路径以 `/api/v1` 为前缀；响应禁止缓存。正式生成DTO来自Go，不接受任意action。

| 方法与路径 | 输入 | 成功响应 |
| --- | --- | --- |
| GET /task-views | project_id；limit默认20、最大100；cursor | SharedTaskViewPage，200 |
| GET /task-views/{id} | project_id | SharedTaskView，200 |
| POST /task-views | project_id、name、visibility、preferences、idempotency_key | SharedTaskView，201 |
| PATCH /task-views/{id} | 同创建字段，加expected_version | SharedTaskView，200 |
| POST /task-views/{id}/archive | project_id、expected_version、idempotency_key | 已归档SharedTaskView，200 |

`SharedTaskViewPage {project_id,items,total,limit,next_cursor,as_of}` 仅列出active且为当前人员private或project可见的对象。ID顺序稳定，游标绑定人员、组织、项目与limit，最长4096字符；不能跨范围复用。每页是当前授权读取，不声称多页同一快照；客户端明确加载下一页或刷新。

GET单对象允许读取当前有资格的archived墓碑；列表不包含墓碑。归档不物理删除、也不提供自动复活或回收规则，迟到更新不能重新建立对象。

## 写入、并发与恢复

写入使用调用方稳定idempotency_key（8–128字节）；更新与归档必须携带所见正整数版本。一次明确操作固定完整正文和操作ID；网络中断后重试使用原正文和原ID。相同ID但不同内容产生冲突，不能当新命令执行。

授权、当前版本检查、对象变化、审计和操作回执由同一事务完成。冲突不覆盖当前对象；用户可以重读后审阅或另存。重放仍先核对当前权限及对象可见性；视图已归档时旧create／update回执不能复活它，应返回冲突要求读取当前对象。相同archive回执可返回原归档事实，但当前can_edit／can_delete均为false。

未经批准的来源不授予更高权限；客户端不选择owner_id、版本或审批事实。本领域没有模型调用、外发、运行派发或业务验收副作用。

## 初始化与交付边界

初始化遵循当前组件基线声明，发现未登记域schema先拒绝，不清空用户数据或引入历史兼容回填。已有浏览器本机视图保持独立，只有用户明确保存才成为服务器对象；不自动上传旧本机内容。

本阶段跨设备能力是多个已授权客户端读取同一服务端对象。完整离线许可、设备水位、CRDT、双向同步、外部工具接入、其他数据库及物理多机验收继续按唯一实施计划推进。

## T-MULTICA-ITERATION-156：迭代工作入口

保存并原样恢复 `iteration_id`／`iteration_unset` 和 `group_by=iteration`。名称与任务归属不存入偏好；应用时按当前项目资格重新读正式迭代目录及 `suma_task_plans`。已完成迭代保留；未知或已不可见的ID显示待核对并保留精确条件，不能当作未关联或不限。更改筛选与保存视图不改任务、排期、批准或执行。复用现有JSON偏好、CAS及原操作回执，不增表、迁移或自动回填。


## T-TASK-COLUMNS-160：列表显示列

columns使用完整可见数据列的准确顺序，title固定首项；省略保持默认展示且不改原操作摘要。严格输入、空值、顺序、URL和交互见[显示列合同](task-columns.md)。列不改变任务查询范围、任务数据或权限；个人／项目视图继续当前权限、CAS及原body／key恢复。

## T-MULTICA-SHARED-VIEW-LIVE 在线版本核对

`GET /api/v1/task-views/sync?project_id=P` 只接受明确的项目参数，按当前会话或用户项目 API 凭证校验读取权限，响应禁止缓存。仅返回 `{project_id,revision}`。版本标识绑定当前账号、组织、项目、视图管理资格及该账号可见视图的版本变化。含归档墓碑，其他账号的私有视图不参与。标识只用于比较相等，不能排序、重放或授权，也不作为任务快照。

客户端先观察版本标识，再读取正式目录。后续版本变化时刷新目录和当前选中项，分页位置不当作授权。较新条件、归档或权限变化都须用正式读取核实。同步查询不会修改视图、筛选、排期、任务或运行。写入继续沿原版本比较和原操作回执处理。
