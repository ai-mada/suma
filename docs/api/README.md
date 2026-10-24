# HTTP、事件与客户端合同

状态：当前v1实现；线协议Schema和TypeScript类型由`suma-contracts`生成。基础路径 `/api/v1`。浏览器同源HttpOnly Cookie；Runner/Agent/API使用分别绑定项目的短期Bearer凭证。

## 生成与校验范围

从仓库根目录执行 `go run ./cmd/suma-contracts` 生成 `docs/api/openapi.json` 和 `packages/contracts/src/index.ts`；`go run ./cmd/suma-contracts --check` 只检查文件是否与生成器一致，不写入文件。`go test ./cmd/suma-contracts` 覆盖字段展开、nullable工时、模型引用闭包、权限元数据、分页与漂移检查；`node scripts/check-docs.mjs` 检查本地文档链接和OpenAPI引用。

Go导出域模型直接反射，包括PlanningPeriod、TaskPlan、PlannedTask、CapacityEntry、BaselineSummary和NodeRun。PlannedTask嵌入的Record字段按真实JSON展开；指针工时保留`number | null`。组织治理与计划汇总等由map构造的响应、以及本轮新增请求字段，采用生成器中与HTTP处理器核对过的DTO，类型不会直接参与服务端请求解析。

生成器覆盖本轮新增路由的请求字段、基础取值约束和响应结构。旧接口仍使用部分通用请求信封，在OpenAPI的`x-request-schema-coverage`中标明。跨字段条件、UTF-8字节长度、当前权限、CAS版本、操作回执、真实身份关系、完成门禁和邀请秘密语义继续由服务端校验；JSON Schema与TypeScript类型不代表完整请求验证器。路由清单和map响应DTO仍是维护的合同，不是从全部HTTP分支自动推导；`--check`不证明全部处理器与合同自动一致。

## 请求规则

签名事件接收器是独立边界：`POST /hooks/{id}` 使用入口签名，拒绝浏览器Cookie/Bearer和任何query，不由调用者提供project_id。T-P4-003新增7个签名入口路径；本阶段再加入7个工具及任务派发路径，当前生成合同为147个模型、117个API路径；另加统一`Error`，OpenAPI实际共有148个Schema组件。跨字段、原文字节签名和事务规则见[事件入口合同](../../contracts/automation-ingress.md)；工具资格、固定任务输入和结构化进度见[工具执行合同](../../contracts/tool-execution.md)。这些计数描述合同覆盖，不表示真实厂商或完整平台已经验收。

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
| automations、automations/preview、automations/{id}/commands | 人工/cron规则、固定版本预演与CAS启停，限真实组织会话 |
| automations/{id}/occurrences、revisions | 触发与真实运行关联、不可变规格修订的范围绑定分页 |
| runs/{id}/events/stream | 持久SSE回放、当前权限重验、断线游标续传；GET events 为每页128条历史 |
| runner/claim、heartbeat、results、progress | 真实领取、取消意图、当前尝试的真实进展与持久ACK |
| planning、iterations、milestones | 全项目计划指标、任务窗口、迭代及里程碑完成门禁 |
| planning/schedule、planning/capacity | 双版本原子排期与周一开始的nullable资源工时 |
| planning/baselines、planning/baselines/{id} | 全范围不可变捕获、分页快照与全范围当前比较 |
| workspace、workspace/binding、workspace/file | 授权本地文本编辑与摘要冲突 |
| search、capabilities | 来源可定位检索与分层能力报告 |
| agent/tools、agent/tools/invoke | 原生智能体工具目录与受控调用 |

## 状态与事件

节点状态：pending/ready/running/waiting/reconciling/succeeded/failed/skipped/cancelled；运行另有paused/cancelling。waiting原因和节点类型决定可用操作，不能对所有等待统一显示审批。

事件sequence为持久化顺序，分页使用after游标。当前浏览器通过增量读取事件更新；持久WebSocket/SSE与完整日志流属于后续阶段。新增事件必须记录run/node/attempt及固定因果关系，ACK不早于事务提交。

`runner/progress`请求包含`project_id,assignment,operation_id,message`；必须匹配当前worker、attempt与fence。1–8192 UTF-8字节的真实输出经过控制符清理和凭证形态脱敏，事务提交后返回`{acknowledged:true}`。`node_progress`的`data`遵循生成的NodeProgressData；message是累计输出尾部快照，不能拼接为不重复的日志，也不是完成百分比。

NodeRun现返回固定运行定义的`idempotent,compensatable`和可选`workspace_evidence`。动态map子节点也使用实际固定定义；客户端恢复按钮不得从当前可编辑工作流推断。工作树证据可包含base/head提交、差异和证据SHA256、真实changed_paths及覆盖/截断信息；该证据不表示已合并。

## 治理与计划的额外约束

邀请列表与撤销响应只含InvitationMetadata，不含token或token_hash。创建只在首次响应含token；同操作重放为`token_revealed:false`及恢复说明，秘密不会写入持久回执。公共接受不要求已有登录，主动提供项目范围令牌仍会被拒绝；已有账号必须证明原密码，不能被邀请覆盖。已用、过期、撤销或邮箱不匹配的令牌失败。前端只从手动输入或URL片段接受令牌，不使用查询参数、日志或localStorage；未知接受结果通过普通登录恢复，不自动重用一次性令牌。系统不发送邮件。

组织成员移除撤销其组织会话、项目授权和人类团队归属。最后所有者保护与`transfer_owner`由服务端检查。团队归属不产生项目授权；human引用组织user_id，agent/tool引用对应项目真实资源。project-access和团队成员列表包含已移除墓碑，重新授予使用其当前版本，不能总用0。组织所有者与管理员的继承项目权限独立于显式授权。人类项目资源必须绑定有项目访问的真实组织账号`data.user_id`，不可创建假名册。

`GET planning`的limit默认200、最多200，cursor限定当前组织/项目；可用`task_id`单独读取一项最新计划。total与metrics保持全项目范围，coverage标明loaded_tasks、whole_project指标及loaded_tasks时间线。任务计划有独立version；排期changes要求完整字段、`expected_task_version`与计划`expected_version`（初次为0），一批1–200项原子保存。估算和容量的null为未知，0为明确零；容量member_id引用项目资源，week_start必须为周一，人工工时独立于机器配额。迭代/里程碑完成要求所有关联任务accepted；成功执行不代替交付验收。

基线列表和创建响应只有BaselineSummary。捕获在事务中复制全部任务范围；详情snapshot.tasks按cursor/limit分页，snapshot.total是完整快照数量，comparison则覆盖当前完整项目范围，as_of标明比较读取时间。范围变化、定义变化、排期变化与当前验收状态分别表达，快照不会随计划修改。
