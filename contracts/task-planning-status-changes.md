# 批量调整任务计划状态

T-MULTICA-TASK-PLANNING-STATUS。任务列表和看板共用已加载任务选择。在批量栏打开“调整计划状态”，界面读取所选任务的当前版本和可编辑资格。选择目标后预览原状态、将变更及原样保留的任务。任一任务无法调整时，逐项显示原因，整批确认暂停。用户可返回调整选择或到原运行／验收流程处理。

支持的计划状态为 `draft`、`todo`、`in_progress`、`blocked`、`cancelled`。状态只描述任务计划。保存“进行中”不会启动工具，“已取消”不会取消执行。任务尚未产生运行，或者当前运行已确认失败或取消时，可继续核对调整资格。旧运行、子流程和补偿仍未结束或无法核实时，阻止调整。普通已结束的历史运行保留原记录。已完成、待验收、已验收、已归档、自定义状态及带正式验收依据的任务由原业务入口处理。原运行成功、仍在执行或等待、来源缺失或结果待核对时，显示相应原因。

`POST /api/v1/task-planning-status-changes/preview` 只读。正文 `{project_id,tasks:[{task_id,expected_version}]}`，响应 `{project_id,items:[{task_id,version,status,eligibility}]}`。资格为 `editable`、`workflow_status`、`active_run`、`review_required`、`unverified_run`。任务越界或版本不符时整批拒绝，避免返回其他项目的数据。

`POST /api/v1/task-planning-status-changes` 正文增加 `status` 和 `idempotency_key`。两个入口接受当前浏览器账号或该项目的用户 API 凭证，限定 1 至 16 个显式任务，不接收查询参数。提交在当前项目写权限及调度锁下重查每个版本与资格。只更新变更项的计划状态、任务版本和更新时间，并保存原任务活动及同事务回执。排期、依赖、负责人、执行、测试及人工验收按各自事实保留。一个记录不符合则整批零写入。

响应包含 `project_id`、`operation_id` 和有序 `changes`。每项给出原版本、新版本、原状态、目标状态及是否改变。中断后沿原正文和操作 ID 恢复回执，当前授权检查先于回执读取。既有任务草稿需按最新任务版本核对。
