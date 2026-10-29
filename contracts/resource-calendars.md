# 资源工作日历与估算分摊合同

状态：T-CALENDAR-134，当前1.0.0；负责人：计划领域／工作台。关联P3排期、资源和智能建议；全平台范围及58／34／92总账保持不变。

资源使用项目`/members`标识，不是组织用户ID，也不是Runner配额。每个租户／项目／资源只有一份当前日历；独立版本和原操作回执控制修改，不并存每周手工总量写入口。

## 接口与当前权限

- GET `/resource-calendars/{member_id}?project_id=…`返回`ResourceCalendar {project_id,member_id,version,schedule,updated_at,can_edit}`。未配置为version0、七个null及空例外；不推断40小时。`can_edit`只是当前界面提示，写入仍重新授权。
- PUT同一路径携带`{project_id,expected_version,idempotency_key,schedule}`，完整替换日历。`schedule.weekdays`准确七项，周一至周日；`exceptions`为明确数组，最多366项。
- 日期例外`{date,available_hours,note}`以唯一YYYY-MM-DD定位。每天工时为null或0–24的0.25倍数；null未知、0明确不可用。日期例外覆盖周工作制，明确null例外仍保留exception来源。备注可为空字符串，最多200个Unicode码点，保留用户原文。
- GET `/planning/capacity`仍是指定周的只读投影；原POST周总量写接口与`CapacityWriteRequest`删除，没有别名、回填或兼容分支。

租户来自当前凭证；读取、修改与原成功回执恢复都先校验当前项目及资源资格。真实人员须仍属于当前组织并有项目资格。当前授权、资源检查、版本CAS、变更记录和原回执在同一事务；同操作重放恢复原结果，不把回执改成当前头版本。冲突保留草稿；权限变化不能以旧成功回执绕过。

## 估算与未知

任务只有同时具有明确起止日期才可分摊，单日显式使用相同起止日期。仅开始／截止日期仍是合法计划期限，但不会被推断成单日全部工作。完整区间含任一未知日或可用小时总和为0时，正估算计入无法分摊诊断，不回退自然日均分。明确0估算保持0。

可计算时，周分摊=完整估算×本周重合可用小时／完整任务区间可用小时。周容量只在七天全部已知时返回总数；否则null。来源、容量与分摊均不代表实际消耗、在线状态、派发资格或CPU／GPU／模型额度。

`CapacityEntry`使用`calendar_version`和七项`days`；另列`unestimated_tasks`（本周完整区间缺估算）、`unscheduled_tasks`（该资源全项目缺少完整起止）及`distribution_unknown_tasks/hours`（本周相交但无法分摊的完整估算）。后两者不可跨周相加；界面不能据不完整范围显示确定剩余容量。

完整日历版本／内容进入正式智能排期来源摘要，包括所选周之外的例外。依据变化必须新预览，不自动采用建议或执行任务。已捕获计划基线、实际验收和执行配额沿原领域保存，不被修改日历重写。
