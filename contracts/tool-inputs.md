# 运行中智能体问答

T-TOOL-INPUTS-187；suma1.0.0；原31项／P0–P7继续。接续 Codex原活turn、当前执行身份、原journal与单stdin writer；问答为补充输入，不属于命令／文件权限，不增加派发器或恢复旧模型会话。

当前共享输入领域已由[T190合同](cursor-inputs.md)扩展为Codex／Cursor独占来源；本文保留原Codex原生协议依据。通用送达投影与实际native TurnClosed分别保存，不使用旧版本别名或fallback。

## 当前协议与配置

支持原生 item/tool/requestUserInput。按本机已生成Schema：必填threadId／turnId／itemId／isBlocking／questions；autoResolutionMs可省略或null／非负安全整数；question.isOther与isSecret可省略并按当前Schema默认false，null或非bool拒绝。options可省略／null／空，统一为无选项文本题；原完整params摘要仍区分原报文。无startedAtMs或command/file item依赖。

每请求1–3题，问题ID唯一，最多12个不重复选项；ID100、header80、question4000、label200、description1000 UTF-8字节，文字完整不静默裁剪。isSecret=true整请求在Runner journal／HTTP前拒绝，控制面再次拒绝，不保存题目／答案正文；敏感凭据仍经原安全工具／模型配置。密码类问答的专有安全传输不冒充本次能力。

官方当前Default模式提问由default_mode_request_user_input实验性开关控制。新增本地Codex配置enable_user_input布尔项，当前默认false；明确启用后仅在本attempt独立CODEX_HOME写固定官方feature。配置探测须离线证明该flag受支持，公开profile仅在开启时带 user_input_protocol=codex-user-input-v1；摘要绑定新配置。不开启则不宣称可提问。界面明确实验性，不能暗改collaborationMode／experimentalApi、sandbox、网络、模型或预算；不修改用户全局Codex配置。

## 共享类型与答案

internal/codexcontract定义CodexUserInputQuestion／Option、CodexUserInputRequest（typed RPCID、method、thread／turn／item、binding_sha256、params_sha256、is_blocking、questions、可选auto_resolution_ms）、CodexUserInputSelection、CodexUserInputObservation和CodexUserInputEvidence。问题与答案只出现在受当前权限保护的问答领域，不重复进入执行日志／证据摘要。

Selection为question_id、kind=option|text|other|skip、可选option／text：option必须是原选项，可附说明；text仅无选项题且有正文；other仅原is_other=true，可附说明；skip不带文字。每次提交准确覆盖原题目，各题仅一项。平台从确定选择生成原生{answers:{id:{answers:[]}}}：option为准确原label，other为原生客户端的None of the above，说明／文本以user_note前缀加入；skip为空数组，不发null或伪造同意。答案摘要是该确定原生result JSON摘要，server重算。

internal/toolinputcontract定义独立ToolInputRequest（operation key、protocol=Codex协议、codex source）、ToolInputAnswer（选择、answer_sha256、actor_id、answered_at）、ToolInput（原scope／assignment identity／profile摘要、version、state=pending|answered|closed|expired、原request、可选answer／observation、created_at／expires_at、allowed_actions）、ToolInputList、ToolInputAnswerRequest。allowed_actions仅answer；跳过也经明确answer写入，不使用accept／decline或IsGrant。

## 控制面与存储

新独立suma_tool_inputs表及当前初始schema声明，复用原事务、当前身份、完整ordinary／Pool assignment、原派发、Profile／CodexBinding、当前库存与Grant；不扩ToolPermission的Choice或存储union。操作与原生ID唯一；同attempt的approval与问答合计最多32请求，跨类型重用RPCID拒绝。

接口：ordinary与Pool各有 POST /runner/tool-inputs、/runner/tool-inputs/poll、/runner/tool-inputs/observations（Pool使用/pool-runner前缀）；open携project_id／assignment／request，poll携project_id／assignment／input_id，observation携project_id／assignment／input_id／idempotency_key／observation。GET /runs/{run}/nodes/{node}/tool-inputs?project_id&attempt&fence 返回当前授权列表；POST /tool-inputs/{id}/answer 携project_id／expected_version／idempotency_key／answers。精确字段由同一Go合同生成。

当前人权限、原问题scope、版本、pending／截止与native闭合事实先核验，再保存答案及原receipt。回复截止取数据库时间、原节点deadline及15分钟上限，不采用客户端时钟。重复写恢复原回执，不能覆盖答案或较新观察。机器poll与实际写前复核当前执行资格；历史答案回执不意味着仍可发送。原结果／停止观察与新输入授权分别处理。

### 从终端核对工具问题

读取问题时，控制面在当前授权事务内核对持久化的原运行、节点、尝试号与 fence。错误的尝试或 fence 返回 404，准确尝试尚无问题时返回空列表。原历史尝试仍可在当前权限下查看。

`suma run inputs` 使用已有登录会话读取该范围，在终端输出问题标识、状态、已保存答复和送达阶段。题目、选项、答案和原始证据正文留在工作台。链接只定位原项目、运行、节点和尝试，浏览器沿当前授权读取。终端诊断与问题答复、运行续接及正式验收各用原流程。 [终端操作](../docs/operations/task-dispatch-options.md#核对智能体等待的问题)

## 真实写入与恢复

复用原typed RPCID、单writer与enqueueReply写前／写后回调。Runtime同attempt共用轻量ID→method／paramsSHA注册表，完全相同重复帧忽略，改正文／跨类型复用拒绝。原请求先持久化，再发平台；用户答案保存、执行器观察、入队、write开始、完整write及native resolved分别留证，入队不能算已回答。

Observation为单调sequence、params_sha256、可选answer_sha256、answer_observed、reply_started、reply_written、request_resolved、turn_closed、可选固定failure_code。resolved或turn_closed可在未答时发生，阻止后续新write；已开始的真实写入事实仍可回写，不倒退。isBlocking=false不伪暂停任务；回合结束会关闭问题，不能把native清理当人已回答。autoResolutionMs不触发平台自动选择、自动跳过或授权。

执行证据仅有input_id、原RPC／thread／turn／item、binding／params摘要、问题数量、is_blocking和观察，平台按原记录核对。unknown write不可自动重答；断线只重传原HTTP操作及观察，不重启模型进程，不resume旧thread。成功运行不要求每题都有人作答，但必须准确记录问题闭合与无未知写入。

## 简洁交互与交付

在原节点“工具确认”旁增加独立“待回答问题”；不新建管理页。List返回protocol和items，protocol由服务端按当前受保护节点与准确Profile摘要计算：codex-user-input-v1为已明确配置能力，空串为未配置，不表示在线。客户端可在已知Codex协议节点读取此能力投影，但不能仅凭旧协议标记已开启；protocol空且无历史时不显示问题区。已授权历史仍可只读，不能借历史标记当前能力。

1–3题直接选择／文本／Other说明，无默认提交；明确提交或跳过当前请求。原ID／摘要按需展开，答案已保存、执行器已观察、正在写出、已写出、原请求已关闭／待核对分别表达。历史只读；撤权／范围切换清理私有内容；未知提交保留原body／key，禁止盲目新写。

中英完整、390px、键盘与焦点沿原组件。Root拥有本合同、cc／ic共享类型、toolcontract capability、生成接口／初始schema公共声明、最终集成与Git；控制面拥有table／当前身份／HTTP／答案／观察／结果核验；Runtime拥有native解析／固定feature配置／checkpoint／writer／证据及探测；Web拥有节点面板、localCodex设置及模型／样式／双语。先合同后并行，只运行新增与受影响检查，一次最终类型／词条／代表交互；真实CLI与模型、受控协议、HTTP／SQLite和生产分别记录。

参考：[官方app-server](https://learn.chatgpt.com/docs/app-server)、[当前官方handler](https://github.com/openai/codex/blob/main/codex-rs/core/src/tools/handlers/request_user_input.rs)、[当前官方客户端](https://github.com/openai/codex/blob/main/codex-rs/tui/src/bottom_pane/request_user_input/mod.rs)及本机T174已锁定Schema。默认模式feature仍为上游开发中能力，不把协议夹具当真实厂商验收。
