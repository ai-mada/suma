# 修复计划修订与运行溯源架构

状态：T-P4-033／1.0.0，2026-10-27。领域实现、同机实际闭环、最终冷回读和主预览前向恢复／warm已验证；见[限定验证](../quality/candidate-repair-revisions-verification.md)。本文不宣称厂商、多物理机器、生产或完整平台验收完成。
依据：[033合同](../../contracts/candidate-repair-revisions.md)、[PRD](../product/candidate-repair-revisions.md)、[032架构](candidate-conflict-repair.md)、[操作与AI SOP](../operations/candidate-repair-revisions.md)。

## 计划、任务和运行

修订采用新`CandidateCompositionPlan.ID`，不在旧计划上替换流程或覆盖运行。版本族保留同一受保护修复任务、同一原不可变冲突及固定顺序的来源。新计划独立使用现有`composition-<plan_id>`运行标识、repair-v1执行信封和原生八工具，无需另造执行器或修改Git合并协议。

| 记录 | 责任与变化边界 |
|---|---|
| `suma_candidate_compositions` | 各计划的固定定义与状态；修订不改旧行 |
| `suma_candidate_composition_executions` | 准确run／node的不可变执行快照；旧结果与回执独立查找 |
| `suma_candidate_composition_repair_sources` | 原冲突与唯一修复任务的保护关系；仅当前计划指针通过CAS移动 |
| `suma_candidate_composition_repair_revisions` | 不可变版本族、父计划、序号、定义摘要、真实修订人／理由及恢复依据；限制每父项一个后继、每族序号唯一 |
| `suma_task_run_provenance` | 运行到单一任务的永久归属；新Start与任务重绑定同事务登记 |

原失败身份以不可变composition／assignment／result／evidence确定，排除可变task／run版本。新请求仍须提交当前完整`ConflictBasis.SHA256`，不能以稳定身份代替新鲜度核对。查找键不创造来源权限；当前子计划的完整受保护依据决定冲突身份。来源的验收依据、管理员锚点和顺序保持固定。

初始旧计划只在第一次显式修订时建立可追溯的观察快照，记为ordinal1；不伪造原准备时刻或缺失作者。后继记录真实当前操作者及修订时刻。公共`repair_revision`投影只暴露安全主体信息、根／父／当前计划、序号、理由、定义摘要及observed标记，不暴露凭据、Token或凭据摘要。

## 修订与Start事务

`GET /candidate-compositions/{id}/repair-revision-context`在当前项目授权下区分`ready`与`certified_startup_failure`，返回当前任务版本、完整冲突依据、必要恢复依据和阻断。只读请求不补写版本族或运行历史。

`POST /candidate-compositions/{id}/repair-revisions`接受当前计划／任务版本、完整冲突SHA、新流程及工具版本、理由、明确确认和稳定操作编号；失败替换还必须匹配`expected_recovery_sha256`。客户端不能提交任务ID、来源、assignment或自称已授权的证明。

修订事务依次重核当前实际权限、当前保护指针、原冲突身份与新鲜度、来源验收、任务上下文、全部恢复事实、新选工具的实时探测和授权及执行路由，再保存新计划、必要初始快照、不可变子版本、当前指针CAS、活动和原操作回执。任一步失败均不留下新任务、运行、半条版本链或已移动指针。

修订只准备新计划。失败前驱仍是任务当前run；新Start重新核对相同任务／前驱运行版本、完整尝试集合及摘要、来源和当前授权。建立新managed run、固定执行快照、登记运行归属和任务重绑定原子完成。并发修订、旧版本确认或前驱变化不能产生第二个后继或孤立运行。

## 类型化宿主证明与用量

`internal/executioncontract`定义`suma.execution-startup-failure.v1`。`StartupFailureProof`绑定完整私有／Pool assignment、准确固定工具profile、`purpose=candidate_conflict_repair`及`stage=before_workspace`，明确`workspace_started=false`和`process_started=false`。

Runner仅可在固定工具及purpose校验后、进入`prepareWorkspace`和主进程前的失败分支记录证明。进入准备流程后，即使失败且没有输出，也不能补发该证明。私有和Pool结果入口均校验类型化信封与持久assignment；证明与成功、停止、未知或工作区／工具／制品／集成效果并存时拒绝。

恢复依据固定计划、任务、运行版本以及全部有序已尝试assignment、准确结果字节摘要和证明摘要。最后一次可信启动失败不能遮蔽更早的模型或工作区执行。恢复还要求没有已执行后继、候选／传输、制品、补偿及活跃或未知工作；超出支持范围保持原运行恢复路径。

证明认证读取原不可变执行、节点、assignment和工具内容，不依赖当前计划头、当前工具启用状态或旧创建者仍有权限。当前工具暂停可以阻止新执行，却不删除真实旧事实。只有证明真实且不存在较早用量矛盾时，现有账本才可用`host_startup_proof`记录准确零输入／输出Token并且只结算一次；没有价格时保留unpriced，不推断币种、费率或追退旧的未报告用量。

## 当前准入与历史回执

新Start、refresh／dismiss、领取、代码访问及retry／resume均重核当前保护指针。被取代计划即使尚未有新运行，也不能继续产生新工作。现有范围内的cancel／reconcile以及准确原结果、Start和修订回执恢复，使用原执行和原操作身份，不借当前新计划授权。

Start回执命中后在当前实际授权下直接返回原已存结果，不编译后继流程。修订回执同样返回原准备DTO；后续Get才展示当前事实。任务指针切换不会导致旧run转查新计划，也不会把旧失败改为成功。

`GET /tasks/{id}/run-history`按当前项目／任务权限有界分页。永久归属登记与当前任务指针、受保护执行快照共同阻止跨任务借用运行。`bound_at`与`task_version`为可空历史事实：只能在实际绑定或原不可变依据能证明时填写；`observed_at`只表示观察时间，不替代原绑定时间。GET不追写历史。

## 交互、AI与验证边界

修复子任务使用折叠修订表单，路由展示服务端事实。返回准确新ID后更新链接并清除Start确认，保留原计划链接；不能用界面选择修改Runner本地管理员配置。响应结构验证先于清除持久待核对请求。401／403隐藏保护投影，409保留草稿，未知沿用原path／body／key。系统标签中英配对，原证据与用户文字保持字面内容。

AI只可在当前获准的新运行中按既有宿主约束解决全部来源冲突。新候选的封存、独立准确提交测试、人工Review、目标写入和下游基线绑定分别门控。旧计划历史、开发自测、启动证明或新运行成功都不能替代正式验收。

定向验证聚焦事务、并发、当前授权、完整尝试集、旧回执和私有／Pool边界；不重复未变的16来源Git／SDK路径。实际本地旅程、主预览前向升级、跨物理机器、厂商与生产接受度需独立证据。本页不据设计或代码存在推断这些结果。
