# 同任务候选续作

T-DEVELOPMENT-CONTINUATION-186；suma1.0.0；原31项／P0–P7完整目标继续。接续 developer-tool-handoff.md 当前八类结构化开发能力与原 task-discussion-development 流程，不按早期 native-only 描述收窄。

## 明确语义

上一轮任务可以产生已封存代码，但新派发默认从管理员基线开始。本能力让有当前项目写权限的人明确选择同任务上一终态运行的已封存候选，将新审阅的目标交给新的独立 attempt／工作树。它提供代码连续性；Codex 仍每 attempt 使用新 thread／turn、独立模型选择与用量。既有跨任务已验收 CodeBaseline 合同保持独立，未验收来源不填入 AcceptedBasisSHA256。

来源必须是当前任务准确 RunID、已终态运行版本、当前成功 producer 的 node／attempt／fence／worker、原成功结果和完整 sealed transfer。允许旧正式测试失败或人工退回，因为来源只是明确授权的代码输入；未知、运行中、未封存、跨任务／跨项目和不匹配证据拒绝。新任务已正式验收时不继续改写，按新迭代需求处理。

## 共享合同

权威包 internal/developmentcontract：DevelopmentContinuationSource 为 RunID、NodeID、Attempt、Fence、ExpectedRunVersion 的明确选择；DevelopmentContinuationBinding 包含 Protocol=suma-task-development-continuation-v1、ID=新派发计划ID、ProjectID、TaskID、TaskVersion（预览时当前版本）、SourceTaskRunID、SourceTaskVersion（原受保护任务运行绑定版本）、SourceRunVersion、Source candidatecontract.Binding、BindingSHA256。摘要绑定全部原字段，SourceTaskRunID 必须等于 Source.Source.RunID；不含目录、命令、令牌、验收或模型授权。

原 TaskDiscussionDevelopmentCreateRequest 新增可选 continuation_source；原 Plan 与其 dispatch_plan 返回可选 development_continuation。客户端不能提交完整 Binding。创建在原授权／稳定操作／CAS 事务内解析来源，固定到原计划与开发节点。旧 assistant 消息不能自动代替新任务版本下的审阅来源。新目标、验收补充、工具与模型／预算仍按原明确预览和 Start 两步。

GET /tasks/{id}/development-continuation-options?project_id&limit&before：当前项目任务读取权限；limit 默认20、最大100；返回 project_id、task_id、task_version、source_task_run_id、source_run_version、source_run_state、items（selection、candidate、node_name）、total、limit、next_cursor?、complete、reason?。来源为当前任务终态运行中成功且封存的 agent／command producer。按 node_id／transfer_id 稳定 seek；游标绑定当前用户、租户、项目、任务版本和原运行ID／版本，漂移须刷新。total 是完整筛选计数，complete 表示当前遍历是否还有下一页。reason 是明确闭合的产品代码 task_accepted／no_run／run_not_terminal／no_sealed_candidate，由客户端中英文词条展示，可用来源时省略；不直接显示代码代替说明。读取不创建计划或执行。

原 dag.Assignment／runner.Assignment 新增可选 DevelopmentContinuation。原 CodeInputRequest 增加 input_kind=development_continuation。完整 Binding 只保存于原受保护派发计划及 typed assignment；流程节点仅带 development_continuation_id=plan.ID 的公开 marker，不重复完整 Binding。派发构建先剔除模板里的两种同名声明，只有本次正式创建才设置准确 marker。无 typed binding 的 marker、原配置内伪造的完整 Binding 和不匹配 ID 均拒绝；本能力只用于未声明 purpose 的普通可写开发节点。包括空值在内的 purpose 声明均明确拒绝，不能静默清除专用业务配置。

只有原持久派发计划与执行记录可授予该输入；客户端 transfer ID 或本地存在某提交均不授予代码权限。控制面在新执行、导入前／原件读取后复核当前消费 lease、原计划绑定、任务运行 provenance 以及来源版本与证据。Start 切换当前任务 RunID 后，用原来源 provenance 核验历史关系，不能仍错误要求旧 RunID 等于新当前值。

任务有既有已验收祖先基线时保留 TaskContext 中的原事实；新开发节点的直接输入改为 continuation，并移除该节点 code_baseline。执行 envelope 的直接输入模式互斥，不能让一个节点同时使用两种来源。模型、技能、工作区工具和测试门禁不因续作而放宽。

## 执行与证据

Runner 每次新 attempt 都经原 code-input broker 下载并校验完整 bundle，即使 M 已在本地。复用已有 SHA／大小／前提锚点／Git ancestry 验证及独立工作树：选中输入 M，新产物 N，完整 bundle 仍锚定管理员 B。original ImportReceipt、候选封存和传输沿原链持久化，不复制旧工作目录或用户登录。故障按原操作／lease／fence 核对，不能盲目重跑未知副作用。

新独立测试必须消费 N 的正式候选，不复用 M 的测试结论；人工 Review 仍绑定新运行和证据。旧失败、退回和问题记录保留，不因新预览清除。后续任务接力仍必须采用已验收正式来源。

## 简洁交互

原讨论计划卡新增“补充想法继续开发”，进入同一任务讨论输入，保留历史计划与运行。新回复可在原开发预览中明确选择“接着上一轮代码”，自动读取准确候选、显示原运行／提交及“仅代码输入，尚不代表通过质量验收”。默认不擅自选择候选或启动执行；模型与预算仍新选。已存在计划、待核对写入、无权限、来源漂移和切换项目时保留原恢复边界。预览显示来源与新测试链，详情可展开原证据；中英文能力对等。

## 并行与必要验证

Root 维护本合同、新共享包、dag／runner Assignment 字段、CodeInputRequest kind、生成接口及集成交付。控制面代理维护 platform 下来源解析／列表／派发／传输／当前关系、必要平台 types 与原 HTTP；Runner 代理维护真实导入／工作树／候选及相关必要 guard；Web 维护原讨论与开发预览／详情和双语。分工不修改彼此文件。

定向覆盖旧终态来源→新预览／Start→真实隔离导入 M→修改／封存 N→同提交独立测试与原 Review；模型边界可用明确受控协议夹具。另覆盖权限、版本／来源漂移、伪造 config、旧 lease、重复请求恢复、包摘要／锚点及新 attempt 隔离。只重跑新增或受影响检查，最后一次类型／双语／代表界面；不重复185配对／系统库／SDK或无关安装矩阵。SQLite、协议夹具、实际本机Git／进程、合成UI与物理多机／真实厂商分别留证。
