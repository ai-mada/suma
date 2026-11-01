# 知识索引部署与恢复操作

状态：本阶段限定实现；suma 1.0.0；T-KNOWLEDGE-PROJECTION-197。[合同](../../contracts/knowledge-projection.md) · [架构](../architecture/knowledge-projection.md) · [ADR](../adr/knowledge-index-domain-generations.md)。

## 部署连接

| 场景 | 连接与拥有者 |
| --- | --- |
| 本机独立 | 控制面连接回环知识服务；业务SQLite与派生索引目录分别保存 |
| 单机Compose | `deploy/compose.knowledge.yaml`配置私网`http://knowledge:4312`及对应精确HTTP origin，使用现有secret文件入口 |
| 私有团队／SaaS | 控制面可以分布式部署；知识服务每个索引目录一个受控拥有者，跨机器连接使用HTTPS或明确的受信私网origin |
| Helm外部知识服务 | `knowledge.url`、`knowledge.existingSecret`及`knowledge.tokenKey`配置连接；私网HTTP再显式设置`knowledge.httpOrigins` |

控制面`SUMA_KNOWLEDGE_HTTP_ORIGINS`为逗号分隔的精确HTTP origin，包含协议、主机及实际端口，不含凭据、路径、查询、片段或通配符。默认保留HTTPS及回环地址；配置该项不跟随远端重定向。内部服务凭据沿现有secret加载入口提供，不能提交到Git或传给浏览器。

知识服务使用`SUMA_KNOWLEDGE_DIR`、`SUMA_KNOWLEDGE_BACKEND`及原嵌入配置。实际向量配置需要明确模型、维度和处理版本。队列默认64，可通过`SUMA_KNOWLEDGE_MAX_QUEUED_OPERATIONS`设置1–4096；满载明确背压，未开始的取消查询不继续消耗嵌入。已经进入不支持取消的原生查询只在返回后丢弃结果。

## 显式处理代次重建

操作员在知识服务进程配置中同时提供以下四项，才请求一次有界重建：

| 配置 | 意义 |
| --- | --- |
| `SUMA_KNOWLEDGE_REBUILD_OPERATION_ID` | 稳定的明确操作身份 |
| `SUMA_KNOWLEDGE_REBUILD_MAX_SOURCES` | 本次最多处理的来源数量 |
| `SUMA_KNOWLEDGE_REBUILD_MAX_EMBEDDING_REQUESTS` | 本次实际嵌入请求预算 |
| `SUMA_KNOWLEDGE_REBUILD_MAX_DURATION_MS` | 初始截止时长；重启不重新计时 |

同ID恢复保留消耗、进度和截止；增加授权范围或重新开始使用新的明确操作ID。重建使用准确保存输入与墓碑；同处理配置可复用受核向量快照，处理配置变化重新嵌入。普通来源请求上限与显式重建总预算分别核算，不能让旧的普通预算耗尽阻止新的明确恢复授权。

完整目标代生效前，查询明确返回待重建、重建中或阻塞状态。`GET /health`不带内部身份只提供基本状态，携带正确服务Bearer才读取完整代次与操作进度；这些是操作员服务事实，不作为其他项目的来源权限或完整覆盖证明。项目界面继续展示当前项目投递事实，按需明确切换关键词查找。

## 备份与故障核对

先正常关闭拥有者或取得一致文件系统快照，再备份业务数据及完整知识目录，包括来源head、不可变输入、向量快照、catalog与原生表。保留原操作及摘要，恢复先核对格式、来源和持久意图；不要删除未知目录或清空业务数据库来使启动通过。

本阶段实际验证覆盖目录保留情况下的重启、墓碑和显式新代重建。目录整体丢失后的正式事件重灌、跨物理机器共享所有权、完整灾备演练、真实OceanBase和规模验收仍待独立交付；新空目录可启动不能代表旧知识已经恢复。
