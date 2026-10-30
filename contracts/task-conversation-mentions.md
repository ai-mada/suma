# 任务讨论精准提及合同

状态：T-MENTIONS-140，实现基线，初始版本1.0.0。负责人：协作领域／主集成。接续Multica M09／M24／M27；完整平台范围不变。

## 用户行为与权威

任务讨论仍是纯文本。发送者在独立人员选择器明确选择接收人；正文中的`@名字`不解析、不派发。只支持当前项目规范绑定、活跃、有当前项目读取权的人员；不支持全员、团队、智能体或工具广播。提及不会启动模型、任务执行、审批或验收。

`TaskConversationAppendRequest`增加可选`mention_resource_ids`：最多20个不同的合法人员资源ID，保持输入顺序。未选择时省略字段。服务端在原讨论追加事务中先恢复原操作回执，再核验新发送的准确任务版本和全部接收人。一个接收人无效则整次发送失败；消息、提及关系、通知和回执原子提交。自提及记录真实事件。禁止任意创建系统通知的公开接口。

`TaskConversationMessage`增加可选`mentions`，元素为`TaskConversationMention {resource_id,user_id,name_at_post}`。这是发帖时由服务端解析的不可变会话历史，与既有author_name同类；无提及省略字段。单独关系表保存，禁止回填历史消息或修改旧表形状。普通读取和受保护回复／讨论转开发的来源读取必须由同一元数据函数水合同一份固定快照，避免成员改名或撤权改变已冻结来源摘要。受保护智能体回复不能生成提及关系。

## 当前人员目录

`GET /api/v1/task-conversation-mention-targets?project_id&task_id&q&limit&cursor&resource_ids`。

当前人／用户API项目读取权限，拒绝Runner／Agent；每次重新核验任务／项目。limit默认20、范围1–50；q为有界搜索。分页有完整匹配total、确定顺序及闭合规范游标（绑定当前租户、用户、项目、任务、查询与limit）。未知／重复参数拒绝。仅返回当前规范活跃人员且有项目读取权，不猜测成员绑定。

可选resource_ids为至多20个逗号分隔的唯一资源ID，用于当前草稿选中项的重新授权水合；与q／cursor互斥，保持给定顺序，未授权／不存在项不返回，不回显历史人员资料。此模式total为实际可见返回数、next_cursor为空。

`TaskConversationMentionTarget {resource_id,user_id,name,resource_version}`；`TaskConversationMentionTargetPage {project_id,task_id,limit,items,total,next_cursor}`。界面将未返回的已选ID明确显示为不可用，不能静默删除后发送。

## 原消息定位与通知

`GET /api/v1/task-conversation-messages/{message_id}?project_id&task_id`返回`TaskConversationMessageDetail {project_id,task_id,task_version,can_post,message}`。当前任务读取权限、准确租户／项目／任务／消息定位，失败不泄露其他范围。只读、不增加已读或会话记录；不遍历全部历史。

通知kind增加`task_mentioned`。与任务分配复用同一持久化序号及唯一写入权威；来源标识绑定固定消息和接收人，关系与通知互相校验。个人生命周期沿用已读／未读／归档／恢复。查询仅水合当前有权的任务摘要，不缓存讨论正文。仅完整任务＋原人员消息＋正确提及关系时source_available=true并返回可选message_id；缺失来源保留个人事件引用，无跳转或历史正文。assignment_current仅解释task_assigned。

## 界面与恢复

讨论输入下方为可搜索的明确人员选择器，已选项可移除，最多20人。中英文、键盘、窄屏、深浅主题齐全。未发送草稿以当前用户／组织／项目／任务隔离，仅保存正文和资源ID；当前目录重新解析显示名称。正文或提及变化均为未保存内容。发送成功才清空；结果未知时保留原请求字节、操作ID、预期版本及接收人，不重造请求；有草稿或未核对写入时不能自动触发智能体回复。

收件箱“打开讨论”转入开发工作台对应任务，URL使用`panel=workbench-task:<task_id>&conversation_message=<message_id>`。直接读取目标一次，聚焦并高亮原消息；已在当前窗口中不重复显示，不强制加载全部历史。不自动标为已读、不自动发送或启动执行。任务／身份／项目变化清理旧定位；401／403／404清理受保护内容与人员目录，保留该身份本地草稿及待核对原请求。

## 同源CLI

`suma --project PROJECT discussion list --task TASK`读取一页正式讨论；`discussion people --task TASK`读取当前人员目录；`discussion message MESSAGE --task TASK`准确读取原消息；`discussion post --file request.json`或`--file -`发送明确原JSON。发帖不自动生成操作标识，不添加查询参数，不重写正文或提及顺序；project_id必须与全局项目一致。未知结果使用完全相同文件／输入明确恢复。帮助和本地错误支持中文／英文。原`inbox`命令管理同一接收人的私有状态；专用讨论命令不改通用API客户端行为。

## 验收边界

必须验证：两人员发送到真实接收人、同事务回滚／序号不消费、重复与更改原请求、当前撤权、非人／非规范绑定／其他项目拒绝、零提及旧来源JSON稳定、成员改名后固定快照不变、原回复与讨论转开发来源验证、缺失来源降级、超过最新20条的直接定位、草稿隔离及未知发送恢复。仅执行受影响定向检查；实际浏览器独立记录。邮件／IM投递、偏好降噪、全员／团队广播及跨项目聚合仍为后续工作。
