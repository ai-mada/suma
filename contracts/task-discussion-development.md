# 任务讨论到原生开发交接

状态：T129，当前单一1.0.0设计；源码与实际证据分别验收。负责人：协作、原生执行、质量、工作台。复用[讨论回复](task-conversation-replies.md)、[原生编码](native-coding.md)与正式任务派发合同；不创建第二调度器。

## 明确交接与固定来源

只有当前任务的真实已核验、已保存assistant回复可作为来源。创建时绑定准确reply版本、protected消息身份、任务／需求版本与原任务运行。公开API不能伪造assistant、来源摘要、工具授权或质量结果。后续普通讨论不会悄悄改写已审阅计划。

人明确编辑`reviewed_goal`及`acceptance_criteria`，选择固定流程／开发节点、当前启用的可写原生工作树工具和**新的**模型部署、调用／输出上限、预算，确认真实工作区执行。编辑文本按JSON固定到原开发节点，不能提供额外shell、工具、路径、环境、权限或验收结果。这些条目是本计划补充说明，不修改任务、需求或已发布测试的正式验收合同。任务范围变更使用原任务／需求编辑流程后重新请求准确回复。

当前工具遵循[开发工具交接](developer-tool-handoff.md)已实现的八类结构化可写开发协议，不限于原生 suma 工具。平台原生模型与 Codex 独立模型选择互斥；固定工具自带配置按其实际能力处理，不冒充已固定的厂商模型。只读回复工具、通用命令及未核实适配器仍拒绝。当前库存、实际路由和费用由原控制面与Runner权威核验，回复模型预算不能借给开发执行。[同任务候选续作](task-development-continuation.md)为新一轮开发增加明确代码输入，不复用旧测试或验收。

## 共享API

- POST `/task-discussion-development-plans`：`TaskDiscussionDevelopmentCreateRequest`→`TaskDiscussionDevelopmentPlan`。只创建可审阅派发预览，尚未启动执行。
- GET `/{id}?project_id`：当前Browser／固定项目API读权限，读取正式派发状态及准确来源。
- POST `/{id}/start`：`TaskDispatchStartRequest`→正式`Run`，明确第二步启动。沿用原任务派发的权限、同库预留、版本、稳定操作、任务绑定、唯一DAG和Runner独立工作树。

Create包含project/task/expected_task_version、reply_id/expected_reply_version、workflow_id/workflow_version、agent_node_id、tool_profile_id/expected_tool_version、按当前工具选择的model_execution_selection或codex_execution_selection、可选明确continuation_source、reviewed_goal（1..8000 UTF-8字节）、acceptance_criteria（1..32条，每条1..1000字节）、acknowledge_execution与idempotency_key。最终完整开发goal不超过32000字节；超限明确拒绝，不静默裁剪。未知／重复JSON字段、非法查询、跨项目API及Runner／Agent代人发起拒绝。

响应包含准确assistant消息、source_sha256、审阅文本及原`dispatch_plan`。wrapper与原派发计划共用ID／版本；ready版本1，start后读取版本2。来源表、正式预览、活动及原回执同事务保存，创建失败不留下部分计划。相同原操作恢复原计划，即使后来已启动；换文本使用同key冲突。start重传恢复原运行，不因当前工具后来失效而重新运行。

## 执行、测试与验收

流程必须具备从开发节点取得固定候选的独立test节点；派发继续使用原工作树、版本模型与预算、实际工具进展、宿主候选封存、制品、独立执行节点测试同一候选、人工验收与问题回流。交接预览不改Task.Version／RunID／status；真正start由原BindRunTx绑定运行并递增任务版本。模型回复或进程退出不等于候选已封存、正式测试通过或任务验收通过。

只把reviewed_goal作为有权限的人明确提出的开发目标，不把聊天文字自动发给交互shell。PTY会话与本交接使用各自准确会话／attempt，不能同时获得同一工作树写权限。当前独立工作树避免编辑器主目录与自主代码执行共享写入。

*English: A verified assistant message can be explicitly reviewed into an existing native task dispatch preview. The human selects a fresh model and budget, reviews the goal and supplemental criteria, then separately starts the original dispatcher. Source proof, task scope, independent candidate tests and human acceptance remain distinct. No chat text is automatically executed as shell input.*
