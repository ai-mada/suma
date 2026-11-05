# 原云任务的持久交互回合

T-WORKBUDDY-CLOUD-TURNS-169；suma 1.0.0；2026-10-31。实现合同；完整31项／P0–P7／58限定／34待交付／92总账保持。沿[原云执行](workbuddy-cloud-execution.md)、[原Runner](workbuddy-cloud-runner.md)、[原会话](workbuddy-cloud-collaboration.md)和[任务派发](workbuddy-task-dispatch.md)继续交付持续协作，不以历史加载代替新输入或真实回复。

## 用户路径

原已知云任务 → 展开“继续协作” → 输入后续目标 → 明确发送一次 → 阅读持久回复／工具状态 → 对准确权限请求明确一次决定，或请求取消当前回合。刷新、关闭面板及原HTTP响应丢失后只读／恢复同一操作，不重发最初任务、不再创建云任务。原节点工作区提供简洁中文／英文界面；高级来源默认收起。

活动回合在展开且页面可见时自动有界增量读取本地GET（约1秒、单inflight）；完成／unknown／过期／失权／卸载时停止，手动刷新另留。原DAG仍waiting、revision不变也应显示新回复与权限。本地GET不是Attach，不另建dispatcher；语义Version不变也不能丢弃新sequence。

## 领域及公共接口

新增独立Turn领域，复用CloudCollaborationSource并固定原assignment的attempt/fence/dispatch_id与摘要。当前真人owner、项目权限、原submission／task／node摘要、当前grant及client绑定、原节点waiting及Run running、两级预算在新输入前重新核对。旧Runner租约已排空，不续造旧机器资格；本次明确输入形成独立有限授权。代码、测试、验收和Run状态均不由会话内容推进。

公共DTO置于internal/workbuddycloudcontract/turns.go，同源生成OpenAPI／TypeScript：

- CloudTurnSource：collaboration（CloudCollaborationSource）、attempt、fence、dispatch_id、assignment_sha256。
- CloudTurnPromptRequest：project_id、expected_submission_version、expected_run_version、expected_version（head，首次明确0）、prompt、idempotency_key。prompt为1…32000 UTF-8字节，不接受模型、URL、任意RPC、ticket或task ID覆盖。
- CloudTurn：id、version、operation_id、head_version_at_start、source、owner_id、state、prompt_sha256、started_at、deadline_at、overdue、finished_at?、stop_reason?、error_code?、usage_state、usage?。state仅prepared/running/waiting_permission/completed/unknown/rejected；completed只表示准确RPC回合结束，绝不等同任务交付／验收或远端进程停止。无usage保持unreported，不补零，不形成已定价账单。
- CloudTurnView：source、version（head）、latest_turn?、allowed_actions。归属原owner，只读不Attach。新Prompt仅在无活动／unknown回合且当前资格成立时出现。
- CloudTurnEntry：sequence、kind、text、tool_call_id?、tool_state?。kind仅assistant_text/tool_state/plan_summary/notice/unsupported/artifact。artifact字段见本合同的产物观察章节。thought、原始packet、artifact URI和任意输出对象保持在读取合同之外。独立sequence是平台本地序号，不称供应方游标或历史覆盖。
- CloudTurnPermissionOption：id、name、kind。CloudTurnPermission：id、turn_id、version、tool_call_id、title、details、details_sha256、approve_allowed、state、options。只提供真实原选项；本阶段仅allow_once/reject_once可决定，不扩大为always授权。操作细节无法完整安全显示、被脱敏或超限时approve_allowed=false，仍可准确拒绝。供应方文字是待审数据，不能变成平台权限。
- CloudTurnCommand：id、turn_id、operation_id、kind（permission/cancel/reconcile）、state（prepared/sent/acknowledged/unknown/rejected）、permission_id?、option_id?、error_code?。CloudTurnPermissionReplyRequest：project_id、expected_version（permission）、option_id、idempotency_key。CloudTurnControlRequest：project_id、expected_version（turn）、idempotency_key。
- CloudTurnPage：turn、items、permissions、commands、next_after、has_more。after首次0、limit默认50／最多200；回合最多1000项／1MiB显示文本／单项32KiB、总计最多32个权限对象（包括已决与待决）。超限停止本地采集，保留unknown与已有前缀，不丢弃已决记录或补完整成功。

精确枚举：View.allowed_actions为prompt/cancel/reconcile的有序子集；Permission.state为pending/prepared/sent/acknowledged/unknown/rejected；usage_state为partial/unreported，partial也不表示可定价计费。安全error_code仅workbuddy_cloud_turn_unknown/interrupted/limited/authorization_changed/rejected/unavailable这组前缀完整名称，不能保存原provider错误文本。公开version描述语义状态，文本游标与lease心跳独立；未知回合释放已停止的本地处理资源，但继续阻塞其原submission，不阻塞无关任务。

供应方option_id／tool_call_id是有效Unicode、无控制字符的有界token，最长128；不强套平台ID字符集。所有可选对象省略表示不存在，不能返回null占位。

接口均当前浏览器账号专用；POST无query，GET只接受明确project_id及限定分页字段；拒绝重复／未知字段、null必填标量、非法Unicode和版本。错误与显示来源中英完整：

| 方法／路径 | 回应 |
|---|---|
| GET /workbuddy-cloud-submissions/{id}/turns?project_id=… | CloudTurnView，200 |
| POST /workbuddy-cloud-submissions/{id}/turns | CloudTurn，202 |
| GET /workbuddy-cloud-turns/{id}?project_id=…&after=0&limit=50 | CloudTurnPage，200 |
| POST /workbuddy-cloud-turns/{id}/permissions/{permission_id}/reply | CloudTurnCommand，202 |
| POST /workbuddy-cloud-turns/{id}/cancel | CloudTurnCommand，202 |
| POST /workbuddy-cloud-turns/{id}/reconcile | CloudTurnCommand，200；仅本地失联核对，无供应方调用 |

## 一次发送、分布式控制及恢复

新head／turn／entries／permissions／commands／tools为正式事务数据，未知既存schema在任何DDL前拒绝。采用当前授权的组织→项目／原运行→领域锁序，数据库时间、CAS及唯一操作键。先保存准确来源、prompt摘要／私有原文、固定head、lease owner/fence、deadline及receipt，提交后仅原调用栈持一次不可重建的运行permit。Browser返回202后原runtime继续接收，不依赖浏览器HTTP长连接存活；重放命令返回原Turn，不启动新goroutine、Attach或Prompt。

外部链路只用原GetSessionAccess→Attach initialize/session/load→新Session.Prompt→Next，不重发Create正文。Prompt HTTP发送与Next消费必须并行，以免供应方等待权限回复时死锁。权限和Cancel命令先落数据库inbox；原lease owner核当前资格及fence后领取并执行，任何控制面副本均可接收浏览器请求，不依赖sticky session或向另一个副本伪造内存连接。一个命令仅原领取permit可以发送；sent／unknown不自动重发。

回合默认最长15分钟，运行lease以数据库时间续约；每项目最多4个活动回合。心跳不改变公开语义版本，文本通过独立sequence持久化；权限／状态变更按其对象版本CAS。未知或过期lease保留阻塞原head；明确reconcile仅将原回合封为unknown并增加fence，阻断迟到写回，不授予另一次Prompt。重启不能重建原连接／权限引用、不能通过新Attach绕过未知副作用。正常终态或明确未发送拒绝才释放下一回合资格。

每次外发、采集及权限决定重验原owner／当前权限、grant/client binding及原attempt。发生撤权、取消、暂停或来源变化时，不再发布新内容／允许决定；私有原permit可持久记录本次已发生的终态或unknown事实，不能因当前用户失权而抹掉外部操作。凭据、ticket、连接头和wire RPC ID只在控制面内存；文本持久化前去除已知秘密并保护跨chunk边界，不把JSON转义当作脱敏。

检查区分风险：新Prompt／allow_once核当前Run与预算；仍有原连接时，明确cancel／reject_once继续核owner、grant、原scope和lease，但不能仅因预算变enforce或暂停而挡住降低风险的操作。若本地流已因来源／权限失败关闭，则明确unknown，不再宣称取消可用，也不自动重连或代替用户Cancel。

## 权限、终态与取消

SDK声明不提供客户端文件读写／终端；本平台不执行这些客户端请求，也不把它称为云OS沙箱。只管理供应方实际发出的权限请求，不能声称所有远端副作用必经回调。权限绑定原Turn／原tool_call_id／准确选项及完整细节摘要；审批时重新授权、核版本。审批准备、HTTP发送／确认、供应方采用分开记录；响应未知不再次回答。

按tool_call_id维护持久工具状态。孤立更新、未知状态或缺失终结不能因end_turn而冒充全部工具完成。只有准确原Prompt RPC终态、发送事实和工具／待决权限／控制命令事实都已核实时标completed；否则保留终态观察但state=unknown。stop_reason按end_turn/max_tokens/max_turn_requests/refusal/cancelled分别保存，不自动续Prompt。

Cancel只请求取消本连接、本次主动Prompt；HTTP确认不是远端任务停止。仅对应RPC的cancelled终态及已核实工具事实可证明回合结束，原REST任务／Run／node状态独立。Close只关闭本地流，不自动发Cancel。权限拒绝、断线、截止、采集失败及进程退出保留原操作及部分输出，原DAG不得刷绿或生成伪候选／测试／验收。

## 交付组织与验收

主集成冻结公共合同、同源生成、全局接线、文档与Git；后端代理负责事务／授权／inbox／schema；运行时代理复用成熟ACP SDK处理原连接、并行收发、投影、工具账本和持久控制；工作台代理负责独立composer／原journal及原节点最小接线。内部接口先冻结再并行实现，不各建一套dispatcher。

定向检查覆盖原key恢复零重发、两副本控制inbox、撤权／旧fence／过期、权限原选项、Prompt等待权限HTTP死锁、end_turn未终结工具、未知HTTP／SSE断线、取消只证明回合、跨chunk秘密和初始schema。真实隔离HTTP／SQLite／受控SDK＋中英生产组件做一次代表旅程，来源和失败分别保留；真实厂商／多机／部署／完整CLI及全平台继续逐项验收。必要源码及文档阶段commit并正常推送dev，缓存／原正文／DB／凭据／图片不进入Git。
## WorkBuddy 产物观察

T-WORKBUDDY-ARTIFACT-TIMELINE 沿原会话消费官方 `_codebuddy.ai/artifact` 通知。解码器核对当前会话 ID 后，原回合持久化一条带本地序号的观察记录。对外记录 `kind=artifact`，并同时携带 `artifact_type` 与 `artifact_event`。类型仅 plan、tasks、media、overview。动作仅 created、updated、deleted。产物项没有工具调用 ID 或工具状态，原其他条目没有产物字段。

计划和总结在新增或更新时可带最多 8192 UTF-8 字节的纯文本。预览复用原控制字符边界和脱敏策略，包含原凭据、该产物 URI 或其显式下载地址时遮盖匹配值。文本无效、缺失或超限时保留实际类型和动作。任务清单、媒体及删除记录的文本为空。客户端沿原时间线展示类型、动作和可用预览。

这是原会话已观察到的事件。工作台只读本地事件序列，原附着、权限、版本、预算及成功判定继续由既有流程控制。代码候选、测试和验收各由原领域事实确认。完整产物清单及实际文件接纳需要独立的来源、权限和内容验证。

[官方事件协议](https://open.workbuddy.cn/docs/openapi) · [交互](../design/workbuddy-cloud-turns.md) · [阶段验证](../quality/workbuddy-artifact-timeline-verification.md)。
