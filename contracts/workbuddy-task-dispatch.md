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

### 迟到的云端完成观察

原授权人可在运行节点刷新已知云任务。服务端在事务外查询固定的供应方任务，随后重核本人授权、原任务与提交版本，并按组织、执行池、项目的顺序加锁。原运行仍在运行、原节点仍在等待输入或等待确认，且派发、尝试、执行代次与原 Runner 等待回执一致时，供应方 `completed` 将原节点更新为 `cloud.delivery_required`。已经请求停止、已改代次或不在可推进状态的节点保留当前原因。

一次有效更新在同一事务保存供应方观察、操作回执、原运行版本及 `cloud_delivery_observed` 事件。重放原操作读取回执，后续重复查询仅更新供应方观察。原尝试结果、租约、外部副作用占用和下游依赖保持原值。任务交付页沿当前正式绑定定位原节点，可按[本机候选合同](terminal-recovery-candidates.md#从原-workbuddy-尝试继续本机开发)另行发起代码候选，后续仍需独立测试及人工验收。

### 核对已登记仓库的分支提交

任务「交付与质量」在正式 WorkBuddy 尝试待交付时提供「核对仓库来源」。用户从当前项目已登记的仓库分支中选择一项，再请求实时读取。注册、分支配置及凭据继续由[仓库集成](repository-integration.md)管理。

`GET /tasks/{id}/delivery/repository-source` 要求 `project_id`、`task_version`、`run_id`、`target_id`、`target_version` 各出现一次，浏览器会话须有项目写权限。服务端在调用仓库提供方前后重核当前任务版本、精确原尝试及登记目标版本。仅使用服务端固定地址、凭据和登记分支读取当前 HEAD。停用目标、在途仓库写入、绑定变化和配置偏差均拒绝返回证据。响应只包含任务、运行、节点、尝试、目标版本、分支、提交、Git 对象格式及观察时间。

观察结果描述一次读取时的登记分支 HEAD。它与原云任务记录分别显示，Git 提交不能单凭该关联归因给 WorkBuddy。读取不写仓库、不创建候选，也不触发测试或验收。后续正式导入需固定可信基线、核验完整 Git 对象，并在隔离环境执行同提交独立测试。

### 批准并保留确切仓库来源

`POST /tasks/{id}/delivery/repository-source/intakes` 接受项目审批人的浏览器会话。请求体固定 `project_id`、`task_version`、`run_id`、`target_id`、`target_version`、`expected_head`、`expected_base` 和 `idempotency_key`。基线由提供方管理员配置的登记关系决定，请求中的基线只用于确认确切提交。项目编辑者或 API 令牌不能代替人工批准。

控制面在取得审批权限和原任务执行范围后释放数据库锁，再读取登记的两条分支和 Git 包。请求摘要、字节摘要及长度校验通过后，包暂存到既有受限制品目录。提交前再次核对当前审批权限、任务版本、原 WorkBuddy 尝试及执行代次、仓库登记和提供方配置。事务保存来源记录、审批人、原操作回执及活动。同一操作键恢复原回执，正文变化拒绝。单次原运行最多保留八份来源。若提交回执不确定，保留可能已被事务引用的字节供后续核对。

回执中的基础提交、来源提交和摘要用于后续隔离导入。当前接口只返回回执元数据。候选封存、任务改绑、Runner 派发、独立测试和正式验收使用各自的合同。

### 派发已批准仓库来源的候选和独立测试

项目审批人在 `POST /tasks/{id}/delivery/repository-source/intakes/{intake}/dispatch-plans` 选择现有导入工具和另一 Worker 的 JUnit 工具。请求体指定 `project_id`、`expected_task_version`、`producer_profile_id`、`expected_producer_profile_version`、`test_profile_id`、`expected_test_profile_version` 和 `idempotency_key`。任务需已有发布的测试用例，且原 WorkBuddy 尝试仍待交付、任务没有活动终端。控制面生成固定的导入节点和同提交独立测试节点，并保留原审批包的提交、摘要、大小及当前工具版本。

使用现有派发计划的启动接口明确启动。该类计划的启动和回执恢复均要求当前浏览器审批权限。DAG 和任务改绑同事务，改绑前再核对审批留存、任务、原尝试、目标登记和执行路由。原 WorkBuddy 运行作为来源保留。任务当前运行切换到独立候选 DAG。

只有该候选生产节点的当前 Runner 租约可从受控接口读取原 Git 包。普通项目 Runner 和 Pool Runner 分别使用自己的机器凭据及 `X-Suma-Assignment`。读取前后重核任务绑定、租约和包摘要。导入节点在隔离 Git 对象库中从固定管理员基线验证原包并检出确切提交，使用现有候选协议封存同一份字节。生产节点成功回执须匹配确切封存记录，另一 Worker 通过既有候选传递协议取得同一提交并执行独立测试。人工验收核对新运行及测试证据。

### 工作台查询与恢复

`GET /tasks/{id}/delivery/repository-source/options` 使用同一组五项查询选择器并重核浏览器项目审批权限。响应包括配置的确切基线及来源、适用于当前原尝试和登记版本的留存回执、已发布用例数、活动终端及可独立配对的工具档案。实时元数据不可用时，`live_state=unavailable`，实时提交为空，已批准的留存来源仍可选择。工具查询最多读取 200 个启用档案，通过 `profiles_truncated` 明确说明覆盖范围。

前端沿现有操作回执协议分开处理批准留存、派发预览和确认启动。恢复使用原操作标识和精确请求。已启动时通过原计划读取对应运行。 [工作台交互](../docs/design/repository-source-workbench.md)

## 验证及参考复用

必要检查覆盖固定上下文／两阶段零外发、输入变化、原owner权限、CAS与准确回执、撤权重放、原事务失败回滚、私有／Pool原Runner领取与一次创建、供应方completed不绿。中英界面检查单账号默认、多个账号、无账号、预览、未知响应同key恢复及打开原节点；代表桌面／窄屏实际浏览器交互只验受影响范围。

参考Multica提交e8ae72c9dfc1b4474b61a9087f51b33cbee6a048的任务分配与运行工作区模式，复用本项目成熟Go DAG、WorkBuddy SDK、事务和Web组件；不复制该版本附加许可限制代码。Qoder／通用ACP、云候选导入、云持续输入与权限、多操作系统／真实供应方／规模和生产继续属于完整平台待交付范围。
