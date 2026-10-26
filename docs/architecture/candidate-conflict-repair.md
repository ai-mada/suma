# 固定冲突修复架构与 AI SOP

状态：T-P4-032／1.0.0 限定本地已验证，2026-10-27。源码、定向检查、实际本地运行及生产验收分别记录。
依据：[修复合同](../../contracts/candidate-conflict-repair.md)、[PRD](../product/candidate-conflict-repair.md)、[交互](../design/candidate-conflict-repair.md)、[验证报告](../quality/candidate-conflict-repair-verification.md)。

## 领域和不可变来源

CandidateCompositionConflictBasis 固定原 composition／task／run版本、准确 node／attempt／fence／worker／assignment、原结果字节摘要、typed证据摘要、完整原 Git 冲突证据及整体依据摘要。`expected_conflict_sha256` 指整体 basis.sha256；原 evidence.conflict_sha256 是独立的 Git 冲突摘要，两者不可混用。

去重使用原不可变 composition／assignment／result／evidence身份，不包含当前可变 task／run版本；当前完整 basis SHA仍用于新操作的CAS。原任务编辑不能藏起已有修复或绕过单任务约束，新的来源或权限也不会回填历史依据。

GET `/candidate-compositions/{original}/conflict` 返回只读上下文、完整依据、已有修复计划及当前创建资格。路径列表不足以授权；缺少原生冲突树、stage或准确失败结果时保留可读阻断。

POST `/candidate-compositions/{original}/repairs` 在同一事务建立新的未开始任务和新的 CandidateCompositionPlan，保存 conflict_basis、固定原来源／顺序、工具版本／摘要及路由。稳定操作回执和原冲突去重均返回准确新任务，不重写原失败，不提前建立run。

priority与owner_id为可选字符串。省略priority在回执摘要前归一为medium，显式无效值或null拒绝；省略owner仅在当前用户唯一匹配活动项目human资源时绑定该资源，否则不分配。明确负责人和优先级的既有实际旅程保持原事实。

新计划沿用既有独立 start 确认、managed DAG、candidate_from、原浏览器Review及 accepted composition 基线合同。修复任务保留给固定计划，不走普通派发绕过原依据。新候选必须获得自己的正式独立测试和明确验收。

待启动修复计划可显式 commands.refresh，使用 expected_version、理由和稳定操作编号，重核固定任务／冲突／来源／工具／路由后只延长有效期并更新计划版本。它不改变任务或流程定义、不创建运行、不自动重新执行，前端沿用原命令恢复。普通合成仍保留原 dismiss 合同。

## 宿主执行与成熟接缝

原生能力只有在管理员本地 `NativeWorkspacePolicy.ConflictRepair`明确开启时才探测为 `suma.candidate-conflict-repair.v1`。浏览器工具配置不能创造本地能力。每次创建／启动／代码读取仍检查当前工具、源验收、原委托、路由与consumer租约。

purpose标记或typed修复字段出现即进入专用路由校验；必须同时匹配agent、`purpose=candidate_conflict_repair`、完整typed执行、独占输入模式、本地native opt-in和真实探测profile。宿主通过原生桥提供工具与预算，不把该标记传成只读文档模型purpose；缺配、混合模式、普通或只读能力均不能获得修复权限。

可信宿主导入准确授权2–16份bundle并复现固定原Git冲突。模型经既有受限工作区工具读取、CAS修改、开发检查和受控 resolve／continue 逐步处理全部来源。固定宿主步骤、父历史及每步证据必须闭合，才封存新候选；不使用 ours／theirs、静默丢源或直接Git通用写权限。

独立测试继续导入新producer的精确候选。来源测试、开发自测、封存、正式测试、人工验收和目标写入回执各自保留语义。已发生的证据不随之后源变化而删除，但新使用、批准及基线绑定会被阻断。

专用修复seal的previous_commit是全部来源已处理后的真实合并前缀。若随后没有额外编辑，seal可记录no_change／0；候选相对管理员锚点B仍可包含多文件差异。UI仅在完整typed修复与准确工作区head匹配时使用“合并完成后无额外编辑”，不改普通seal规则或其不可变原件。

受管同运行重试只适用于已知失败且固定定义幂等的节点，重核来源、原调用凭据、工具和路由后新增attempt／fence；已执行后继、非幂等或未知效果均阻断。失败attempt保留，未知先核对原操作；该入口不放宽其他受管发布运行的命令权限。

## 前端与 AI SOP

CandidateConflictRepairPanel 负责当前只读上下文、表单、明确创建及原操作恢复；子任务复用 CandidateCompositionPanel。TaskCodeBaselinePanel继续绑定新已验收计划ID，不增加伪装成原计划的来源联合。

使用当前账号／组织／项目／原计划 scoped storage，保存原 path／body／key／身份。409保留草稿、读取最新；未知只核对原创建，不能重新构造请求。读取及写入401／403隐藏受保护投影，scope变化卸载与Abort。

CandidateCodeInputFacts只展示typed宿主事实，无读写API。校验新结果与工作区head及步骤数量一致后展示记录；不代替验收。

AI实施顺序：读原精确冲突 → 明确修复任务与完整来源 → 当前原生工具能力 → 创建预览 → 独立确认start → 宿主约束全部来源修复 → 新候选正式测试 → 人工Review → 明确目标／下游交付。外部模型、跨物理机器、完整设备与生产验收保持未完成边界。

## 限定实际证据与交付

Source X先完成4项正式检查及fixture测试owner Review；原A＋X合成保留已知冲突。修复attempt1在模型前失败，人工同运行retry产生attempt2／fence2并封存21cc5159候选；独立Runner B再完成4项正式检查与单独Review，D仅更新为task v2／baseline v1且未开始。实际共3项新运行、8项新正式检查、2次明确fixture Review、6次本地SDK模型协议请求及6次工具调用；用量unreported、厂商调用0，同一物理主机两个Runner。独立16来源Git测试不计入实际旅程。

实际执行使用b8a0运行切面；最终3114a切面补默认优先级语义，针对变化完成校验与构建，未重放已完成运行。最终web-delivery固定328项源输入和4269词条，代表性中英与冷回读已闭合。完整来源、产物及保留失败见[验证报告](../quality/candidate-conflict-repair-verification.md)和[机器摘要](../quality/candidate-conflict-repair-summary.json)。

主预览1.0.0前向升级到184表：首次预热前183旧表完整schema／行保持，仅新增1空表、无新索引；预热后仅4张协调锁表（共34行）的sequence合计增加22，其余179表完整行相同。原来源、授权与会话状态保留。这是本地维护和回读证据，不是生产或完整平台验收。
