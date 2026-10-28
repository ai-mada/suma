# HTTP、事件与客户端合同

当前任务协作新增[业务依赖图合同](../../contracts/task-dependencies.md)和[交付与质量投影合同](../../contracts/task-delivery-chain.md)：`/task-dependencies` 的完整图／原子 CAS 与 `GET /tasks/{id}/delivery` 的确切当前运行摘要分别建模；后者每类引用最多 20 条，不提供执行或验收授权。[Runner 本地诊断](../../contracts/runner-journal-inspection.md)是本机只读命令，不是服务端巡检接口。当前限定证据见[交付链路报告](../quality/task-delivery-chain-verification.md)。

## 多语言显示合同（实施中）

客户端通过 `Accept-Language` 协商显示语言，可用 `X-Suma-Language` 显式选择覆盖；首次支持 `zh-CN` / `en-US`，默认简体中文。有效浏览器会话的优先级为显式请求头、账户偏好、当前组织默认、语言协商及产品默认。账户和组织偏好通过独立GET/PATCH持久化，写入必须绑定原视图身份头、明确版本和操作标识；详见[精确偏好合同](../../contracts/locale-preferences.md)。响应 `Content-Language` 表示采用语言；不影响授权、业务版本、签名原文或不可变证据。

错误保留 `code`、`message`、`details`，增加 `message_key`、`locale`、`localized` 和可选 `message_params` / `fallback`。只有精确文案或明确消息键的翻译才标记 `localized:true`；旧具体诊断缺少目录时保留原文并明确回退，不能用通用 `invalid_input` 英文掩盖细节。日志、文件和用户内容不自动翻译。完整范围见[国际化规范](../design/internationalization.md)。

T-P4-007在共享池合同基础上增加账户／组织偏好，当前生成切面为212模型、152路径（加统一Error共213组件），并声明浏览器身份、组织角色和期望身份头。下方169/125为T-P4-005历史已验收切面，不能混称同一次验证。

`GET /localization/catalog` 是无需登录的只读编译目录，包含 `version`、`template_format:named-dot-v1`、默认与支持语言及消息声明。`version` 的SHA256同时绑定格式、回退语言和消息；目录无租户数据、配置路径、用户原文或秘密。占位符仅 `{{.name}}`，参数按 `text` / `identifier` / `integer` 声明校验，以纯文本显示；客户端不得执行Go模板或HTML。此目录用于即时重译已有错误和明确展示原因，不能用于替换用户内容和历史证据。

状态：当前v1实现；线协议Schema和TypeScript类型由`suma-contracts`生成。基础路径 `/api/v1`。浏览器同源HttpOnly Cookie；Runner/Agent/API使用分别绑定项目的短期Bearer凭证。

## 当前权限与原始页面上下文（T-P4-014 实施中）

应用服务接收从真实凭证解析的封闭 `Actor`，不接受客户端 Principal、角色、system 或授权标记。写入、成功回执重放、受保护读取和实时事件批次使用同一事务中的当前来源、凭证、组织／项目角色和数据库到期时间。无可信来源的旧会话保留审计记录，要求重新登录；旧 API／Runner／Agent 还需明确重新签发，不自动补造批准。

浏览器命令必须携带操作发生时页面的 `X-Suma-Expected-User` 和 `X-Suma-Expected-Organization`，账户全局偏好仅携带前者。缺失、重复、格式错误返回 `428 view_context_required`，与实际身份不一致返回 `403 view_context_changed`；这些头只比较原页面身份，不选择或授予身份。纯读取、明确的 POST 查询、机器接口和幂等退出不强加浏览器头。合法项目 API 凭证仍按自身范围执行，无需浏览器视图。生成合同通过 `x-browser-original-view` 表达条件要求，分类与处理器共用函数。

`POST /auth/tokens` 必须给稳定 `idempotency_key`，明确签发独立 24 小时 Grant。同键同正文恢复时在同一固定到期 slot 轮换秘密、撤销上一次秘密；响应仍是 token／expiry／label／scope／project。秘密不存入成功回执。普通浏览器退出撤销该 session family，不撤销独立 token 或已批准的持久 activation。网络结果未知时保持原操作标识与正文，不读取新会话后悄悄切换请求身份。

实现进度和验收边界见[当前权威性合同](../../contracts/current-credential-authority.md)。此段描述正在集成的 014 源码，当前已验收运行包仍是 013。

## 生成与校验范围

从仓库根目录执行 `go run ./cmd/suma-contracts` 生成 `docs/api/openapi.json` 和 `packages/contracts/src/index.ts`；`go run ./cmd/suma-contracts --check` 只检查文件是否与生成器一致，不写入文件。`go test ./cmd/suma-contracts` 覆盖字段展开、nullable工时、模型引用闭包、权限元数据、分页与漂移检查；`node scripts/check-docs.mjs` 检查本地文档链接和OpenAPI引用。

Go导出域模型直接反射，包括PlanningPeriod、TaskPlan、PlannedTask、CapacityEntry、BaselineSummary和NodeRun。PlannedTask嵌入的Record字段按真实JSON展开；指针工时保留`number | null`。组织治理与计划汇总等由map构造的响应、以及本轮新增请求字段，采用生成器中与HTTP处理器核对过的DTO，类型不会直接参与服务端请求解析。

生成器覆盖本轮新增路由的请求字段、基础取值约束和响应结构。旧接口仍使用部分通用请求信封，在OpenAPI的`x-request-schema-coverage`中标明。跨字段条件、UTF-8字节长度、当前权限、CAS版本、操作回执、真实身份关系、完成门禁和邀请秘密语义继续由服务端校验；JSON Schema与TypeScript类型不代表完整请求验证器。路由清单和map响应DTO仍是维护的合同，不是从全部HTTP分支自动推导；`--check`不证明全部处理器与合同自动一致。

## 项目知识与固定上下文（T-P4-015）

`/project-knowledge` 管理 memory／spec／skill 三种已审阅文本，使用当前 Actor、完整草稿替换、CAS 和稳定回执。发布／退役仅真实浏览器当前审批权限；Agent 不访问管理和历史路由，仅通过 `project.knowledge` 工具读取明确发布引用。`skill` 不是可执行插件，`extractive` 是文本提取，不代表模型提炼。

每次编辑／发布／退役追加不可变修订；新草稿保留旧发布，版本历史有独立分页。列表 q 最多 200 UTF-8 字节，单页最多 100，正文最多 16,000、来源最多 8。`TaskDispatchPreviewRequest.knowledge` 最多选 16 个 id／revision／sha256，服务器解析快照并在启动时复核当前发布与来源；完整 goal 仍最多 32,000 UTF-8 字节。来源原文留在历史，消费上下文仅传一次正文和来源摘要，避免重复占模型预算。完整语义见[项目知识合同](../../contracts/project-knowledge.md)。

## 请求规则

签名事件接收器是独立边界：`POST /hooks/{id}` 使用入口签名，拒绝浏览器Cookie/Bearer和任何query，不由调用者提供project_id。T-P4-003新增7个签名入口路径；T-P4-004加入7个工具及任务派发路径；T-P4-005增加8个调度策略、就绪队列与真实尝试路径，当前生成合同为169个模型、125个API路径；另加统一`Error`，OpenAPI实际共有170个Schema组件。跨字段、原文字节签名和事务规则见[事件入口合同](../../contracts/automation-ingress.md)；工具资格、固定任务输入和结构化进度见[工具执行合同](../../contracts/tool-execution.md)及[调度内核合同](../../contracts/scheduling-kernel.md)。这些计数描述合同覆盖，不表示真实厂商或完整平台已经验收。

- 项目请求必须给出project_id；租户由身份取得，客户端不能指定其他tenant。
- 创建/写入携带稳定idempotency_key。相同标识与内容恢复同一操作；不同内容返回409。
- 更新携带expected_version。文件保存携带expected_hash与固定操作标识。
- 返回错误为`{error:{code,message,details}}`；未配置/不支持与内部错误分开。
- 列表为`{items,total,next_cursor?}`，总数与本页数量分开。
- 业务记录与运行列表默认100、最多200条，使用limit和不透明cursor。运行列表仅返回元数据；完整节点图通过runs/{id}读取。概览只带最新10次运行，counts.runs为完整运行数量。
- 事件响应为`{items,next_cursor}`，next_cursor为整数sequence，后续请求使用after；它不返回业务列表的total。
- 流程发布与运行版本不可变；运行重试为新attempt，保留原证据。
- 验收检查本次已选的测试节点；未选分支的skipped不计测试通过数，也不阻塞其余真实测试。至少一个test实际成功，全部跳过不能满足门禁。
- Runner/Agent凭证只允许绑定项目的对应接口，禁止会话枚举、创建项目及再次签发凭证；API凭证只枚举其绑定项目。管理员浏览器会话负责签发，24小时到期。
- organizations、organization/*、teams/*、project-access和auth/switch只供组织浏览器会话使用；所有API/Runner/Agent项目范围令牌均被拒绝，即使其绑定项目正确。Runner接口要求独立的Runner范围Bearer，浏览器Cookie不能代替。

## 接口组

| 接口 | 功能 |
|---|---|
| auth/setup、login、session、logout | 所有权与会话 |
| auth/tokens | 一次展示短期凭证，指定project_id和scope |
| organizations、auth/switch | 列出/创建组织与切换浏览器组织会话 |
| organization/members、organization/members/{user_id} | 真实组织账号、CAS角色治理、显式所有权转交与移除 |
| organization/invitations、invitations/{id}/revoke | 24小时邮箱绑定邀请、首次秘密显示与版本化撤销 |
| auth/invitations/accept | 公共一次性接受；邮箱、令牌和密码证明放JSON正文 |
| teams、teams/{id}、teams/{id}/members | 组织团队与真实人类/项目Agent/工具归属 |
| project-access | 显式项目授权及已移除版本墓碑 |
| projects、overview | 项目与真实统计 |
| requirements、tasks、members、issues、documents、releases、tests、evidence | 授权业务记录 |
| requirements/{id}/decompose | 编辑后的所选任务原子创建 |
| tasks/{id}/review | 绑定需求/任务/依赖/测试依据的人工验收 |
| tool-profiles、tool-profiles/{id}/commands | 既有执行身份下不可变工具配置、现场清单、CAS启停及永久撤销；当前只供项目浏览器会话管理 |
| task-dispatch-plans、task-dispatch-plans/{id}/start | 固定任务/需求/流程/工具版本预览、期限、当前依据复核、原子派发及相同操作恢复 |
| runner/tool-inventory | 已登记且绑定凭证的执行节点上报非秘密本机探测；旧操作恢复不刷新60秒有效期 |
| issues/{id}/intake | 唯一来源问题回流需求 |
| workflows、workflows/validate | 不可变版本发布与编译 |
| runs、runs/{id}、events、commands | 运行/事件/控制/审批/核对/补偿 |
| scheduling/policy、scheduling/policy-previews、policy-previews/{id}/commit | 当前项目准入额度、不可变影响预览与人工确认；人类write+approve、版本及稳定回执同事务 |
| scheduling/queue、runs/{id}/nodes/{node}/scheduling | 持久就绪队列、当前真实阻塞及资源预留；不是硬件负载或跨租户派发顺序 |
| runs/{id}/nodes/{node}/attempts、attempts/{attempt} | 持久尝试元数据分页、精确读取原始结果；不从最近事件推测历史 |
| automations、automations/preview、automations/{id}/commands | 人工/cron规则、固定版本预演与CAS启停，限真实组织会话 |
| automations/{id}/occurrences、revisions | 触发与真实运行关联、不可变规格修订的范围绑定分页 |
| runs/{id}/events/stream | 持久SSE回放、当前权限重验、断线游标续传；GET events 为每页128条历史 |
| runner/claim、heartbeat、results、progress | 真实领取、取消意图、当前尝试的真实进展与持久ACK |
| planning、iterations、milestones | 全项目计划指标、任务窗口、迭代及里程碑完成门禁 |
| planning/schedule、planning/capacity | 双版本原子排期与周一开始的nullable资源工时 |
| planning/baselines、planning/baselines/{id} | 全范围不可变捕获、分页快照与全范围当前比较 |
| workspace、workspace/binding、workspace/file | 授权本地文本编辑、原文件摘要＋工作区上下文CAS及不可变原回执；[字段与未受理证明](../../contracts/workspace-file-context.md) |
| search、capabilities | 来源可定位检索与分层能力报告 |
| agent/tools、agent/tools/invoke | 原生智能体工具目录与受控调用 |

## 状态与事件

节点状态：pending/ready/running/waiting/reconciling/succeeded/failed/skipped/cancelled；运行另有paused/cancelling。waiting原因和节点类型决定可用操作，不能对所有等待统一显示审批。

事件sequence为持久化顺序，分页使用after游标。浏览器使用持久SSE回放与历史分页兜底，连接正常轮换及重连保留当前运行、节点和attempt。完整无限日志归档仍需后续实现。新增事件必须记录run/node/attempt及固定因果关系，ACK不早于事务提交；调度事务只在所有领域写入和锁完成后的最终尾部为已缓冲事件分配全局序号，之后直接提交，避免跨提交的小号事件被游标漏读。

`runner/progress`请求包含`project_id,assignment,operation_id,message`；必须匹配当前worker、attempt与fence。1–8192 UTF-8字节的真实输出经过控制符清理和凭证形态脱敏，事务提交后返回`{acknowledged:true}`。`node_progress`的`data`遵循生成的NodeProgressData；message是累计输出尾部快照，不能拼接为不重复的日志，也不是完成百分比。

NodeRun现返回固定运行定义的`idempotent,compensatable`和可选`workspace_evidence`。动态map子节点也使用实际固定定义；客户端恢复按钮不得从当前可编辑工作流推断。工作树证据可包含base/head提交、差异和证据SHA256、真实changed_paths及覆盖/截断信息；该证据不表示已合并。

## 治理与计划的额外约束

邀请列表与撤销响应只含InvitationMetadata，不含token或token_hash。创建只在首次响应含token；同操作重放为`token_revealed:false`及恢复说明，秘密不会写入持久回执。公共接受不要求已有登录，主动提供项目范围令牌仍会被拒绝；已有账号必须证明原密码，不能被邀请覆盖。已用、过期、撤销或邮箱不匹配的令牌失败。前端只从手动输入或URL片段接受令牌，不使用查询参数、日志或localStorage；未知接受结果通过普通登录恢复，不自动重用一次性令牌。系统不发送邮件。

组织成员移除撤销其组织会话、项目授权和人类团队归属。最后所有者保护与`transfer_owner`由服务端检查。团队归属不产生项目授权；human引用组织user_id，agent/tool引用对应项目真实资源。project-access和团队成员列表包含已移除墓碑，重新授予使用其当前版本，不能总用0。组织所有者与管理员的继承项目权限独立于显式授权。人类项目资源必须绑定有项目访问的真实组织账号`data.user_id`，不可创建假名册。

`GET planning`的limit默认200、最多200，cursor限定当前组织/项目；可用`task_id`单独读取一项最新计划。total与metrics保持全项目范围，coverage标明loaded_tasks、whole_project指标及loaded_tasks时间线。任务计划有独立version；排期changes要求完整字段、`expected_task_version`与计划`expected_version`（初次为0），一批1–200项原子保存。估算和容量的null为未知，0为明确零；容量member_id引用项目资源，week_start必须为周一，人工工时独立于机器配额。迭代/里程碑完成要求所有关联任务accepted；成功执行不代替交付验收。

基线列表和创建响应只有BaselineSummary。捕获在事务中复制全部任务范围；详情snapshot.tasks按cursor/limit分页，snapshot.total是完整快照数量，comparison则覆盖当前完整项目范围，as_of标明比较读取时间。范围变化、定义变化、排期变化与当前验收状态分别表达，快照不会随计划修改。

## 执行策略、领取与用量

`GET scheduling/policy`是只读事实，未落库的明确默认策略为并发1、version0/default=true。`concurrency_limit`不可为null；`resource_limits`的单项和`budget_limit`以null明确表示不限制，0确实阻止新准入。数值为0–9007199254740991的严格整数。`budget`是累计admission_units，每次worker attempt默认至少1，与token/金额及人员周工时分开；既有消费不退款，旧阶段未跟踪历史标为partial。

预览请求完整提供project_id、expected_version、policy和idempotency_key，30分钟数据库期限内固定原策略、差异、SHA256及as_of影响快照。确认再次检查当前人类write+approve和两层版本，策略、预览状态、审计与操作回执原子提交。降低容量不停止已有工作。API项目token可读其范围但allowed_actions为空，不能预览/确认；Runner/Agent token不访问这些工作台路径。队列和attempt分页默认50/上限100，游标绑定组织、项目和节点集合；精确attempt读取不依赖第一页是否包含该项。

`runner/claim`完整提交project_id、worker_id、实际capabilities与稳定idempotency_key。Runner先把原请求持久化，再调用；丢ACK重复同请求恢复同dispatch/attempt/fence，空结果也有原操作回执。active恢复重验当前worker及工具/发布资格；closed/reconciling仅是历史/核对观察，绝不启动进程。节点租约未知时先核对，不猜测未执行。收到响应后先存回执、再存执行inbox、最后确认本地claim完成；崩溃恢复保守处理可能效果。

`ToolUsage`各计数字段可省略但不可为null；省略为未知，0是明确报告的零。`usage_state=complete`要求input/output均已知，partial保留至少一个真实小计，unreported不含usage对象（允许外层null等价缺失）。普通非usage事件可以没有usage/state；usage事件或携带usage对象时必须有state。生成Schema使用条件约束表达这些关系，服务端仍检查事件身份、字节上限、当前授权和原始JSON。新scheduling/runner严格DTO请求保留原数字词法，防止接近2^53的小数先经float64舍入后被误接纳；其它map路由尚未统一为严格DTO，不能据此宣称所有接口具备同等数值合同。

Attempt列表只查询元数据，`result_source=not_loaded`；精确详情才读取原Result，并区分runner_result、manual_reconciliation、child_workflow、managed_observation和unreported。人工核对不伪造机器Result；提供方观察的合成状态码不称进程退出码，原未知回执仍保留在managed resolution链。新generation2 worker仅对明确transient且幂等的失败自动重试，最多2次、1/2秒持久退避；自动序列总截止显式使用TimeoutSeconds，未配默认300秒，人工retry新开持久窗口并保留旧窗口。TimeoutSeconds算术上限9223372036秒，避免纳秒时长溢出；旧generation1运行保留其固定解释。

## T-P4-016 需求设计与正式任务采用

`/requirements/{id}/design` 提供当前设计及严格PUT草稿，子接口为commands／只读preview／adopt／revisions／adoptions。正式类型来自Go，公开SDK类型由同源生成。保存允许不完整提案；发布必须完整设计、闭合依赖与验收覆盖；preview不是预留或批准。采用对全部相关范围重算摘要并原子应用，不重复创建稳定key对应任务。

批准限当前浏览器ProjectApprove；API只编辑提案。TaskContext.design只读，由正式绑定及批准修订解析，进入派发与验收依据。当前249 Schema／164路径；合法输入与跨字段规则见[合同](../../contracts/requirement-design.md)，实际范围见[阶段验证](../quality/requirement-design-verification.md)。

## 项目MCP（T-P4-019）

入站端点 `/api/v1/mcp/projects/{project_id}` 使用固定官方Go SDK协议实现，无状态JSON HTTP；JSON-RPC协议封套不在本REST OpenAPI内重写。目录、参数Schema、读写效果和允许凭证种类从同一应用定义生成到[mcp-tools.json](mcp-tools.json)。元数据REST DTO在OpenAPI与TypeScript中同源生成。连接配置和当前授权、幂等／CAS／原页面头、超时／取消约定见[统一合同](../../contracts/application-tools-mcp.md)与[接入文档](../integrations/project-mcp.md)。目录声明不授予权限；SDK协议层自有原始诊断与平台可本地化业务错误分别处理。

## 模型规划与站内风险（T-P4-023）

新增`/planning/analyses`、单项及commands、`/planning/alerts`和commands；共享Go DTO生成，当前294模型／181路径。使用固定只读规划profile、来源和显式采纳，原件、风险决策及回执保留。详细[合同](../../contracts/intelligent-planning.md)与[当前限定验证](../quality/intelligent-planning-verification.md)。

## 用量、费用估算与额度（T-P4-024）

`GET /usage`、`POST /usage/policies`、`POST /usage/rates`、`GET /usage/attempts`和单项`confirm`复用当前授权／严格DTO；304模型／186路径，旧模型和路径未改。整数micros、空值未知、月度scope、原始事实与人工确认边界详见[合同](../../contracts/usage-accounting.md)和[实际限定验证](../quality/usage-accounting-verification.md)。

[Application contracts](../development/project-applications.md) define the formal catalog, exact integrated source previews, immutable registration, declared graph and impact reads. Go types generate the shared TypeScript/OpenAPI and fixed source-inspector runtime constants; preview and impact are pure reads, registration remains an explicit current-authorized browser command.

[Application release-set contracts](../../contracts/application-release-sets.md) bind immutable reviewed code/artifact associations to the original signed per-member deployment plans. Preparation creates pending-approval plans only; package evidence and JUnit case reports remain distinct.
