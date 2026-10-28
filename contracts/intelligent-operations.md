# 统一智能输入与本地草稿计划合同

阶段：T-INTENT-114；产品：1.0.0；状态：限定范围实施，实际验证见[本阶段报告](../docs/quality/intelligent-operations-verification.md)。`FND-INTENT-001`／需求26及完整P0–P7仍进行中。正式DTO为 [intelligent_operations_types.go](../internal/platform/intelligent_operations_types.go)，目标架构与后续范围见[架构及SOP](../docs/architecture/intelligent-operations.md)。

## 范围与领域所有权

当前入口将自然语言目标和明确选定的项目来源交给一次真实模型调用，形成可编辑、可校验、可预览的多步草稿计划。允许的动作只有：

| action | 领域效果 | 批准边界 |
| --- | --- | --- |
| `requirement.create_draft` | 创建当前项目正式存储中的需求草稿 | 不发布需求、不创建正式任务 |
| `requirement.design.save_draft` | 保存指定需求的产品／体验／技术设计及任务提案草稿 | 不发布设计、不采用任务提案、不启动执行 |
| `knowledge.create_draft` | 创建memory／spec／skill项目知识草稿 | 不发布、组织共享、安装技能或授权工具 |

需求、设计、知识继续拥有各自校验和版本规则。应用层复用现有领域事务helper，不另建记录表、任务状态机、调度队列或模型SDK。三类效果只涉及同一数据库，采用在一个真实SQL事务内完成；这个保证不覆盖网络、文件、提供方、部署或其他外部副作用。当前动作语言也不允许这些副作用。

低风险策略固定为服务端 `suma-local-drafts-v1`。用户确认只授权本次已预览的草稿批次，不建立长期自动批准策略；自然语言中的“自动发布”“忽略权限”等文字不会扩大动作集合或当前主体权限。

## 输入、固定来源与模型端口

创建输入为 `IntelligentOperationCreateRequest={project_id,goal,locale,sources,tool_profile_id,expected_tool_version,max_output_tokens,acknowledge_model_use,idempotency_key}`。locale为zh-CN／en-US；sources为0–8项 `ProjectKnowledgeSourceInput{kind,id,version,quote}`，沿用documents／requirements／tasks／issues正式来源合同。零来源允许从目标开始，不允许模型自行检索未选资料。

服务端在当前项目授权下读取真实来源、版本、quote及SHA。上下文只包含允许的标题、description和需求／任务验收条目，不附Record.Data、文档元数据、配置、环境、凭证或登录文件。有限预检查拒绝明显私钥、常见长API key／Token及带凭证的URL，不声称能识别任意文本中的全部秘密；用户仍须只选择可交给模型的正文。来源引文是数据，不能成为系统指令。完整prompt与接收输出各最多32000 UTF-8字节，超限明确拒绝，不截断验收条件或拼接残缺JSON。来源摘要是服务端事实，客户端和模型不能指定可信SHA或身份。

执行复用唯一Go托管DAG及同一原生模型运行时。专用只读profile显式声明protocol=`suma-intelligent-operations-v1`、固定model及endpoint_origin；purpose=`intelligent_operations`，max_steps=1、MaxRetries=0、一次模型调用，无项目工具、工作区桥、任意HTTP或命令执行。max_output_tokens为1–8192，默认4096。专用能力探测仅证明固定协议与本地配置，不证明模型账户健康或厂商效果。秘密仍留执行节点受控环境，不进入浏览器、计划、预览或日志。

创建、领取、心跳和结果投影沿用当前凭证、项目、profile、执行路由、attempt／fence及真实用量边界。模型运行成功与业务输出有效分别记录：非法JSON、越权动作或无效引用使分析失败，保留真实运行与用量，不自动再调模型。缺失用量、未知费用不记零。

## 封闭计划语言

`IntelligentOperationOutput={schema_version:1,summary,rationale,questions:string[],steps:IntelligentOperationStep[]}`。

summary为1–2000字节，rationale为1–8000字节。questions与steps各0–8项，单问题1–1000字节；零步骤必须有待补充问题。问题未解决、零步骤、来源或目标已变化、权限不足时不能采用。补充缺失事实后，明确修订并保存提案，再生成新预览；不把猜测当作已确认参数。数组使用`[]`，不以null替代。严格解析拒绝未知字段、重复JSON键、非法整数词法、额外对象和超限内容。

每步为 `{key,action,depends_on,requirement?,design?,knowledge?}`；key稳定且同计划唯一，depends_on只引用本计划有效key、最多8项，不允许自依赖或重复；服务端拓扑排序，依赖必须闭合无环。每步只带对应action的一种payload：

| payload | 字段 |
| --- | --- |
| requirement | `title,description,priority,acceptance_criteria` |
| design | `requirement_id,requirement_step_key,content:{product,experience,technical},tasks:RequirementDesignTask[]` |
| knowledge | `kind,title,summary,content,source_indices` |

design的requirement_id与requirement_step_key只选一个：前者必须是本次选定的正式需求；后者必须是本步已经依赖的 `requirement.create_draft` 步骤。新需求的真实ID由同一事务内前序写入结果解析，不由模型预生成。一个计划不能多次写同一设计目标。任务提案沿用既有设计key、验收覆盖、负责人及内容预算；保存不等于发布校验全部通过。知识source_indices是固定sources中不重复的0起始索引，不能自造来源。

新需求title为1–200字节、description最多16000字节、验收1–16项且每项1–1000字节；priority允许空或low／medium／high／urgent，空值的实际默认来源在预览展示。知识正文／标题／summary沿用16000／200／1000字节限制，source_indices可为空；设计继续沿用三段合计16000字节及完整任务提案预算，不另放宽已有领域上限。

模型语言没有租户／主体、权限、批准、版本、SHA、操作key、HTTP、路径、命令或自由data字段。默认值由服务端确定，并在预览中提供字段、值及来源，不能把默认值包装成用户已明确提供的事实。

## 预览与原子采用

`IntelligentOperationPreview={id,project_id,version,source_sha256,proposal_sha256,preview_sha256,policy,can_apply,basis_current,questions,defaults,steps}`。

defaults为 `{step_key,field,value,source}`。每项影响为 `{key,action,target_kind,target_id,target_step_key,current_version,requirement_version,title,depends_on,required_permission,risk}`；新对象用target_step_key关联，不伪造已存在ID。预览只读，不创建对象、不预留资源、不批准后续动作。

preview_sha256由服务端绑定分析版本、完整提案／来源、所有现有目标版本及固定策略。apply提交同一SHA和 `acknowledge_drafts:true`。前端编辑提案后立即清除旧预览与确认；保存修订、补充问题、来源／目标变化或切换作用域后重新预览，不能复用旧确认。

采用在同一SQL事务内重新核验当前授权、固定来源、提案、预览、策略和所有目标版本，按依赖顺序调用已有三个领域事务helper。草稿、不可变修订、领域活动／事件、真实步骤Results、分析CAS和原操作回执一起提交。任一步失败整个事务回滚，不留下半个需求或知识草稿；不引入partial步骤或虚构补偿状态。模型调用在事务外，apply不调用模型或外部服务。

Results保存每步key／action、真实resource_kind／resource_id／version／revision／sha256及source_sha256／proposal_sha256／original_output_sha256／run_id。原模型正文、可修订proposal和不可变采用Results分别保存。后续人工编辑／发布不会改写最初采用事实，亦不能凭旧版本继续下一次CAS。统一计划的结果链不冒充knowledge_intelligence的独立model_adoption。

## 权限、接口与恢复

统一入口允许当前浏览器主体或限定项目API主体：读取／预览需要ProjectRead，生成／修订／采用需要ProjectWrite；底层领域规则不放宽。Agent／Runner不能借此调用管理入口，API也不能借此批准；发布、正式任务采用、执行、团队治理及生产操作仍由各自现有批准入口负责。每次恢复成功回执也先核验当前身份、租户、项目和权限。

所有路由位于`/api/v1`，项目来自显式project_id，租户来自真实会话。

| 接口 | 请求／响应 |
| --- | --- |
| GET `/intelligent-operations/analyses?project_id&limit&before` | `IntelligentOperationPage{items,total,next_cursor}` |
| POST `/intelligent-operations/analyses` | CreateRequest → `IntelligentOperationAnalysis` |
| GET `/intelligent-operations/analyses/{id}?project_id` | 当前授权的分析、固定依据、原件及结果 |
| POST `/intelligent-operations/analyses/{id}/preview` | `{project_id,expected_version}` → 只读Preview |
| POST `/intelligent-operations/analyses/{id}/commands` | `{project_id,expected_version,action,proposal?,preview_sha256?,acknowledge_drafts?,reason,idempotency_key}` → Analysis |

列表默认20、最多100项，以created_at／id稳定keyset分页，before绑定当前租户项目。goal为1–4000字节，稳定idempotency_key为8–128字符；sources必须显式为数组，不能省略或以null代替。

commands的action只允许revise／apply／dismiss；revise保存可编辑proposal，apply只能使用服务端已有proposal，不能在采用请求中夹带新正文。running／reconciling不能以dismiss掩盖未知运行；取消沿已有run commands。

Analysis包含id／project_id／version／state、目标／语言／作者／时间、固定profile及run、max_steps／max_output_tokens、source、可选proposal、original_output及SHA、reason、results、applied_by／applied_at／applied_preview_sha256、allowed_actions和basis_current。状态为running／ready／failed／reconciling／applied／dismissed；ready仅表示提案可审阅，applied只证明本批草稿已写入，不表示需求批准、开发完成、测试或验收通过。

超时或响应丢失保留原账户／组织／项目、method／path、完整body、expected_version和idempotency_key；先恢复原操作，再读取当前分析与实际对象。同key同正文恢复同一批真实ID；同key不同正文冲突。不得因403／409换key猜测重跑，不自动再次消费模型。已确认版本冲突后保留编辑内容，明确核对当前依据并重新预览。当前来源变化可保留旧提案供读取，但阻止采用。

## 验收与交付边界

按[架构中的验收SOP](../docs/architecture/intelligent-operations.md#本阶段验收sop)验证实际三类草稿、跨步骤引用、事务回滚、原操作恢复、当前授权、输入越权、秘密、预算及中英文交互。实际命令、环境、结果和未验范围仅由[阶段报告](../docs/quality/intelligent-operations-verification.md)及[精简摘要](../docs/quality/intelligent-operations-summary.json)记录；本合同不是测试通过证明。

当前不完成全平台意图目录、通用补全、持久自动采用策略、配置模板／实例／秘密引用／验证／变更服务、CLI／桌面独立智能旅程、真实厂商评测、外部效果恢复或完整需求26。单一1.0.0按当前初始化声明建立新namespace；不增加旧格式别名或升级用户库，不删除原始用户数据。
