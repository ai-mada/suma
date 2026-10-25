# 需求设计基线、任务变更与采用合同

状态：T-P4-016 限定范围本地已验证，2026-10-25。负责人：平台集成；完整平台目标继续实施。
关联：[正式类型](../internal/platform/requirement_design_types.go)、[实施台账](../docs/delivery/implementation-plan.md)。

## 领域语义

需求 Record 是当前正式需求。需求设计是独立版本聚合，包含产品、体验、技术三段设计及完整任务提案；不是通用知识 SPEC 的另一个名字，也不修改正式需求来保存设计。
每次保存、发布、退役都追加不可变修订，包含服务端读取的正式需求 id/version/title/description/acceptance_criteria。设计聚合计数 version 与修订号分别维护；任务采用推进聚合 version，但不改写设计修订。

草稿可以保存尚未完成的设计；安全结构、大小与 key 唯一性须先合法。发布需要三段设计非空、正式需求至少一项验收、1–100个任务、每个任务验收非空、依赖闭合无环，并覆盖固定需求的每项验收。Covers 是固定需求版本验收条目的1起始索引。任务 key 在版本间稳定，不能以列表位置当身份。
产品／体验／技术合计最多16,000 UTF-8字节，单段亦不得超总限；整个任务提案 JSON 最多256KiB，单任务标题200、说明4,000，最多20条非空验收、每条1,000字节，最多100个依赖及100个覆盖索引。超过限制明确拒绝，不截断。
key 为1–64个 ASCII 字母、数字、下划线或短横线；priority 为urgent/high/medium/low，owner_id 是同项目合法资源或空。

编辑已发布设计形成新草稿，旧发布在新发布／退役前保留。发布或退役原子失效已绑定任务的当前验收及依赖链，不改写旧运行；需求变更使用既有失效链。来源失配、归档或删除阻止新发布、采用和新派发。
资格码 none/current/requirement_changed/retired；历史回执自身发布指针被替换时使用publication_changed，不能画成当前资格；展示资格不替代当前授权。

## 权限、接口与恢复

所有入口复用实际 Actor／当前事务 proof；读取、预览为 ProjectRead，保存为 ProjectWrite，发布／退役及任务采用为 BrowserProjectApprove。API可编辑提案，API／Agent／Runner不能批准。回执前检查当前身份和权限。

| 接口（均带project_id） | 语义 |
|---|---|
| GET /requirements/{id}/design | 当前需求与设计；尚未创建返回version0和空修订，不伪造已保存设计 |
| PUT /requirements/{id}/design | 完整草稿替换；expected_version、expected_requirement_version、正文、完整任务提案、key |
| POST /requirements/{id}/design/commands | publish或retire，绑定当前聚合及正式需求版本 |
| POST /requirements/{id}/design/preview | 只读预览当前发布的任务变更；expected_version/revision/sha256，不预留资源或创建任务 |
| POST /requirements/{id}/design/adopt | 明确采用已审阅的同一plan_sha256，当前聚合／需求／发布／任务范围再核验 |
| GET /requirements/{id}/design/revisions | 当前授权的不可变修订分页，默认20、最多100，游标绑定需求与租户项目 |
| GET /requirements/{id}/design/adoptions | 追加采用记录分页，同上 |

严格 DTO 拒绝未知字段／null集合／非法整数词法；所有写入带稳定操作key和预期版本。未知结果保留原caller、path、method、body、key；不因后续403/409换新请求。首次明确冲突可人工核对后重置基准，保留编辑正文；历史成功回执后重新读取当前设计，不用历史版本接下一次CAS。

## 任务变更预览与原子采用

四张正式表保存设计聚合、不可变修订、受保护的稳定key→task_id当前绑定、追加采用记录。绑定不能放在可编辑Record.Data中充当可信来源。
预览返回create/update/keep/exclude以及当前正式任务／预期任务版本和提案；plan_sha256绑定当前需求、发布、设计聚合和全部有关任务／绑定的完整范围。不能只比较前端提供的任务子集。

- create：尚无绑定的key创建todo任务，依赖key在同事务映射为正式task_id。
- update：正文／分配／验收／依赖或已采用设计修订变化，明确展示后CAS更新；不得无提示覆盖独立任务编辑。
- keep：当前正式任务与同一采用修订一致，保留状态、版本和绑定adoption_id。
- exclude：新发布不再包含的活动key明确归档任务并停用绑定，保留记录、旧run_id及历史，不删除。重新加入相同key复用原task_id，恢复todo并重新核验。

采用先重新计算完整预览并比较SHA，批次任务写入／映射／校验／失效传播／采用记录／聚合CAS／审计／回执同事务。任一非法依赖、负责人、版本或预算冲突全部回滚。
同key同正文恢复原结果；新key重新采用不会重复创建，因为映射复用。无变更预览不重写任务或绑定，可恢复最近采用结果；不凭空制造新交付事实。
保留原run_id供历史追溯，变更清除当前verified证据，旧执行不能通过新设计的验收。移出范围先失效链再显式归档，避免失效传播将归档任务重新打开。
本轮不把无绑定的旧手动任务自动认作同名任务；跨来源匹配／手动任务导入需要后续明确采用能力，不能按标题猜测。

## 执行与验收

受保护绑定解析 RequirementDesignContext，包含固定需求／设计版本与摘要、task_key、adoption_id及三段批准正文。正文只传一次，不附整份任务提案。
TaskContext.design 为可选字段；无设计绑定的既有任务保持原合同。新派发要求活动绑定、当前发布与需求、正式任务basis一致；新草稿不漂移，发布／退役／移出／独立任务偏离要求重新审阅采用。
预览及启动同事务复核相同设计快照；完整goal仍最多32,000字节，不调整预算掩盖过长内容。
绑定与验收使用服务端TaskDispatchPlan中已保存的设计上下文核对实际run，不能以任意Record.Data冒充；新设计不能凭旧run取得验收。已开始运行定义和证据保持，不自动取消。

## 交互与实施边界

原需求详情单入口“设计与拆解”，URL panel=requirement-design。局部产品／体验／技术／任务四段，主界面保持简单；任务细节按需展开，依赖和验收覆盖可配置。保存草稿、审阅发布、预览并采用是不同动作，均不自动启动开发。
中英文产品文字成对，用户正文和快照保持。空态、加载、当前来源变化、发布问题、冲突及未知结果可操作，采用后链接真实任务。
当前为人工设计／提案；未来模型及工具提交的设计也只能先成为同合同草稿，不能将未接通的模型称为自动设计。完整UED原型编辑、模型提案、旧任务显式导入和跨项目设计复用仍属于完整平台后续范围。

并行：领域代理只写requirement_design*.go（Root的types除外）及其两份PRD／架构文档；UI代理只写新工作区／模型／目录和App接线；集成代理只写独立集成测试；Root负责入口、生成合同、本地化、任务上下文、绑定／验收与交付。只跑改变行为相关组，不重复完整套件。

阶段证据及待验范围见[验证报告](../docs/quality/requirement-design-verification.md)。
