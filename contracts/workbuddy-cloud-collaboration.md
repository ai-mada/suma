# 原云任务会话协作合同

T-WORKBUDDY-CLOUD-COLLAB-164；产品1.0.0；2026-10-31。状态：原会话显式加载、私有存档与原请求恢复已实现，并完成Windows／SQLite／受控SDK的HTTP与真实App限定验收；[证据](../docs/quality/workbuddy-cloud-collaboration-verification.md)分别记录范围和异常。负责人：控制面、工具接入、工作台与主集成。关联[原云任务执行](workbuddy-cloud-execution.md)、[149协议底层](workbuddy-cloud-submission.md#准确会话读取与acp通道)、[状态呈现](workbuddy-cloud-status-ui.md)。完整协作目标保留；本阶段实现原会话加载快照及当前授权读取，不把它计为完整人工协作。

## 正式边界

使用原已知提交的Task.ID，经现有`GetSessionAccess`准确GET核对；149将SessionID绑定为同一task_id，只接受固定官方ACP origin及准确路径。临时票据、连接ID与RPC引用仅在控制面，不下发浏览器、不写普通日志或快照。复用固定ACP Go SDK及现有initialize／session/load，绝不再发送原创建正文。

加载包含临时连接和session/load POST，不能称为供应方无副作用的HTTP只读。只有明确的“连接并加载原会话”动作触发；页面打开、重读存档、Run更新及刷新浏览器均不自动Attach。此动作不发送Prompt、不回答权限、不发送Cancel，也不建立新调度器。

已匹配的load RPC回应和HTTP发送均成功，仅证明本次加载确认。现协议没有可证明的供应方历史分页、水位、完整覆盖或自动续传，因此coverage固定`unverified`；`loaded`不能解释为完整历史、云任务完成、代码候选或验收成功。平台本地序号不能作为供应方游标。现DAG已有外部等待事件，快照不代替原attempt／fence事件，不自动清除waiting_input。

## 资格与来源

外读命令复用当前真人planningAuthority与cloudObserver：当前项目write、原提交／委派owner、原run/node/spec/request摘要、准确task_id、当前本人OAuth grant代次与客户端绑定。原机器租约不作为本人加载依据，也不恢复它。原执行委派的消费、到期或撤销不自动授权；本人此次明确加载是独立动作，沿现cloudObserver规则核对当前账号连接，不复活机器创建资格。

外部调用前、每次持久采集和最终发布重新核对当前资格。旧资格不能因票据仍可用而延续。锁外网络I/O，事务内收齐当前用户／组织／项目锁、核来源并写快照；不持数据库锁等待网络。

存档GET要求当前真人项目read、准确原提交owner及同一来源。没有当前项目访问权或来源不符即拒绝并让界面清除正文；只知道快照ID不赋权。存档是已有项目资料，不自动要求旧机器仍在线，也不以存档重建OAuth资格。其他成员不能自动继承本人云会话访问权；所选观察复制到任务讨论沿[独立共享合同](workbuddy-cloud-history-sharing.md)明确目标与副本范围，本存档仍不自动开放给团队。

`CloudCollaborationSource`闭合字段：`project_id,run_id,node_id,submission_id,submission_version,task_id,node_spec_sha256,request_sha256`。来源由服务端还原，客户端不得填task_id、URL、Prompt、ticket或任意packet。submission_version保留加载起点；后续仅观察更新版本且同一身份／摘要不变，不使已经保存的快照无故失效。新加载开始核对准确当前submission_version，发布仍重新核身份、摘要与当前资格。

## 冻结DTO与接口

所有DTO严格拒绝未知／重复字段、null标量、非法Unicode、非法版本及超限内容；沿原错误信封和UTC平台时间。省略表示不存在，不生成伪造的供应方事件时间。

- `CloudHistoryLoadRequest`：`project_id,expected_submission_version,expected_version,idempotency_key`。expected_version为协作head版本，首次明确0；必须存在且为非负整数。其余写版本沿现上限。
- `CloudHistoryLoad`：`id,version,operation_id,head_version_at_start,source,state,coverage,started_at,deadline_at,overdue,finished_at?,entry_count,unsupported_count,text_bytes,error_code?`。operation_id固定原请求idempotency_key，head_version_at_start固定原expected_version（包括首次0），两者不随采集改变；load.version不是head版本。前端同时核对它们、固定source身份和原expected_submission_version，不能只凭HTTP成功消费回执。state仅`loading,loaded,interrupted,limited,rejected`；coverage仅`unverified`。deadline_at为平台UTC截止，overdue仅按本次DB时间派生，只有loading可为true，不改变正式state／version；终态为false，浏览器时钟不裁决过期。错误仅闭合安全码，不含原始协议错误。finished_at为平台采集结束，不是消息发生或远端任务结束。
- `CloudHistoryEntry`：`sequence,kind,text?`。kind仅`user_text,assistant_text,tool_summary,plan_summary,unsupported`；sequence从1递增，只在该load内稳定。text仅对应受支持的字面文本。`unsupported`不携原始packet、URI或正文。
- `CloudCollaborationView`：`source,version,latest_load?,last_loaded?,allowed_actions`。allowed_actions只允许`load_history`，按本次当前资格给出；空数组不等于没有存档读权。latest_load表示最近尝试，last_loaded保留最近确认加载；失败不抹掉原观察。
- `CloudHistoryPage`：`load,items,total,next_after,has_more`。items始终为数组；after是本地序号，limit默认50、范围1–100，未知query拒绝。total／has_more只描述本地已存条目，不证明供应方历史覆盖。

| 方法与路径（/api/v1） | 当前资格及语义 | 回执与重放 |
| --- | --- | --- |
| GET `/workbuddy-cloud-submissions/{id}/collaboration?project_id=...` | 当前owner项目read；读取来源、head及已存观察 | 无外部调用；首次version0、无load，不称为空历史 |
| POST `/workbuddy-cloud-submissions/{id}/history-loads` | 当前owner项目write及cloudObserver；明确连接原会话 | 200返回准确load，先持久固定操作／来源，再锁外加载；不代表任务完成 |
| GET `/workbuddy-cloud-history-loads/{id}?project_id=...&after=0&limit=50` | 当前owner项目read及原来源资格 | 仅分页本地存档，无Attach、Prompt、权限回复或取消 |

所有响应no-store／no-referrer，拒绝机器凭据冒充真人。400合同非法、401无效身份、403当前资格不足、私有／跨项目对象404、409版本／依据／并发冲突；供应方或采集失败保留安全load状态，不泄露返回正文。

## 持久性、幂等与限额

head、load及entry分表，由当前platform基线管理并执行新域初始化前schema guard；已有不完整／不符表必须写前拒绝，不重置数据或做历史兼容回填。绑定租户、项目、owner、原提交及稳定操作正文摘要；相同key恢复原load，相同key换正文冲突。外发前持久准备，任何重放不得再Attach；未知发送结果不能生成新key自动重试。

head.version仅管理加载尝试及发布关系，load.version管理该次采集；同一owner／原节点最多一项有效loading，项目同时最多四项。按DB时间保留30秒总截止、每次最多500条、文本总计1MiB、单项文本64KiB；超限终止本次采集并标limited，保留已授权前缀，不静默截断或冒充完整。超时／失联保存interrupted或明确过期loading，下一次明确操作先事务fence旧采集，旧回调不能追加／发布。

消息按本次收到顺序逐条持久，不能按相同文字去重。只展示用户／assistant文本、工具标题与状态摘要、计划条目摘要；思考内容、任意工具参数／原始结果、外部资源、artifact URI和未知扩展不直接存入展示正文，不自动访问链接。未支持事件以有界unsupported记录计数。原始事件时间缺失时不生成当前时间替代。

HTTP／RPC加载均确认且最终授权／CAS成功才将load标loaded并更新last_loaded。中途失败或授权变化不能发布为成功；保留最近已加载快照及每次尝试的准确状态。请求断线／进程重启不自动重连；用户可沿原key核对原load，新的加载必须是明确新动作。关闭本地连接不能标远端停止。审计只记来源、load和计数，不向普通activity或搜索索引投放会话正文。

## 工作台与多语言

在现原节点云工作区挂独立协作区域，不重复读取本人连接，不另建常驻导航。默认读取当前有权的已存投影；外部加载用次要按钮，保留原Panel主操作。独立actor／组织／项目／run／node边界：切换、401／403或来源失效清除自身受保护内容，旧返回不能覆盖新范围；会话域拒绝不能擅自清除仍有权读取的原节点事实。

尚未采集、此次返回无可展示记录、加载中、确认加载、limited、interrupted、旧快照、无资格和读取失败明确区分。本次加载范围始终标“覆盖未核实”；本地分页不称远端历史翻页。正文纯文本、有界分页，不执行其中工具／代码或自动加入模型上下文。中文／英文配对，原正文及标识不自动翻译。

## 完整人工协作的后续门禁

| 能力 | 必须满足的正式语义；本阶段无对应可用API |
| --- | --- |
| 追加输入 | 明确新Prompt回合、固定来源／预算和稳定平台操作；ACP无幂等保证，sent／unknown／匹配原请求stopReason分开，未知不可自动重发 |
| 权限处理 | 原实时连接、回合、请求及准确option绑定；禁止自动批准，旧引用不可用于新连接，发送完成不等于远端已采用；持久待决对象与当前权限另行冻结 |
| 等待输入 | 历史文字不制造可回答请求；现codec未证明独立用户输入请求回调，未收集不能显示“没有待决请求” |
| 取消／停止 | SDKCancel仅针对本连接active Prompt，不能冒充停止原REST任务；本地断开、取消发送、原回合结束和远端进程停止分别记录 |
| 实时／分布式恢复 | 控制面连接归属、租约／fence、持久事件及失联核对；不把provider事件ID当平台水位，不自动接管未知副作用 |

验证覆盖准确来源／当前owner、原key丢回执不再Attach、外读中撤权／来源变化、超限前缀、旧回调fence、RPC／HTTP不一致及schema拒绝。源码、协议夹具、真实HTTP／GUI与供应方／生产分别记账；完整平台及P0–P7不因只读存档而关闭。
