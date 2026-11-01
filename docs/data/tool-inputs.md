# 问答数据与一致性

状态：1.0.0当前实现；负责人：控制面；需求：T-TOOL-INPUTS-187。实现：`internal/platform/tool_input_store.go`、`tool_input_observations.go`；[合同](../../contracts/tool-inputs.md)与[架构](../architecture/tool-inputs.md)。

正式状态使用独立事务表 `suma_tool_inputs`，并沿用原命令回执；不把答案存入日志或工具权限表。

| 字段组 | 用途 |
|---|---|
| id、tenant_id、project_id | 原记录与租户项目隔离 |
| run_id、node_id、attempt、fence | 固定原执行作用域，阻止迟到旧执行写新尝试 |
| worker_id、dispatch_id、assignment | 原机器身份、派发及完整执行证明 |
| profile_id、profile_version、profile_payload | 固定原受保护能力，不从可编辑Config推断 |
| operation_id、request_sha256 | 原Open操作幂等与正文冲突检测 |
| source_session_sha256、source_request_sha256 | 原typed RPC身份在原生会话内去重 |
| version、state、expires_at | CAS、生命周期与数据库时间期限 |
| payload | 原问题、可选答案与累积观察的受保护当前记录 |

主键为id。唯一约束一为tenant/project/worker/operation；唯一约束二为tenant/project/run/node/attempt/fence/source-session/source-request。并发Open、跨类型原ID与combined32上限在原执行事务锁内核验，问题本身不能扩大身份权限。

答案保存采用预期版本、原问题摘要和确定原生结果摘要；答案与原操作回执原子提交。观察采用原操作标识、单调sequence和累积事实，不能清除已观察、已开始、已写出或已关闭事实。原回执回放返回原快照，当前读取另行展示较新记录。

初始化先检查注册基线、准确列与关键唯一约束；未声明或损坏结构在其他写入前拒绝，不重置用户数据、不回填旧格式。SQLite已进行实际事务及坏schema无写入验证；OceanBase／可选PostgreSQL仍需独立实库证据。

题目与答案不进入知识索引、模型公共日志或执行Evidence；Evidence仅保存原身份、摘要、题数和投递事实。密码题在原始Journal／网络登记前拒绝，不用明文问题表代替凭据管理。备份和审计应按原租户敏感业务数据策略保护本表与命令回执。
