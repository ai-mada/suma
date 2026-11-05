# Governance, planning and execution expansion

Status: current initial1.0.0 contract, updated2026-10-29 / T-CALENDAR-134. All writes use stable idempotency_key and expected_version; authenticated tenant comes from the session. API/Runner/Agent project tokens cannot use organization governance. Tests use repository-local temp/cache. Stage commits/pushes follow current AGENTS and explicit session authorization.

## Organization and teams (governance agent)

`Service.InitializeGovernance(ctx)` declares the current domain tables without importing historical data. `Server.governance(w,r,p,path,m) bool` handles authenticated governance routes before normal project authorization; root performs body decoding and rejects all scoped tokens for these namespaces. `Service.AcceptInvitation(ctx,m)(Principal,string,error)` handles the public one-time invitation acceptance route; root sets cookie and returns normal session. `Service.SwitchOrganization(ctx,p,m)(Principal,string,error)` returns a new browser session; root handles cookie and normal session response.

- GET/POST `/organizations` -> list `{id,name,role}` / create with `{name,idempotency_key}`. Creation does not change active organization.
- POST `/auth/switch` `{organization_id,idempotency_key}` -> normal session in chosen organization; current credentials must be browser sessions.
- GET `/organization/members` -> `{items:[{user_id,name,email,role,version}],total}`.
- PATCH `/organization/members/{user_id}` `{role,expected_version,idempotency_key}`. Preserve the last owner; roles owner/admin/member; owner transfer is an explicit owner-only operation, never arbitrary escalation by admins. Removing membership must revoke its sessions and project grants.
- GET/POST `/organization/invitations` -> list metadata / create `{email,role,grants:[{project_id,role}],idempotency_key}`. Create shows token once, expires after 24h; no token on listing. Replay must not issue another secret; receipt can return metadata with `token_revealed:false` and explain recovery. Existing org admins cannot promote themselves or replace owners.
- POST `/organization/invitations/{id}/revoke` `{expected_version,idempotency_key}`.
- POST `/auth/invitations/accept` `{token,email,name,password,idempotency_key}` public. Existing account must prove password, not be overwritten. Token is single-use, email-bound, constant-time checked, stored hashed. Cross-org/project grants are checked when created and accepted; expired/revoked invitations fail. No email is sent.
- GET/POST `/teams`; PATCH `/teams/{id}`. Team `{id,version,name,description,status}`; create/update fields same plus operation/version. Team is organizational structure, not implicit privilege escalation.
- GET/POST `/teams/{id}/members`: `{resource_kind:'human'|'agent'|'tool',resource_id,project_id?,role,action?:'remove',expected_version,idempotency_key}`. Human references real org users; agent/tool references scoped project resources. Writes require org admin. Membership response includes stable identity and version; team membership alone does not grant project access.
- GET/POST `/project-access?project_id=...` -> actual project grants; write `{project_id,user_id,role:'admin'|'reviewer'|'developer'|'viewer',expected_version,idempotency_key}`. Only org owner/admin or project admin can govern project grants; cannot grant a role above own authority.

Governance owns internal/platform/governance*.go, governance*_test.go, docs/product/governance.md. Root owns database.go/auth.go/http.go hook lines. Use existing Service.q/ddl/receipt/activity and public errors. Organization operations use project scope `@organization` in receipts with explicit organization authority checks.

## Planning (root)

- GET `/planning?project_id` -> `{iterations:[],milestones:[],baselines:[],tasks:[],capacity:[],metrics,as_of}`. Tasks contain canonical identity/title/status/version/owner/dependencies plus `plan:{version,iteration_id,milestone_id,start_date,due_date,estimated_hours:number|null}`. Real accepted state drives delivery progress; execution progress is separate. Totals and coverage show loaded scope.
- GET/POST `/iterations`; PATCH `/iterations/{id}`. `{id,version,title,description,status:'planned'|'active'|'completed',start_date,end_date,goal}`.
- GET/POST `/milestones`; PATCH `/milestones/{id}`. `{id,version,title,description,status:'planned'|'active'|'completed',target_date}`. Completing iteration/milestone requires all linked tasks accepted.
- POST `/planning/schedule` `{changes:[{task_id,expected_task_version,expected_version:planVersionOr0,iteration_id,milestone_id,start_date,due_date,estimated_hours:number|null}],idempotency_key}`. Atomic batch with complete edited plan fields; scope, dates and references validated. It updates independent plan versions, never fabricates task acceptance or changes delivery scope.
- POST `/planning/iteration-assignments` applies up to 16 explicit task and schedule versions to a current iteration version or removes the assignment. It reuses the same scheduling transaction and skips unchanged rows. See [interaction and command contract](../docs/design/task-iteration-assignments.md).
- GET/POST `/planning/baselines`: POST `{title,idempotency_key}` captures immutable task/plan scope, estimates and actual statuses; GET metadata includes snapshot. Capture is an explicit action. Scope changes and actual completion remain distinct from baseline assumptions.
- GET `/planning/capacity` derives a Monday-based week from the current resource calendar. GET/PUT `/resource-calendars/{member_id}` owns the sole calendar write contract: seven Monday–Sunday nullable daily hours and bounded date exceptions, independent expected version and stable original operation. See [work calendar contract](resource-calendars.md). A complete task date interval is allocated by actual configured available-hour weights; missing boundaries, unknown days and zero denominator stay explicit. Planning estimates are separate from actual worklogs and machine quotas.

## 组织项目组合

`GET /api/v1/project-portfolio` 只接受当前组织的浏览器会话，支持可选 `q`、`limit`、`after`。搜索按标题与描述的字面值，去除首尾空白后最多 120 UTF-8 字节。默认 20 项，最多 50 项。新项目优先，创建时间相同按项目 ID 降序。请求不能指定组织、项目或任意过滤字段。

`ProjectPortfolioPage` 返回 `items`、UTC 读取时间 `as_of` 及可选 `next_cursor`。续页传原游标，重新核对身份、组织、搜索、页大小及锚点当前可见性。每页按当前权限生成。仅组织 owner/admin 或当前具备有效项目角色的成员可查看对应项目。列表、统计、游标校验在同一授权事务完成。响应 `Cache-Control: no-store`。

`ProjectPortfolioItem` 包含项目公开名称与状态、全部任务数、`accepted` 任务数、`blocked` 任务数、活动风险数（`open`、`mitigating`、`in_review`）。可选里程碑指本项目有计划日期且处于 `planned` 或 `active` 的最早一项，同日用 ID 稳定排序。没有合格里程碑时省略字段。该读不返回组合总量，不修改项目、任务、发布或调度事实。 [交互规格](../docs/design/project-portfolio.md)

### 跨项目排期重叠

`GET /api/v1/project-portfolio/schedule-overlaps?week=YYYY-MM-DD` 只接受当前组织的浏览器会话。参数是必填的周一日期，拒绝其他查询字段与重复值。响应 `week_start`、`as_of` 与 `items`，使用 `Cache-Control: no-store`。候选项目及任务在同一当前授权事务内读取。项目读取资格变化会影响下一次结果。

按真实人员账号合并当前有效的项目内人力绑定。仅纳入当前可访问项目中、已明确负责人、起止日齐全、尚未验收且未退出计划的任务。对周内每个自然日，统计该人员有排期的不同项目。两个及以上项目的日期交集构成一条重叠事实。同一项目的多任务不能单独形成重叠。响应按人员列出确切日期和参与重叠的任务来源，隐藏项目及失去访问资格的绑定不参与计算。

扫描限制为每周 2000 条符合条件的任务。超限返回 `409 schedule_overlap_scope_too_large`，不返回部分事实。日期区间使用业务计划中的自然日。重叠表达多项目计划占用同一天，估算工时、已记录工时、资源日历与全组织实际可用容量使用各自的事实和规则。 [项目组合交互](../docs/design/project-portfolio.md#跨项目排期重叠)

### 跨项目周负荷与容量

`GET /api/v1/project-portfolio/capacity?week=YYYY-MM-DD` 只接受当前组织的浏览器会话和一个必填周一日期。响应 `ProjectPortfolioCapacityPage {week_start,as_of,items}`，使用 `Cache-Control: no-store`。最多读取 2000 条当前可读项目内符合条件的计划任务。超限返回 `409 project_capacity_scope_too_large`，整页无结果。与排期重叠使用同一人力绑定、当前资格和任务区间范围；一个项目内的任务也计入周负荷。缺少完整起止日的任务没有可确定的周，排除在这个周视图之外。

每个真实账号返回组织工作日历版本、七日可用小时、七日已分摊估算、每项任务来源和周汇总。普通成员可在有权任务的周视图读取相应人员的推导工时；完整工作制、例外备注和成员邮箱仍沿各自授权读取。已分摊小时沿[工作日历计算规则](resource-calendars.md#估算与未知)，使用组织日历覆盖完整任务区间，包括所选周外的日期。明确零估算分摊为零。估算缺失计入 `unestimated_tasks`；正估算的完整区间有未知日或可用工时为零时，计入 `unallocated_tasks`。这两种任务的 `allocated_hours` 均为 null，不纳入已分摊小计。

七天可用工时全部明确时才汇总周容量。已分摊小时已超过已知周容量时返回 `over_capacity`，即使另有待核对任务。其余有未知周容量、缺估算或无法分摊时返回 `basis_incomplete`；完整且未超出时返回 `within_visible_capacity`，结论限于当前可读项目。投影和权限在同一事务中读取。该投影独立于实际工时、在线状态和机器调度额度。 [交互](../docs/design/project-portfolio.md#跨项目周负荷与容量)

## UI (workbench agent)

Own apps/web, packages/ui, docs/design/planning-governance.md only. Preserve existing navigation hierarchy and design tokens. Add development tabs iterations/planning (milestones, timeline, baseline and capacity), settings tabs organization/teams/project-access; organization switch via real GET organizations + POST auth/switch followed by session reload. Invitation acceptance page reads token only from URL fragment/manual entry, never query/log; no automatic acceptance. Root routes public acceptance and switch to cookie session. All loading/error/empty/no-permission/conflict states and uncertain write replay guard remain. UI may store a new invitation token only in component memory until dismissed. No external design uploads (previously rejected).

## Runner expansion (execution agent)

Own internal/runner, cmd/suma-runner and new internal/dag/progress.go plus minimal additive dag/types.go fields; root owns HTTP route hook. Add `Engine.Progress(ctx,tenant,project string,a Assignment,operationID,message string)error` with current attempt/worker/fence ownership, stable operation receipt, durable event, bounded redacted text. Root adds POST `/runner/progress` `{project_id,assignment,operation_id,message}` -> persisted ACK. Runner journals progress before send and retries without re-execution. No automatic model/paid calls.

For code writes use attempt-specific Git worktrees, pin base commit/repository from administrator config, stable path derived from assignment key, durable operation metadata; never switch/reset the user's existing checkout. Local commands can be explicitly read-only in the shared workspace; write commands require isolation. Capture diff/status and commit metadata as evidence. Refuse missing or dirty isolation assumptions; unknown outcomes stay unknown. Never auto-merge. Provide a reviewed merge-preflight operation with expected target HEAD and tests/evidence checks; do not claim actual merge if only preflight is implemented.
