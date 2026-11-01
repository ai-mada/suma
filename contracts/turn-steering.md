# 原执行追加指令

T-TURN-STEERING-188；suma1.0.0；原31／P0–P7继续。接续Multica明确评论steering能力，复用当前任务讨论、原DAG、模型预算、工具工作区与原单writer；不创建新turn、不resume旧thread、不替换原计划或验收基线。

## 类型与明确语义

共享类型以 internal/codexcontract/steering.go、internal/steeringcontract/types.go 为准。CodexSteeringSource固定protocol=codex-turn-steer-v1、thread_id、turn_id、binding_sha256。只有原配置及turn/start均ACK且未停止／关闭时，原Runner可公布live session；节点running或旧讨论run_id本身不是活turn证明。当前稳定Codex文本steer无需T187实验性问答开关，也不增加模型／私有配置或profile字段。

SteeringSession绑定project/task/run/node/attempt/fence/worker/dispatch/profile、source、version、state=active|closed、opened_at／expires_at与allowed_actions（仅send）。可发送动作由服务端当前身份、任务、当前机器／库存／Grant及lease计算；active记录不单独宣称在线。

SteeringMessage是原已保存本人human消息的准确ID／sequence／author_id／body／body_sha256；不自动沿普通评论触发。SteeringInstruction固定原session与执行tuple、message、actor、期限、version、可选观察。每session同message唯一，最多32条；再次补充须另存新消息。正文完整保留，与原讨论相同的32,000 UTF-8字节／8,000 Unicode字符上限，不裁剪；不支持秘密／凭据输入。原讨论、知识与技能权限仍分别处理。

SteeringContext返回task_id／task_version、准确message、targets、instructions、can_send及可选reason。reason闭集：not_own_message、task_accepted、no_live_turn、current_execution_unavailable；不返回原私有Record。浏览器只提交SteeringCreateRequest：project_id、task_id、message_id、expected_task_version、session_id、expected_session_version、idempotency_key；完整原生地址与正文只由当前服务解析。

## 接口与权限

GET /tasks/{id}/execution-steering?project_id&message_id 返回SteeringContext；POST /task-execution-steering 返回SteeringInstruction。只允许当前浏览器项目写者将本人human消息送给同任务当前未验收原运行的准确session；API／Runner／助手消息不冒人发送。读取和回执重放始终先核当前人权限。原提交回执先于fresh task/session版本、状态、截止与活执行检查；相同key不同body拒绝。

普通与Pool各有四个机器POST（Pool使用/pool-runner）：/runner/steering/sessions（Open）、/runner/steering/poll、/runner/steering/observations、/runner/steering/close。完整普通Assignment或PoolRunnerAssignmentRequest证明与原成熟机器边界一致。

Open request含project_id、assignment、source、idempotency_key；poll含project_id、assignment、session_id；observation含project_id、assignment、session_id、instruction_id、idempotency_key、observation；close含project_id、assignment、session_id、idempotency_key。Open返回SteeringOpenResult：有session时无reason；原身份／完整scope有效但不是受管任务开发执行时，仅reason=not_task_execution，不伪造session／cap，不影响原普通工作流。权限、数据库或未知HTTP错误不得归入此正常结果。Close返回SteeringSession，Poll返回SteeringPoll，Observation返回SteeringInstruction。所有200；生成Schema严格拒未知字段。原HTTP结果回传继续沿旧Result，不新派发器。

新增领域存储与索引由控制面深模块负责，原操作回执同事务保存。初始schema当前声明、唯一性及无写预检与现规则一致，不迁移旧形状或重置数据。机器当前身份／完整原scope先核；已经提交的exact观察／关闭回执在fresh live门之前恢复，不能复活过期身份。fresh poll／write需复核原提交者当前资格、原任务运行、预算／pause与lease；已开始真实事实不错误套新work门。

## 原生写入与证据

原native turn/steer参数只为threadId、expectedTurnId、input=[{type:text,text:准确原正文}]、clientUserMessageId=instruction.id；此ID不被当成厂商幂等保障。共享BuildSteeringParams／SteeringParamsDigest生成确定JSON摘要，不接受其他原生参数或模型／沙箱覆盖。独立pending ACK映射共用原单调client RequestID，不能覆盖启动waitingID；不能与server发来的approval／input ID混同方向。

CodexSteeringObservation有单调sequence、原typed request_id、params_sha256、observed、write_started、write_written、native_accepted、native_rejected、turn_closed、可选固定failure_code。写前原意图持久化→当前平台复核→原singlewriter；完整write与native ACK独立。ACK必须是原请求且turnId准确相同；RPC拒绝与unknown区别。ACK可早于write回调，暂存／累积序号不能碰撞。原native关闭在新write之前到达则不发送，已开始写入的迟到事实保留。

未知write／ACK不能重发native、换key自动重试或新开回合；只重传原HTTP及观察。Session在原turn结束／停止后关闭，未派发项如实未送达；原观察和Close回执先于Result重放，普通／Pool一致。成功Result须与所有原登记指令、持久观察及session闭合匹配；unknown阻止成功，native拒绝或未派发关闭不伪装接受。

CodexSteeringEvidence只含session_id、source、closed、各instruction_id／message_sha256／可选观察，不复制正文。原模型预算、工具授权、代码候选、独立测试及人工验收保持各自事实，native接受不证明模型遵从。

## 简洁交互与所有权

原讨论human消息旁加一个明确入口，一次只读所选消息的权威context，不能每行轮询；选择准确目标、原文预览后单独发送。原生接受／平台保存／写出／未知／未送达分别展示。未知意图按user/org/project/task恢复，原意图保存完整旧session／message／tuple／body／key，新attempt不能掩盖原待核对请求。当前撤权清空私有内容，源切换及迟到响应保持原epoch门。与现讨论回复、据此开发、草稿guard分别聚合busy，不能互相false解锁。中英、390px、键盘沿原组件，不新增一级页面或desktop开关。

Root拥有共享cc／sc、生成合同、文档、最终集成及Git；控制面拥有新领域、数据库声明、普通／Pool HTTP与结果核验；Runtime拥有liveSession、原Journal／singlewriter／ACK、Client／普通Pool恢复及Worker生命周期；Web拥有原消息入口／单一Panel／模型／双语。只跑新增和直接受影响检查，一次最终类型／词条／组件构建与代表交互，真实CLI＋自有受控模型另行记证。

参考固定本地Multica提交5a2cc891ac338eed1919aa11a1eef5957cd82cec，以及[官方turn/steer](https://learn.chatgpt.com/docs/app-server#steer-an-active-turn)、本机174相同SHA已生成TurnSteerParams／Response。复用交互与串行活turn机制，原Multica timeout重试／新回合fallback不带入；未复制商用或受附加条件约束代码。
