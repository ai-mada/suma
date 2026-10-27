# 远程工作区只读观察合同

状态：T-P4-037／1.0.0，有界实现，2026-10-27。负责人：Control管授权与持久队列，Runner管当前原生句柄与出站回执，工作台／CLI管呈现。[共享类型](../internal/workspaceobservationcontract/types.go)是字段依据，[架构](../docs/architecture/remote-workspace-broker.md)说明内部边界。GUI、物理多机／TLS及生产验收另行记录。

## 范围与授权

仅允许 `list` 与 `read`。目标必须是当前项目内仍运行、租约有效的准确原生 `agent` attempt，工具档案已明确启用、当前库存与批准摘要一致，声明 `attempt-workspace-v1`。运行暂停时仍在执行的节点可被观察；取消、关闭、attempt／fence变化或租约失效停止读取。观察不创建运行、模型调用、候选、测试或人工验收。

公开 `scope` 固定 `run_id`、`node_id`、`attempt`、`fence`、`worker_id`、`tool_profile_id`、`profile_sha256`。组织与项目来自真实凭据和外层 `project_id`。实际宿主在领取时另交 `workspace_policy_sha256`；它不是工具档案摘要的别名，同一观察重新领取不能换策略。

浏览器会话和项目API凭据需要当前项目读取权。请求固定原调用凭据的非秘密引用；领取、回写及结果读取均重查该原凭据和当前权限。另一个会话即使属于同一用户，也不能读取原请求。私有Runner限自己的准确当前派发；Pool还核对原完整派发、当前机器凭据、active Pool与相同ID／版本的当前双边项目授权。旧事实收尾可用的撤回／撤销授权，不能用于新的工作区读取。

## 公共接口

API前缀 `/api/v1`，响应禁止缓存。GET要求单个非空 `project_id`；未知、重复或空查询字段拒绝。JSON拒绝未知／错大小写字段、重复键、非法Unicode及不合法整数。

| 接口 | 行为 |
| --- | --- |
| `GET /workspace/targets?project_id=…` | 当前可尝试目标，不创建观察或预留执行资源 |
| `POST /workspace/observations` | `{project_id,scope,operation,arguments,expected_sha256?,idempotency_key}` |
| `GET /workspace/observations/{id}?project_id=…` | 原调用者当前再授权后的状态与有界结果 |
| 同上加 `metadata=true` | 做相同授权、目标和到期检查，省略结果正文；不重读文件 |

`metadata`只接受 `true`。发现最多扫描201条运行记录、展示50个可尝试目标；`total`是扫描窗口内的可尝试数，`truncated`表示结果受限，不承诺全局完整库存。

`arguments`为封闭结构：

- `list`：`directory?`、`limit?`（0取宿主默认值，最大200）、`cursor?`（最多4096字节）。目录空串表示工作区根。
- `read`：`path`、`start_line?`（默认1）、`max_lines?`（最大1000）。`expected_sha256?`可固定后续分页的原文件字节摘要。

路径为规范相对路径，禁止绝对路径、反斜杠、冒号、`..`和敏感配置路径；Runner仍执行原生 `os.Root`、精确大小写、路径身份及敏感内容检查。词法校验不代替宿主检查。

## 出站机器接口

| 私有Runner | Pool |
| --- | --- |
| `POST /runner/workspace-observations/claim` | `POST /pool-runner/workspace-observations/claim` |
| `POST /runner/workspace-observations/result` | `POST /pool-runner/workspace-observations/result` |

Claim提交 `{project_id,worker_id,scope,workspace_policy_sha256,operation_id}`，返回 `{lease:null}` 或 `{lease:{observation,lease_fence,lease_until}}`。Pool外层额外携带 `tenant_id`、`runner_id`及原完整 `assignment`，不能从浏览器scope拼造。

未实际领取租约的 `lease:null` 是无效果的无状态轮询：仍做完整当前授权与scope检查，但不写操作回执。同一操作ID在空轮询后可领取后来出现的观察，不承诺固定“未来仍为空”；实际领取／重领及结果ACK继续按原操作幂等恢复。

Result提交原 `request_id`、`scope`、`lease_fence`、`workspace_policy_sha256`、`observed_at`、`result_sha256`、稳定 `operation_id`，以及互斥的 `observation_result`／`error_code`。它不复用DAG的 `result`。共享 `ResultDigest`固定计算 `{observation_result:值或null,error_code:字符串}`；机器只获得无正文ACK：`{request_id,lease_fence,result_sha256,acknowledged}`。

## 生命周期与结果

请求存活60秒，观察租约10秒，使用控制面数据库时间。`requested → leased → completed|failed`；到期进入 `expired`，当前目标失效时读取转为 `cancelled`并停止披露。撤权或原凭据失效直接拒绝读取／回写。每项目最多200项未到期观察。

只读请求可在到期前重新领取，`lease_fence`单调增加；旧fence、旧策略、过期租约或不同scope不能回写。观察时间不得早于本轮租约。相同操作恢复原ACK，不重读变化后的文件，不拼接不同时间的快照。

结果二选一：

- `list`：`entries[{path,kind}]`、`next_cursor`、`complete`，最多200项。
- `read`：`path`、`content`、原文件 `sha256`／`bytes`、`start_line`、`next_line?`、`complete`。源文件最大1MiB，单次正文最多48KiB，完整结果结构JSON最大64KiB。摘要对应原文件字节，不把分页正文误称为完整文件。

失败只返回封闭代码：`workspace_closed`、`workspace_unavailable`、`path_not_allowed`、`file_missing`、`text_required`、`output_limit`、`cursor_stale`、`file_changed`、`sensitive_content`或 `request_invalid`。参数／跨scope错误为400／403／404，当前目标或租约冲突通常为409。错误不附宿主绝对路径、MCP地址、工作区Bearer或任意执行环境。

Control单独存放正文，读取时检查到期，后台每轮最多清除100项到期正文；操作回执不保存正文副本。Runner用有界私有journal固定准确租约和待确认结果，恢复丢失ACK；过期／旧租约不再执行。本地恢复文件另有清理时机，不能宣称所有磁盘副本在60秒时同时删除。

## 验证与边界

[Control用例](../internal/platform/workspace_observation_test.go)覆盖当前调用者／Pool授权、旧fence、到期、准确ACK、跨scope、metadata无正文和DAG状态保持；[真实Worker联合入口](../internal/platform/workspace_observation_runtime_test.go)覆盖实际原生子进程、独立attempt工作树、出站HTTP、真实list/read与关闭后拒绝，未调用模型或代替正式任务验收。

远程Git、写入、终端、PTY、LSP／DAP、历史工作树恢复、物理多机／TLS、生产隔离与大规模调度仍待独立交付。现有本地项目工作区是独立能力，远程失败时不能静默代用。
