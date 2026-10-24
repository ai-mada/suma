# T-P4-003 签名事件入口合同

状态：合同冻结，实施中。负责人：主集成、服务端和工作台代理。关联：[自动化](automation-events.md)、[完整计划](../docs/delivery/implementation-plan.md)。本阶段实现 GitHub 原生接收协议和 Suma 通用 Git/CI/监控签名协议；真实厂商账户/公网投递、生产 CI 身份与证明分别验收。

## 边界与状态

事件入口独立附着一条既有人工或 cron 自动化规则，不新增 AutomationSpec.trigger。入口明确绑定 rule_revision。来源秘密仅授予向该入口交付受限事实的权限，不能读取业务、修改流程、提供执行命令、改变仓库/代码版本或代替人工验收。

事件只触发已批准固定工作流；规范化上下文保存在交付记录，不注入 command/config，不获取事件中的 URL。Git SHA/ref 是来源证据，不等于实际执行代码。规则现有 overlap、审批、资源和测试门禁继续适用。

入口 state=`paused|enabled|revoked`；创建为 paused、没有密钥。revoked 终止，擦除密文且保留回执。spec 修改形成新的不可变 revision，并暂停；轮换立即废止旧密钥并暂停，重新启用必须明确确认。入口暂停/修改/撤销只阻止自己的 queued 触发；不能取消已派发运行。规则暂停/修订仍阻止所有该规则 queued 触发。接收和派发均核验入口启用人及规则启用人的当前项目 write+approve 权限。入口绑定修订不等于规则当前修订时不可启用或派发。

业务 version 对命令递增；revision 仅 spec 修改递增；key_generation 单调递增，不是业务修订。密钥配置与回执不包含明文秘密。所有管理接口仅真实组织会话，拒绝全部非空 API/Runner/Agent scope；读需项目访问，写需 write+approve，并在组织锁内核验，旧操作回放前同样授权。

## 类型与管理 API

所有路径以下省略 `/api/v1`。请求严格拒绝未知/重复字段、无效范围及多余 JSON。

`AutomationIngressSpec`：`name,provider,events,repository,repository_id,ref,conclusions`。

- name：去首尾空格后不变，1..200 UTF-8 字节。
- provider：`github|suma`。
- events：1..8 个不重复白名单事件。github 支持 `push,workflow_run.completed,ping`；suma 支持 `git.push,ci.completed,monitor.firing,monitor.resolved`。ping 永不创建运行。
- repository/repository_id：github 必须同时指定 owner/repo 和正十进制数字 ID；正文内二者均匹配，名字按不区分大小写比较。suma 可为空；非空时精确过滤。ref：空表示所有分支和标签，否则精确 refs/heads/... 或 refs/tags/...；workflow_run 按 head_branch 规范化。空并非未知，界面明确表示所有分支和标签。
- conclusions：0..16 个不重复小写状态（1..40 ASCII 字节）；空表示不过滤，非空要求精确匹配，不把 success 映射为平台测试证明。

`AutomationIngress`：上述 spec 平铺，加 `id,project_id,rule_id,rule_revision,version,revision,state,activated_by,key_configured,key_generation,key_fingerprint,endpoint_path,created_at,updated_at,last_error`。fingerprint 为 secret 原文字节 SHA256，不是秘密。endpoint_path 为 `/api/v1/hooks/{id}`，不包含 token 或 query。

`AutomationIngressRevision`：`revision,rule_revision,spec,created_at`。`AutomationIngressList`：`items,total,next_cursor`。

- `GET /automations/{id}/ingresses?project_id&limit&cursor` → AutomationIngressList。稳定分页，limit 默认50/max200，cursor 绑定 tenant/project/rule/collection。
- `POST /automations/{id}/ingresses` → 201 AutomationIngress；`AutomationIngressCreateRequest={project_id,expected_rule_version,expected_rule_revision,idempotency_key,spec}`。固定当前规则，创建 paused。
- `GET /automation-ingresses/{id}?project_id` → AutomationIngress。不读取密文/秘密。所有读含当前项目授权。
- `POST /automation-ingresses/{id}/commands` → 200 AutomationIngress；`AutomationIngressCommandRequest={project_id,action,expected_version,idempotency_key,expected_rule_version?,expected_rule_revision?,spec?,secret?}`。
  - action=`revise|enable|pause|revoke|rotate_key`。
  - revise 必须 spec 及 expected_rule_version/revision，改绑当前规则并暂停；其他 action 不接受 spec。
  - enable 必须 expected_rule_version/revision，与入口绑定及当前已启用规则一致，且 key_configured。
  - rotate_key 仅接受 secret：64个小写十六进制字符；HMAC 使用64字符 UTF-8原文，不hex解码。由浏览器安全随机生成或调用者提供；服务器只保存 AES-256-GCM 密文。相同稳定操作可重放非秘密回执，绝不重发明文。未知结果用原操作核对；丢失当次秘密后可在读取最新版本后明确轮换新值并重新配置发送端。
- `GET /automation-ingresses/{id}/revisions?project_id&limit&cursor` → `AutomationIngressRevisions={items,total,next_cursor}`，完整不可变历史分页。
- `GET /automation-ingresses/{id}/deliveries?project_id&limit&cursor` → `AutomationIngressDeliveries={items,total,next_cursor}`。按 received_at DESC,id DESC 稳定分页，历史浏览不被自动刷新覆盖。
- `GET /automation-ingresses/{id}/deliveries/{delivery}?project_id` → AutomationIngressDelivery。

`AutomationIngressEvent`：`schema_version,event,repository,repository_id,ref,commit,conclusion,external_id`。schema_version 固定1。其余为白名单短字符串；event 必须，external_id 对 suma 必须。repository≤201字节，repository_id≤30位数字，ref≤250字节，commit空或40/64小写十六进制，conclusion≤40字节，external_id≤128个ASCII标识字符。配置过滤器非空时，正文对应字段缺失或为空必须判定不匹配，不能绕过过滤。禁止控制字符。GitHub 实际大正文只提取上述字段；其余字段不保存。Suma 正文严格使用该类型，拒绝未知字段。

`AutomationIngressDelivery`：`id,ingress_id,ingress_revision,rule_id,rule_revision,delivery_id,provider,body_sha256,key_generation,received_at,last_received_at,duplicate_count,state,reason,occurrence_id,occurrence_state?,run_id?,run_state?,event`。

state=`accepted|ignored|blocked`。accepted 只指创建正式触发；运行另查。ignored 是有效签名但未匹配/连接检查，blocked 是当前入口/规则/权限/流程门禁不允许。交付保留当时 revision 与原因，后续配置变化不重新评估旧交付。重复只累计 duplicate_count/last_received_at，不生成另一触发。原始正文/签名头/秘密均不保存。单条详细读取只返回受限上下文。

## 公共签名接收

`POST /hooks/{id}` 无浏览器会话认证，拒绝 cookie/Authorization、query、Origin、非 JSON content-type、content-encoding 和重复协议头。公开返回 `AutomationIngressReceipt={id,state,duplicate,occurrence_id}`；不返回业务或秘密。提交正式数据后返回202；重复返回200恢复原回执。无效/缺少签名401，无效正文400，超限413，同 delivery ID 异正文409，限流429。无效签名不写可被攻击者污染的交付记录，界面不能假装有完整攻击审计。

GitHub：验证 `X-Hub-Signature-256: sha256=<64 hex>` 对**原始正文**的 HMAC-SHA256，使用恒时比较；拒绝SHA1降级。`X-GitHub-Delivery` 长度8..128安全ASCII标识，`X-GitHub-Event`只作分流，并验证对应正文结构。push要求repository/id/full_name、ref/after/deleted；删除分支不启动构建。workflow_run仅completed、验证repository与head_branch/head_sha/id/conclusion，其他action记ignored。ping只连接检查。不得声称这些未签名头或新鲜度受到GitHub签名保护。

Suma：正文为 AutomationIngressEvent。`X-Suma-Delivery` 8..128安全ASCII标识，`X-Suma-Timestamp`规范十进制Unix秒，DB时钟±300秒；`X-Suma-Signature-256: sha256=<64 hex>`。签名字节严格为：

```text
suma-v1\n<ingress_id>\n<timestamp>\n<delivery_id>\n<raw_body>
```

这里 `\n` 表示单个 LF，无末尾补行。绑定入口防止跨入口重放。重试保持 body、delivery_id；可以刷新 timestamp 和签名。CI/监控示例用 suma CLI 发送协议，不能标记 Jenkins/Prometheus/WorkBuddy 已原生接入。

## 持久防重放与事务

跨入口所有修订持久唯一 `(ingress_id,delivery_id)`。github 另以 `(ingress_id,body_sha256)` 防改未签名Delivery头重放；保守语义为“同字节仅触发一次”。suma 同正文不同有效 delivery ID 可表示独立事件。回执摘要不自动清理，未来保留策略必须同时保留防重放墓碑。GitHub未见过的历史有效正文无法证明新鲜度，公开此限制。

验签后进入组织锁事务，重新读取当前密钥 generation/配置并比较，防止在轮换窗口用旧验证结果提交。接收使用DB时间、入口与规则当前授权、受保护流程隔离；交付与唯一 occurrence 同事务提交。occurrence.source=`webhook`，去重身份包括入口/交付，不靠含 revision 的旧 occurrence唯一键。dispatch继续使用Go调度器原子绑定run，且在其组织锁内再次检查入口启用人当前权限和绑定规则修订。重复/撤权/修订/重启不得复活旧输入。任何DB失败都不返回成功。

每项目最多50个入口；新有效交付身份每入口每DB分钟最多120条（包含GitHub同正文新Delivery-ID别名；既有ID原回执可核对）。不得通过duplicate快速路径无界写入新别名墓碑。公共读正文≤2MiB，进程并发接收槽位≤32，解析有深度/字段边界，网络读写仍遵守普通超时。大量GitHub正文可能超过本平台边界，明确413，不静默截断。队列/全租户预算和目标规模另验收。

## 秘密与部署

Service.AutomationIngressKey 为32字节AES主密钥。`LoadAutomationIngressKey(dataDir,dialect,getenv)` 读取 `SUMA_INGRESS_MASTER_KEY`（规范base64的32字节）；SQLite未提供时在dataDir持久生成私有密钥文件，不覆盖既存无效文件；并发启动须fail-closed或有界重读。OceanBase/PostgreSQL必须明确共享环境密钥，不生成每节点私钥。AAD绑定tenant/project/ingress/key_generation，防密文挪用。备份数据库必须单独安全备份主密钥；丢失主密钥不能恢复来源secret。日志/公开错误不包含密钥/密文/请求正文。

每个入口使用独立秘密。持久的全服务 fingerprint 唯一占用/墓碑拒绝在新操作中复用任何已经使用过的secret（含原入口已轮换/撤销）；同一幂等操作仍可恢复原公开回执。此约束防止GitHub正文签名被不同入口共享secret接受，不能暴露另一租户身份或数据。原生协议仍有“从未见过的历史正文”新鲜度限制。

浏览器秘密只在当次内存；不进入URL、local/sessionStorage、通用恢复信封或活动日志。保存未知时保留原操作与内存草稿并阻止误重发；重新加载后从最新元数据核对fingerprint，必要时显式轮换，不能伪装已配置远端。

## 实施与验证

主代理：合同、生成器、CLI、集成文档和真实旅程。代理一：服务端/存储/HTTP/签名与事务测试。代理二：工作台/模型/交互与前端回归。代理三：官方协议与独立审查。采用现有共享dev checkout，避免相互覆盖，不单独提交；最终由主代理按阶段commit。

先验证RED再实现GREEN：官方GitHub签名向量/UTF8/篡改；缺失/重复/未签名头换ID重放；Suma过期/未来时间/跨入口与刷新签名；修订/重启/双连接竞争；旧密钥轮换/暂停/撤权与queued派发竞态；回执+触发原子回滚；固定流程；原始正文/秘密不落公开状态。界面创建、配置、启用、实际HTTP投递、交付下钻真实运行、错误与刷新恢复；本机真实SQLite，其他数据库和真实账户分别记录。

依据：[GitHub验签](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)、[头与事件](https://docs.github.com/en/webhooks/webhook-events-and-payloads)、[最佳实践](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)、[重投](https://docs.github.com/en/webhooks/testing-and-troubleshooting-webhooks/redelivering-webhooks)。GitHub不自动重投失败事件，发送方需配置失败观察与明确重投。
