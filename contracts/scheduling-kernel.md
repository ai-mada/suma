# T-P4-005 — 增量调度内核、可解释准入与用量事实

状态：实施合同，2026-10-24。完整目标保留在`docs/delivery/implementation-plan.md`。本轮先完成共享资源池公平调度所需的内核前置：增量推进、持久唤醒、短事件提交尾锁、项目执行策略及真实阻塞诊断、准确用量事实。跨组织共享池与明确授权的多项目Runner、分层加权公平、模型请求许可和费用结算仍是同一完整目标的后续接入，不能用项目轮转或配置权重代替。

## 所有权与集成

- Core代理：`internal/dag`、相关测试/基准及scheduler内核文档；不改platform/runner/toolcontract。
- Runtime代理：`internal/toolcontract`、`internal/runner`、`packages/agent-runtime`与对应测试/专项文档；不改dag/platform/UI。本轮先实施用量presence及原生过程额度，不创建假账单。
- UI代理：`apps/web`调度策略、队列、节点准入事实和attempt历史；接口在本合同冻结后实施。保持既有本地紧凑设计，不上传外部设计服务。
- Root：platform HTTP/授权/策略预览/commit、共享DTO生成器、集成、迁移审查、实际验证、状态文档与本地阶段commit。

不同代理开始改共享类型前先通知；不覆盖其它代理或用户修改。当前基线`1e9d77b9ffc314c1c55df48a402ffd0bf7e0e8de`。新构建/测试/临时文件仅在仓库`.cache`；保留所有旧证据与数据。

## 内核不变量

1. 正式状态、attempt/fence、事件和操作回执同事务提交；任何优化不削弱当前授权、候选固定、生产门禁或未知副作用核对。
2. `GetRun/Events`只读，不为读取取得全局调度写锁、推进timer或增加run version。完整图接口可以读取完整图，热路径不得因此重写所有节点。
3. 维护已编译的邻接/条件观察关系、持久边结算及节点依赖状态、ready与wake索引；完成节点只处理受影响后继/条件观察者/map父级，保存dirty记录。禁止将全图扫描换成全图内存扫描后宣称增量。
4. 每次Claim有稳定操作标识；同worker/相同请求丢ACK恢复原assignment，不创建第二attempt。旧worker/fence及撤权仍先于回执恢复核验。未执行副作用不等于可由新操作猜测重派。
5. ready查询分批且有稳定扫描进度，能力、工具、候选和资源不匹配让出候选；不占attempt、额度或公平份额。没有匹配信息时表示未知，不伪造不存在执行器。
6. Tick按到期索引有界处理、分run/家族隔离失败；未来到期项不扫描整库。暂停timer/retry不因每秒Tick重复写version；恢复时再处理到期事实。
7. 父子生命周期保持原子；采用同root run家族锁或可证明无环的稳定锁顺序，不能直接引入父→子与子→父的反向锁。现有组织授权锁如果保留，明确组织内仍串行。
8. 全局事件序号不得由未提交事务预先取得后跨提交跳号。可先使用**事务最终尾部**短锁批量分配/插入：所有其它锁与领域写入在此前完成，之后只提交，不再取其它锁或执行外部I/O。SSE/投影不能越过晚提交的小号事件；保留旧游标语义。
9. 新表/索引与迁移账本保留原run、事件、fence、unknown占用和budget consumed。禁止清空用户表。迁移需可重入、验证、原子代际切换；切换后新增写入的回退不能假装无损。

### 汇合、重试与展开

- 默认汇合等待全部入边解析；仅所有已选必需分支成功才启动；全部未选则skip。被未选分支跳过的普通后继边不作为成功必需分支，显式skipped边可选择该结果。
- `all_done`只等待已选分支终态，清理成功不能自动删除原失败事实。any_success/quorum持久冻结首次获胜集合，后继只激活一次，非获胜分支默认继续，run收尾仍等待它们终结。
- 容错失败接纳必须是固定流程中的显式策略并保留被接纳来源；未处理失败/未知结果阻止run成功。未实现的取消剩余分支选项须编译拒绝，不能伪装成功。
- 重试每次新attempt/fence，明确幂等且`Result.failure_class=transient`才自动重试，最多两次、持久1/2秒退避。新generation2 worker自动重试序列总截止从首次attempt开始，显式`TimeoutSeconds`作为序列截止，未配默认300秒；每次派发/恢复/心跳均受剩余时间约束。人工retry是明确新操作，开启新持久window/generation并保留旧window/attempt，不刷新原审批。缺少分类默认不自动重试；unknown永不凭超时重派。TimeoutSeconds最大9223372036，防止Go纳秒时长溢出，不静默截断。
- map先记录固定清单、模板/输入摘要、稳定条目标识、总量预留和展开边界，再幂等分块。边界未闭合不放行父下游；0项为空结果。默认总量10000、单map1000；超限在执行前拒绝。不能因分块中断重复子节点。

## 项目准入与公开合同

终端 `run queue` 和 `run scheduling RUN --node NODE` 复用现有项目只读队列和精确节点准入。队列按服务端游标分页；节点读取返回当时的阻塞项、资源与固定路由依据。两条命令与事件跟随独立使用当前项目授权，不改变执行状态。 [终端使用](../docs/operations/task-dispatch-options.md#终端运行跟踪)

Web 就绪队列和尝试列表按项目路径与当前用户、组织隔离。已加载身份时，读取携带原视角并中止失效的在途请求。401、403 或 404 后清除该列表的记录、计数和游标。尝试列表失去访问时同时收起精确尝试及其工具证据，保留深链中的原选择。重新获准后从第一页读取并恢复精确详情。暂时失败时保留同范围的已读快照，明确重试原页。空结果和计数须来自成功读取。 [工作台交互](../docs/design/scheduling-workspace.md#队列与尝试读取恢复)

就绪队列与准确节点读取在各自的当前授权事务中使用相同的资源池诊断。原运行的共享池暂停、配置缺失或项目授权版本变化时，队列项显示对应阻塞。结果仍表示本次读取时的条件，领取时继续重新核验。筛选队列状态只处理已加载记录，服务端的项目总数、续页和派发顺序沿原合同。

本轮项目策略与当前单项目Runner隔离配合，**不称跨租户公平**。既有人员周容量继续独立。资源表示管理员配置的准入容量，不等于OS硬限制或测得硬件负载。

拟由Core暴露事务内策略读取/修改、资源快照与诊断；Root发布HTTP。Policy写入与domain回执同SQL事务、当前write+approve在回执前核验。降低上限阻止新派发，不终止已开始执行，不退款既有累计budget。

冻结数据形状（完整字段以`internal/dag/scheduling_types.go`、`internal/platform/scheduling_types.go`和生成合同为准）：

```
SchedulingPolicy {
 project_id, version, default, concurrency_limit, resource_limits:{name:number|null},
 budget_limit:number|null, budget_spent:number, unit:"admission_units",
 fairness:{mode:"project_run_round_robin",scope:"project"},
 budget_history, resources, allowed_actions:["preview","commit"], as_of
}
SchedulingBlocker {code,message,source,resource?,unit?,required?,capacity?,used?,available?,retry_at?,run_id?,node_id?}
NodeAdmissionSnapshot {run_id,node_id,run_version,node_state,attempt,fence,policy_version,evaluated_at,status,blockers,requests,reservations}
```

- 默认worker节点补1个execution slot与1个累计admission_unit，omitted resources不能绕过并发额度或预算。默认项目并发容量为1，策略未持久化时version=0/default=true；创建运行或领取后按实际持久化版本显示。
- 其它未配置资源明确“不限制”，并保留请求/累计消费事实；所有数值为非负安全整数，容量0确实阻止派发。
- `budget`是累计准入单位，和token/币种分开；没有真实价格及用量证据时不显示金额或真实消费。
- 不在本轮策略页提供尚未实施的共享池/跨组织权重编辑。真正跨项目领取必须有新增明确的pool身份和项目授权，旧project token绝不能扩大范围。
- 队列有授权的完整范围总数、keyset分页、稳定ready时间及精确run/node链接；不显示不能证明的派发名次或ETA。
- 节点诊断是当前条件，独立于已发生attempt/工具/测试/验收事实。多个阻塞同时保留；权限、能力、工具、候选、资源、预算、重试时间和unknown分别表达。
- attempts从持久表分页读取，不由最近SSE合成；只返回当前授权项目的证据，不能泄漏其它租户占用者。
- 策略预览固定输入、原策略版本、差异、摘要和30分钟有效期；预览不写策略/占额度。确认时当前write+approve、策略CAS、预览CAS、审计和幂等回执同事务。已提交回执只恢复原结果，不再次写策略；新操作不能重复提交旧预览。范围token只读，不能修改准入策略。
- HTTP接口：`GET /scheduling/policy`、`POST /scheduling/policy-previews`、`GET /scheduling/policy-previews/{id}`、`POST /scheduling/policy-previews/{id}/commit`、`GET /scheduling/queue`、`GET /runs/{run}/nodes/{node}/scheduling`、`GET /runs/{run}/nodes/{node}/attempts`、`GET /runs/{run}/nodes/{node}/attempts/{attempt}`。读取携带project_id，列表limit默认50/上限100，游标绑定查询范围。
- `Assignment.dispatch_id/dispatch_state/lease_until`绑定已持久claim操作；dispatch_state为active/closed/reconciling。Runner只执行active，丢ACK恢复相同操作。closed/reconciling绝不重新启动。租约到期先核对，不能根据没有本地完成记录猜测未执行。
- Attempt列表只提供真实持久元数据，完整Result按精确attempt接口按需取回；没有结构化机器结果的人工核对保持缺失事实，不造默认成功/零用量。
- Attempt列表不读取完整result，`result_source=not_loaded`；精确详情区分unreported、runner_result、manual_reconciliation、child_workflow与managed_observation。人工核对带独立outcome/reason；子流程带child_run_id；提供方独立观察由dag_managed_resolutions明确定位，不能称Runner进程退出事实。
- generation1旧运行迁移只新增投影，保留旧已发生状态/版本/事件/汇合和超时解释，不自动推进；新运行使用generation2语义。迁移不能把未记录的历史预算回补成默认1，明确标partial；旧unknown占用与fence保留。

### 按执行路径和运行定位待派发队列

`GET /scheduling/queue` 支持可选 `route=project|pool` 和 `run_id`。运行 ID 使用 `[A-Za-z0-9][A-Za-z0-9_.-]{0,99}` 精确匹配。两条件取交集，省略条件表示不限，显式空值、重复参数和非法值均拒绝。

路径以当前授权事务内的原运行固定路由判定。固定到非空池 ID 的运行为共享池路径，其他为项目路径。撤回项目授予或暂停池后，原共享池路径继续可查，当前阻塞按原准入诊断显示。不存在或不属于当前授权项目的运行 ID 返回空结果。

总数和有序分页使用相同条件。游标绑定租户、项目及全部筛选条件，不能在变更条件后继续使用旧游标。查询使用原队列和准入模型，不变更运行、领取、预算或池状态。

### 按固定工具配置查看待派发队列

可选查询参数 `tool_profile_id` 使用与固定工具引用一致的 ID 语法，按大小写精确比较。与 `route`、`run_id` 取交集，拒绝显式空值、重复值或非法 ID。总数与分页在同一授权事务内使用相同筛选，游标额外绑定该工具条件。结果按节点冻结的工具引用筛选，共用命令名的其他配置和没有固定工具的节点不会命中。配置暂停、撤销或无法通过当前准入检查时，仍可查看冻结引用与各自的原阻塞原因。

浏览器 `queue_tool` 和 CLI `run queue --tool PROFILE` 使用同一个只读接口。查看队列保留项目权限、原节点事实及当前准入检查。工具启停、调度配置和执行均由各自入口处理。

### 固定工具准入

队列与精确节点调度使用相同的当前项目授权事务。待派发或等待依赖的节点有完整固定工具时，`tool_binding` 返回 `profile_id`、`profile_sha256` 与 `command_id`。按该引用核对当前项目的档案。档案缺失、固定指纹或命令变化、暂停、撤销及启用审批权失效分别返回 `tool_profile_missing`、`tool_profile_changed`、`tool_profile_paused`、`tool_profile_revoked`、`tool_activation_revoked`。阻塞的 `source` 为 `tool`。

同页多节点引用相同档案时复用事务内的档案和授权读取。固定档案当前有效时，仍独立核对额度、共享池及领取节点。状态不推断机器在线、模型登录或探测结果。工作台可以打开当前项目的准确工具诊断，再返回原执行节点。缺失档案保留原执行入口。

## 真实用量与原生过程额度

1. `ToolUsage`每个计数字段保存presence，已报告0必须序列化为0，未报告字段省略；整数范围0..9007199254740991。空usage不是零用量，拒绝错型、负数、小数及溢出。
2. Codex保留三基础计数必需/拆分可选的已核实形状；Suma逐步累加防溢出。部分步骤缺用量时记录`usage_state=partial`，不可把部分已报告小计称作整轮完整总量；全部未报告为unreported。
3. 原生模型响应只有合法的prompt/completion整数才是已报告基础用量。`usage:{}`/缺字段不补0；其它存在的拆分值严格校验。模型端fixture与真实厂商验收分开。
4. 原生过程每次请求先确定输出allowance，已知usage按真实计数消耗；未知usage保守保留该请求的完整allowance，不能通过不报告usage绕过多轮max_tokens。超出请求上限的实际报告要记录并拒绝继续工具动作。该额度是过程准入限制，不承诺不受控厂商账户的最终账单。
5. 未提供max_steps/max_tokens用明确默认值；显式非法、0、负数、小数或超上限参数应在任何外部调用前拒绝，不能静默放大成默认预算。
6. 所有新输出保留schema事件、当前attempt/fence、敏感值脱敏和未知事实。完整模型网关、每次请求许可与真实结算仍按完整目标继续实现。

## 本阶段验收

- 先建立现有逆序链/菱形/大量无关ready/未来timer的实际基线；记录查询/写行数、候选数、锁持有和耗时，不只比较机器时间。
- 新增RED→GREEN：选定分支默认汇合、winner冻结、条件观察祖先、暂停timer无version抖动、重复领取恢复、旧timer/fence拒绝、故障项隔离、单完成不重写无关千节点。
- 资源真实API：容量0保持ready且attempt/预算不变；提高后实际领取；大请求不足而后项可执行；双连接最后额度争抢只一笔。
- 原生HTTP fixture：缺usage/空usage/小数/过量、显式0与缺失、未知多轮保留allowance；不得调用付费模型来掩盖协议测试。
- UI实际完成策略审阅→提交→队列阻塞→条件改变→运行/节点/attempt证据下钻；窄屏、键盘、主题及刷新/返回验证。
- 阶段结束运行完整适当测试、生成合同漂移检查、最终网页/运行包构建，更新docs与证据，按用户要求本地commit，不push。
