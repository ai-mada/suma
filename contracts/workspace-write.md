# 远程工作区 CAS 保存合同

状态：T-P4-038／1.0.0，有界实现，2026-10-27。负责人：Control 管当前授权、命令和回执；Runner 管活动原生句柄及准确文件效果；工作台／CLI 管草稿和人工恢复。[共享类型](../internal/workspacewritecontract/types.go)为字段依据，[架构](../docs/architecture/remote-workspace-write.md)解释内部边界。本地GUI和真实联合证据见[限定报告](../docs/quality/workspace-write-verification.md)；物理多机／TLS和生产继续待验收。

## 范围和当前授权

仅覆盖当前原生 attempt 内**已存在的完整 UTF-8 文本文件**，新旧正文均不得超过48KiB。保存固定规范相对 `path`、原完整字节 `expected_sha256`、新正文及 `content_sha256`；不提供创建、删除、强制覆盖、任意路径、终端或 Git 写操作。路径和敏感内容仍由原生 `os.Root`、身份及策略检查。

编辑来源使用[只读观察](workspace-observation.md)的 `read` 与 `arguments:{path,full_file:true}`；此模式禁止行分页参数，结果必须完整、从第1行开始，正文长度和 SHA 对应全部原字节。分页、截断或超限结果不能作为完整编辑基线。原观察到期不独立撤销草稿的 CAS 依据；发出新保存仍需当前授权。

创建及 execute 领取均要求原浏览器／项目API凭据当前 `purposeProjectWrite`，准确当前 run／node／attempt／fence／worker、批准工具档案和库存，以及实际活动句柄的 `workspace_policy_sha256`。若运行关联任务，任务必须仍绑定该 run、尚未 accepted、`run_task_basis` 未变化。Pool 还要求原完整派发、当前机器凭据、active Pool 和准确 ID／版本的当前双边授权。`targets.can_write`只表示当前人和任务具备写资格，不是宿主健康承诺。

## 公共接口

前缀 `/api/v1`，响应禁止缓存。JSON 使用封闭类型，拒绝未知／重复／错大小写字段、非法 Unicode 和整数。GET 只允许单个非空 `project_id`。

| 接口 | 请求／返回 |
| --- | --- |
| `POST /workspace/writes` | `{project_id,scope,workspace_policy_sha256,path,expected_sha256,content,content_sha256,idempotency_key}` → `Command` |
| `GET /workspace/writes?project_id=…` | 原凭据自己的最近50项 metadata，含准确 `total`、`limit`、`truncated` |
| `GET /workspace/writes/{id}?project_id=…` | 原凭据当前项目读取权下的 metadata 与效果事实 |
| `POST /workspace/writes/{id}/inspect` | `{project_id,expected_version,idempotency_key}` → 原命令及新的核对身份 |

`scope`沿用准确 `run_id,node_id,attempt,fence,worker_id,tool_profile_id,profile_sha256`。命令公开路径、摘要、版本、状态、执行／核对身份、时间和效果，永不返回原新正文。GET／列表需要原调用凭据当前项目读取权；同用户的新会话不能读取旧命令。目标关闭后仍可查已发生的 metadata 事实，不重开文件。

创建同 key 固定原请求摘要，内容改变返回冲突。POST回执丢失后，同原凭据、key和完整请求摘要的已存在命令可凭当前项目读取权恢复metadata；这是读取原回执，不授予新写或execute许可。新key创建仍要求写权限。核对响应丢失时，保持同命令 ID、原 key 和原 `expected_version`：服务先恢复原操作回执，再做新核对的版本 CAS。每次新的核对需显式动作、新 key，且此前核对已终结；不自动循环。

## 单次许可、核对和回执

请求 TTL 为120秒；execute 许可最多10秒，仅发出一次，固定 `execution_fence=1` 和 `execute_claim_id`。`requested → leased → applied|not_applied|reconciling`；未领取到期为 `expired`，已领取不确定或超时进入 `reconciling`，永不回到可执行队列。每项目最多200项 requested／leased／reconciling 命令。

| 私有 Runner | Pool |
| --- | --- |
| `POST /runner/workspace-writes/claim` | `POST /pool-runner/workspace-writes/claim` |
| `POST /runner/workspace-writes/result` | `POST /pool-runner/workspace-writes/result` |

Claim 提交 `{project_id,worker_id,scope,workspace_policy_sha256,operation_id}`。Pool 另带 `tenant_id,runner_id,assignment` 原完整派发。返回 `{lease:null}` 或 `{lease:{command,claim_id,mode,lease_until,content?}}`；仅 execute 带正文。空轮询仍查当前授权，不落操作回执，同 key 不冻结未来空状态；实际非空领取保持准确幂等。

显式核对产生独立 `inspection_id`，领取后有10秒 inspect 许可，不含正文，只核对同一个 `remote-write:{command_id}` 原生操作。原准确完成回执可证明 applied；活动原句柄在与模型写入相同锁内持久记录 `not_started` 屏障，可证明 not_applied 并阻止晚执行。缺失／关闭宿主、prepared 或效果不明保持 unknown；不能靠当前文件相同推断曾经成功，也不能重开历史宿主或重新执行来消除未知。

Result 固定原 scope、policy、request SHA、execution fence、claim ID、mode、`observed_at`、`result_sha256` 和稳定 `operation_id`。applied 必须提供准确 `{native_operation_id,path,previous_sha256,sha256,generation}`；not_applied 提供封闭原因；unknown 只用 `operation_unknown`。ACK 仅含命令／claim／结果摘要和确认标志。迟到的准确效果回执可排空，已 applied 不降级。

事实排空核对原接受命令、原 assignment 和当前原机器凭据；目标关闭、原调用者撤权或 Pool grant 撤回后仍可接受固定效果。这不是新写授权，不能恢复正文领取或再执行。Control 授权点与远端 rename 不是分布式原子事务；已发出的短许可存在至多10秒的撤权传播窗口，不能承诺撤权瞬间撤销远端已发许可。

## 存储、质量和验证

Control 的正文只在独立 delivery 列中，超时、未领取到期或收到效果事实时清除；命令和通用操作回执只存 metadata。后台每轮最多处理100项到期命令。Runner 的私有 journal 与原生操作账本另有恢复规则；逻辑清除不等于所有磁盘副本同时擦除。

保存沿原生 CAS 操作账本和同一文件锁执行，增加工作区 generation，使此前原生 checks 不再证明新版本可封存。已固定执行计划不是新文件质量证明；正式候选、准确 SHA 的独立测试、人工验收和部署仍为独立事实。保存不调用模型、不改变任务或 DAG 状态。

验证依据：[Control／HTTP](../internal/platform/workspace_write_test.go)与既有观察回归合并定向通过（8.856秒）；[Runner 用例](../internal/runner/workspace_write_test.go)覆盖原生 CAS、与模型并发、旧 checks 失效、准确回执和 not_started 屏障。实际联合与 GUI 结果另记，不据此声明物理多机／TLS、生产或完整 IDE 已完成。
