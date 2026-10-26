# T-P4-023 模型规划、事件回写与站内预警合同

状态：实施中；产品1.0.0，不增加历史兼容层。完整P0–P7及新移动消息/独立移动App目标保持进行。本轮纵向限定真实模型驱动的现有任务排期建议、依据固定/重验、明确采纳、执行事件回写与持久站内风险预警。规则指标不能冒称模型智能；任务代码开发的全自动pipeline、自动策略授权/外部消息发送与移动App继续后续实现。

## 主路径

当前浏览器项目写权限创建PlanningAnalysis，选择1..20个既有任务和已启用/新鲜观察的Suma只读规划工具profile，确认选中内容将交执行节点配置的固定模型分析。服务封存正式Task/TaskPlan/关联Requirement/当周容量上下文及SHA，prompt完整UTF8最多32000字节，超限拒绝不截断。使用既有Go CreateManagedRunBound创建一个固定agent节点；MaxRetries=0，max_steps1..4(默认3)、max_output_tokens1..8192(默认4096)，timeout120秒；没有第二调度器。

工具profile新增可选 Planning:*PlanningCapability，字段 protocol='suma-schedule-plan-v1', model, endpoint_origin。仅Suma adapter/read_only=true/无NativeWorkspace且可信本地配置显式启用planning、固定SUMA_MODEL与SUMA_MODEL_BASE_URL，经过实际原生planning协议probe后产生。普通代码profile不自动变planner；API不能伪造程序/参数/环境。第三方key仍留执行节点，不进浏览器。Endpoint/model固定值同spec纳入profileSHA，key可通过EnvFrom。当前资格不等于模型账号健康，真实调用另记。

Root Runner构造purpose='planning'及上述预算，原生TS运行时禁workspaceBridge、只使用既有项目只读工具，系统约束规划输出纯JSON。沿用既有started/usage/completed/error JSONL和Journal；completed.output保存模型JSON（沿用既有凭证脱敏与控制字节清理），不用新终态假冒成功。原生协议probe无模型调用。平台在已验证当前Runner/Pool结果的同一tx内派生analysis/proposal/alerts；解析失败只让analysis失败，保留真实run结果，不触发模型重跑。

## 结构化输出和来源

PlanningOutput={schema_version:1,summary,rationale,actions:PlanningScheduleAction[],risks:PlanningRisk[]}。summary1..2000，rationale1..8000；actions1..20，task_id只允许所选任务且不重复。Action字段task_id,iteration_id,milestone_id,start_date,due_date,estimated_hours(number|null)，日期和工时沿用现有BatchSchedule规则；版本由封存source解析，不能由模型指定或覆盖。模型只提供排期建议，不提供任意命令/工具参数/权限/任务验收。

Risk={key,severity(info|warning|critical),title,detail,task_ids[]}，最多20；key为1..64安全标识、同输出唯一，title1..200/detail1..4000、task_ids只限选中范围。原始模型内容不翻译，宿主状态/错误/操作成对中英。

PlanningSource={sha256,week_start,tasks:PlanningSourceTask[],requirements:TaskContextRecord[],capacity:CapacityEntry[]}；SourceTask={task:TaskContextRecord,owner_id,status,priority,depends_on:string[],plan:TaskPlan}。SHA还覆盖正式源和当前上下文，采用前同事务重验；不把客户端source/hash当授权。模型结果原件与原始SHA不可改；人工revise产生当前版本并保留原模型输出。

## DTO与接口

PlanningAnalysis字段id,project_id,version,state(running|ready|failed|reconciling|applied|dismissed),goal,locale,created_by,created_at,updated_at,tool_profile_id,tool_profile_version,tool_profile_sha256,run_id,max_steps,max_output_tokens,source:PlanningSource,proposal?:PlanningOutput,original_output_sha256,reason,allowed_actions:string[],basis_current。所有数组序列化[]非null。原模型output原件可存私有payload；original_output公开当前项目授权范围内收到的模型JSON（安全清理后的接收事实）；公开摘要/说明按受控范围返回，不透出登录文件或环境。

- POST /planning/analyses request={project_id,goal,locale,task_ids,tool_profile_id,expected_tool_version,max_steps,max_output_tokens,acknowledge_model_use:true,idempotency_key} → PlanningAnalysis；服务签名 CreatePlanningAnalysis(ctx,engine,actor,project,PlanningAnalysisCreateRequest)。
- GET /planning/analyses?project_id&limit=20&before=ID → PlanningAnalysisPage{items,total,next_cursor}，1..100稳定created_at/id keyset。
- GET /planning/analyses/{id}?project_id → PlanningAnalysis。
- POST /planning/analyses/{id}/commands request={project_id,expected_version,action:revise|apply|dismiss,proposal?:PlanningOutput,reason,acknowledge_schedule_changes:bool,idempotency_key} → PlanningAnalysis。revise/apply仅ready；apply需要当前写+批准权限、确认影响与依据不变，复用BatchSchedule同tx helper，计划行/analysis CAS/receipt/activity原子提交。dismiss只关闭已形成或失败结果，运行中取消走已有run命令，不盲目丢弃未知运行。
- GET /planning/alerts?project_id&state=open|acknowledged|resolved(可省略)&limit=20&before=ID → PlanningAlertPage{items,total,next_cursor,open_count,critical_count}。
- POST /planning/alerts/{id}/commands request={project_id,expected_version,action:acknowledge|resolve,reason,idempotency_key} → PlanningAlert。

PlanningAlert={id,project_id,version,analysis_id,run_id,risk_key,severity,title,detail,task_ids,state,created_at,updated_at,decision_by,decision_at,decision_reason}。每analysis/risk_key仅一条，当前结果重复回执不得重复产出。此为模型风险建议，不标成已确认故障。站内提示/列表不授权发送微信/钉钉/企微消息。

## 文件所有权

- 后端代理独占 internal/platform/intelligent_planning*.go(除HTTP Root拥有)，定义上述DTO、来源、领域/迁移/列表/结果投影/采纳/预警与必要测试；可以抽取planning.go的BatchSchedule事务内helper供旧正常排期和新apply共用(不重复验证逻辑、不改其有效业务语义)。migrate方法 MigrateIntelligentPlanning(ctx)。其余Service methods GetPlanningAnalysis/ListPlanningAnalyses/CommandPlanningAnalysis/ListPlanningAlerts/CommandPlanningAlert 命名按DTO清晰一致，签名ctx,actor,project,id?,req/limit/before/state。
- 结果投影函数 recordPlanningOutcomeTx(ctx,tx,tenant,project,result dag.Result,authorize func(*sql.Tx)error) error：只能Root在当前Runner/Pool authority及tool结果已核对的完成回调中调用；方法在写入前调用authorize(tx)，以实际run/node/attempt/fence绑定analysis。重复结果幂等，未知不得伪成功。来源admission helper checkPlanningExecutionTx(ctx,tx,tenant,project,runID)用于Root claim/heartbeat/result路径重核分析来源当前资格；不伪装browser actor。
- 前端代理独占 PlanningIntelligence.tsx/.css、intelligent-planning中英域及必要测试，props session,project,tasks:PlannedTask[],refresh:number,onChanged。规划页一个折叠入口/面板：goal+选择已有任务/真实plannerprofile+模型使用确认→真实运行链接/事件驱动刷新→可编辑JSON结构化方案/风险/原始依据→明确apply；站内预警过滤/ack/resolve。复用api-context/AbortController/write-action与现有run-events，不新建全项目高频轮询。Root挂入口与轻量全局预警提示。
- Root拥有 toolcontract/Runner/TS原生purpose与probe、HTTP/database/结果与admission接线、OpenAPI/生成类型、宿主本地化、共享入口、文档/必要集成/提交。

当前主预览73371/control247120/holder247472，不能覆盖活动web/停止服务/改原库。隔离构建后只做代表性交互和真实本地协议fixture；厂商模型不自动消费。仅提交必要源码、合同、配置和精简事实，不push，不加无关测试/依赖。
