# WorkBuddy云任务：原DAG节点委派与控制面单次提交

阶段：T-WORKBUDDY-CLOUD-152。状态：**领域、HTTP、Runner控制面客户端和原运行节点工作区已实现并完成限定检查；专用公开领取与完整Execute链路继续接线，浏览器完成范围单独记录，不是完整工具可用或生产验收结论**。唯一目标仍为[现有实施计划](../docs/delivery/implementation-plan.md)；本合同接续[143／149协议底层](workbuddy-cloud-submission.md)及[147本人授权连接](workbuddy-connections.md)，不建立第二调度器。不能据此将工具目录的WorkBuddy云入口改为ready或健康。

## 最小交付闭环

本人在已有项目、run与node上确认固定请求，委派自己持有的WorkBuddy授权；原DAG把该节点分配给真实Runner。Runner使用现有机器身份和完整Assignment向控制面申请一次创建，控制面重验当前资格后持久准备、单次调用供应方并保存准确task_id。后续仅查询该task_id，公开安全事实；长期OAuth令牌、ACP票据和连接均留在控制面。

本阶段限一次Create及准确Get，不同时开放追加prompt、权限选择、Cancel、任意RPC、产物下载或候选导入。创建正文可能已被供应方执行，禁止在Attach后再自动Prompt同一正文。控制面记录task_known不代表DAG节点成功；供应方status、回合结束、代码候选、独立测试和人工验收各自保留来源。

## 当前可复用实现与必须收紧之处

| 现有实现 | 本阶段复用及边界 |
| --- | --- |
| `command_authority.go`的`authorizeTx`、`commandReceipt`与`planningAuthority` | 真人委派、撤销和安全读取沿当前身份与项目权限，不接受JSON构造Actor。委派使用明确的项目写目的，浏览器仍保留当前用户／组织写入头及原回执规则。 |
| `command_delegation.go`的锁定顺序、`currentProjectMembershipLockedTx` | 新委派在锁定完整用户／组织协调集合后重验原owner当前项目写资格；该membership函数自身不加全套协调锁。新增私有领域proof，不能把owner_id拼成真人Actor，也不能冒用已有automation的purpose。 |
| `candidate_transfer_store.go::candidateAuthorityTx` | 复用项目Runner及共享池的真实身份、原节点spec、worker／attempt／fence／DB租约校验。该函数容许run paused；云任务新建必须额外要求run与node均running。 |
| `repository_integration_authorization.go::repositoryAuthorizationCurrentTx` | 复用机器凭据hash、完整Assignment以及当前双边共享池授权核验模式。新外发须精确匹配共享池grant版本及credential版本；旧callback允许排空历史事实，不等于新外发资格。 |
| `workbuddy_connections_store.go::rowTx`、`workbuddy_connections_crypto.go::open` | 控制面在当前权限与绑定检查通过后读取／解密原owner连接。连接配置匹配、实际scope、grant代次、状态及过期分别检查，不能只看connected。 |
| `internal/workbuddy::Client.CreateTask/GetTask/CreateRequestDigest` | 固定官方方法、地址、摘要、响应验证和错误脱敏，无供应方幂等／自动刷新。机器不能指定endpoint、header、token或新正文。 |
| `usage_accounting_hooks.go::CheckAttemptTx/ReserveAttemptTx/RecordResultTx` | 沿唯一DAG attempt账务及原组织／项目策略；新增云节点必须显式纳入绑定检查，不能因`dag.ToolBinding`尚未识别而走“无绑定即放行”分支。 |
| 143 Runner本地journal | 可作为本机诊断，不能作为跨副本发送资格的权威；本阶段控制面事务记录拥有一次外发权。不能再并行执行旧的本地Create组合。 |

## 固定对象与新增类型

领域服务持有现有`WorkBuddyConnections`与同一个`Service`，不能复制密钥、连接表或运行数据库。Runner／控制面共享`internal/workbuddycloudcontract`，平台可直接import或type alias，不复制另一份DTO。

原不可变节点固定为`Type=external_workflow`、`config.plugin=workbuddy-cloud`、`config.workflow=workbuddy-cloud/v1`；`config.workbuddy_cloud`只能包含`{connection_id, grant_generation, request:{prompt,name}}`。其中name可缺省。`NodeBinding`与`ParseNodeBinding(raw any)`只校验这份结构，调用方另外核验节点Type／plugin／workflow及当前权限。编译器要求plugin并不等于已启用可执行能力；本阶段不自动注册`external_workflow:workbuddy-cloud`为完整工具。

服务端只从原节点还原正文；`node_spec_sha256`固定为现有`digest(stringify(spec))`，其中spec为解码后的完整`dag.Node`，对应`json.Marshal(typedNode)`字节的SHA256，不是数据库原JSON排版字符串的摘要。`NodeBinding.RequestDigest()`复用`workbuddy.CreateRequestDigest`，包含实际prompt和name，不能用客户端摘要取代服务端复算。

**CloudDelegation**：`id, project_id, owner_id, connection_id, version, run_id, node_id, node_spec_sha256, connection_version_at_issue, grant_generation, client_binding_sha256, request_sha256, expires_at, max_creates, creates_consumed, state, created_at, updated_at`。state仅active／revoked；到期根据DB时间读取，不为每次查询写业务版本。`max_creates`本阶段必须为1，不能由模型提高。请求摘要覆盖经过143同一校验的完整CreateRequest（prompt及name），同时绑定原节点准确spec；不把摘要当已审阅正文的替代。

**CloudSubmission**：`id, project_id, delegation_id, version, run_id, node_id, assignment, request_sha256, state, task, observed_at, last_error, usage_state, created_at, updated_at`。Assignment复用`candidatecontract.AssignmentRef`完整结构，公开投影省略机器凭据hash。state仅prepared／unknown／task_known；供应方status留在`*workbuddy.Task`内，不混成平台state。prepared／unknown的task必须缺省且observed_at为空；task_known要求准确Task及观察时间。usage_state固定unreported，数值和金额缺省，不填0。

**CloudDelegateRequest**：`project_id, connection_id, expected_connection_version, grant_generation, run_id, node_id, expected_run_version, node_spec_sha256, request_sha256, expires_at, max_creates, idempotency_key`。owner、tenant、实际scope与固定正文由服务端来源读取，不能由请求自报。期限必须由调用方明确提供且在DB当前时间之后、不超过24小时；这是本地授权边界，不是供应方期限。相同节点已存在委派时不另建平行委派。

**CloudCreateRequest**：`project_id, delegation_id, expected_version, assignment, idempotency_key`，预期版本指委派。**CloudRefreshRequest**：`project_id, expected_version, assignment, idempotency_key`，submission ID只在准确路径参数中，预期版本指提交记录。**CloudRevokeRequest**：`project_id, expected_version, idempotency_key`，委派ID在路径参数中。Create及Refresh均不接受prompt／task_id／URL；控制面从原节点和提交记录推导，不提供通用action。共享池tenant及pool_runner_id由Assignment原字段承载，不再额外接收一组可矛盾的顶层身份。

**CloudObserveRequest**：`project_id, expected_version, idempotency_key`，submission ID只在路径参数中。它表示原owner本次明确读取原供应方任务，不接收Assignment或机器身份，不能Create。**CloudNodeView**：`project_id, run_id, node_id, run_version, node_spec_sha256, request_sha256, binding, delegation?, submission?, allowed_actions`。binding来自原run.spec，属于当前项目run读取范围，不含OAuth秘密；delegation只对原owner返回，submission为当前项目可读的安全事实。所有关联对象必须同项目／run／node、相同请求摘要；本人delegation还核同节点spec与连接／grant。AllowedActions按delegate、revoke、observe顺序取子集，无重复；revoke需存在active委派，observe需存在task_known提交。形状校验不代替服务端当前授权。

所有DTO提供`Validate() error`，委派另有`ValidateAt(dbNow time.Time)`；校验只判断结构，不建立当前权限。公共记录版本允许1–2147483647，新写入请求预期版本允许1–2147483646，更新CAS前仍须由服务端按完整动作预留足够增长余量。operation ID沿现有8–128字节规则；`Decode(raw,out)`复用现有严格JSON合同，拒绝未知／重复／大小写别名、null标量及非法Unicode。平台自有时间为RFC3339 UTC；供应方Task的created_at／updated_at保留合法RFC3339明确时区偏移，前端使用单独校验，不强制改成Z。request_sha256及spec摘要使用固定64位小写十六进制。原正文无法还原或摘要不符即拒绝，不从名称、讨论最新版本或模型输出猜测。共享池Assignment必须保留完整字段且与顶层project相同；真实凭据、scope状态与租约由控制面再核。

## 冻结窄接口

所有路径均在`/api/v1`之下。实现时沿原HTTP身份解析区分项目Runner与共享池机器，必要的两套transport入口调用同一内部函数；不能把pool token先转成真人。

| 方法与路径 | 输入及当前资格 | 结果及重放 |
| --- | --- | --- |
| POST `/workbuddy-cloud-delegations` | DelegateRequest；真人项目write且本人连接，准确run/node属于同项目 | 201安全Delegation；同key同正文返回原回执，当前读／对象资格仍需检查 |
| GET `/workbuddy-cloud-nodes?project_id=...&run_id=...&node_id=...` | 当前真人项目read、准确原run/node | 200 CloudNodeView；页面重开从此定位原委派／提交，不靠本地最后一次操作记忆，不触发供应方调用 |
| POST `/workbuddy-cloud-delegations/{id}/revoke` | project_id、expected_version、idempotency_key；原owner当前写权限 | 200撤销记录；只阻止后续资格，不声称远端停止 |
| GET `/workbuddy-cloud-delegations/{id}?project_id=...` | 当前owner、项目read | 200原安全委派；不返回OpenID、令牌或授权明文 |
| POST `/runner/workbuddy-cloud-submissions` | 强类型Create请求；真实机器当前Assignment、委派与owner资格 | 首次创建持久成功201；已知同一结果200；prepared／unknown返回409及安全恢复引用，不重复供应方POST |
| POST `/runner/workbuddy-cloud-submissions/{id}/refresh` | 强类型Refresh请求；完整原Assignment及当前资格 | 200安全已知任务观察；只GET原task_id，无Create。观察成功也按CAS更新版本与回执 |
| GET `/workbuddy-cloud-submissions/{id}?project_id=...` | 真人当前项目read且可读原run/node | 200已持久安全事实；不触发厂商请求、不要求原机器租约仍有效 |
| POST `/workbuddy-cloud-submissions/{id}/observe` | CloudObserveRequest；原delegation.owner本次浏览器身份与当前项目write | 200准确原任务观察；独立真人命令，无机器冒充、无Create；锁外GET加版本CAS／原回执 |

暂不新增全量列表：从原run/node关联准确submission。接口不发可复用供应方ticket；既有机器凭据加短期Assignment租约和有限委派已是调用边界，无需再建立“凭ticket任意调用”的网关。若共享池当前HTTP路由必须单列，冻结其准确路径时保留同一领域命令，不扩大权限。

拒绝语义沿原APIError：400参数／合同不符；401当前凭据无效；403有身份但当前操作不允许；私有owner或跨项目对象按既有规则404；409版本、原依据、已消费或待核对冲突。供应方失败只返回安全码和是否需要核对，不能直传响应正文。读写均no-store。当前撤权时不能因知道operation ID继续读回执。

## 原运行页读取与交互合同

工作区已挂入原运行选中节点详情。识别依据是该运行准确的workflow_id／workflow_version、不可变定义中的节点ID及固定type／plugin／workflow，不使用最新修订、显示名称或NodeRun配置猜测。工作流缓存按项目隔离，当前caller、project、run和安装定义对象必须对应；草稿未发布、准确定义不可用或运行读取被拒绝时不挂载云工作区。已打开子工作区的身份／主来源拒绝移除受保护详情；其他后台读取失败保留已核实事实、显示过时状态并暂停新写入。

正常状态的次级“读取当前记录”和原运行共享快照run.version变化，只重新GET准确CloudNodeView及本人连接，读取平台已保存事实。相同版本重复呈现不额外读取；在途读取合并，期间新的版本或明确读取意图补读下一份快照。不增加卡片轮询、不自动调用供应方、不刷新OAuth，也不形成新的委派／创建／观察命令。中央云记录可能晚于运行快照提交，明确记录读取入口继续保留。

Workspace的revision提示不进入原用户／组织／项目／run／node作用域键。来源重读保留本scope草稿，依据变化撤掉确认；正常重读与共享快照均不能解除未知操作锁。本人明确“查询原云任务”使用Observe；未知命令仍只由原path／body／idempotency_key核对。恢复存储不含prompt或供应方秘密，存储失败时不外发新写入。

授权与期限字段按侧栏容器宽度排列，窄容器单列；平台记录读取保持次级操作。运行／节点上下文保留完整DOM与title、视觉最多两行，路由、作用域和回执继续使用完整身份。

## 授权与事务时序

**委派事务。** 依原锁顺序核验当前真人、租户、项目write、本人connection、准确run/node和预期版本。连接须同app／client绑定、connected、grant_generation相等、实际及当前配置允许的scope同时包含`user.task.invokable`与`user.task.readable`，已知令牌期限有效；不隐式刷新。读取并校验固定请求摘要，锁定组织和项目用量策略，拒绝任一enforce策略，写委派／原回执／审计同事务。委派只授权消费该节点，不启动DAG、不改负责人、不自动发任务。

委派持续性由明确owner grant拥有，不依赖浏览器页面仍打开。内部绑定固定connection ID／owner／OpenID／appDigest／grant_generation／规范化实际scopes，OpenID不进入项目公开投影。每次外发仍重验owner存在且有当前项目write、委派未撤销／过期，以及当前connection身份／app／grant。普通同账号、同scope刷新可增加token_generation和connection.version；这两项作为每次外发观察记录，不应使已批准grant天然失效。重新授权、换账号、断开或scope变化必须依grant_generation／绑定使旧委派失效，不凭新token恢复旧资格。机器路径只读并使用原grant，不调用仅接受真人owner的saveRowTx改写它。

**准备事务。** 使用真实机器subject和完整Assignment；先收集机器与原owner参与的完整用户／组织集合，沿既有一致顺序取得协调锁，不能在已取得项目／运行锁后逆序补锁owner。复用candidateAuthorityTx的检查后，额外核run running、固定云节点类型／绑定、owner当前membership、精确pool grant版本、当前connection及两级预算策略。原run/node不可由不同delegation或新的attempt再次创建。核对业务回执，锁委派／submission，原子消耗唯一一次额度并写prepared、完整assignment摘要、machine credential hash、准备时token_generation及凭据绑定；同事务写安全回执、审计和必要outbox。提交成功才有一次Create资格。

**外部调用。** 数据库锁外调用一次143强类型客户端。只有本次成功插入prepared的活跃调用栈持有内存发送资格；重复HTTP、另控制面、超时重入、服务重启都不能从prepared重建发送资格。prepared后即使本地取消、token恰好过期或配置失败，也不能自行删记录／退次数以重试。无法以数据库事务和外部服务构成原子提交；准备提交是本地授权线性化点，撤权与已准许外发之间存在在途窗口，不能承诺即时取消已发送请求。

**回执事务。** 返回准确Task后，按原prepared ID／请求摘要／完整Assignment及固定消费凭证CAS写task_known。即使发送后的owner撤权或租约到期，内部收尾仍可保存这一次已授权外发的安全回执，防止丢失task_id；它只记录原事实，不获得再调用厂商或向撤权者返回材料的资格。此私有收尾不得暴露为允许机器任意填task_id的公共接口。保存失败保持待核对，不能将可返回的安全task_id当作已持久成功。

创建返回错误、无准确ID或响应无法保存进入unknown；崩溃遗留prepared在读取中显示待核对，不由后台重发。不存在供应方按operation ID反查合同，因此不提供“重试创建”按钮或按相似名称自动绑定。重启恢复只能读取持久事实；已知task_id后的查询失败保留原ID与最后观察时间。

**准确观察。** Refresh在短事务重验原机器、owner和当前grant／scope、当前原run/node/attempt/fence/lease及submission版本，只固定原task_id；锁外GetTask；第二事务以读前版本CAS记录安全Task与DB观察时间。并发后来的读取不可被旧响应覆盖；CAS冲突返回原恢复入口，不重发Create。正式终态／租约过期后，当前用户仍可读取已保存事实；本阶段没有绕过机器lease去继续云调用的隐式通道。

**本人明确核对。** Observe使用当次浏览器身份、当前项目write及原delegation.owner，核对原submission与task_id，以及当前原connection的owner／OpenID／app／grant／scope／令牌期限，再锁外执行GetTask并CAS保存。旧委派revoked／过期、原machine lease到期或run终态不妨碍本次本人明确只读核对；它不复活原委派、不恢复机器资格，也不Create。当前connection断开、换grant或scope不符则拒绝。enforce预算不阻止此原任务只读核对。读取回执依然核当前人／原对象资格；平台无法重新取得原grant时保持已存事实与核对入口，不能换凭据查询别人的任务。

## 最小持久结构

使用两个新增组件表，沿`Service.Initialize…`及component baseline注册；先检查注册、完整列、主键、唯一约束与版本，再允许写入，拒绝未知既有schema，不reset／回填假历史。

1. `suma_workbuddy_cloud_delegations`：上述委派字段，加内部tenant_id、app_digest及准确来源绑定；PK id，UNIQUE(tenant_id, project_id, run_id, node_id)。撤销为墓碑，不删除后复用同节点。版本、max_creates、creates_consumed有界，跨副本在同一权威数据库执行CAS／唯一约束。
2. `suma_workbuddy_cloud_submissions`：上述提交字段，加tenant_id、完整assignment_json／assignment_sha256、worker_credential_hash、token_generation_at_prepare、prepared_at、私有准备消费标记、last_observation_operation_id；PK id，UNIQUE(tenant_id, project_id, delegation_id)，另UNIQUE(tenant_id, project_id, run_id, node_id)。唯一键不能仅含attempt/fence，否则重试换attempt会重复创建。不存在接口重置prepared、替换task_id或改绑另一节点。

operation回执沿`suma_operations`，审计沿现有审计机制；不新增任务调度表、余额表或供应商token表。SQL使用现有`s.q`／`currentSQL`／`ddl`及项目guard。新增表只拥有委派和外部调用事实，不能直接修改别域的任务验收／测试／发布状态。

## 用量与事实展示

组织或项目任一策略为enforce时，云委派／准备均拒绝，错误明确为“当前云入口尚无可证明的硬预算能力”。通过observe或无强制预算策略不代表消费免费，不能借未绑定profile、未定价或已预留次数绕过检查。

首次实际attempt沿现有账务保留unreported与未解决状态；请求次数上限1只是平台外发次数，不是供应方token／金额上限。143的Task没有费用数据，不能生成零费用ToolEvidence或“未启动”证明；ACP上下文使用量与累计cost也不能在未来直接冒充完整供应商账单。

UI／读取投影分别表达：已委派、待执行、已准备待核对、供应方ID已确认、观察更新时间／查询失败、未上报用量。没有持久task_id时不生成厂商跳转链接；有task_id也不公开ACP URL或票据。owner连接私有字段不能随项目可见的run投影泄露。

## 本闭环验收边界

至少用真实本地HTTP、SQLite持久重开及独立服务实例覆盖：双控制面竞争只有一次Create；同key不同正文冲突；不同key／不同attempt不能为同node再Create；prepared写失败零外发；准备后崩溃／HTTP响应丢失／回执保存失败均不重发；task_known只Get准确原ID。

授权测试使用真实Actor／Runner／pool凭据，覆盖跨租户、owner降权／移除、grant变化、token同grant刷新、scope收紧、池双方撤权／版本变化、run paused、node非running、旧attempt/fence、租约到期和DB时间到期；回执重放仍检查当前资格。发送后撤权允许私有原事实收尾但不能新增外发或泄露读取。未知费用与两级enforce拒绝需有真实断言。

当前共享合同类型／校验、控制面领域、HTTP身份路由、Runner窄客户端与原节点工作区已接入，阶段实测数量和浏览器完成范围只由集成质量证据记录。WorkBuddy专用公开Runner领取、Execute组合及完整执行恢复仍待接线；真实WorkBuddy账号／厂商、ACP进度持久化、云候选导入、独立测试、正式验收、多物理机竞争及生产可靠性继续待验，不能以协议fixture或本文完成代替。
