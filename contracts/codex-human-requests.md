# 工具一次性权限与Codex人工请求合同

T-CODEX-HUMAN-REQUESTS-174；suma 1.0.0；2026-10-31。负责人：主集成、权限领域、Runner、协议及工作台。依照原P1／P2／P4人工门禁、分布式工具协作与完整Codex目标实施；完整31／P0–P7范围不变。状态：限定实现、定向与受控协议实际旅程已验证；真实厂商及完整平台继续交付。

## 单一权限领域

保留原`tool-permissions`浏览器、Runner与Pool路由、同库事务、当前身份、版本、原操作回执和租约语义。将通用记录／校验提取到`internal/toolpermissioncontract`，公共名称仍为`ToolPermission*`；ACP原生来源留在`acpcontract`，Codex原生来源留在`codexcontract`。不建立第二张权限表、第二套身份或第二个派发器；不保留旧产品wire别名或按缺失字段猜协议。

`ToolPermissionRequest {idempotency_key,protocol,acp?,codex?}`必须恰有一个原生来源，protocol固定`acp-stdio-v1`或`codex-app-server-v1`。ACP来源为`ACPToolPermissionSource {session_id,tool_call_id,title,kind,details,details_complete,options}`，完整保留原生选项；Codex来源为`CodexApprovalRequest`，不得伪造ACP session／tool-call／option。

`ToolPermissionDecision {protocol,acp?,codex?,actor_id,decided_at}`必须与原请求协议相同：ACP选择`ACPToolPermissionChoice {option_id,kind}`，仅allow_once／reject_once；Codex选择`CodexApprovalChoice {decision}`，仅accept／decline／cancel。浏览器`ToolPermissionDecisionRequest {project_id,expected_version,idempotency_key,protocol,acp?,codex?}`同样闭合分支。来源、请求、决定和完整人类Actor由服务端校验，不能由机器冒充批准者。

记录复用原IDs／attempt／dispatch／fence／Worker／Profile SHA、version、pending／decided／expired／superseded、时间和当前allowed_actions；增加可选`observation:CodexApprovalObservation`。索引改为明确的`source_session_sha256/source_request_sha256`：协议和原生身份的摘要，不把Codex thread/item塞进ACP字段；同item不同RPC请求可以独立存在，同RPC ID改正文必须拒绝。单一1.0新库声明及初始化探测同步修改，旧不匹配结构在任何DDL前拒绝，保留用户数据，不升级、回填或重置。

## 固定原生策略与准确身份

本轮依据SHA `3097c909fb6d5cd63510d44aceb1d42df9f6e85722a3e447e477def1ecf60e16`原生程序离线导出的稳定schema，非凭文档示例猜字段。原命令、模型、工具配置、隔离HOME、workspace-write及网络默认策略保持各自合同；审批策略统一为固定`on-request`并进入配置摘要、真实ACK及原Profile／库存校验。它不表示OS隔离或硬预算已经实测，单次工具批准也不增加平台权限或正式验收资格。

只接`item/commandExecution/requestApproval`和`item/fileChange/requestApproval`；`experimentalApi=false`。RPC ID保留string／number类型与精确值，用`CodexRPCRequestID {kind,value}`表达，数值使用规范十进制字符串，拒绝浮点、超范围或null。原生必需threadId、turnId、itemId和startedAtMs；command的可选approvalId独立保留，同item可有多个请求。请求关联当前原线程、回合和已观察item，不覆盖host waitingID或interruptID。

`CodexApprovalRequest`固定`request_id,method,thread_id,turn_id,item_id,started_at_ms,approval_id?,binding_sha256,params_sha256,item_sha256,title,details,details_complete,scope_supported,unsupported_reason?,command?,cwd?,changes?,reason?`。文件`CodexApprovalFileChange {path,kind,diff,move_path?}`仅使用规范工作区相对路径，cwd也为规范相对位置；完整本机根目录不进入公开凭证。两个原生摘要绑定未脱敏原请求与item，公开内容在Journal和网络之前脱敏；被删改或截断则details_complete=false，只能拒绝／取消。

审批不接受acceptForSession、执行规则修订、网络策略修订或永久写目录。networkApprovalContext、非空grantRoot、writeStdin及未知／实验权限扩展不升级为可批准范围，展示明确不支持原因且只允许拒绝／取消。命令与原item必须一致，cwd及文件目标由Runner核对当前独立工作区；不通过自造shell解析器声称命令无副作用。请求可能由本机工具按自身策略执行，批准不冒称原OS沙箱已得到验证。

## 异步、持久与取消

保留最多32请求／attempt、64KiB请求及32KiB可见详情、原deadline。锁内只校验、登记并放入有界队列；持久化、HTTP等待和文件IO不得占用Codex stdout／Cancel的互斥锁。不同请求可独立发布与等待，单一stdin写入器按原类型ID回覆`{result:{decision}}`，不得调用host request递增器或重发turn。

共享传输复用原Open／Poll与同body/key恢复；运行本地Journal记录请求、平台ID、准确决定、reply_started／written、resolved及item终态。写入前先持久化一次开始意图并重验本地原generation／thread／turn／item和停止状态；尚未开始的批准被取消拦截，已开始的写入可能结果未知，不能因超时或重启重放。实际完整Write成功后才标written，不以入队代替；部分写或崩溃保留unknown。取消、Finish、Close不得持锁等待HTTP或其它goroutine。

`serverRequest/resolved`仅表示答复或清理，不代表操作执行成功；`item/completed`分别记录completed／failed／declined。远端消息可能先于本地写入回调被处理，应暂存并在可证明的因果条件满足后发布，不能制造假ACK。turn/completed不能把未结、错配或响应未知的请求推成成功。进程、候选、独立测试、人工验收继续分别处理。

## 回写与公共接口

`CodexApprovalObservation {sequence,params_sha256,decision?,decision_observed,reply_started,reply_written,request_resolved,item_status?,failure_code?}`为原机器对原权限记录的单调观察，不能代替人决定。新增原命名空间`POST /runner/tool-permissions/observations`及`/pool-runner/tool-permissions/observations`，携原Assignment、permission_id、稳定key与typed observation；当前机器／scope／lease／fence／Profile和原请求／决定均须相符。序号／回执同事务，重试恢复原观察，不覆盖较新事实。执行事件原通道可唤醒界面，正式事实以记录为准。

`CodexEvidence.approvals`保存有界`CodexApprovalEvidence`：准确permission_id、request_id／method／thread／turn／item／approval_id、params_sha256及上述观察。记录不重复命令／diff或私有路径。成功结果需所有实际请求准确闭合；平台按原持久权限和节点核验，不接受别的attempt或伪造人决定。平台决定、执行节点观察、写出响应、供应方清理与操作结果分开，网络结果未知均沿原操作核对。

## 界面与验收

当前节点复用紧凑“工具确认”区；命令／文件、影响、是否完整及允许动作优先显示，精确ID／摘要按需展开。中文／英文完整，390px单栏44px控件与可见焦点；刷新不启动进程，历史默认只读，撤权清理私有内容，原提交恢复不丢body/key。ACP同一新公共wire做明确适配，不放开未知字段。

定向覆盖共同权限域的当前授权／事务／原回执、协议分支、重复ID、不同请求同item、取消和乱序、异步锁及实际写入、普通／Pool与原生ACK；复用未变的质量和技能证据，不重复173。代表界面与受控CLI协议验证、当前原生schema／策略验证、真实厂商模型及完整规模分别记录。持续会话、其它服务端请求、完整审批策略和原平台范围继续实施。

参考：[官方app-server](https://learn.chatgpt.com/docs/app-server)；Multica固定`e8ae72c9dfc1b4474b61a9087f51b33cbee6a048`，HelixHarness固定`44ffc536f73d8af525b4795ad10384c516898576`。复用平台现执行与权限实现，无新增上游源码复制。
