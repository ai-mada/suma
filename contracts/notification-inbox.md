# 个人通知收件箱合同

状态：T-INBOX-138，初始产品1.0.0；负责人：协作领域／主集成。继续Multica M24／M25／M27及原项目／团队协作目标。完整平台与P0–P7范围保持不变。设计、实现和实测边界在本阶段交付时分别更新。

## 权威与真实来源

收件箱是当前人员的个人投影，不代替任务、调度、预算、测试、审批或验收。事件只能来自平台任务创建／负责人变化的正式事务：普通任务、需求拆解、设计采用、问题修复任务及已接受交接。创建任务或旧负责人不同于新负责人时，为新负责人的规范活跃人员资源生成 `task_assigned`；自分配也记录实际事件。没有规范账号绑定的人员、智能体、工具或非活跃资源不会被猜测为接收人。

内部投影核对已保存的准确任务ID、版本、负责人及规范人员绑定，并在同一源事务中写入接收记录与序号。源事件标识由任务ID、提交版本及新资源ID构成；同源／同接收人唯一约束、原事务回执及回滚共同防止重复或孤立通知。通知写入失败使该源事务失败，不能悄悄丢失。不得增加可任意创建系统通知的公开接口。

接收人以当时的真实用户ID固定。再次分配不转交旧通知；读取时可以明确告知原分配已变化。通知不保存任务标题、讨论正文或模型输出快照；标题／业务状态只在当前项目授权事务内从原任务读取。任务不存在时保留个人事件引用，显示来源不可用，不提供任务链接或历史正文。归档任务仍是正式来源；归档通知是另一件事。

## 读取合同

`GET /api/v1/notification-inbox?project_id=...` 仅允许浏览器或当前有权的用户API身份，拒绝Runner／Agent。身份与组织从认证凭据推导；不能传接收人覆盖当前用户。每页重新核验当前账号／组织／项目读取权限，计数与行及来源水合同一事务；响应no-store。

参数仅有project_id、filter、limit、cursor，未知／重复／空显式枚举拒绝。filter为inbox（默认，所有未归档）、unread（未读且未归档）、archived（已归档）；limit默认20、范围1–50。

`NotificationInboxPage`：project_id、user_id、limit、items、total、counts、next_cursor、as_of。counts为inbox／unread／archived三项完整当前范围计数，total是所选filter的匹配量。items是当前有界页，不是完整项目列表。

`NotificationInboxItem`：id、version、sequence（十进制字符串）、kind=task_assigned、task_id、source_task_version、source_resource_id、created_at、read、archived、read_at、archived_at、source_available、assignment_current、可选current_task。

`NotificationInboxTask`只包含id、version、title、status；source_available=false时不得返回current_task。assignment_current仅在当前来源可用且当前负责人仍为原资源时为true，不据此宣称执行或验收完成。

序号由租户／项目内持久化计数器在同事务分配，顺序固定，不随已读、归档或任务更新变化；数据库有界BIGINT，JSON使用字符串避免浏览器精度损失。顺序为sequence降序；闭合规范游标绑定当前人员、租户、项目、filter、limit及上一页最后序号。新事件需明确刷新，不把跨页读取说成冻结快照。游标最长4096字符。

## 私有生命周期与回执

`POST /api/v1/notification-inbox/{id}/commands`，正文 `NotificationInboxCommand {project_id,expected_version,action,idempotency_key}`，action为read／unread／archive／restore。闭合字段及正整数版本、原操作标识均由服务端验证。

命令使用当前项目读取权限，但只允许更改当前用户自己的通知；项目viewer也能管理自己的已读和归档，无需项目业务写权。不能更改接收人、来源、业务任务或他人通知。无权／不属于自己的ID不泄露其他接收人的内容。

read／unread只变已读状态，archive／restore只变归档状态；恢复不隐式标为已读。目标状态已满足时为受版本保护的无副作用命令；真实改变才递增通知版本。重放原操作恢复原回执，当前授权仍须重新验证。旧版本冲突不自动换版本重试。

`NotificationInboxCommandReceipt`：idempotency_key、notification_id、version、action、read、archived、read_at、archived_at、updated_at。回执仅确认个人状态，不缓存来源标题。个人已读／归档不写团队公开活动；使用已有事务操作回执保留审计依据。

## 界面与恢复

开发→收件箱，紧凑列表、三个过滤入口、完整当前数、明确刷新及更多。打开来源与标为已读分开，不在挂载、滚动或打开详情时自动写入。来源不可用时禁用跳转；没有通知时解释真实来源，不造演示提醒。

身份／组织／项目变化同步清理私有列表与目标；401／403／404范围拒绝清除整个范围。旧身份响应不能覆盖当前页面。读失败可重试；命令结果未知时保留原操作标识和预期版本供明确恢复，阻止导航丢弃未核对命令。中英同一词条、主题和键盘焦点；390px行内操作自然换行，不要求拖拽。

本阶段只实现任务分配来源。讨论提及、审核请求、告警、消息通道、邮件／IM可靠投递、偏好降噪与跨项目聚合属于完整后续计划；不把当前读投影称为全部通知或智能预警已完成。
