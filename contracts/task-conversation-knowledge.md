# 讨论、回复与开发的固定知识引用

状态：T-CONTEXT-145实施合同；产品初始1.0.0。属于既定资料、记忆、技能、任务协作与原生智能开发范围。复用[项目知识](project-knowledge.md)、[原讨论](task-conversations.md)、[原回复](task-conversation-replies.md)及[开发交接](task-discussion-development.md)，不新增发布、调度或模型权威。

## 版本与事务

`TaskConversationAppendRequest.knowledge?`为最多16个现有`ProjectKnowledgeReference{id,revision,sha256}`，按ID唯一。完整闭合字段、规范ID、正整数修订及小写摘要；显式null、重复和调用者正文／标题快照拒绝。未选资料省略字段。原追加事务先恢复同一原回执，再核验新发布资格；消息、固定引用关系、活动及回执原子保存。原操作恢复不重新发布或重解析已退役的引用，不自动消费模型。

消息增加可选`knowledge:TaskConversationKnowledgeReference[]`，每项为`id,revision,sha256,title_at_post,kind_at_post`。标题／类型从服务端批准快照取值；不返回全文或在历史读取时改成当前标题。新关系表保存准确tenant/project/task/message、ordinal与引用；不修改旧消息基础表、不回填、不允许assistant伪装为人类关系行。

## 历史读取与当前消费

GET `/project-knowledge/{id}/revisions/{revision}?project_id`返回`ProjectKnowledgeRevisionDetail{project_id,knowledge_id,snapshot:ProjectKnowledgeRevision}`。同当前项目读取权，严格规范路径和查询，直接读取准确不可变修订并核验摘要；不遍历分页、不回退latest。历史可读取不代表可用于新的模型输入，退役或来源变化不重写历史。正文只在明确展开后读取，401／403及范围切换立即清除受保护内容。

`taskConversationMessageMetadataTx`统一水合原消息、准确读取、回复来源及开发来源。人类消息从固定关系读取；assistant只能从同租户／项目／任务、真实completed backing Reply继承其Source.Knowledge。必须核对受保护关系及reply的MessageID、sequence、RunID和Source.SHA256；不能相信消息自报的reply_id，不能接受assistant的人类关系，也不递归嵌套历史Messages。

## 模型与开发

近期消息的准确引用按消息顺序并集，同ID同修订摘要去重，不同修订／摘要明确冲突；总数最多16。Reply.Source增加可选`knowledge:ProjectKnowledgeContextItem[]`，通过原`resolvePublishedKnowledgeTx`按当前授权解析，参与Source.SHA256及原32KB上下文／64KB请求限制。未选引用不引入空字段改变原业务历史摘要。原规则仍要求窗口包含要回答的未答人类提问；“assistant-only来源”表示带引用的原人类消息已超窗，但窗口中的助手仍携带引用，不能据此允许无人类提问的新回复。

创建、原受控执行领取／心跳和发表前的`checkTaskConversationReplySourceTx`再次核验固定知识及当前资格。新发布、退役、来源变化或撤权阻止新消费／发表；保留真实原运行、用量及输出事实。SDK不新增模型回退、工具权限或第二调度器，也不宣称提供方每字节请求具有原控制面事务的原子性。

已核验回复转开发时，使用`knowledgeReferences(reply.Source.Knowledge)`传原`TaskDispatchPreviewRequest.Knowledge`，不是仅把资料拼入reviewed_goal。原派发及Start再次核验资格，独立测试、正式Review和问题回流保持原权威。

## 界面与验收

composer下折叠添加资料，展开才有界搜索／分页。用户明确选择，不自动添加；已选按准确ID单独水合，失效项保留引用并可移除，不静默升级。草稿只保存正文、人员ID及知识pin；未知写恢复原JSON／身份／版本／操作，不补造字段。消息显示发表时标题及修订，按需读准确历史；回复和开发只展示服务端冻结来源，不再选择。

新增文案提供中英，保留原文／摘要，沿现有焦点、窄屏和按需展开组件。定向证据须覆盖：原回执、原子失败、当前发布／新草稿分离、退役／再发布／来源变化、准确历史、摘要损坏、跨项目／撤权、assistant-only近期窗口、伪造backing关系、16项／32KB限制，以及引用传递到真实模型请求和原开发Start。组件或协议夹具不能称为厂商模型、完整附件解析、OCR、安装、多机或生产验收。

文件所有权：A新增知识关系／schema／准确revision读取与其tests；B讨论选择器、固定来源呈现、草稿／恢复及CNEN；Root接原追加／回复／开发、初始化注册、CLI与生成合同及整合。验收实例在前阶段最终复核完成后，使用SQLite备份副本切换至独立145构建；原数据／旧构建保留，不覆盖活动静态目录。
