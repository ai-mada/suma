# T-P4-002 自动化与持久事件合同

状态：T-P4-002 本机实现与验收完成；完整平台继续实施。负责人：主集成代理。关联：implementation-plan P4；既有 DAG 与项目授权合同继续适用。

## 本阶段范围

交付持久 cron 与人工触发规则、不可变规则修订及触发记录、固定流程版本的原子运行绑定、规则启停和运行事件断线续传。Webhook/Git/CI/监控签名接入、厂商账号真实执行、完整分层公平调度另行实施，不在本阶段伪装完成。

规则变更与触发均携带稳定操作标识和预期版本。启用需当前审批权限；每次派发重新核验启用人的当前项目写入与审批权限。撤权、暂停、归档、流程不可用时停止新派发并留下原因。自动化不能代替节点审批、任务验收或生产授权；禁止引用受保护的服务发布流程。

## 自动化 API

所有接口位于 `/api/v1`，除组织会话外均携带 `project_id`；项目范围 Runner 凭证不能访问业务规则。严格校验未知字段和版本。

- `GET /automations`：`items,total,next_cursor`，稳定分页。
- `POST /automations`：`idempotency_key` 与规则规格；新建为 `draft`。
- `POST /automations/preview`：规则规格，返回从数据库当前时间起的未来五次 UTC 与本地时间；不产生业务状态。
- `GET /automations/{id}`：规则详情。
- `POST /automations/{id}/commands`：`action,expected_version,idempotency_key`；action 为 `revise,enable,pause,archive,trigger`。revise 携带完整 `spec`；修改启用规则先暂停，必须重新启用。trigger 仅用于已启用规则，持久化一次人工触发。
- `GET /automations/{id}/occurrences`：`items,total,next_cursor`；包含真实运行关联和等待/跳过/阻塞原因。
- `GET /automations/{id}/revisions`：不可变规格历史的稳定分页；详情中的修订摘要只包含最近 100 项，不代表全部历史。

规则规格位于请求 `spec` 内：`name,workflow_id,workflow_version,trigger,cron,timezone,misfire,catch_up_limit,overlap`；读取规则时这些字段位于顶层。trigger 为 `cron|manual`；cron 为五段数字表达式（列表、范围、步进、星号），timezone 为 IANA 时区。misfire 为 `skip|latest|catch_up`；catch_up_limit 为 1..100；overlap 为 `allow|forbid|queue`。限制表达式与预演计算成本。使用随程序交付的时区数据，明确夏令时：不存在的本地分钟不触发，重复的分钟按不同 UTC 时刻触发。日与星期同时受限时遵循 cron OR 语义。

规则公开字段：规格加 `id,project_id,version,revision,state,activated_by,next_at,created_at,updated_at,last_error`。revision 仅规格修改递增；version 对每次业务命令递增。历史修订不可修改。系统推进游标不消耗用户 CAS 版本。

触发字段：`id,rule_id,revision,source,scheduled_at,state,reason,run_id,created_at,updated_at`。source 为 `cron|manual`；state 为 `queued|started|skipped|blocked`。运行终态由关联运行读取，保留触发事实。触发去重键绑定租户、项目、规则、修订和 UTC 名义时刻；人工触发绑定稳定操作标识。

恢复：正式时间来自数据库；调度游标与触发清单同事务推进。每轮工作有上限；误点和进程重启不重复派发。skip 记录过时窗口，若当前分钟匹配则保留一次；latest 合并为最新一次；catch_up 补跑窗口内最早的 N 次，超限范围明确记录 skipped，不在后续轮次无界补齐。overlap 仅计算该规则尚未终结的运行；forbid 显示跳过，queue 持久等待，allow 可并行。运行与 occurrence 绑定同事务提交，多调度器竞争只产生一个运行。结果未知/暂停/等待审批仍算未终结。排队规则不能用大量等待项占满派发候选，各规则的队首参与轮转。

命令统一返回 `{rule,occurrence?}`；人工触发先持久化 queued，由 tick 再派发。未来预演返回 `{items:[{utc,local}]}`；人工规则没有定时时刻，items 为空。所有规则接口仅允许真实组织会话，拒绝 API/Runner/Agent 范围凭证。

### 从运行回查原触发

`GET /runs/{id}/automation-origin?project_id=…` 使用当前浏览器项目读取权限。返回项目、运行及 `origin`。普通运行的 `origin` 为 null，项目范围内不存在的运行返回 404。正式来源包含规则 ID、触发时的规则名称与不可变修订、准确触发 ID、触发方式及 UTC 名义时刻。控制面核对原绑定与运行的固定流程，损坏或歧义的绑定返回错误。修改或暂停规则不改写历史来源。 [操作与验证](../quality/automation-run-origin-verification.md)

## 持久运行事件

`GET /runs/{id}/events/stream?project_id=...&after=N` 使用同源 HttpOnly 会话或现有 Bearer；不接受 URL 令牌。`Last-Event-ID` 与 after 均为非负安全整数，冲突拒绝。每条已落库事件格式 `id: <sequence>\nevent: run_event\ndata: <Event JSON>\n\n`，按全局持久序号递增，允许其他运行造成的间隙。只发送当前运行已持久事件。

断开后从最后一条完整解析的事件游标重连，重复事件幂等忽略；游标不能跨租户/项目/运行混用。受控短连接轮换和注释心跳；每次读取重新验证有效会话、当前项目授权与凭证范围，撤权立即结束后续发送。写入使用逐次截止时间和取消上下文，不取消普通接口超时。终态运行仍可回放全部历史；不以终态替代最后事件持久化。

公开 Event.data 始终为对象；旧持久事实中的空元数据在读取时映射为 `{}`，不改写数据库原值。浏览器按 UTF-8 字节限制单帧 16 MiB（容纳 2 MiB 请求经 Go JSON 转义的膨胀），实时保留窗口同时限制 300 条和 16 MiB。未知协议错误停止盲重连，提供持久历史核对；不能跳过坏帧推进游标。正常连接每 20 秒完整 EOF 轮换，心跳间隔 5 秒；单次慢写 2 秒，成功 Flush 后恢复空闲截止与最多 2 秒结束宽限，避免把空闲或最终分块误当慢写。

前端保留发布布局、视口与选择。事件驱动刷新正式运行快照，合并突发通知；事件列表有界并提供更早历史，明确“实时连接/重连/权限失效”等状态。项目、运行变化取消旧连接，陈旧回调不能覆盖当前上下文。事件通知不修改业务状态或推测成功。

### 终端跟随

`suma --project PROJECT run watch RUN [--after SEQUENCE] [--max-events N]` 使用同一事件流和当前项目权限。每行标准输出是一条已验证的事件 JSON。运行 ID、SSE ID 和内容序号必须匹配。重复已读序号不再输出；只有完整写入一行后才推进游标。注释心跳用于判断连接。标准输出只保留事件，终止错误使用标准错误通道。

每次连接使用 CLI 请求超时；短连接轮换和可重试连接失败从最后成功输出的游标重连。连续 5 次无有效进展的连接失败后终止，重试使用有界退避。鉴权失败、无效游标或无效协议停止读取，脚本可保留最后完整一行的 `sequence`，核对持久历史后继续。默认持续跟随；Ctrl+C 或达到 `--max-events` 后退出。限量只计算新输出的事件。 [终端使用](../docs/operations/task-dispatch-options.md#终端运行跟踪)

## 验收

覆盖 cron 边界/时区/DST/无效表达式、有界预演、暂停/修改/重启、误点和竞争、overlap 三策略、missfire 三策略、撤权与受保护流程隔离、运行绑定回滚；SSE 断线续传/重复/乱序/非法游标/隔离/撤权/背压/取消；浏览器创建预演、启用、人工触发、刷新恢复、真实运行和事件连接状态。SQLite 为本轮真实数据库；OceanBase/PostgreSQL 只报告实际执行的范围。
