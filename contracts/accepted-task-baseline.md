# 已验收普通智能体候选的后续任务基线

T-ACCEPTED-TASK-BASELINE-222，速码1.0.0。基线87a9bc3ea85e5a55f2d02a6e1e07388574e874e3；完整31项／P0–P7及58limited／34pending／92总账保持active，前序T221为已提交／已推送的progress。

## 目标与共享合同

普通agent任务的当前正式验收代码候选可明确采用为另一未开始任务的基线，并通过原代码输入broker导入独立工作树；不要求伪造两源合成或终端计划。保持一个现有code_baseline、正式验收、独立测试及唯一Go调度权威，0新表／HTTP路径／CLI命令／dispatcher。

CodeBaselineBinding新增可比较标量source_task_id?:string、source_task_version?:int；其值只用于第三种普通agent任务来源，其他来源全空。完整source仍为原cc.Binding，含准确run/node/attempt/fence/worker、evidenceSHA、head及bundle；accepted_basis_sha256继续绑定RepositoryIntegrationBasis的当前正式Review与所有test proof，binding_sha256覆盖全部字段。三个origin严格互斥，不将0／空／null当其他origin的缺省。

原POST /tasks/{target}/code-baseline请求新增三个可选字段：source_task_id、expected_source_task_version、source_node_id。选择任务来源时三项全部明确提供；composition_id/expected_composition_version和terminal_candidate_plan_id/expected_terminal_candidate_plan_version均不得出现。请求不接受用户提供CandidateBinding、ReviewID或测试成功标志。目标原expected_task_version/expected_binding_version、acknowledge、reason、idempotency_key及未知结果原操作核对不变。

原GET /candidate-compositions/options的每条source新增显式bool can_bind_baseline。true只表示该同事务下的准确普通agent producer已通过原repositoryBasisReadTx、当前TaskVersion/Binding匹配及类型检查；不是派发或验收许可。原eligible与compositionCreate至少两源语义不改变。仅200项有界加载，不宣称全量；多producer按task_id/source_node_id分开，必须明确选择，不选第一个／最新。基线替换仍以原GET code-baseline.can_bind为准，不误用options.can_create（已有基线时它针对合成创建为false）。

## 服务端与执行

服务端复用repositoryBasisReadTx→currentReleaseTaskBasisTx、candidateSourceTx、当前attempt/fence/worker transfer、validateCodeTestEvidence的准确同SHA／不同Worker证据和原递归来源复核。第三来源要求原dag_nodes中准确spec.Type=agent，不绕过合成／终端专门计划；binding.Source.Source.NodeID从服务端确定。来源task版本须一致，来源不能等于目标。绑定／预览／启动／输入传输／正式验收每个原阶段都按当前授权及原来源重新校验。源退回、需求／Review／候选／测试变化阻止新动作，保留原完成与停止事实。

接受来源memo key增加task来源的task/version以及完整run/node/attempt/fence/worker/源摘要，防止同任务多producer互借依据；保留原有界深度／访问数和循环拒绝。Runner沿现typed Binding及原受保护code-input broker、bundle hash/commit校验和独立worktree，不新增执行分支。

## 简洁界面与恢复

原TaskCodeBaselinePanel在普通task模式提供“已验收任务候选／合成结果”来源选择；固定composition prop保持准确合成计划。普通来源使用原options、can_bind_baseline和明确producer选择，显示任务版本、步骤、提交SHA及原run节点链接；明确原因和确认后使用原POST。手工JSON／输入SHA不能自授，读失败／无权限／未知／空范围有中英提示。沿原组件/44px/窄屏/键盘，尽量不加页面或嵌套向导。

原账号／组织／API／项目／目标隔离、准确来源代次、双CAS与useWriteAction原回执保持。未确认写入保留原body/key/route，不因新options失效／列表外而丢原恢复；复核不是以新版本重造同意。来源、版本、节点、SHA或目标变化撤销旧确认；独立read投影不会写正式状态。

## 所有权与必要验证

Root owns本合同、candidatecompositioncontract shared Binding/validator及focused tests、原HTTP闭形分支、OpenAPI生成约束和generated contracts、文档／状态／实际整合／Git。后端代理 owns platform绑定DTO/BindTaskCodeBaseline/普通来源resolver/acceptance memo及原options additive bool和定向测试，不动Root shared/HTTP/gen。前端代理 owns原panel／model／两域词条及定向测试，不动sharedgenerated/平台；实际代理 owns isolated验证夹具和当前资源检查，等待freeze后唯一必要构建，不改业务源码。

重点验证闭形三origin/摘要；单agent正式验收候选→双CAS绑定→原派发/不同Worker读取准确bundle；来源版本/Review退回/跨项目/非agent/多producer缓存隔离/旧callback/未知原操作恢复；直接旧composition/terminal回归。只检查变化边界，不跑无关供应商／模型／全Go套件。真实Git与测试夹具Review、代表界面、实际厂商与生产验收分别记录，不以代码或夹具称完整平台完成。
