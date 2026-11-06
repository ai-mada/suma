# T-P4-024：执行用量、版本化费用估算与额度合同

状态：实施中；产品1.0.0。完整需求14/18与P0–P7不缩减。本轮正式交付：所有受管工具attempt独立用量账本；明确固定模型的Suma规划工具费率、组织/项目月度token及费用估算额度；领取原子预留、最终事实幂等结算、未知保留、人工确认与简洁中英管理。团队/个人/专家组归属、模型网关逐调用许可、厂商账单/支付结算及真实vendor验收继续后续，不用当前准入额度冒充。

## 核心真实性

1. 原dag budget=admission_units保持自身语义。新money=micros(1货币单位=1,000,000micros)，API整数0..2^53-1，最初CNY/USD；从不换汇或跨币种相加。费率是用户配置的估算价，不下载/硬编码厂商价格。使用math/big或等价精确整数计算ceil((input*input_rate+output*output_rate)/1,000,000)。cached/reasoning/cache_write为原计数展示，不与input/output直接相加或用于未证明的折扣。
2. 最终ToolEvidence是attempt用量来源，progress有截断/不同CLI累计语义，不能与最终值再次相加。只有complete且input/output明确时才能自动完成结算；partial/unreported保留未知风险。模型不可确定的CLI记录unpriced，不猜模型/金额。null是未知，实测0仍是0。
3. 初轮定价仅允许当前ToolProfile.Planning固定model/origin的Suma只读工具。费率revision不可变，冻结profile SHA、model、origin、currency和输入/输出预留token；创建时当前profile/版本重验；费率修改要求组织管理+项目管理，不能由项目管理员通过调价绕过组织限额。费用预留是执行准入的保守估算，不是厂商硬消费限额；供应商真实超额要记账/标overrun并阻止后续突破，不能隐藏。原生output reservation必须>=实际node max_output_tokens(默认4096)，input reservation为运营者明确估计。输入硬上限/逐请求网关后续实现，不宣称已完成。
4. 额度scope首轮organization或project，UTC自然月YYYY-MM；policy每scope一条，version0默认observe/unlimited/CNY。mode observe|enforce，token_limit/cost_limit_micros指针显式null无限，0严格0。organization策略只owner/admin当前组织权限可改/看全组织数；project策略需当前项目管理权限。普通项目读者可看本项目账本/策略和组织限制是否启用，不能借此读其它项目金额或账号。
5. 账户分资源持久化：scope+UTCmonth+resource(tokens或cost:CNY/cost:USD)。token按input+output计，不重复计算拆分；钱按各currency独立。policy enforce时每个scope都要满足当前明确费率、currency匹配该scope policy和额度；未知model/费率或币种不匹配拒绝新领取，不能绕过组织限制。observe不阻止领取，但保留真实未知。所有已发生计量仍入ledger与账户，不因策略更改抹去历史。

## 核心DAG接缝（runtime代理拥有）

新增可在启动时设置的Engine.AttemptAccounting接口（线程启动后不修改），内部领域扩展，不接受HTTP客户端注册：

- AccountingAttempt{TenantID,ProjectID,RunID,NodeID,WorkerID string; Attempt int; Fence int64; Spec Node}。
- CheckAttemptTx(ctx,tx,ref AccountingAttempt)(bool,error)：同领取事务/当前Runner权限之后、资源预留之前；无收费，只按稳定顺序锁并检查当前账户、费率和策略。仅工具绑定节点参与，无绑定直接true。
- ReserveAttemptTx(ctx,tx,ref)error：资源全部预留成功后，确切attempt/fence固定前调用（ref用Attempt+1/Fence+1），同事务冻结不可变费率/策略/主体和额度。若错误整个领取回滚，不能泄漏部分资源/金钱。Check获得的锁保证Reserve不重新返回false。已有claim操作replay不得再预留或再次收费。
- RecordResultTx(ctx,tx,ref,result Result)error：CompleteBound完成来源校验和平台commit callback后、同tx提交前；只使用当前正式result，用attempt/fence/resultSHA幂等，重复回执不重复结算。
- RecordResolutionTx(ctx,tx,ref,decision string)error：人工DAG reconcile前后同tx保留财务未知；不得把not_started/人工成功当实测0或释放钱/token。租约过期/Tick无需写费用，原reservation仍active/held；UI按formal DAG attempt状态呈现未知。

Check deny要保留可诊断的静态阻止原因，不消耗attempt/槽位/金额，可考虑core node reason='Model token or cost allowance is unavailable'仅变更时保存事件。不能在现eligible回调收费。private/pool两路径均接；main显式注入Service实现。Server测试fixture亦需显式接入（不以用户ID伪造权限）。普通无hook纯DAG库测试继续独立；产品控制面必须启用hook。

## 账本与结算（backend代理拥有）

每新attempt只一条UsageAttempt，固定tenant/project/run/node/attempt/fence/worker/profile SHA及当前费率/两个scope policy versions。不可变result身份/原usage快照单独保存，人工确认追加审计不能改原件。表/索引由MigrateUsageAccounting(ctx)集中新增，方言复用Service.q/ddl/store.EnsureIndex，不删除旧记录。

- 领取有明确费率时预留input+output tokens与对应估算micros；没费率且observe时也建立unpriced attempt，不把缺失预留显示为已知0费用。
- complete已知input/output：实际reported token与estimate cost入账，释放未用预留；超过预留计真实值并标overrun，策略后续新领取按余额拒绝。result成功与费用状态独立。
- partial/unknown：可明确的已上报input/output下界入账，持有max(原预留-已入账,0)，unknown状态/待核对计数保留；未报告不能当0。任何未知结果、不完整usage、停止/租约过期/业务reconcile不自动退款。重复result和人工确认幂等。
- 整数溢出不得丢实际执行结果；保留usage原件，费用/合计置null、标需核对并保留预留。字段更新和账户/CAS/receipt同tx，真实结果不因展示汇总错误被拒。
- 人工confirm只适用于未完整结算/未知记录，需当前项目批准权限（涉及组织账户还要求组织owner/admin），完整新usage input/output和>=原已知每个计数、source_ref+reason+明确确认。标manual_confirmation，不冒充厂商实测；不允许降低已知使用量。新估算/剩余预留结算与审计/receipt原子提交；原始工具事实保留。不能直接清空余额。
- 人工确认成功结算后，已校验的晚到结果按尝试身份和结果摘要追加事实，保留其原始观察。已确认的计数、估算、预留及人工依据保持稳定。若确认发生在首条结果之前，仅补记原结果摘要并推进账本修订。差额调整入口待交付。 [阶段验证](../docs/quality/usage-confirmed-receipts-verification.md)

## DTO（backend先落类型，Root统一生成）

UsagePolicy{id,scope,project_id,version,mode,currency,token_limit:*int64,cost_limit_micros:*int64,updated_by,updated_at,allowed_actions:[]}。
UsagePolicyWriteRequest{project_id,scope,expected_version,mode,currency,token_limit,cost_limit_micros,reason,acknowledge_enforcement:bool,idempotency_key}。
UsageRateRevision{id,project_id,tool_profile_id,version,profile_sha256,model,endpoint_origin,currency,input_rate_micros_per_million:int64,output_rate_micros_per_million:int64,input_token_reservation:int64,output_token_reservation:int64,reason,created_by,created_at}。
UsageRateWriteRequest{project_id,tool_profile_id,expected_tool_version,expected_version,currency,input_rate_micros_per_million,output_rate_micros_per_million,input_token_reservation,output_token_reservation,reason,idempotency_key}。
UsageMoneyBalance{currency,spent_micros,reserved_micros}。
UsageBalance{scope,project_id,period_start,reported_tokens,reserved_tokens,unresolved_attempts,costs:UsageMoneyBalance[]}。
UsageDashboard{project_id,period_start,as_of,project_policy:UsagePolicy,organization_policy?:UsagePolicy,organization_enforced:bool,balances:UsageBalance[],rates:UsageRateRevision[],rate_total:int,can_manage_project:bool,can_manage_organization:bool,can_manage_rates:bool}。rate列表有界100并显示rate_total，UI更多profile使用现toolprofiles，不假全部覆盖。
UsageAttempt{id,project_id,version,run_id,node_id,attempt,fence,worker_id,tool_profile_id,profile_sha256,model,endpoint_origin,currency,rate?:UsageRateRevision,state(reserved|settled|partial|unreported|reconciling|confirmed|unpriced),usage?:toolcontract.Usage,usage_state,reported_tokens:*int64,estimated_cost_micros:*int64,reserved_tokens:int64,reserved_cost_micros:*int64,held_tokens:int64,held_cost_micros:*int64,usage_source(none|tool_evidence|manual_confirmation),decision_by,decision_at,decision_reason,source_ref,overrun:bool,source_result_sha256,created_at,updated_at,allowed_actions:[]}。
UsageAttemptPage{items:UsageAttempt[],total,next_cursor}。
UsageConfirmRequest{project_id,expected_version,usage:toolcontract.Usage,source_ref,reason,acknowledge_usage:bool,idempotency_key}。

## API（Root拥有HTTP）

- GET /usage?project_id&month=YYYY-MM →UsageDashboard（缺month取当前UTC月）。GetUsageDashboard(ctx,actor,project,month)。
- POST /usage/policies →UsagePolicy；WriteUsagePolicy(ctx,actor,project,req)。
- POST /usage/rates →UsageRateRevision；WriteUsageRate(ctx,actor,project,req)。
- GET /usage/attempts?project_id&month&state(optional)&limit=20&before=id →UsageAttemptPage；ListUsageAttempts(ctx,actor,project,month,state,limit,before)。limit1..100稳定created_at/id keyset。
- POST /usage/attempts/{id}/confirm →UsageAttempt；ConfirmUsageAttempt(ctx,actor,project,id,req)。
- GET /usage/attempts/{id}/facts?project_id&limit&before →UsageAttemptFactPage。授权及游标绑定当前项目和精确尝试，limit 默认 20、上限 100。排序按实际 UTC 时间降序，纳秒相同按事实 ID 降序。仅返回证据 ID、类别、存档时间、摘要、原始结果标记及可公开计数和覆盖状态。该接口用于核对观察与人工确认，无写操作。 [验收](../docs/quality/usage-evidence-verification.md)

当前权限在receipt replay前重核；Runner/Agent token不能访问管理读写。严格未知字段/null/完整显式nullable字段与月份/游标范围校验。金额只显示各自币种，不用汇率。宿主诊断中文/英文成对，原用量、模型ID、用户reason原样保存。

## 文件所有权

- backend代理：internal/platform/usage_accounting*.go（除Root HTTP），DTO/迁移/费率/策略/账户/ledger/实现DAG hook/有界读取及7左右必要权限/幂等/事务/unknown/精确计算检查。可在本域自己的测试显式注入hook；共享http/main不改，Root接。请先落types。
- runtime代理：internal/dag/accounting*.go、新接口与scheduler.go/pool_claim.go/commands.go/engine.go必要接缝及必要corehook顺序/rollback/replay测试。只改DAG，不再改TS/Runner；当前实际最终usage协议足够，不新增模型调用/依赖。
- UI代理：UsageWorkspace.tsx/.css、usage-accounting中英目录及4项必要交互测试；Props session,project,refresh；唯一设置sub=usage，Root接菜单。页面月度+当前项目为主，组织卡片仅服务端给出；原始counter/unknown/estimate货币隔离、费率/额度CAS编辑、分页来源run/attempt链接与unknown确认，默认简洁折叠进阶。
- Root：HTTP/main/database/共享导航/OpenAPI及生成类型、NLS、文档/内存、最终整合/本地真实必要fixture/提交。Live89312/control250364/holder227944不能覆盖/停机，除Root最终配对升级。无新依赖/全套重复测试/厂商付费调用/push，raw证据与截图保留.cache。
