# 任务交付验收责任请求

状态：T-REVIEW-142，实现基线，初始产品1.0.0。负责人：质量协作／主集成；继续原P3／P5与Multica M27准确待处理目标。需求、实现、协议检查、真实运行和最终GUI分开记录，完整平台范围不变。

## 责任与验收分离

使用者在原任务详情明确选择一名当前规范人员，请其审核这次准确交付。请求不授予权限、不启动运行、不代表测试通过，也不代替DAG approval、责任交接或正式`Service.Review`。当前组织owner/admin或项目admin/reviewer才具备原审核资格；人员资源标签不构成角色授权。允许自请求，不新增未经确认的禁止自审政策。

任何当前具有原验收资格的人员仍可通过既有Review接受／退回。指定责任人不收窄或扩大原权限；正式决定同时关联准确请求，展示实际决定人。原Review继续独立核验准确代码及独立测试、任务／需求／依赖／设计／测试计划、当前授权与补偿事实；退回继续同事务创建问题并使下游失效。不得新增另一套acceptance状态机或`can_accept`捷径。

初始1.0.0的正式任务结论只接受`accepted`或`rejected`，移除`approved`输入别名；真实工具授权、运行审批和部署审批的独立状态词不受此清理影响。

## 数据与生命周期

新建独立`suma_task_review_requests`和`suma_task_review_pending`域，不修改既有reviews、task或run表，不回填旧历史。请求保存准确任务版本、绑定run ID／版本、任务内容摘要、发起账号和发起时名称、固定审核resource/user与当时名称、备注、通知关系及时间。状态只持久化pending／cancelled／closed；closed只关联真实`suma_reviews.id`，决定内容从正式Review水合，不另存第二份决定。

pending槽以tenant/project/task主键及request_id唯一约束保证每任务至多一个未结束请求。绑定变化或审核资格失效不读时改写；保留请求历史，显示binding_current=false或reviewer_available=false，由发起人或当前组织／项目admin明确取消后再请求。取消只结束请求，不删除通知、不改变正式任务。取消需要当前业务写权及请求版本；原发起人当前无写权不能沿旧身份操作。

新请求必须指向同项目真实任务当前run且运行succeeded，准确task/run预期版本及任务内容依据一致。此条件只证明请求定位，不证明完整质量门禁。`binding_current`只表示当前任务版本、task/run绑定、任务摘要和run版本／状态的直接一致；界面明确显示正式验收仍须原质量门禁，不能称为可通过。closed来源损坏时不伪造决定。

请求、pending槽、唯一`task_review_requested`通知、活动及原回执同事务。通知使用已有Inbox单一序号／插入权威，事件标识绑定请求ID及固定接收人；通知失败整笔回滚，不消费序号。原请求重放在当前授权后、重新解析接收人或检查新版本之前恢复原回执。取消同样保留原操作回执，不以新版本／新操作自动重试。

## HTTP与类型

接口仅浏览器／当前用户API项目身份；拒绝Runner／Agent。读需要当前项目读取权，写需要当前项目写权；未知／重复／空参数和闭合JSON拒绝，响应no-store。每次授权，不从客户端身份字段推导接收人。

- `GET /api/v1/task-review-targets?project_id&task_id&q&limit&cursor&resource_ids`：沿精准提及目录相同有界合同，只允许当前规范活跃人且有approve角色。默认20、最大50、查询最多200 Unicode；选中ID至多20且与q/cursor互斥；闭合游标绑定当前身份／组织／项目／任务／q／limit。SQL有界及精确total，普通SELECT解析接收人，不能引入相反身份锁顺序。公用规范人员SQL应作为通用私有解析器，避免另造账号权威。
- `GET /api/v1/task-review-requests?project_id&task_id&limit&cursor`：当前任务的完整total、当前pending引用及有界历史。创建时间统一固定9位小数UTC格式，按created_at DESC、id DESC排序，游标绑定当前范围和limit。只读取，不创建请求或标为已读。
- `GET /api/v1/task-review-requests/{id}?project_id`：准确历史请求，响应同时包含当前有权的任务摘要和当前请求事实；404不泄露其他项目。
- `POST /api/v1/task-review-requests`：`TaskReviewRequestCreate {project_id,task_id,expected_task_version,run_id,expected_run_version,reviewer_resource_id,note,idempotency_key}`。note为非空有效Unicode，最多2000字符／8000字节，无NUL；ID及正整数版本按当前平台约定。当前pending存在则409，不能静默替换。
- `POST /api/v1/task-review-requests/{id}/commands`：`TaskReviewRequestCommand {project_id,expected_version,action:cancel,reason,idempotency_key}`，reason约束同note。只允许pending、发起人或当前组织／项目admin；原命令恢复先于新状态／版本冲突判断。

`TaskReviewTarget {resource_id,user_id,name,resource_version}`；`TaskReviewTargetPage {project_id,task_id,limit,items,total,next_cursor}`。

`TaskReviewRequest`字段：id、version、project_id、task_id、task_version、run_id、run_version、task_basis_sha256、requester_user_id、requester_name_at_request、reviewer_resource_id、reviewer_user_id、reviewer_name_at_request、note、state、created_at、updated_at、cancelled_by_user_id、cancel_reason、closed_review_id、binding_current、reviewer_available、is_current_assignee、can_cancel、source_available、可选decision。

`TaskReviewDecision {review_id,reviewer_user_id,decision,reason,created_at}`只来自真实同租户／项目／任务／源task_version／run的Review。`TaskReviewRequestPage {project_id,task_id,task_version,can_request,limit,items,total,next_cursor,pending_request_id}`，`can_request`为当前写权提示，发起仍独立验证。`TaskReviewRequestDetail {project_id,task_id,task_title,task_version,can_request,request}`。变更返回准确`TaskReviewRequest`原回执；回执恢复不水合新的事实。

## 既有Review挂钩与通知

在原Review事务写入正式review_id后、保存原回执前，读取准确pending槽，只有请求固定task版本／run／run版本及当前直接依据对应时同事务关联并closed、删除准确pending槽。重放旧Review不新写通知或重做关闭；新请求不能被旧Review回执关闭。管理员代审记录真实决定人。缺少／不对应请求时原Review行为保持独立，不制造审核请求。

Inbox kind新增task_review_requested，可选review_request_id只在原请求＋固定接收人＋通知反向绑定与当前任务均完整时返回。历史取消／完成不删除私人事件；打开原请求与已读分开。不能由task.owner、任务状态、估计等待或旧attempt猜测接收人。

## 简洁界面与同源CLI

原任务详情“交付门禁”处新增紧凑可展开面板：默认展示验收责任、准确运行、直接绑定变化、当前审核资格及原证据入口；展开后选择一名审核人并填写说明。历史有界读取、准确来源可单独打开；`panel=task-review:<request_id>`在同一任务详情展开对应请求。原Review按钮／弹窗继续使用原当前权限和操作恢复，不复制验收表单。

未发送和未知写入保留原身份、范围、版本、操作标识及正文；范围变化清旧响应，401／403／404清受保护内容。中英、键盘、窄屏及主题均需实际复核；不可用浏览器的缺口如实记录。

专用`review-requests` CLI支持有界list、targets、get，以及create／cancel的明确原JSON文件。写入不添加查询参数，不造新操作标识，不修改原依据。个人收件状态仍用原inbox命令。

测试计划界面的当前业务写权限统一为组织owner/admin或项目admin/reviewer/developer；去除无效editor判断。发布／退役继续使用服务端allowed_actions，不扩大权限。

## 必要验证

规范当前审批人员、viewer/developer不可被指定为审批人、其他合法审批者原Review不被拦截；同事务投影失败全回滚与序号；重复／更改原操作／并发pending；撤权及准确历史访问；绑定变化保留旧请求但不伪称质量通过；取消／重新请求；原Review准确关闭、实际决定人、拒绝及问题回流，旧Review重放不关闭新请求；损坏来源降级；中英与准确通知跳转、未知写入恢复；开发者／审核者测试计划操作可达且发布权限不扩大。集中执行受影响检查，重复结果不累加。
