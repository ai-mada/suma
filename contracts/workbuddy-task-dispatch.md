# 任务直接派发到 WorkBuddy

阶段：T-WORKBUDDY-TASK-DISPATCH-168；suma 1.0.0；日期：2026-10-31。状态：实现合同。完整31项需求及P0–P7保持有效，本阶段交付任务入口到原云Runner链路。

## 产品路径与边界

任务详情选择 WorkBuddy → 选择本人当前项目授权账号 → 预览固定任务及需求、设计、已发布测试计划和明确选择的知识 → 明确确认派发 → 打开原运行节点。唯一账号可以自动选中，预览和派发保持两个明确动作。模型由供应方管理，费用未核实；不能显示为平台选定模型或硬预算。无授权时直达原工具账号面板。

控制面生成一个单节点、固定版本的 `external_workflow/workbuddy-cloud/workbuddy-cloud/v1` 工作流，不要求用户编辑JSON、复制摘要或伪造本机ToolProfile。沿原Go DAG、私有／Pool领取、一次中央创建、持久观察与原节点工作区，不产生第二个派发器。供应方completed仍为等待可信交付；独立测试、人工验收、部署分别保留真实事实。当前绑定代码基线的任务拒绝此路径，直到云端基线交付合同得到实现；不悄悄丢弃代码基线。

## 合同

新增 `WorkBuddyTaskDispatchPreviewRequest`：project_id、task_id、expected_task_version、connection_id、expected_connection_version、grant_generation、knowledge（可选，原ProjectKnowledgeReference）、idempotency_key。全部版本为正整数。固定请求不接受prompt、URL、供应方task ID或模型覆盖。

新增 `WorkBuddyTaskDispatchPlan`：id、project_id、owner_id、version、state（ready/started）、task_id、task_version、connection_id、connection_version、grant_generation、run_id、node_id、delegation_id、created_at、expires_at、context（TaskContext）、workflow（Workflow）、execution_route（ProjectExecutionRoute）、node_spec_sha256、request_sha256、valid、reason。委派ID在预览时预留，委派实体仅在派发原事务创建。固定任务数据是开发内容，不是权限指令。

新增 `WorkBuddyTaskDispatchStartRequest`：project_id、expected_version、idempotency_key。启动返回原 `Run`。所有字段闭合解析，GET仅允许准确project_id查询；POST不接受查询参数。正常浏览器账号专用，API密钥、Runner和代理委派身份不能使用。

计划中的Workflow是固定编译输入，新节点的command／depends_on显式空数组；node_spec_sha256则绑定Core规范化后实际持久化的Node。现内核从Edges重建DependsOn，无前驱时为nil；摘要必须采用这一真实规范形状，不能比较显示输入的不同JSON表示或放宽摘要校验。既有Run公共Node类型的空数组／null约定尚需全局合同收口，本阶段不改已有Run或操作摘要。

- `POST /workbuddy-task-dispatch-plans`：预览，201；无供应方调用／解密／运行／委派／任务修改。
- `GET /workbuddy-task-dispatch-plans/{id}?project_id=…`：仅原owner且当前仍有项目权限；显示当前有效性，200；已派发预览可恢复并打开原run。
- `POST /workbuddy-task-dispatch-plans/{id}/start`：明确派发，201。

预览30分钟过期，派发授予24小时内的一次云创建。固定任务、需求、设计、测试计划、知识、连接版本／grant／client binding以及执行路由；任务上下文上限32000 UTF-8字节，超限拒绝，不截断。预览检查当前owner写权限、账号scope／有效期、组织和项目预算（enforce拒绝）及任务不是已验收；不宣称账号外部健康、节点在线或真实厂商已验证。

## 一致性及恢复

独立领域持久表 `suma_workbuddy_task_dispatch_plans`，按tenant/project/owner授权，run_id唯一；既存未知或残缺schema在任何DDL前拒绝。复用原command receipt、固定route basis和engine组织→调度→领域锁序。

启动在 `CreateManagedRunBound` 的同一事务中复核原owner、原plan、当前全部输入、精确连接／grant／client绑定、原预算和路由。原工作流、Run、唯一委派、任务绑定／正式provenance、设计及测试依据、plan CAS和receipt全部一起提交。bind回调时原run为version0、原node attempt0/pending；内部准确断言，不能放宽公共委派协议版本校验。事务提交前没有可领取空窗。

相同原请求／key恢复同一个plan或run；异正文同key拒绝。恢复旧派发回执不再复核已经消费的创建额度，不重新创建Run／委派／供应方任务，但必须重核当前owner项目权限。GET预览的有效性不是启动授权。其他owner、撤权、旧task/connection/grant/route、预算变成enforce均不能用新操作启动。原Run暂停／取消／等待语义继续复用原云合同。

`TaskDispatchRunTx`及task provenance识别此正式来源；设计任务只接受同一固定设计的原计划。不要把普通external workflow直接关联task当作具备固定上下文的新入口。

## 验证及参考复用

必要检查覆盖固定上下文／两阶段零外发、输入变化、原owner权限、CAS与准确回执、撤权重放、原事务失败回滚、私有／Pool原Runner领取与一次创建、供应方completed不绿。中英界面检查单账号默认、多个账号、无账号、预览、未知响应同key恢复及打开原节点；代表桌面／窄屏实际浏览器交互只验受影响范围。

参考Multica提交e8ae72c9dfc1b4474b61a9087f51b33cbee6a048的任务分配与运行工作区模式，复用本项目成熟Go DAG、WorkBuddy SDK、事务和Web组件；不复制该版本附加许可限制代码。Qoder／通用ACP、云候选导入、云持续输入与权限、多操作系统／真实供应方／规模和生产继续属于完整平台待交付范围。
