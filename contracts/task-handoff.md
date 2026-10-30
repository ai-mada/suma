# 团队任务交接合同

状态：初始产品1.0.0，T-HANDOFF-INBOX-148补充[请求通知与准确定位](task-handoff-inbox.md)；沿用既有责任交接与正式验收边界，完整P0–P7保持进行。实现和实际验证分别记录。

## 业务与状态

独立交接域：pending → accepted / returned / withdrawn。交接接收不等于任务人工验收。每个任务最多一个 pending，使用独立唯一槽位。发起时固定任务版本、taskBasis、原负责人、当前需求版本、关联 run 的版本／状态及交付摘要。禁止对活跃或结果未知的关联运行进行责任转移；未知不得自动重跑。任务、需求、目标身份或依据变化阻止接收，但允许有权限者退回／撤回关闭旧提案。

接收必须在一次现有授权事务＋项目 guard 中，核对当前身份／任务和交接 CAS，原子改 owner_id、task.version+1、status=in_progress，删除 verified_run_id，并按现有 InvalidateDependentsTx 失效下游；原 run_id、原运行／测试／审查和来源保留。不得自动创建执行、审批或验收。接收需要 acknowledge_invalidation=true，前端用明确说明与勾选确认。API 错误不能清空表单；网络结果未知重用同 operation，不盲目换 ID。

当前接收人仅真实项目 human 资源，owner_id 使用 suma_members.id，user_id 才是接收账号。必须仍有项目写权限和有效资源；可选 active 目标 team，团队 human 的 resource_id=user_id、project_id为空、role 为 member 或 lead，observer 不合格。不自动授权或创建账号。agent/tool 仍经现有任务派发选定正式工具／节点，后续专家组自动策略单独实施。

发起：当前项目写权限。accept/return：仅指定接收账号且当前项目写权限／目标团队资格。withdraw：仅发起账号且当前项目写权限。先当前授权后幂等回放；同 ID 换内容冲突。

## HTTP 与共享 DTO

- GET /api/v1/task-handoffs?project_id=P&task_id=T&limit=20&before=ID：limit 默认20，1..100；before 为本任务已有交接 ID，稳定 created_at/id 降序游标。返回 TaskHandoffPage；items bounded，total 正式总数，next_cursor=下一页 before ID，无更多为字符串空。不得用其它项目的游标。
- GET /api/v1/task-handoffs/{id}?project_id=P&task_id=T：当前授权下直接读取准确TaskHandoff并重新投影依据和允许动作；不枚举历史或接收人，不写通知状态。
- POST /api/v1/task-handoffs：TaskHandoffCreateRequest → TaskHandoff。
- POST /api/v1/task-handoffs/{id}/commands：TaskHandoffCommandRequest → TaskHandoff。

CreateRequest：project_id,task_id,expected_task_version,to_resource_id,to_team_id?(空为不绑定团队),note,idempotency_key。CommandRequest：project_id,expected_version,action(accept|return|withdraw),reason,acknowledge_invalidation(bool),idempotency_key。note/reason 为1..4000 UTF8字节，固定 ID 最长64，operation ID 复用原严格合同。所有未知字段、重复／非法查询和版本不接受。

TaskHandoff：id,project_id,task_id,version,state,created_by,created_at,updated_at,to_resource_id,to_resource_version,to_user_id,to_name,to_team_id,to_team_name,to_team_version,to_membership_version,note,basis:TaskHandoffBasis,decision_by,decision_at,decision_reason,result_task_version,basis_current,blockers:string[],allowed_actions:string[]。

Basis：task_version,task_basis_sha256,requirement_id,requirement_version,from_owner_id,source_run_id,source_run_version,source_run_state,delivery_sha256。服务器从正式行计算，浏览器不能输入。

Recipient：resource_id,resource_version,user_id,name,teams:TaskHandoffTeam[]。Team：team_id,name,team_version,membership_version。

Page：items:TaskHandoff[],total,next_cursor,recipients:TaskHandoffRecipient[],can_create,create_blockers:string[],task_version。allowed_actions / blockers 为当前只读展示，不代替命令重核权限；blockers 使用固定可本地化宿主原文。

## 文件所有权与集成

- 后端代理独占 internal/platform/task_handoff*.go（含 DTO、领域、迁移方法、必要测试）。Service：InitializeTaskHandoffs(ctx)、TaskHandoffs(ctx,actor,project,taskID string,limit int,before string)(TaskHandoffPage,error)、CreateTaskHandoff(ctx,actor,project string,req TaskHandoffCreateRequest)(TaskHandoff,error)、CommandTaskHandoff(ctx,actor,project,id string,req TaskHandoffCommandRequest)(TaskHandoff,error)。复用 authority transaction、commandReceipt/activity、dag.GuardProjectTx。不要修改 shared database.go/http.go/records.go。
- 前端代理独占 TaskHandoffPanel.tsx、TaskHandoffPanel.css、task-handoff 中英目录及必要组件测试；使用生成合同类型、既有 api-context/write-action/tool-read。props session,project,taskId,taskVersion,refresh,onChanged。默认折叠；单一接收人、可选团队、说明；分页历史、明确接收影响，来源按需展开。只在展开后读取，不增加仪表盘或新导航。
- Root 拥有 HTTP 接线、database.go 迁移接线、OpenAPI/生成类型、App.tsx 入口、宿主本地化、文档/证据、最终构建与必要联调/提交。

活动预览、构建隔离与阶段Git交付遵循当前AGENTS.md和用户授权。交接记录及通知的业务历史保留，不包含旧进程、旧预览或过期交付规则。
