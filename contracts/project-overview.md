# 项目交付概览与处理队列

T-EXPERIENCE-131，1.0.0。所有新增读取复用当前项目授权事务；不引入写入、执行、模型调用或验收入口。

`GET /api/v1/overview?project_id` 延伸原快照，增加 `delivery`、`attention`、`milestones`。已有项目、数量、活动、运行和能力事实保持原语义。

## 交付事实

`delivery.scope=whole_project`；`total_tasks`为项目全部任务，`scope_tasks`排除cancelled，`accepted_tasks`只统计正式accepted。`running_tasks`、`waiting_tasks`、`reconciling_tasks`仅来自任务明确run_id所关联的同租户同项目运行，排除已验收／取消任务；waiting包括pending/ready/waiting，不能称为全部等待资源。`review_tasks`为in_review，不证明用例通过或当前用户可批准。`blocked_tasks`为未验收／取消任务中业务blocked或原关联运行failed。

这些是可重叠的不同事实，不可相加为进度饼图。没有测试计数、实际工时或完成日期预测；不能从运行成功推断人工验收。

## 处理队列

`attention`包含review、blocked、overdue、reconciling四个最多6项的当前预览，每页含items、total、next_cursor、as_of、date_basis。`GET /api/v1/overview/attention?project_id&kind&limit&cursor`继续读取，limit默认6、范围1..50。队列按updated_at DESC,id DESC排序；游标绑定租户、项目、类别和UTC自然日，重复／未知参数拒绝。跨日刷新第一页，不把可变任务列表承诺为不可变快照。

overdue仅为有due_date且早于当前UTC日期、未accepted／cancelled的任务。返回最小任务引用、负责人名称、真实关联运行状态和排期日期；不返回任意Data、原始日志、凭据或执行命令。计数覆盖全项目，预览有界，分页后对象可能发生变化，当前详情与新的读取仍需重核。

milestones只含有target_date且未completed／cancelled的里程碑，最多6项，按target_date、id排序，total为相同范围总量。不从计划日期推算完成状态。

前端默认显示项目待审而非“待我审核”，按需读取分页；项目或读取代次变化不得安装旧响应。权限失败清除私有页面。活动类型未知时使用中性图标；通用reviewed事件不等同接受。

## 验收

超过100个任务仍正确统计；同更新时间完整翻页无重复；跨项目／类别／日期游标拒绝；当前撤权拒绝；取消和验收任务排除逾期与活动运行；历史运行、其他租户运行、正文伪造字段不能影响统计。此合同不代表个人审批分配、逐例测试汇总、跨项目组合或预测已完成。
