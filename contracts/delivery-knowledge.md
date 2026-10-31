# 已验收交付经验与团队知识闭环

T-DELIVERY-KNOWLEDGE-172；速码suma 1.0.0；2026-10-31。基线38ea67e，完整31／P0–P7／58限定／34待交付／92总账有效。沿既有知识提案、发布引用、团队逐任务Codex选模及唯一Go DAG；不新建知识库、验收表、模型调用或派发器。

## 用户路径

任务正式验收后提供一个“提炼交付经验”动作，打开原知识提炼工作区并GET准确来源。用户预览、选择memory／spec／skill、工具和预算并明确允许模型使用；生成提案可编辑、放弃、采用为草稿，之后单独发布。下一次Codex团队任务明确选用同一已发布修订，经原快照／Start／claim核验后消费。导航、GET和刷新不调用模型；草稿不自动发布或安装技能。

## 共享合同

- 新来源`kind=accepted_delivery`，`id`为任务ID、`version`为当前正式验收后的任务版本。
- 只读`GET /api/v1/tasks/{id}/delivery-knowledge-source?project_id=...`返回`DeliveryKnowledgeSourceView {project_id,source:ProjectKnowledgeSource}`。同当前项目读权限与正式质量事实；无新写操作。缺失／越项目按原404／403规则，不合格或来源漂移409。
- 原`ProjectKnowledgeSourceInput`新增`basis_sha256?`；accepted_delivery必须为预览得到的64位小写SHA，其他来源禁止非空该字段；客户端不能提交delivery对象。quote仍为原正文子串，空值选完整可见正文，不静默截断。
- `ProjectKnowledgeSource`与`ProjectKnowledgeContextSource`新增可选`delivery:DeliveryKnowledgeBasis`；accepted来源必须有，普通来源不能冒充。纯DTO见`internal/platform/delivery_knowledge_types.go`，所有TS与Schema从Go生成。
- Basis固定protocol=`suma-accepted-delivery-v1`，包含原`ReleaseTaskBasis`、RunVersion、真实测试及可选代码候选metadata。Tests／Candidates始终数组，最多256／128项，稳定身份排序后将自身SHA置空计算Go canonical摘要。Source.SHA仍是完整规范来源正文SHA；analysis来源及知识修订SHA继续包含整个Sources（包括typed basis）。

## 正式事实与权限

复用`currentReleaseTaskBasisTx`及原证据检查：Review.decision=accepted、同tenant／project／task／run，Review.task_version+1==Task.Version，Task.RunID及verified_run_id相同，原任务／需求／依赖／设计、测试计划、候选合成及当前运行证据均有效。成功运行、验收申请、任务列表状态、模型输出均不能代替正式验收。

读取真实持久节点／attempt／fence／worker／result摘要、已发布测试报告和实际候选，不从可编辑Task.Data制造测试或代码事实。Candidate.kind明确为workspace_revision或sealed_bundle；前者只描述真实工作区修订，后者必须核实ValidSealed、transfer／bundle及当前来源后才带封包字段。纯测试交付允许Candidates=[]；缺代码提交不生成假的SourceCommit。正文只含当前项目可读的任务描述／验收条件／正式评审说明及紧凑证据metadata，不含raw日志、XML、私有会话、thought、RPC或凭据。正文／上下文仍受原有边界限制，超过范围明确拒绝，不伪造精简后的完整验收。

当前消费者授权在外层先核；内部basis helper只做同事务tenant／project事实读取，不能构造Actor、嵌套新事务或创造消费权限。Create两事务、claim、proposal采用、知识publish、资格投影和下次发布引用消费均重核同一来源；旧评审者后来登出不抹掉当时有效接受事实。

原回执恢复先核当前调用者，再恢复原body／key对应结果；依据改变阻止新的采用和消费，但保留历史提案／修订与已经发生的输出。不能因漂移删除原件、自动重新提取、再发模型或自动发布。同一原操作不得重复建立Run或草稿。

## 模块所有权与实现

主集成拥有共享DTO／类型增量、HTTP总挂点、生成合同、文档、状态与Git。交付领域代理拥有新delivery_knowledge resolver／HTTP helper与正式Review夹具；知识领域代理拥有原source解析／current-check／发布投影及相关定向验证。Web代理拥有简洁任务动作、独立来源导航、typed来源卡与原提炼工作区。原21项工作尤其TaskConversationKnowledge文件不混入。

私有领域入口固定`resolveDeliveryKnowledgeSourceTx(ctx,tx,tenant,project,taskID,current) (ProjectKnowledgeSource,error)`；公开Service入口`GetDeliveryKnowledgeSource(ctx,actor,project,taskID)`。知识领域分支调用该helper后核版本／basisSHA／quote；不得将accepted_delivery传入`suma_ + kind`普通Record查询，也不得伪造Record。保留普通来源批量读取，accepted来源在同事务按任务缓存去重。

## 复用与验收

HelixHarness固定44ffc536f73d8af525b4795ad10384c516898576，借鉴Propose→Apply→Archive、固定来源及证据摘要、确定路由和按需上下文；原数据库／CAS／DAG／预算是业务权威。Apache2／NOTICE适用于实际复制，改编材料另保留其许可；不搬审批／沙箱旁路启动器，不把本机process pool、hooks或mock评估当分布式质量门禁。Multica参考e8ae72c9dfc1b4474b61a9087f51b33cbee6a048，附加许可边界保持。

本阶段仅做新增与受影响的授权、来源漂移、原回执、正常Review、提案采用／独立发布／固定Codex上下文检查。一次必要类型与中英文检查、代表实际界面；不重171旅程或无关全套。正常Review流程建立验收，不用SQL伪造；受控模型与真实厂商、单机与物理多机分别记账。完整Codex、技能优化、跨设备同步及全部平台目标继续实现，阶段不等于完整交付。
