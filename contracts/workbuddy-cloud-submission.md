# WorkBuddy云任务单次提交与原任务读取

阶段：T-WORKBUDDY-143；速码1.0.0。此合同限定尚未开放的云任务HTTP协议、原attempt的持久检查点及单次提交恢复底层。不是完整执行适配器，不新增公共API、工具目录能力、自动派发入口或供应商验收。WorkBuddy云入口仍为planned；CodeBuddy CLI及通用external workflow保持各自协议。

## 协议边界

依据[官方Open API](https://open.workbuddy.cn/docs/openapi)，创建使用`POST https://www.workbuddy.cn/openapi/v2/tasks`，Bearer鉴权，JSON含必填prompt及可选name，接受201直接任务对象；单任务查询使用同路径追加准确task_id，接受200直接对象；分页列表接受200的tasks／total／pagination。创建权限为user.task.invokable，读取为user.task.readable。本文不把文档样例当厂商执行证据。

任务创建没有已核实的供应方幂等键或按速码操作ID查询接口。不得增加虚构的Idempotency-Key、dispatch_id或取消路径。不能将兑换码接口的request_id约定套用到任务。没有明确取消合同，连接取消或请求超时不证明远端任务停止。

HTTP客户端只对本合同的固定方法、路径、请求字段和返回投影负责；任务／模型不能选择任意URL、Header、Shell或SQL。非预期状态码、畸形／超限响应和任务身份不符明确失败，不因HTTP200推定业务执行成功。不自动换提供方或重复写入。

冻结Go接口：

- `NewClient(Config{BaseURL, Token, HTTPClient}) (*Client, error)`；`Endpoint() string`返回规范origin加`/openapi/v2/tasks`。
- `CreateTask(ctx, CreateRequest{Prompt, Name}) (Task, error)`；`CreateRequestDigest(req) (string, error)`对通过同一校验的JSON编码字节计算SHA256。
- `GetTask(ctx, id) (Task, error)`；`ListTasks(ctx, page, size) (TaskPage, error)`。
- `Task`仅含ID（JSON task_id）、Status、Name及可选CreatedAt／UpdatedAt；`TaskPage`含Tasks、Total、Pagination，后者为Page／Size／Total。
- `Error`包含Code、StatusCode、OutcomeUnknown；HTTP客户端Code为invalid_config、invalid_request、cancelled、transport、http_status或invalid_response。错误不携带原响应正文／底层cause或秘密。POST进入HTTP发送后异常保留OutcomeUnknown，明确发送前取消为false；该错误分类不能推翻已有prepared检查点的保守恢复规则。

响应上限1MiB，拒绝非法UTF-8、重复字段及尾随JSON。task_id限200字节ASCII字母、数字、下划线或连字符；GET响应必须匹配原ID。分页page≥1、size为1–100；核对total一致、唯一任务ID及返回数量不超本页容量和剩余范围。Prompt上限256KiB、Name上限512个Unicode字符，是速码本地边界，不是厂商额度。未知status保留，不推断成功。默认HTTPS官方origin，只允许同origin配置或显式本机HTTP测试入口，不跟随重定向。

## 原attempt持久状态

持久记录绑定既有Journal中准确Assignment，沿原租户／项目／run／node／attempt／fence及共享池身份隔离，不建立第二任务数据库或第二调度器。新增检查点只记录固定提交依据的摘要及核对所需身份，不保存返回的ACP token、link、sandboxLink或sandboxDataLink，也不新增访问令牌、提示全文或原响应日志。原Assignment的既有持久字段仍按原合同管理，不宣称此次改动移除了全部历史任务上下文。

Journal冻结接口：`WorkBuddyBinding{RequestSHA256, CredentialRef, CredentialVersion, Endpoint}`与`WorkBuddyCheckpoint{Binding, State, TaskID}`；`BeginWorkBuddySubmission(a, binding) (bool, error)`授予一次准备资格，`RecordWorkBuddyTask(a, binding, id) error`保存准确ID，`WorkBuddySubmission(a) (checkpoint, found, error)`读取安全快照。内部额外保存检查点版本及完整Assignment摘要，防止仅相同索引key但租户、项目、worker、共享池路由或配置已变化时复用。

Begin及Record核对实际持久inbox，不能接纳Accept保存失败留下的内存对象；拒绝已有最终结果和其他执行通道。保存失败回滚独立副本。供应方ID仅首次写入，同值可重复确认，不能替换；最终结果已有的检查点仍可只读核对。既有Runner重启恢复继续产生未知结果，不绕过其恢复流程重启云提交。

| 状态 | 事实与允许行为 |
| --- | --- |
| 尚无提交记录 | 先验证固定输入并持久写入prepared，写入失败不能发出POST |
| prepared | 已消耗本地一次发送资格；可能在发送前、发送中或收到响应后崩溃。恢复时无法证明未发送，直接待核对，不重发 |
| task_known | 已持久保存供应方task_id；恢复只GET该准确ID，不重新创建，不按相似名称或时间找任务 |

首次成功持久准备的调用方才能尝试一次创建。重复调用、重启、并发调用不能取得第二次发送资格。收到创建响应但保存task_id失败仍属于未知提交结果，不能把这个本地失败视为“供应方没有创建”。取得已知任务后仍须校验查询响应的task_id一致。

输入依据变化拒绝沿用原提交；不能通过改prompt、凭据、项目或attempt绕过恢复。prepared与task_known不代表任务成功、终态或正式验收，供应方status保留来源语义。供应方completed、失败或归档不是速码任务完成、独立测试或人工验收凭证。

## 组合与资格

HTTP模块不决定用户权限，Journal不授予执行资格。原调用方必须已经持有准确任务与执行授权；未来开放前还需服务端授权实例、秘密引用、当前撤权检查及固定预算／网络范围。此次底层不得仅凭配置存在或token非空就登记为可派发。

组合入口为未导出的`submitWorkBuddyTask(ctx, journal, assignment, client, request, credentialRef, credentialVersion) (workbuddy.Task, error)`，位于`internal/runner/workbuddy_submission.go`；不接入Execute或能力发现。它先检查上下文和请求摘要，再读取原检查点；尚无记录才Begin。首次准备才Create，已有task_known才Get；已有prepared直接返回待核对。原Runner重启记录未知结果后，仍可只读查询已知任务，但不能写入新的提交记录。查询失败保留原task_id。

组合错误使用submission_configuration_invalid、submission_context_cancelled、submission_checkpoint_unavailable、submission_requires_reconciliation、submission_task_checkpoint_failed。检查点不可用或需核对时OutcomeUnknown=true；task_id已取得但持久保存失败时返回该安全Task及未知错误，保留原ID供显式核对，不能仅见Task非空就当提交已持久完成。若准备后发生本地取消，底层客户端即便返回OutcomeUnknown=false，也不删除prepared或授予重发资格。

不在HTTP客户端中创建后台调度、无限轮询、隐式刷新凭据或自动修复循环；读取和创建均受调用上下文约束。已有任务读取失败保持原ID和原记录，不重置检查点。任何“从未知改为可重新提交”的操作都需要后续明确核对合同，当前不提供。

## 本阶段验证与未实现部分

本地协议测试应覆盖准确方法／路径／鉴权、201与200的响应形态、分页、身份不符、错误及有界响应；持久测试覆盖首次发送、重复／并发、重启、prepared崩溃窗口、保存失败、错误scope和变化依据。结果由专项质量记录维护，不在本合同预填通过数量。

仍缺正式OAuth用户授权与授权实例管理、ACP进度及续谈、权限请求、用量与硬预算能力、取消／停止证明、云产物导入、准确Git候选、独立测试、真实厂商及多机验收。调用协议夹具不能关闭这些范围。架构及恢复理由见[设计](../docs/architecture/workbuddy-cloud-submission.md)。
