# 固定失败证据、智能分诊与修复需求

阶段 T-P4-030；suma 1.0.0；2026-10-27；Owner：平台集成。
状态：共享实现合同。完整 P0–P7、真实厂商／远端及多候选受测代码基线交接仍在范围中。

## 产品与领域决定

在原运行失败节点、专家计划阻断项及问题详情内提供同一默认折叠入口；不增加导航或独立问题系统。
使用真实持久失败 issue、准确 run/node/attempt/fence 及版本化证据，进行一次明确授权的只读模型分析。
模型观察、假设、建议与实际执行事实分开；编辑修复草稿并明确采用后创建正式需求池 draft。
需求可回到原问题／运行；创建草稿不代表已修复、不改变任务验收、不恢复／重跑节点、不自动关闭问题。

已失联／reconciling 或实际停止不明确时，只引导核对原运行。没有已证明 failed 的准确节点，不能启动此分诊。
多个前置任务的代码组合另沿受控 Git 集成与准确结果独立测试实现，不借此放宽代码或运行权限。

## 执行与成本

新专属 `Profile.FailureDiagnosis`，protocol `suma-failure-diagnosis-v1`，字段 model/endpoint_origin。
仅 suma、read_only、无工作区，与 Planning／KnowledgeExtraction 互斥；实际离线
`--failure-diagnosis-probe` 固定模型和 origin，配置存在不代表在线可领取。
原生 `purpose=failure_diagnosis`，max_steps=1、max_tokens 1..8192（默认4096），无平台／工作区工具，
一次 chat/completions JSON 请求，不能以模型请求工具扩展范围。复用成熟原生客户端、Go Runner、托管单节点 DAG、
官方 SDK 及当前费用账本，不增加模型网关／执行器或依赖。节点 MaxRetries=0。

创建需当前浏览器项目写权、明确 acknowledge_model_use。领取前核对原真实创建凭据、当前权限、来源、profile及路由；
完成回执按实际 Runner／Pool、attempt、fence和固定工具保存发生过的原输出与真实用量，不能因退出／来源漂移抹除事实。
采用前再次核对当前来源。来源变化保留原件但 `basis_current=false`，禁止采用。未知结果保留，默认不自动重新调用模型。
若实现同一未停止只读 attempt 的晚到已知回执核对，必须有本 purpose 专属精确身份／当前创建者／工具／租约证明；
不得直接沿用知识提炼的特许分支。任何不支持的恢复都显示原运行核对入口。

## 固定来源

`FailureDiagnosisSource`：sha256、issue_id/issue_version、run_id/run_version/run_state、node_id/node_state、
attempt/fence/worker_id、可选 task_id/task_version/task_title、可选 requirement_id/requirement_version、evidence数组。
只能从当前 tenant/project 的原 issue 关联 run、已持久准确节点及相应任务读取，不接受调用方自造输出或来源正文。
节点必须 failed 且是当前所选 attempt，结果已知；原 run 不得 reconciling，也不得以一次旧失败覆盖新尝试。
固定 issue/task/requirement/节点原内容与实际执行身份；下次执行／任务或问题依据变化使采用失效。

`FailureDiagnosisEvidence`：kind、text、sha256（完整来源字节摘要）、total_bytes、selected_bytes、selection（full|tail）。
固定当前 issue说明、关联任务说明／验收和准确失败节点的 error/output，按既定顺序；最多6项。
每项最多8000 UTF-8字节，超出时取UTF-8边界安全的末段并明确 tail/原总量，界面展示这次实际发送范围，
原完整证据仍可在运行中打开。不能把截段宣称完整证据；完整 prompt ≤32000 UTF-8字节，超出明确拒绝，不二次静默裁剪。
source SHA 包含全部实际身份、版本与选中内容／范围；模型不读取未选资料、其它项目或秘密配置。

## 结构化输出与可编辑草稿

`FailureRepairDraft`：title（1..200 UTF-8字节）、description（1..16000）、priority（low|medium|high）、
acceptance_criteria（1..16条，每条1..1000字节）。

`FailureDiagnosisObservation`：statement（1..2000）、evidence_indices（1..6个唯一0-based固定来源索引）。
`FailureDiagnosisHypothesis`：同 statement/evidence_indices，加 confidence（low|medium|high）。
`FailureDiagnosisOutput`：schema_version=1、summary（1..2000）、observations（1..8）、hypotheses（0..8）、
recommended_actions（1..8个1..2000字节文本）、test_plan（1..8个1..2000字节文本）、repair（FailureRepairDraft）。
所有内容只是模型提案；观察亦需用户核对，不因为有引用就成为实际原因。
严格解析重复／未知字段、整数／Unicode／大小写、范围和完整JSON；不接收模型制造的租户／权限／命令／来源ID／SHA。
原件与SHA单独不可变保存，解析错误记录失败与真实用量，不编造 ready 或静默再次调用。
人工只编辑 `repair`，模型 observations/hypotheses 与原件保留；建议不直接作为命令执行。

## 需求采用与去重

沿用正式问题→需求关系 `source_issue_id` 和稳定 `issue-intake-<issue_id>`。
若问题已有回流需求，当前读取返回其真实记录并提供打开；不得默默用模型正文覆盖原需求，也不伪造新的模型采用关系。
尚无回流需求时，adopt 同事务提交新draft、明确修复正文和验收标准、来源／模型／人工修订采用关系、analysis CAS、活动与回执。
仅在当前浏览器写权下采用，后续需求基线／开发／验收按原领域处理。并发采用只允许一个正式问题回流草稿，
丢响应恢复同一操作；先核对当前权限再读回执。一个问题可有多次分析，但不意味着多份重复修复需求。

## 公共接口与DTO（A先实现精确Go类型）

- GET `/failure-diagnosis/context?project_id&run_id&node_id&attempt`：`FailureDiagnosisContext`，
  project_id/run_id/node_id/attempt、source?、issue?（Record）、requirement?（Record）、eligible、can_create、blockers[]。
  是只读，不在GET中造问题；issue尚未投影时明确等待，不假空成功。当前有权者可看事实，创建能力单独核验。
- GET `/failure-diagnosis/analyses?project_id&run_id&node_id&attempt&limit&before`：
  `FailureDiagnosisPage{items,total,next_cursor}`，默认20、1..100、稳定范围绑定游标。
- POST `/failure-diagnosis/analyses`：`FailureDiagnosisCreateRequest`，project_id、issue_id、expected_issue_version、
  run_id、node_id、attempt、expected_source_sha256、goal（1..2000）、locale（zh-CN|en-US）、tool_profile_id、
  expected_tool_version、max_steps（可省略默认1）、max_output_tokens（可省略默认4096）、acknowledge_model_use、idempotency_key。
- GET `/failure-diagnosis/analyses/{id}?project_id`：准确 `FailureDiagnosisAnalysis`。
- POST `/{id}/commands`：`FailureDiagnosisCommandRequest`，project_id、expected_version、action（revise|adopt|dismiss）、
  repair?（仅revise必需，其它必须省略）、reason、acknowledge_draft（adopt须true）、idempotency_key。

`FailureDiagnosisAnalysis`：id/project_id/version/state、goal/locale/created_by/created_at/updated_at、
tool_profile_id/tool_profile_version/tool_profile_sha256、run_id（诊断运行）、max_steps/max_output_tokens、source、
proposal?（原结构化输出）、repair?（人工可编辑稿）、original_output/original_output_sha256、
adopted_requirement_id、reason、allowed_actions[]、basis_current。
state：running|ready|failed|reconciling|adopted|dismissed。source.run_id 是原失败运行，不能与诊断run混淆。
Context另提供可选同位置 `blocker_diagnostics`；Analysis提供可选 `reason_diagnostic`，均为正式Rendered显示元数据。
canonical blockers/reason原文保持；只有受控平台警告生成message_key，人工决定原因不生成此元数据。
客户端按当前目录动态重显，不因语言切换重新读取或把旧译文存成业务依据。
列表返回完整有界analysis，原件不本地翻译。所有受控blockers/reason/HTTP错误、CLI／页面配置提供中英闭合目录。
写入携带CAS与稳定操作key；严格拒绝重复／未知参数、显式null可选正文、非规范安全整数与无效Unicode。

## 交互、分工与验收

同一折叠面板：失败事实／实际发送证据 → 模型、预算和明确同意 → 真实诊断进度 → 可核对观察／假设和修复草稿 →
明确创建需求／打开已有需求。原始依据高级折叠；加载／空／无工具／失联／失败／403／CAS／原操作恢复均可解释，
范围变化和关闭后停止读取；只对打开的活动诊断做有界只读刷新，不自动发起模型或采用。
复用现有Radix／Field／令牌与用户已接受布局，无需重复付费视觉生成；中英切换保留输入、来源原文和真实深链接。

A拥有 `internal/platform/failure_diagnosis*.go`（Root HTTP除外）；C拥有toolcontract／Runner／native TS purpose；
UI拥有独立面板、模型hooks、领域词条与PRD／架构／交互文档；Root拥有共享HTTP／迁移／admission／private及Pool完成／
费用与原操作核对接线、App／Workflow入口、生成合同、配置入口、阶段证据和提交。
必要验证为来源/权限/用量/事务/并发采用、一次本地模型协议真实Control→Runner→保存输出→界面采用及中英刷新。
真实厂商、生产、完整设备／实库／规模另记，不能把夹具或成功构建作为这些验收。
