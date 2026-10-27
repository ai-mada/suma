# 远程工作区出站只读 Broker

状态：T-P4-037／1.0.0，有界实现，2026-10-27。本文由036后续设计更新为实际结构；036的 `developer check` 仍只证明CLI到控制面项目MCP的连接，不代替本阶段Runner文件读取。负责人：Control／Runner。公开字段与限额统一见[合同](../../contracts/workspace-observation.md)，运维见[AI SOP](../operations/remote-workspace.md)；GUI和生产验收另记。

## 真实读取链

浏览器或CLI读取当前原生运行目标，明确创建 `list`／`read` 观察。Control固定原调用凭据引用、项目、准确run／node／attempt／fence／worker及工具摘要。实际Worker仅通过既有出站HTTP领取当前assignment的观察；Runner从进程内注册表取得已打开的Native句柄，复用原 `os.Root` 和路径／内容检查，回传有界结果。原调用者再次通过当前权限、目标及TTL检查后才能读取。

宿主MCP仍为执行内部loopback连接，浏览器不取得端口或Bearer。Control不打开Runner文件路径，不创建本地工作区绑定。句柄关闭或assignment取消即撤下注册；journal恢复不重新打开历史宿主路径。

## 模块和事实归属

| 模块 | 责任 |
| --- | --- |
| `internal/workspaceobservationcontract` | 非秘密scope、操作、结构化结果、严格解码和摘要 |
| `internal/platform/workspace_observation*` | 当前授权、持久队列、CAS租约、限额、回执、短期正文与到期清除 |
| `internal/runner/workspace_observation*` | 当前句柄注册、只读调用、出站回写、准确journal恢复 |
| `cmd/suma-control` | 在已有后台周期内执行有界到期清理 |
| 工作台／CLI | 选择目标，呈现观察时间、分页和失效；不建立文件或执行授权 |

新增 `suma_workspace_observations`：不可变请求／原调用凭据引用，以及独立状态、版本、观察租约fence、到期时间与正文列。领取和到期索引服务有界查询；原操作唯一性绑定组织、项目、原调用凭据及操作ID。机器回执使用凭据命名空间，只保留请求摘要和无正文ACK。

空轮询保留完整授权检查，但未取得租约就不写操作回执；`lease:null`是无状态、无效果的结果，同key后续可取得新观察。只有实际领取／重领和结果ACK持久化准确回执，避免空闲worker按轮询频率堆积数据库行。

观察不修改DAG节点、执行租约、任务、候选、正式测试或模型账本。DAG当前attempt是读取前提，观察状态是独立事实；`completed`只表示本次观察完成。

## 当前授权与准确绑定

授权先合并参与者的组织、账户及凭据锁集合，按既有全序取得当前证明，再核验DAG精确状态与Broker CAS。保存的是原凭据的非秘密引用，不序列化 `Actor`，不把展示用 `Principal`升级为权限。

私有Runner复用当前身份和固定run路由边界。Pool核验不可变原派发、当前机器凭据、active Pool及准确ID／版本的双边项目授权。旧结果收尾可允许撤回／撤销授权排空事实，本观察通道额外拒绝这些状态。浏览器查询Pool目标也核验当前授权，不能从历史run获得持续文件权限。

`profile_sha256`绑定批准的执行工具；`workspace_policy_sha256`由实际活动Native句柄在领取时提交并固定。重新领取保持相同scope和策略、增加观察fence；结果时间必须属于本轮租约，旧fence不能覆盖新观察。

## 恢复、失效与内容

请求TTL为60秒，观察租约10秒。租约到期而请求仍有效时，只读观察可重新领取；失联不完成DAG、不触发执行重试。Runner先持久保存原租约，再读取并固定完整结果；ACK丢失后重发原结果，不趁恢复重读变化后的文件。过期、授权拒绝或旧fence回执退休，不新开历史句柄。

结果和metadata查询都检查原调用者、当前目标与TTL。目标关闭／变化后取消披露并清正文；到期正文由读取路径和有界后台清理处理。Runner私有journal有256项／32MiB上界，保存恢复回执；已确认／退休项在后续checkpoint按到期条件裁剪，不等同于Control即时清除所有副本。

路径经过共享词法检查及宿主身份检查，保留精确大小写和敏感文件边界。敏感内容命中即拒绝，不能涂改正文后保留原摘要。单次结果结构JSON最多64KiB，源文件最多1MiB，正文最多48KiB，列表最多200项；分页不构成原子目录或仓库快照。

当前发现最多扫描201条运行记录、展示50个可尝试项。Pool派发定位在相同pool／runner下按run范围最多取1001项检测超限，再严格解码；超过1000项拒绝使用。这是实现上界，不是全局发现或规模验收结论。

## 验证和后续

[领域与HTTP用例](../../internal/platform/workspace_observation_test.go)检查授权、准确fence、到期、幂等ACK、metadata及DAG状态保持；[联合入口](../../internal/platform/workspace_observation_runtime_test.go)启动真实Worker和独立原生协议子进程，通过出站HTTP读取真实attempt工作树，在宿主关闭后停止披露。它是无模型的本地协议验收，独立于GUI、正式候选测试和人工验收。

远程Git观察须另行移植已有Git安全合同；写入、终端、PTY、LSP／DAP、历史工作树恢复、物理多机／TLS及生产隔离不随list/read开放。完整平台目标与P0–P7继续推进。
