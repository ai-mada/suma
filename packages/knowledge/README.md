# Suma Knowledge Service

速码 1.0.0 独立检索服务。正式原件、发布权限和事务 outbox 由 Go 控制面管理；本服务保存可重建的 typed source 分块、不可变来源快照、向量缓存及退役墓碑。默认监听 `127.0.0.1:4312`。

构建：`pnpm --filter @suma/knowledge build`；运行：`pnpm --filter @suma/knowledge start`。唯一私有协议为 `suma-knowledge-index-v1`，接口类型由 Go 合同生成至 [src/contract.ts](src/contract.ts)，不得独立维护另一套 wire。

| 环境变量 | 含义 |
| --- | --- |
| `SUMA_KNOWLEDGE_TOKEN` | 必填，内部服务 Bearer token，与控制面一致 |
| `SUMA_KNOWLEDGE_DIR` | 默认 `.suma/knowledge`，同一目录仅允许一个进程持有 |
| `SUMA_KNOWLEDGE_HOST` / `SUMA_KNOWLEDGE_PORT` | 默认 `127.0.0.1:4312` |
| `SUMA_KNOWLEDGE_BACKEND` | `lancedb`（默认）或 `oceanbase` |
| `SUMA_KNOWLEDGE_OB_URL` / `SUMA_KNOWLEDGE_OB_TLS` | MySQL 模式连接 URI；远程 OceanBase 必须校验 TLS |
| `SUMA_EMBEDDING_URL` | 可选，完整 OpenAI-compatible `/embeddings` 地址；HTTPS 或 loopback |
| `SUMA_EMBEDDING_MODEL` / `SUMA_EMBEDDING_DIMENSIONS` / `SUMA_EMBEDDING_VERSION` | 启用 embedding 时必填；维数 1–4096，处理版本由管理员明确管理 |
| `SUMA_EMBEDDING_API_KEY` | 可选服务商密钥，不进入来源快照或目录元数据 |
| `SUMA_KNOWLEDGE_MAX_QUEUED_OPERATIONS` | 默认 64，范围 1–4096（含执行中）；满队列返回 `503 index_busy` |

## 来源、回执和查询

`POST /index` 与 `POST /search` 必须携带内部 Bearer token 和 JSON Content-Type。内部 token 能选择 tenant/project，必须留在服务端。控制面仍在检索后重新读取当前权限、来源及精确发布引用。

来源身份为 `(tenant_id, project_id, source.kind, source.id)`，其中 kind 为 `documents` 或 `project_knowledge`。`source.sequence` 是正式业务的单调版本；`source.revision/sha256` 是当前文档版本或精确发布引用，不能混用。尚未发布便退役的知识仅可使用 delete、revision=0、空 SHA。delete 的 title/content 必须存在且均为空串。

每次请求包含独立 `event_id`。回执精确返回原 event、scope、source、action、实际 HTTP 原始字节摘要 `request_sha256`、`generation`、`index_watermark` 和 `applied|duplicate|obsolete`。同 sequence 不同内容会冲突；墓碑阻止旧事件复活。控制面只有核验原回执后才能推进 outbox。

搜索必须提供 protocol、tenant/project、source_kind、非空 query 与 limit（1–100）。结果只含 typed source 和 `{unit:"unicode_codepoints",start,end}` 定位；不把索引副本标题、正文或外部排名当作正式业务证据。start 为零起始、end 为排他边界。控制面从当前原文提取展示片段，并再次检查精确版本和权限。

单份来源正文最多 1 MiB、标题最多 4096 UTF-8 字节；私有 index JSON 最多 8 MiB，以覆盖 Go JSON 转义开销。搜索 JSON 最多 16 KiB。分块为 1500 个 Unicode codepoint、重叠 150；emoji 不被 UTF-16 代理对截断。

## 真实原生检索

LanceDB 使用已安装 `@lancedb/lancedb` 原生 FTS、向量查询及 `RRFReranker`：有 embedding 时实际执行 FTS 与向量双路融合，返回 `lancedb_hybrid`；无 embedding 时返回 `lancedb_fts`。所有召回均先应用 tenant/project/source_kind 过滤。ngram tokenizer 支持中英文，无词典下载；未声明已建立向量 ANN 索引。

OceanBase 通过 mysql2、原生全文索引和向量索引查询；有 embedding 时执行两个原生 SQL 召回并在应用层按 RRF 融合，返回 `oceanbase_hybrid`，否则 `oceanbase_fts`。连接先核验 OceanBase 身份，远程连接要求 TLS，不降级冒充其他数据库。本环境尚未进行 OceanBase 物理验收。

## 持久化与有界分代重建

目录内采用小型 `catalog.json`、256 分片的 per-source head、内容寻址不可变 snapshots 和按处理配置绑定的 vectors。正常写入不会重写全库正文。head 更新先保存 durable inflight 意图，重启补全同一意图；来源先 pending、原生全替换成功后才 active。pending 或不属于当前 generation 的来源不会进入结果。旧格式 `manifest.json` 明确拒绝，不隐式转换或清空。

处理配置包括后端、模型、维数、embedding 地址/版本及 chunker。配置变化进入 `rebuild_required`；必须明确提供以下四项才能开始真实新 generation：

| 环境变量 | 必填预算 |
| --- | --- |
| `SUMA_KNOWLEDGE_REBUILD_OPERATION_ID` | 新的操作标识 |
| `SUMA_KNOWLEDGE_REBUILD_MAX_SOURCES` | 最多处理来源数，1–10000000 |
| `SUMA_KNOWLEDGE_REBUILD_MAX_EMBEDDING_REQUESTS` | 最多实际 embedding 请求数，0–10000000 |
| `SUMA_KNOWLEDGE_REBUILD_MAX_DURATION_MS` | 总截止时间，1–604800000 ms |

重建后台串行遍历保存的 typed head 和准确来源快照，保留游标、已消费请求和原截止时间；同 operation 重启不重置预算。超额或失败进入 blocked，新的明确 operation 可继续恢复。已落盘且输入摘要/维数有效的向量批次可复用。正常 outbox 投递有每来源 32 次持久 embedding 尝试上限；明确批准的 rebuild 使用自己的总预算，不清除或受困于旧正常重试历史。

只有全部当前来源均达到目标 generation，且 pending_count=0，才发布 active_generation。重建期间查询和投递明确返回 `503 index_rebuild_required|index_rebuilding|index_blocked`；控制面保留 outbox，用户可继续关键词发现。`index_watermark` 仅为本索引实例的持久投影位置，不是业务全库追平或权限水位。

`GET /health` 不排在重建队列后。匿名访问只给状态/phase；携带内部 token 才返回 generation、来源数、pending 与重建预算进度。embedding_configured 与当前进程真实响应验证 embedding_verified 分开，健康检查不调用 embedding 服务。

客户端离开会取消尚未开始的队列项，并中止 embedding HTTP。已进入的原生 SDK 查询无法强中止时，等待完成并丢弃取消结果；不声称原生树已被停止。重建与普通写入共用单目录 owner，不支持多主机共享目录或 HA；旧 generation 和不可变快照不自动 GC。

## 验证范围

T197 定向验证真实本机 LanceDB 的跨领域同 ID 隔离、乱序/重复/墓碑、缩短全替换、重启、混合检索、预算耗尽与显式恢复、队列取消/背压、原始 HTTP 字节回执与内部身份边界。embedding 使用自有 loopback 二维协议夹具，未调用商业模型。Go 控制面另核验 exact ACK、当前原文 codepoint 片段和本地化安全错误码。完整 HTTP/SQLite/原生索引纵向验收证据由主集成流程统一记录；OceanBase、商业 embedding 和规模验收分别列为待验，不以模拟通过代替。
