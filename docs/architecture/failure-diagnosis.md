# 失败诊断架构与 AI SOP

状态：T-P4-030 已完成必要检查、同机真实链路与主预览前向更新；版本：1.0.0；日期：2026-10-27。
本地模型协议、厂商／跨机器及生产范围分别记录，完整平台仍在实施。
依据：[合同](../../contracts/failure-diagnosis.md)、[PRD](../product/failure-diagnosis.md)、[交互设计](../design/failure-diagnosis.md)、[限定验证报告](../quality/failure-diagnosis-verification.md)。

## 领域与执行接缝

新增 `internal/platform/failure_diagnosis*.go` 领域，复用真实失败投影、当前项目授权、托管单节点 DAG、
原生只读模型请求、私有／Pool Worker 与现有用量账本，不复制问题系统、模型网关或调度器。
界面使用独立 `FailureDiagnosisPanel`，由既有运行／专家计划／问题入口提供准确来源定位。

专属 `Profile.FailureDiagnosis` 固定 protocol、model 和 endpoint_origin；只允许 read_only Suma、无工作区，
与 Planning／KnowledgeExtraction 互斥。离线 `--failure-diagnosis-probe` 证明本机协议和固定配置，不证明厂商账号健康。
原生 `purpose=failure_diagnosis` 复用成熟模型客户端，以单次 chat/completions 返回完整 JSON 与真实用量，
没有平台工具或工作区工具，也不接受模型增权。`max_steps=1`、`MaxRetries=0`，失败不自动再付费调用。

## 来源构造与严格输出

`FailureDiagnosisContext` 在当前 tenant／project 下读取既有失败 issue、准确节点和当前 attempt。
原 issue 尚未投影时返回明确等待；GET 不创建业务记录。`source` 只来自服务端持久事实，
包含 run／node 状态及版本、attempt／fence／Worker、issue／task／requirement 版本和有序证据。
实际结果未知、过期失败尝试或原运行需核对时不能创建诊断。

失败投影、任务交付和诊断共用 `failureIssueIDTx` 所在模块中的回执合同。读取时在当前事务检查准确作用域下的 `run_failed`，再按运行固定流程名称重算投影摘要，并核对原问题 ID 与创建范围。问题可编辑字段仅作为阅读信息，正式关联从原回执读取。创建、领取和采用通过现有来源复核使用同一关系。 [阶段验证](../quality/failure-issue-provenance-verification.md)

问题详情展开时请求 `issue-source`。控制面在已授权项目的故障投影回执中按原问题 ID 反查候选，随后用同一校验器确认运行。只有唯一可验证关联才返回运行号。该读取复用现有回执表及当前项目范围，无新增持久化表或回填。 [界面验证](../quality/failure-issue-navigation-verification.md)

故障投影最多每批处理 50 条。首次处理按事件序号排序，到期重试按数据库时间与事件序号排序；两类都有待办时保留最多 10 个重试名额，空余名额可互补。投影或确认失败按事件记录重试代次，退避从 1 秒递增至最多 300 秒。投影成功才写确认并在同一事务清理重试；若问题已提交但确认失败，沿原回执重放。运维通过 `control.failure_projection` 日志定位事件序号。 [验证记录](../quality/failure-projection-retry-verification.md)

证据最多六项，每项 8000 UTF-8 字节，超限取 UTF-8 边界安全末段并记录 `selection=tail`、selected／total 字节。
单项摘要保留完整原字节身份，source SHA 绑定身份、版本及实际片段；整个 prompt 超过 32000 字节时拒绝。
不能把片段 digest 与完整源 digest 混为一谈，也不能用浏览器提供的输出文本替代事实。

`FailureDiagnosisOutput` 严格包含 schema_version、summary、observations、hypotheses、recommended_actions、test_plan、repair。
观察／假设只能引用服务端固定数组的 0-based evidence_indices；正文不是命令、引用不是授权、置信不是根因证明。
严格拒绝重复／未知字段、无效 Unicode、非规范整数、越界引用与不完整 JSON。
解析失败保存失败及发生过的真实输出／用量，不截断后冒充有效提案。

人工只修改独立 `FailureRepairDraft`：标题、说明、优先级与验收条目。
proposal、original_output 和 original_output_sha256 保持原件；修订不改写模型的观察、假设或建议。

## 持久化与当前权限

| 存储 | 事实 |
|---|---|
| `suma_failure_diagnoses` | 当前 analysis CAS、固定来源、工具、诊断运行、原输出与人工修复稿 |
| `suma_failure_diagnosis_executions` | 准确 attempt／fence／result SHA 的实际完成事实及用量关联 |
| `suma_failure_diagnosis_adoptions` | 同问题唯一正式采用关系及需求、来源／模型／人工修订快照 |

原创建凭据引用、Worker 及 route 保存于私有执行记录，不进入公开 DTO 或模型。
创建和治理写均使用当前浏览器项目写权；领取及结果回写沿现有真实 Worker／Pool 身份、attempt、fence、租约、
工具摘要和路由／grant 核验。撤权或来源漂移不能抹掉已经发生的输出与费用，但阻止新的不合格效果和采用。

若接入同一未停止只读 attempt 的晚到已知回执，须为本 purpose 单独验证准确执行、当前创建者、工具与租约证明，
不能直接继承知识提炼的特许分支。未知保持原诊断运行恢复入口，不创建替代调用。

## 正式需求采用事务

保留现有问题回流的 `source_issue_id` 与稳定 `issue-intake-<issue_id>`，由同一关系保证同问题去重。
存在需求时返回当前真实记录，不覆盖原需求，不伪造该分析的新采用关系。
新采用在同一事务核对当前权限、analysis 版本、固定 source 与问题依据，并提交需求 draft、人工修复内容／验收、
模型与来源溯源、analysis 状态、活动和原操作回执。网络、模型和进程执行不进入此 SQL 事务。
并发分析不等于允许并发创建多份问题回流需求；丢响应只恢复原 key。

需求采用不更改原任务、run、正式 review、专家计划或 Git 基线。
原业务继续从需求草稿进入设计／基线／开发／独立测试与正式验收，不能因为模型建议自动标记修复成功。

## UI、协议与 AI SOP

Context／列表在展开后按准确来源范围读取；原失败运行与诊断运行分别展示。
当前账号／组织／项目／run／node／attempt 键隔离数据及未知恢复；关闭或范围切换 Abort 旧请求。
同路径刷新保留已读值以免依赖读取自取消，但加载和读取失败期间不能据旧事实发起采用。
只对当前打开的活动 analysis 做有界只读刷新，不全项目轮询，也不在浏览器后台提交诊断。

操作时先确认真实 failed 和范围，再取得明确模型同意；模型提案与事实分开审阅，只修改 repair。
409 读最新、保稿、重确认；来源失效准备新分析，执行未知先核对原 run。
采用后打开真实需求，并保留原 issue／run／专家计划入口；不要从模型输出构造任意回链或权限。
必要验证复用成熟 Core／Runner／官方 SDK、成本与问题回流能力，实际本地协议和厂商／跨机器／生产范围分别记录。

## 已核对的本机实际接缝

同一 Windows 主机的真实 Control、Runner、DAG 与账本接入一次自有 loopback 固定模型响应，
固定失败 run 与诊断 run 分开，模型输出／source SHA 保存，人工仅修订 repair，采用事务最终关联同问题的需求 draft。
分析最终为 adopted v4、需求 draft v1；实际用量为一次请求、40 Token、估算 57 CNY micros，厂商调用为 0。
原件、人工稿及原失败事实保持，模型线程停止，Control／Runner 均正常退出 0，来源清单和旧业务行不变。

最终配对使用 `runtime-final` 与 `web-release`，不是中间 `web-reviewed`。
冷启动 Control 进行只读回读，验证原分诊深链、类型化采用来源与中英刷新；未运行 Runner、模型或新增登录。
需求详情只为当前选中的需求补一次 GET 取得类型化采用信息，不按列表逐项读取。
公开 canonical 原因／证据不替换，仅通过诊断元数据在当前语言显示机器理由，人工原因保持原字节。

主预览迁移仅新增三张诊断表及一个索引；原 177 张表 schema 与完整行保持，共 180 表且新表为空。
原 18 条 authority、0 Grant、1 failed／2 succeeded 运行保持，没有发新凭据或额外模型调用。
证据见本地[实际闭合](../../.cache/e2e/failure-diagnosis-stage-030/actual/closed.local.json)、
[最终界面与原件回读](../../.cache/e2e/failure-diagnosis-stage-030/browser/verification.local.json)、
[冷回读闭合](../../.cache/e2e/failure-diagnosis-stage-030/final-readback/closed.local.json)、
[主预览最终核对](../../.cache/e2e/failure-diagnosis-stage-030/primary-upgrade/final-verification.local.json)，
及[质量报告](../quality/failure-diagnosis-verification.md)／[机器摘要](../quality/failure-diagnosis-summary.json)。
这些结果不扩展为真实厂商、跨物理机器、全套回归或生产验收。
