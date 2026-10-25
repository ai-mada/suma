# 任务设计依据的读取、资格与来源索引

状态：T-P4-017 实施基线，2026-10-25。负责人：平台集成。关联：[设计采用合同](requirement-design.md)、[类型](../internal/platform/task_design_view_types.go)。完整平台目标继续有效。

## 用户目标与接口

任务详情无需配置工具或流程即可读取正式采用依据。`GET /tasks/{id}/design-context?project_id=...` 返回TaskDesignContextView；同一当前授权读事务先校验Actor及正式任务归属，再读取受保护绑定、原始采用及不可变设计修订。
Browser与项目API按ProjectRead读取；Agent、Runner无新增管理页能力。错误任务或项目保持正常404／当前授权拒绝；不能从标题或Record.Data推断“已采用”。无绑定返回bound=false、eligibility=none，不制造设计或历史。

## 历史正文与当前资格

Context、Requirement、AdoptedTask始终来自当时实际采用的批准修订；AdoptedBy/At属于该次采用，不返回无关的完整批次任务。若当前来源变化，旧正文仍可审阅，不能换成最新发布正文。当前版本只作为资格元数据。
分类优先级为source_unavailable→excluded→retired→publication_changed→requirement_changed→task_changed→current；none只用于无绑定。
routine status/version/run变化不等于task_changed，必须复用正式taskBasis定义；任务被独立归档／删除或正式字段偏离才判失配。
当前需求缺失、归档或删除为source_unavailable，但已验证的历史快照可保留。原始采用／修订缺失或不一致不能拼造正文，bound仍为true并明确来源不可验证。

## 可重建的关系索引

当前exclude采用会记录一次排除并覆盖link的当前指针；keep记录不改变原活动采用身份。因此不能以最新非exclude或最新keep冒充原批准采用。
新增追加关系表`suma_requirement_design_adoption_tasks`，保存tenant/project/requirement、adoption_id及聚合version、task_id、task_key、action、task_version。按task与版本索引定位实际create/update；关联只定位来源，读取仍核验原采用JSON、任务key/id/action、批准修订SHA及固定需求，不能把索引当权限或内容证明。
采用时与原正式采用记录同事务写入关系。迁移按稳定key分页从既有不可变采用记录补齐，幂等且不改原采用／修订／任务；不得在每次GET全量扫描项目历史。以索引定位后再验证，不引入模型、外部服务或新依赖。
活动link使用其实际采用ID；排除后的来源使用不晚于排除记录的最近create/update，不能使用keep或以新设计中不存在的task key拼装来源。

## 界面与命令行

现有任务详情增加紧凑的只读“设计依据”卡片：原需求／设计修订、当前资格和关联设计工作区。原文、验收及摘要按需展开。保留当前账户／组织／项目／任务作用域；加载失败或权限拒绝不能继续显示前一个调用者的事实，晚到响应不跨任务覆盖。
中英文只改变产品文字，正文和快照保持；展开状态不因语言重置。链接仅允许同项目正式需求路由，并带panel=requirement-design。
CLI复用当前项目、原视图身份、服务端权限、JSON精确数字及稳定操作key，补齐知识与设计读写入口；JSON-only generic api支持PUT。已实现的preview POST只读，不注入idempotency_key、不声称已执行，也不因此省略原视图头；未知路由仍按写入处理。

## 验收与并行

当前／失配／排除／退役／无绑定、伪造Data、跨项目及撤权需要定向服务与真实组件证据；历史索引迁移与幂等需实际SQLite证据。仅复验改变范围，不重复无关全套。
后端代理拥有task_design_view*.go（本types除外），Root负责采纳hook／迁移接线、HTTP、生成合同、本地化；UI代理拥有卡片与App接线；CLI代理拥有internal/cli及其文档。所有缓存、数据库、凭证、截图和程序留忽略目录，阶段完成后只本地提交必要文件。
