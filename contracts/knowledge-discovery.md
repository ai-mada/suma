# 已发布项目知识关键词发现

T-KNOWLEDGE-DISCOVERY-196；初始化版本1.0.0。范围为已发布 memory／spec／skill 的关键词发现、准确预览与明确选入原任务上下文。完整31／P0–P7／58限定／34待交付／92目标继续；不重建T172／T173，不伪装documents，不宣称语义索引或模型应用已完成。

## 单一发现合同

`GET /project-knowledge/discovery?project_id=…&q=…&kind=…&limit=…&cursor=…`。沿当前租户／项目读取权限；当前授权先于域筛选和游标解析，Agent从原只读工具使用同一事务实现。旧 `GET /project-knowledge?q` 是latest／草稿管理目录，不改变语义；原project.search四类记录与semantic索引也不被假扩展为已发布知识全文。

- q可缺省／空串以浏览；最多200 UTF-8字节，拒非法UTF-8和控制字符，trim与Unicode小写规范化后作字面连续子串查询。`%`、`_`、反斜线、引号不是通配符。
- kind可缺省；提供时只允许memory／spec／skill。limit缺省10，1–20；显式0、重复或未知参数拒绝。cursor可缺省／空，最多2048字节，绑定当前actor／organization／project、规范q／kind／limit与排序边界。
- 返回 `ProjectKnowledgeDiscovery{items:[],next_cursor,mode:'keyword',scope:'current_project_publications',scanned,scan_limit:200,exhausted}`。数组永不null，scanned为实际消费的候选数0–200，items不超过limit20。exhausted表示此次当前读已到本次遍历剩余候选尾部：true时next_cursor为空，false时非空。没有未经核实的total、score、索引水位或全库空结论。
- 命中 `ProjectKnowledgeDiscoveryHit{reference:{id,revision,sha256},project_id,kind,title,summary,snippet,matched_field,source_count,published_at}`。标题、说明、类型、发布时间、摘要全部来自准确已发布修订；snippet最多600 UTF-8字节，完整来源保留原修订API。q空时matched_field=none；非空为title／summary／content之一。来源数0–8，不附带来源原文或最新草稿。

单页最多200候选、20命中且编码预算48KiB，在原Agent64KiB输出门内。limit、扫描或字节预算触界即可停；即使items为空，只要exhausted=false都应显示继续扫描／加载，不能显示全项目没有知识。不会把最近200条假称完整搜索结果。

## 当前读、排序与资格

候选只取本项目当前published指针、未退役聚合，以aggregate.created_at降序、id升序遍历。初页固定当时首候选upper tuple；后续cursor固定upper和最后实际消费的候选，已消费ID不因草稿编辑或重新发布重复。未翻过的ID按当前published修订、SHA与来源资格读取；刷新重新开始。此为live current-read keyset，不声称跨HTTP一致快照；同秒新建落在已固定范围后的记录可能在后续页出现，权限／发布／来源变化即时生效。

Go按解析后的发布title／summary／content匹配，不查原始JSON或来源quote，避免JSON转义漏搜、来源元数据假命中和最新草稿覆盖。每命中复用 `resolvePublishedKnowledgeTx`，重新核对当前租户项目、当前发布修订／SHA、retired及原来源资格；失效候选不返回，权限与数据库错误传播。游标推进扫描位置而非最后命中，不能因失效项或空页循环／漏页。

只返回准确引用；预览沿 `GET /project-knowledge/{id}/revisions/{revision}` 原不可变历史，当前可采用资格与历史可读取性分别表达。明确选入沿原TaskDispatchPreviewRequest.knowledge和原预览／Start事务再次resolve；不自动选入、不自动运行，不放宽16引用及32000 UTF-8输入门。新发布、退役、来源漂移或撤权后，旧命中不得继续进入新采用。

## API、CLI与原生智能体

新增原只读应用工具 `project.knowledge.search`，args同q／kind／limit／cursor，供agent／api与原当前browser只读白名单；返回同一紧凑DTO。现有project.knowledge仍要求准确references后解析全文。CLI `knowledge search --query … --kind … --limit … --cursor …` 只调用上述GET并显示是否扫描到尾。原生规划白名单显式增加此只读工具，不新增模型、权限或工具执行通道。

无新依赖、存储或投影派发器。后续LanceDB／OceanBase语义投影、事务outbox、分代／墓碑与外部记忆同步是独立交付，不由关键词命中推导。只验证发布／草稿隔离、当前资格、边界与分页、准确任务引用和当前授权；Root统一Schema、真实HTTP／SQLite旅程、界面与Git，不重复提炼／技能物化整套检查。
