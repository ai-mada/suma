# 已发布项目知识的领域索引投影

T-KNOWLEDGE-PROJECTION-197；初始化版本1.0.0。发布／退役与原outbox同事务、领域化索引、当前权限检索和准确引用沿用既有项目知识、T172／T173／T196。完整31／P0–P7／58限定／34待交付／92继续，不把单向投影冒充外部双向同步或模型应用。

## 唯一内部合同

唯一Go类型在 `internal/indexcontract`：Source、Locator、Request、Receipt、SearchRequest、SearchHit、SearchResult。协议固定 `suma-knowledge-index-v1`；没有旧Record／documents兼容分支。

Source=`{kind,id,sequence,revision,sha256}`；kind仅documents／project_knowledge。命名空间为tenant／project／kind／id，跨领域相同ID互不覆盖、删除或检索。sequence是单调业务版本；documents的revision与sequence均为正式Record.Version，sha256为准确索引正文UTF-8字节摘要。project_knowledge的revision／sha256为准确已发布修订，sequence为产生发布或退役命令后的aggregate.Version。新草稿编辑增加业务版本但不改旧发布正文，不发投影、不令旧发布失效；知识检索不能强制sequence等于今日aggregate.Version。

Request=`{protocol,event_id,tenant_id,project_id,source,action,title,content}`，action仅upsert／delete。知识upsert标题与正文只取已发布快照，正文是published.Content，title独立，不混来源quote或latest草稿。delete两个文本必须为空；已曾发布的退役保最后published revision／SHA，sequence取新业务版本，不能归0。原允许未发布草稿退役时，仅delete可revision=0／sha256空，不伪造批准内容。正式来源版本／哈希、事件和原写操作回执同事务；回执重放不增事件，事务内不调远端。

Receipt精确回显protocol／event／scope／source／action，加disposition=applied／duplicate／obsolete、generation、index_watermark与request_sha256。request_sha256由Node HTTP捕获实际POST原字节计算，Go比对自己实际发送的持久payload字节，不以2xx代替原事件ACK。同sequence不同业务内容冲突409；语义相同但JSON编码不同可duplicate，字节ACK仍绑定当次原请求。更高sequence墓碑阻止旧upsert复活。provider水位只是其本地操作计数，不与数据库source sequence相减，不以最大ACK冒称没有缺口。

单来源正文上限仍为1MiB UTF-8；JSON转义后的私有`/index`请求帧最多8MiB，覆盖最坏编码放大，并不扩大正文限额。公开发现响应最多48KiB、单片段600 UTF-8字节，ACK与查询使用各自更小的帧界限。

SearchRequest=`{protocol,tenant_id,project_id,source_kind,query,limit}`，source_kind只允许两领域，limit1–100。SearchResult=`{protocol,items:[{source,locator}],mode,generation,index_watermark}`；mode只有lancedb_fts／lancedb_hybrid／oceanbase_fts／oceanbase_hybrid，hybrid须实际文本与向量双路融合，不能仅改名。Locator=`{unit:'unicode_codepoints',start,end}`，0起始／end不含，基于准确索引正文；TS Array.from和Go []rune同单位，不能以UTF-16 chars切半字符。

## 原存储与提交边界

原suma_index_outbox的revision列作为单调source.sequence；新增source_kind、content_revision、source_sha256、receipt和indexed_at，按tenant／project／domain／id／sequence唯一。payload为唯一typed Request原字节，pending不提前确认。原失败账本增加source_kind；初始化只接受当前1.0.0精确列／键，未知旧形状在任何DDL前拒绝，不回填、不清库或reset。索引与projector仍复用原队列，不新增业务派发器。

索引网络作用在读取事务之外。只有请求、领域身份、内容修订、原字节SHA及真实结果均匹配的ACK才能落indexed／receipt／indexed_at。失败和未确认保留，原事件可安全重放；重建或模型版本切换保持准确代际，不以旧代ACK证明新代完整。

## 业务只读接口

`GET /project-knowledge/index-search?project_id=…&q=…&kind=…&limit=…`：q必需非空、最多200 UTF-8字节；kind可缺省，提供时memory／spec／skill；limit缺省10，1–20。拒重复／未知参数及显式0。先当前项目授权和输入校验，再释放SQL锁访问索引；返回后即使空结果也再次当前授权。按原resolvePublishedKnowledgeTx复核准确发布指针／SHA、非退役及来源资格，丢弃陈旧、跨域／项目或无效locator候选。不能信provider标题／片段／权限；标题、说明、发布时间及来源数来自原发布，snippet最多600 UTF-8字节，从原正文按codepoint区间重建，公开locator也绑定实际片段。新发布、退役、来源漂移或撤权立即阻止新采用。

返回ProjectKnowledgeIndexSearch：items非null；mode／generation／index_watermark是真provider事实；candidates_examined为实际复核候选数0–100，discarded为不采纳候选数，scope固定provider_candidates。没有假total／全文覆盖／cursor或静默关键词fallback；kind筛选、去重、当前资格与前20上限可减少命中数，空结果只表示本次候选没有可采用命中。未配置／服务失败明确拒绝，T196关键词发现仍可独立使用。

Provider未就绪时仅允许固定安全错误码`index_rebuild_required`／`index_rebuilding`／`index_blocked`传到业务响应，背压仅允许固定`index_busy`提示，展示本地中英文提示，不透传provider原文或其他租户进度。失败响应也在网络返回后重新核验当前权限。公开index-status仍仅表达本项目outbox交付事实；运维侧健康与预算化重建信息不能冒充项目完整覆盖。

每项ProjectKnowledgeIndexHit含原reference、project_id、kind、title、summary、snippet、source_count、published_at、typed source及locator。预览沿原精确修订，选择沿原任务preview／Start再复核，16引用及32000 UTF-8输入门不变，不自动选入／启动。

返回的source完整身份还必须对应本租户／项目的原正式upsert事件：领域、来源标识、sequence、content revision及SHA全部吻合。准确发布引用本身不能使provider随意填写的sequence成为业务事实；新草稿增加today aggregate version也不会使原发布事件失效。ACK尚未回到业务库不影响原事件身份，此核对不冒称索引交付已完成。

原应用工具目录新增只读`project.knowledge.index_search`，参数为必需q与可选kind／limit，不接受cursor。复用上述业务读服务、原当前purpose和闭形参数验证；远端调用不放在通用应用工具的SQL事务内。它仅发现，不调用模型、不创建任务、不采用内容或派发执行。CLI／原生Agent沿同一服务入口消费。

`GET /project-knowledge/index-status?project_id=…`：ProjectKnowledgeIndexStatus含domain=project_knowledge、as_of、configured、coverage=outbox_delivery_only；pending／indexed／failed／total为本domain事件事实，current_publications为当前非退役发布指针数（非来源资格或正确性保证），untracked_publications为没有对应当前发布事件的指针数。latest_event是紧凑event_id／source／action／state／created_at摘要，last_ack是准确Receipt，两者必填但可null。它们不是Request正文，也不是当前provider代际完整覆盖证明。缺配置、待处理、失败、未知覆盖和服务可查询性分别表达，不以计数0或最大水位显示“项目没有知识／全部追平”。

## 验证与剩余范围

定向验证事务回滚、原操作重放、不索引草稿、单调退役／同ID跨领域隔离、乱序／重复／同序冲突、旧字节ACK拒绝、当前权限／来源与locator、代际及空响应；成熟LanceDB／OceanBase适配器与真实混合路由分别记证。Root统一一个实际HTTP／SQLite→projector→本机原生索引／自有embedding→准确任务引用旅程，不运行商业模型或重做172／173／196未变整套。物理多机、真实OceanBase矩阵、外部同步、离线与模型效果仍独立。
