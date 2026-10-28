# 共用应用工具与入站MCP

状态：T-P4-019实施基线，2026-10-26。负责人：平台集成。完整平台目标保持；本轮采用官方Go MCP SDK v1.8.0、Go1.26语言基线及受支持的1.26.10工具链，不手写JSON-RPC。

## 应用模块

公共DTO位于internal/platform/application_tools_types.go；平台代理实现Service.ApplicationTools(ctx,actor,project)与InvokeApplicationTool(ctx,actor,ApplicationToolCall)。所有工具名、参数schema与作用由同一服务端目录维护，返回独立副本，不允许调用者改写注册表。原生HTTP、MCP与开发者连接说明共用该目录和领域实现。

原生 `/agent/tools` 与 `/agent/tools/invoke` 保留四个既有读取工具和原wire{name,description,parameters}投影，不能通过原semantic-read例外调用新增写入。读取仍在当前授权事务中执行；JSON参数严格类型化、拒绝未知字段、重复键及null，整数不经float64转写。

### 本轮目录

| canonical name | MCP name | 范围与效果 |
|---|---|---|
| project.overview | suma_project_overview | Agent/API：当前项目概览 |
| project.tasks | suma_project_tasks | Agent/API：分页任务，保留既有分页语义 |
| project.search | suma_project_search | Agent/API：有界关键词检索 |
| project.knowledge | suma_project_knowledge | Agent/API：明确id/revision/SHA的当前批准知识，最多16引用 |
| project.requirements | suma_project_requirements | API：分页需求 |
| project.requirement | suma_project_requirement | API：单个正式需求 |
| project.task | suma_project_task | API：单个正式任务 |
| project.requirement_design | suma_project_requirement_design | API：需求设计读取 |
| project.task_design | suma_project_task_design | API：原任务设计依据及当前资格 |
| project.knowledge_list | suma_project_knowledge_list | API：分页知识元数据，含当前权利内草稿 |
| project.requirement_draft_create | suma_project_requirement_draft_create | API当前项目写权限：创建draft需求 |
| project.task_draft_create | suma_project_task_draft_create | API当前项目写权限：创建todo任务 |
| project.requirement_design_save | suma_project_requirement_design_save | API当前项目写权限：严格版本化设计草稿保存 |

新增输入类型由application_tool_inputs.go明确声明，schema不含跨项目选择器、自由URL或任意函数分派。草稿创建仅接受明确白名单，状态由服务固定；设计保存复用原完整输入、来源、整数、大小、版本及幂等校验。创建及保存返回紧凑正式标识／版本摘要，避免副作用完成后因大正文输出失败。读取输出有界，过大时明确要求缩小页面／引用，不静默截断或声称完整。

目录不是权限。新增读取只给真实projectAPI；projectAgent保持原四读取，Runner不获得管理工具。写工具只有真实API身份且当前领域写权限／原账户组织上下文正确才可用。目录可在同一当前授权事务内用既有purposeProjectWrite作无副作用权限核验后过滤；调用依然重新授权。不得在Get后另开Update事务实现“仅草稿更新”。本轮不注册评审、验收、运行、证据、派发、发布、采用、生产、文件或进程工具。

每次写调用的typed arguments必须携带稳定idempotency_key，未知结果重试同一工具／正文／key。复用原Service.Create或SaveRequirementDesign当前权威事务及回执，不再加一层不一致的外部回执；旧回执不能绕过撤权、角色或版本变化。

## MCP传输与身份

固定端点 `/api/v1/mcp/projects/{project_id}`，Streamable HTTP，Stateless=true、JSONResponse=true，显式正body上限及每调用有界deadline。每HTTP请求以当前Bearer认证得到真实Actor，再在应用调用内同事务核验当前权限；不从缓存session、Initialize参数、Principal JSON或工具参数重建身份。MCP只接受agent/api凭据，不接受Cookie、browser session或Runner凭据。SDK处理协议、schema、版本协商和取消；旧协议另有应用deadline，不能依赖SDK新协议取消选项。

SDK工具使用共享schema及受验证typed handler。错误保留可用机器code并本地化受控文字，内部错误和无效参数不得回显凭据／完整参数。日志只包含安全操作身份和阶段，不输出token或用户正文。工具只暴露本模块当前目录；关闭动态任意服务器、远程HTTP与开放shell。

每请求Actor／项目来源置于私有context键，工具注册只捕获规范名称。SDK getServer无副作用、可多次调用；private cacheScope、TTL0，HTTP no-store。保留SDK localhost rebinding保护，额外对Origin/Host明确校验，不能为了开发域名关闭所有保护。GET/DELETE的stateless不支持状态按SDK真实返回405，不能宣称持久会话、订阅通知或自动恢复。

`X-Suma-Expected-User`和`X-Suma-Expected-Organization`复用原命令视图规则；API草稿写入需要配置模板中的固定原视图头。SDK token/scope不是平台授权替代。维护模式按既有规则冻结MCP POST，包括初始化和只读POST；保留维护423，不自动重放业务命令。

## 开发者连接说明

新增 `GET /developer-connection?project_id=...`，仅当前browser/API项目读取，返回DeveloperConnectionMetadata。tools是声明目录，client_connection_verified固定false；无token、无自动连接测试。端点URL只来自可信部署PublicURL或cmd监听loopback默认，不信任Forwarded/Host生成外部凭据目的地。

新增部署 `SUMA_PUBLIC_URL`／Server.PublicURL：无userinfo/query/fragment、HTTPS；明确loopback开发可HTTP。监听loopback时默认使用真实listener origin，使Codex等本地CLI可访问；IAB专用pool.localhost不能被假定为OS可解析地址。非loopback未配置canonical public origin时EndpointURL留空并说明管理员配置，保留EndpointPath。

“设置→项目令牌”增加紧凑接入区域：当前项目endpoint、工具效果／允许scope、只引用进程环境SUMA_MCP_TOKEN的真实支持配置模板。不得把新secret自动插入模板、日志、localStorage、URL或剪贴板。复制配置、签发token都不成为Connected/Verified证据；临时secret仍走原一次性显示、未知回执锁与切scope清理。

配置示例优先经本机CLI帮助或官方文档核实的Codex StreamableHTTP字段；原账户／组织ID是固定非秘密上下文。中英文改变产品说明，不改变URL、工具名、原始IDs／凭据、正文或草稿。状态含加载、空目录、失败／手动重试、401/403清除旧事实、复制失败；切换项目／账号／组织清理旧目录和晚到响应。

## 交付和验证

工具链与SDK仅从官方源下载、校验及许可盘点，所有任务缓存和TEMP/TMP在repo.cache、进程内环境，不改机器环境。模块升级及容器／构建入口同步，历史1.24结果保留原切面。Go工具链是全后端集成变化，允许一次必要完整Go里程碑检查；日常功能只运行相关回归。

验收至少含官方SDK客户端真正Initialize→ListTools→CallTool；Agent精确四读取，API读取与draft写入、同key恢复／漂移409、撤权后拒绝、未知／null／重复键／跨项目、保留原批准门禁、原生HTTP共源、可取消及有界输出、私有缓存／恶意Origin／Host、真实中英文连接说明。OAuth discovery、外连MCP、插件执行、A2A、厂商模型和完整平台验收继续独立实施，不冒充本轮完成。

## 当前交付／质量读取与Octop配置

T-OCTOP-116新增仅api当前ProjectRead的`project.task_delivery {task_id}`与`project.quality_execution {task_id,run_id,limit?,after_case?}`。HTTP与MCP共用taskDeliveryTx，quality共用原事务中的testPlanExecutionTx；不嵌套事务、不改变领域判定。后者默认10／最多20例，每例3条证据引用、最多16个来源提交，保留总数／完整性；after_case只能为该运行固定修订中的实际key。原64KiB输出界限、当前授权、四项Agent只读与三项草稿写入保持。无XML／日志／命令输出，也不注册批准／派发工具。公共4DTO及共享schema从同一生成器输出，MCP Implementation版本取产品buildinfo1.0.0而SDK维持1.8.0。

DeveloperConnection增加[Octop](../docs/integrations/octop.md)项目专用配置片段，仅人工合并，不自动外部PUT或探测。HTTP Authorization为字面占位符，实际专用可撤销项目令牌由操作者在Octop受控凭据区填写；不生成不存在的env替换协议。shared/default_open默认false，来源／权限／执行资格不由配置模板推断。[验收](../docs/quality/octop-integration-verification.md)分别记录标准协议、真实界面和Octop实际实例未验边界。
