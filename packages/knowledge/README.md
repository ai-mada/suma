# Suma Knowledge Service

独立 TypeScript 服务，默认监听 `127.0.0.1:4312`。文档原件属于控制服务 SQL 数据库；本服务只保存可重建的检索分块与 revision/tombstone 清单。

构建：`pnpm --filter @suma/knowledge build`；运行：`pnpm --filter @suma/knowledge start`。

| 变量 | 含义 |
| --- | --- |
| `SUMA_KNOWLEDGE_TOKEN` | 必填，内部服务 Bearer token；与控制服务保持一致 |
| `SUMA_KNOWLEDGE_DIR` | 默认 `.suma/knowledge`；一个目录只能由一个服务进程持有 |
| `SUMA_KNOWLEDGE_PORT` | 默认 4312；只绑定 loopback |
| `SUMA_KNOWLEDGE_BACKEND` | 默认 `lancedb`；可设置 `oceanbase` |
| `SUMA_KNOWLEDGE_OB_URL` | OceanBase MySQL-mode 连接 URI，仅从环境读取 |
| `SUMA_KNOWLEDGE_OB_TLS` | 远程 OceanBase 必须为 `true`，校验服务端证书 |
| `SUMA_EMBEDDING_URL` | 可选，完整 OpenAI-compatible `/embeddings` endpoint |
| `SUMA_EMBEDDING_MODEL` | 启用 embedding 时必填 |
| `SUMA_EMBEDDING_DIMENSIONS` | 启用 embedding 时必填，1–4096 |
| `SUMA_EMBEDDING_VERSION` | 启用 embedding 时必填，由运维显式管理 |
| `SUMA_EMBEDDING_API_KEY` | 可选服务商密钥，不写入 manifest 或文档 |

## 内部 HTTP 合同

`GET /health` 实际执行 native FTS 查询，配置 vector 时同时执行 native vector 查询；返回 `available`、`verified`、`mode`、`profile`、`index_watermark`。该只读探针不要求 token。embedding 的 `configured/verified` 分开报告；verified 只有当前进程实际收到并校验了 embedding 响应后才为 true，不因为环境变量存在就置 true。

`POST /index`、`POST /search` 必须携带 `Authorization: Bearer <SUMA_KNOWLEDGE_TOKEN>` 与 JSON Content-Type。服务不接受浏览器会话身份；tenant/project 来自已经完成授权的控制服务，**内部 token 可以选择租户，因此不得发给普通用户或前端**。

```json
{"tenant_id":"tenant-1","project_id":"project-1","source_id":"document-1","revision":3,"title":"发布说明","content":"文档正文","action":"upsert"}
```

`revision` 是 JSON 数值类型的正的单调递增安全整数；同 revision 的内容不可变。删除使用 `action:"delete"` 和更高 revision，title/content 可省略。乱序旧事件不会覆盖新 revision；delete tombstone 阻止相同/更旧 revision 的 upsert 复活。

```json
{"tenant_id":"tenant-1","project_id":"project-1","query":"发布","limit":10}
```

搜索返回 `{items:[{id,title,snippet,source_ref,revision,locator}],mode,index_watermark}`。limit 默认 10，上限 50。每个 chunk 保留原 source、revision 和字符位置 locator；snippet 最多 600 字符。每份源文档最大 1 MiB，JSON 请求最多 2 MiB。

## Native 索引与更新

LanceDB 使用 `@lancedb/lancedb` 的持久表，FTS 使用 native `Index.fts`，查询使用 `MatchQuery(..., Operator.And)` 与 tenant/project 前置过滤。ngram tokenizer 支持中英文字符，避免依赖下载词典。没有 embedding 时不生成伪向量；配置后调用真实 embedding endpoint，按维数校验并执行 Lance 原生向量最近邻查询。当前本地向量查询是精确检索，没有声明已经建立 ANN 索引。参见 [LanceDB FTS 官方文档](https://docs.lancedb.com/search/full-text-search) 和 [FTS index options](https://docs.lancedb.com/indexing/fts-index)。

OceanBase 适配器通过 mysql2 执行真正的 `FULLTEXT ... WITH PARSER IK`、`MATCH AGAINST`、`VECTOR(dim)`、HNSW `VECTOR INDEX` 与 `l2_distance ... APPROXIMATE LIMIT`。不会把 JSON 向量拉回 JavaScript 全量扫描。服务先检查 OceanBase 版本标识，再执行建表与 native 查询探针；不支持的数据库/版本会启动失败，不降级冒充 OceanBase。向量内存设置与数据库权限由管理员配置，本服务不执行 `ALTER SYSTEM`。参见 [OceanBase vector SQL quick start](https://github.com/oceanbase/oceanbase-doc/blob/master/en-US/640.ob-vector-search/120.ob-vector-search-quick-start/100.ob-vector-search-sql-quick-start.md) 和 [全文索引官方说明](https://github.com/oceanbase/oceanbase-doc/blob/master/zh-CN/700.reference/300.database-object-management/100.manage-object-of-mysql-mode/500.manage-indexes-of-mysql-mode/200.create-an-index-of-mysql-mode.md)。

分块固定 1500 字符、重叠 150 字符。更新先将 manifest 标为 pending，再完整替换该源当前 profile 的所有 chunk；源变短时旧尾部不会残留。写入成功后激活 revision 并推进 watermark。进程崩溃留下的 pending revision 不对搜索可见，同 revision 事件重放可以修复；源原件仍在控制服务 SQL 中。manifest 只含源标识、版本与阶段元数据，不存可检索正文。搜索结果再次核对 manifest 的活跃 revision/tombstone，防止跨 profile 旧数据泄出。

模型名、维数、embedding 版本与 chunker 版本共同决定独立表名。切换配置会创建新 profile，需要控制服务重新投递所有存量源；不能混用旧向量，也不会自动假定重建已完成。旧 profile 表保留，当前没有自动 GC；容量管理必须保留 tombstone 元数据。每次应用事件串行执行；这是一进程索引服务，不支持多个实例同时写同一目录。

## 已验证与限制

Windows 下已实际运行 LanceDB native 测试：租户隔离、源缩短全替换、删除与乱序重放、重启后 tombstone、内部 HTTP token 校验、native 向量检索、模型版本表隔离。embedding 测试使用本机协议 fixture，未调用付费模型。OceanBase SQL 适配器已通过 TypeScript 编译，但本环境没有 OceanBase 服务，兼容性与性能仍为 **unverified**；不得标注生产验证完成。

当前一次搜索最多读取 10 倍 limit 的 native 候选，再排除 manifest 中的过期项；profile 切换后大量旧候选可能影响返回条数，运维应尽快完成存量重建。远程 embedding 失败会保留 pending 状态并由控制服务 outbox 重放；不会返回伪造的语义结果。没有实现混合检索/RRF、ACL 到文档级别的授权或自动源回填。
