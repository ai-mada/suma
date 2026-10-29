# 任务会话领域与AI开发SOP

状态：1.0.0，T-COLLAB-127实施；负责人：协作、工作台、平台。关联需求9–12、22、23、24、26。接口类型与语义见[唯一合同](../../contracts/task-conversations.md)，实际范围见[验证](../quality/task-conversations-verification.md)。

## 模块与数据

`task_conversation_types.go`定义消息、页面和追加请求。`task_conversation.go`负责当前授权、原任务依据、会话序号和原操作回执；`task_conversation_http.go`只负责严格协议边界。`suma_task_conversation_heads`按租户／项目／任务唯一，`suma_task_conversation_messages`以消息ID为主键，同一作用域与序号具有唯一约束。追加沿用现有项目事务锁及头CAS，避免进程内锁被误当成分布式顺序保证。

消息和序号、`suma_operations`原回执及固定活动事件同事务提交。活动只记录`task_conversation.posted`和固定标题，不泄露正文到通用活动或日志。任务、需求、运行、候选、用例、正式Review及发布状态均不因消息而改写。当前初始库初始化统一登记新领域；不增加旧产品版本兼容、历史数据猜测或回填。

读操作核验当前项目权限及正式任务，不创建空会话。分页按正式序号，返回最新有界窗口并按升序展示；更早页使用排他的`before`，同页新增记录不改变旧消息身份。`can_post`是当前能力提示，后续POST仍重新授权；数据库失败不能伪装为只读或空会话。

## 原操作与界面

组件复用`useWriteAction`及当前账号／组织调用者，按用户、组织、项目和任务建立恢复空间。未知结果保存原请求、正文、任务版本和操作ID，不允许换ID或继续编辑后误发。原操作恢复先核验当前权限，再恢复不可变回执；任务版本后来变化也不阻止恢复已提交的原消息。

版本冲突保留草稿，明确读取当前依据后才能发送新的消息。草稿、待确认操作和发送中状态进入原未保存保护。403立即隐藏历史、取消旧读取并隔离迟到响应，明确重新核权后才继续。手动刷新与按需展开避免闲置轮询；关闭会话不重新挂载代码编辑器。

正文按纯文本和原空白显示。作者由服务端确定，中文和英文只影响控件、状态与格式，不能翻译消息原文、作者名称或代码。新固定活动按正式事件类型本地化，不按用户标题内容匹配翻译。

## 智能体回复的正式接入约束

当前持久会话是真实的人／API协作，尚不提供模型回复或实时消费。后续回复应新增受保护会话回复领域，以消息序号和准确任务／需求版本冻结上下文，使用当前`ModelBinding`、输出预算及工具版本，通过唯一Go DAG创建独立会话运行，不能替换任务的`RunID`。

复用参考为`dispatch_recommendation`的统一网关只读链路，而不是`intelligent_operations`的固定环境模型配置。需要扩展原有限purpose、能力探测、私有／Pool领取与完成核验、模型原回执核对及SDK结果合同。只有准确attempt／fence／原结果经平台核验，才允许平台生成带来源的智能体消息。用户提交`role=assistant`、直接浏览器模型请求、消息自动恢复业务运行都应拒绝。

真实终端、语言及调试服务分别按[PTY完整架构](../architecture/interactive-terminal.md)推进，不把会话文本当作shell输入。讨论、可编辑规划、人工门禁和执行效果保持清晰的领域关系。

## AI开发SOP

1. 读取当前合同、授权和原任务版本，先明确发言还是派发，不推断工具已消费消息。
2. 输入或恢复原消息时保留准确账号／组织／任务与操作ID；结果未知先核对，不创建替代操作。
3. 变更接口先统一Go类型，再生成TypeScript／OpenAPI；界面只使用正式作者、序号及能力提示。
4. 保护原任务与执行事实；不为让会话测试通过而改动验收状态、源码、预算或执行证据。
5. 对权限、并发序号、分页和未知回执运行定向检查；一次实际HTTP／存储／界面故障旅程，不重复无关SDK、模型或发布。
6. 记录原失败、影响范围和剩余缺口。仅提交必要代码、合同、设计及精简摘要，原始数据库、日志、凭据、截图和构建结果保持在忽略目录。

*English: A task discussion is a durable human/project-API collaboration stream. Posting records an immutable author and source context, without dispatching or approving work. Original-operation recovery, current permission checks and editor leave guards remain mandatory. Model replies must later join the explicit ModelBinding/budget/DAG/attempt-evidence path; stored human notes must never be presented as assistant execution.*
