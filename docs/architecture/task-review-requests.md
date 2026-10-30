# 审核责任与原质量权威

实现合同：[task-review-requests](../../contracts/task-review-requests.md)。这是独立的责任请求域，不修改既有任务、运行和Review基础表，不回填旧历史。

`suma_task_review_requests`保存固定任务版本、run ID／版本、任务内容摘要、规范人员／账号快照、说明、通知反向绑定及生命周期。`suma_task_review_pending`以tenant/project/task主键和request_id唯一约束持有每任务唯一pending槽。状态只有pending、cancelled、closed；closed引用真实`suma_reviews.id`，决定从同租户／项目／任务／源任务版本／run的正式行水合。来源损坏时不合成决定。

创建与取消沿当前用户／项目API身份、现有authority transaction、项目协调和commandReceipt。Runner及Agent不可调用。当前授权先于原回执恢复；原回执先于新资格或版本冲突判断。请求、pending槽、唯一通知、活动和原回执同事务提交。通知沿Inbox唯一插入与序号权威；失败回滚整笔事务与序号。

审核人通过现有人员规范绑定表和普通有界SQL解析。active human资源、JSON字符串user_id、规范resource/user关系、当前组织／项目角色必须同时有效。目录不是新账号或授权权威，不通过相反顺序的人身份写锁解析接收人。资源ID与账号ID含义不同，不能以任务owner、名称、标签或Agent身份猜测接收人。

原`Service.Review`仍是唯一质量决策路径。它完成原测试、代码、任务／需求／依赖等核验，在写入真实review_id后、改变任务并保存回执前，关闭准确匹配的pending请求并释放其槽。其他合法审批者保持原权限，实际决定人来自正式Review。旧Review回执恢复不再次执行关闭，后来新请求不会被旧决定消费。拒绝仍在原事务创建可信质量问题并使下游失效。

`binding_current`仅核对当前任务版本、task/run关系、任务摘要和run版本／成功状态；`reviewer_available`表示当前人员审核资格。两者都不授予Review权限，也不代表最终可接受。历史查询不会修复绑定、取消请求、创建业务事件或标记通知已读。当前字段与immutable姓名快照分开。

HTTP闭合参数、JSON字段及游标。目录和历史默认20／最大50，total是完整范围；历史以固定9位小数UTC的created_at DESC、id DESC排序。游标绑定当前身份、组织、项目、任务、筛选和limit。准确详情通过请求ID和当前项目读取，返回no-store；跨项目不披露请求内容。

新域初始化先验证完整表、键和唯一约束，拒绝部分表或视图后才创建任何相关结构。必要验证使用既有真实账号邀请、规范资源、独立测试Runner和原Review夹具。不能在持有authority read transaction时另开同DB连接计数；这会使单连接SQLite测试阻塞。本文不代替集中测试和最终GUI证据。

共用成熟模块为当前授权、commandReceipt、项目guard、原Review、交付投影、Inbox序号／私有生命周期、Web原请求恢复及作用域读取。没有引入新调度器、审批引擎或第三方依赖。

界面读取运行时遵守正式 `dag.Run` 合同：核验准确运行ID、项目、版本及状态，不要求不存在的 `task_id`。表单同时核对当前任务及历史页版本；创建端仍由服务端确认任务与运行、内容摘要和版本的准确关系。取消后重新请求沿同一合同读取，不把客户端运行快照当作绑定权威。

测试计划已选草稿的编辑／保存来自当前服务端 `allowed_actions`；新建入口使用会话同事务投影的有效项目角色。任务或需求来源尚未确认时保持等待并禁用保存，避免把“尚未读取”误判为零条验收要求。发布和正式Review继续由各自现有门禁决定。
