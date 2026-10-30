# 原生任务会话合同

状态：1.0.0，T-COLLAB-127冻结实施合同。负责人：工作台、协作与平台。关联需求9–12、22、23、24、26；完整平台范围继续保留。

## 领域语义

每个正式任务拥有按需建立的持久会话，当前消息为人或项目API账户的真实协作记录。发送记录不触发模型、任务派发、运行恢复、代码修改、测试或验收。智能体输出只有经明确模型、预算和原执行回执证明后才能成为平台生成的回复；本合同不允许浏览器提交`assistant`或`system`身份。

消息不可变，序号在租户／项目／任务内严格递增。每条记录绑定发言时的任务版本、当前需求ID／版本和准确关联运行ID；它们是原讨论依据，不是新的工作区绑定、执行指令或正式验收。任务版本与状态不因讨论变化，已验收任务也能继续讨论。

作者ID、名称、通道类型、消息ID、序号、时间和依据由服务端赋值。消息正文为纯文本，保留原始空白，不执行Markdown／HTML；必须为有效UTF-8、非空白、无NUL，最多8000个Unicode标量且最多32000字节。前端长度提示不替代服务端验证。

## HTTP与类型

GET `/api/v1/task-conversations?project_id&task_id&limit&before`：当前会话读取。默认`limit=20`，最大50；可选`before`为大于0的安全整数，排除该序号及更新消息。返回本页按序号升序排列；第一次读取为最新一页。`next_before=0`表示无更早页，否则使用本页最小序号读取前页。`total`为当前会话总数，不是本页数；`items`始终为数组。读取空会话不创建数据库记录。

`TaskConversationPage`：`project_id,task_id,task_version,can_post,items,total,limit,next_before`。`can_post`只表达读取时的当前写入能力，不能作为后续写入授权证明。

`TaskConversationMessage`：`id,sequence,task_version,body,author_id,author_name,actor_kind,created_at,requirement_id,requirement_version,run_id`。人／API发送回执的`actor_kind`为`human`或`api`，历史可包含平台核验后生成的`assistant`；没有关联需求时ID为空、版本为0，没有关联运行时ID为空。新增必需字段`reply_id,reply_to_sequence,execution_run_id`：人／API为空／0，助手必须来自受保护关系，见[智能回复合同](task-conversation-replies.md)。全部字段必需，不返回原Record.Data、凭据、环境、执行命令或输出。

POST `/api/v1/task-conversations`：`TaskConversationAppendRequest {project_id,task_id,expected_task_version,body,idempotency_key,mention_resource_ids?}`，返回201和原`TaskConversationMessage`。可选明确人员提及与不可变mentions快照见[精准提及合同](task-conversation-mentions.md)，无提及时省略。严格拒绝未知属性、作者／角色伪造、重复查询参数、路径和数字的非规范形式。支持当前浏览器和当前项目API身份，Runner和Agent凭据不通过此入口写入人类消息。

## 权限、事务与恢复

读写都复用原当前身份、组织和项目授权。项目API凭据不能用请求体／查询改变原固定项目；当前权限撤销后，重复操作也不能恢复私有响应。只读成员可读历史，不能发送；`can_post`在同一授权事务中计算，权限错误与数据库错误分别处理。

追加操作在原授权事务中核对正式任务、当前要求版本、稳定操作回执及序号。头记录CAS、消息、原操作回执和活动记录同事务提交；跨进程竞争不能产生重复序号或覆盖另一条消息。重试相同身份、任务、操作ID和正文恢复原消息；同ID不同正文拒绝。回执检查先于版本冲突判断，使提交成功但回执丢失后仍能恢复原消息，不误发第二条。

序号使用BIGINT且不超过JavaScript安全整数范围。排序依据正式序号，不依赖多机器客户端时间。分页只读取已经持久化的内容；期间新增消息不会重复更早页，不宣称形成全会话历史快照。

## 界面与智能体边界

会话挂在原生IDE所选任务中，按需展开；代码编辑器不因打开会话而重新挂载。原讨论、输入草稿、正在发送、结果未知、版本冲突、无权限和空态清楚区分。中英文词条成对，用户正文和作者名称保持原值。390窄屏采用单列消息与输入，不增加一级导航。

使用原写操作恢复和未保存保护：结果未知时冻结原请求、原调用者与操作ID，明确恢复原操作；不能清空或换ID自动发送。版本冲突保留草稿并提供读取当前任务依据的动作。切换任务、项目、账号或组织不会把旧响应安装到新页面；未发送草稿与未知操作阻止静默离开。

完整持续智能体会话、实时输入／停止与模型自动选择仍属于原平台目标。后续回复必须经统一Go模型网关、明确模型参数／预算、唯一DAG、固定上下文和准确attempt／fence回执接入；不得直接从浏览器调用模型，或复用旧固定模型规划用途绕过当前ModelBinding。
