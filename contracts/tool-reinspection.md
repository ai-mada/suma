# 准确执行节点工具重检

T-TOOL-REINSPECTION-194；suma1.0.0。原31／P0–P7／58限定／34待交付／92完整目标继续。此合同是维护请求，不是第二任务派发器、配置权威或自动安装／启用能力。

## 浏览器入口

- `POST /tool-profiles/{id}/inspections`：`project_id, expected_version, idempotency_key`。沿原工具管理审批权限，固定准确profile版本／摘要／命令及worker；同操作恢复原创建，不换key盲建。
- `GET /tool-inspections/{id}?project_id=…`：查原意图。
- `GET /tool-profiles/{id}/inspections?project_id=…`：当前未结束及最近记录，最多10项。读取不能替换本机保留的原未知操作。

新请求pending，期限300秒；每项目／worker最多一个未结束请求，项目最多10000份请求历史，明确到上限而非删除原件。未知请求不自动新建或执行。请求保存当前人员授权、节点身份和Pool原route/grant/credential依据；变化不得借旧回执扩大权限。

## 原Worker出站领取

`POST /runner/tool-inspections/claim`：`project_id,worker_id,claim_id`。Pool对应`/pool-runner/tool-inspections/claim`，字段`runner_id,tenant_id,project_id,claim_id`。返回`{inspection:ToolInspection|null}`。机器先持久化原nonce，服务端持久claim_id与递增fence；同nonce丢ACK恢复同一领取，不拿新请求。租约60秒，过期显示unknown，不重新派发或续fence。

Worker合并到原库存单一拥有者，与周期上报串行。先ACK已有pending旧库存，再以本请求持久started意图执行原`DescribeTools`。只有本机当前已登记配置，无用户argv／路径／环境值或任意命令。重启看到started且没有已准备报告只上报unknown，不再探测。

## 原库存及失败回执

共享`Reference={inspection_id,claim_id,fence}`。原`ToolInventoryRequest`与本机`InventoryReport`增加可选`inspection`引用；相关库存与请求completed同事务保存，ACK返回相同`inspection`和`inspection_state=completed`。不带引用的旧周期报告不能完成请求。回执丢失恢复同原操作，不重新探测；同原fence迟到结果可在当前身份／人员／配置／Pool依据仍准确时核对完成，不产生新执行权。

`POST /runner/tool-inspections/results`（Pool同前缀）接收准确身份、`inspection`、`idempotency_key`、`outcome`及`failure_code`。明确未开始为failed/not_started；中断、超时或无法证明完整结果为unknown/probe_interrupted、probe_timeout或probe_failed。原记录和本机日志保留，不清空未知状态或自动再执行。

公开状态pending／claimed／completed／failed／unknown／expired／superseded。completed仅表示本次相关原库存已经持久化；结果码reported_available／reported_unavailable／profile_changed／command_missing不等于模型认证或收费／执行成功。原Profile仍暂停或原状态不变，不自动登记、改配置、启用、授权、Claim业务任务或重启机器。

## 初始化与交付

新表沿SQLite、OceanBase和PostgreSQL共同组件基线与确切列／键校验；未知已有形状在任何DDL前拒绝，不reset或回填兼容。写入沿当前权限、稳定操作标识及CAS；原件与证据不提交Git。只验证新增及直接受影响行为，Root统一一条真实HTTP／Worker／Journal复核、Schema、界面与Git；真实厂商、多机和完整平台验收仍独立。
