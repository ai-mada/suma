# Runner 本地执行日志只读诊断合同

状态：1.0.0；关联任务：T-DELIVERY-112。责任域：Runner 本地诊断与命令行。来源：[C206 源码整合记录](../docs/integrations/referenced-conversation-integration.md)。

[中文指南](../docs/agents/runner-journal-inspection.md) · [English](../docs/en/runner-journal-inspection.md)

## 使用与权限边界

```text
suma-runner --inspect-state --state EXACT_LOCAL_STATE_DIR --language zh-CN
```

该命令只读取指定目录内的 `journal.json`，不连接控制面，不读取 Runner 或模型凭据，不读取工具配置，不启动 Git、工具、模型或任务，不创建目录、写锁、结果或 ACK。它使用当前操作系统对该私有目录的读取权限；不新增平台服务端巡检权限，也不导出原始日志。

`--inspect-state` 在普通 Runner、资源池、工具探测和合并预检之前分流。不能与项目／执行器身份、执行许可、工具／模型／资源池配置、仓库／工作区、控制面或合并预检模式混用；冲突返回用法错误，不读取这些配置或凭据。帮助与版本仍先于任何业务操作。

资源池按实际槽位指定目录，例如 `.cache/runner/pool/slots/0`。该入口不递归扫描父目录，不读取 `pool-config`，不把槽位摘要当作整机状态。

### 资源池总览

```text
suma-runner --inspect-pool-state --state EXACT_RUNNER_STATE_DIR --language zh-CN
```

该模式使用启动时相同的 `--state`，从其 `pool` 子目录读取原机器绑定和规范编号的槽位。仅接受这三个选项，显式 `false`、空目录和其他模式的组合均返回用法错误。输出 `schema_version:1`、`component:local_pool_runner_state`、`status`、`complete` 和按数值排序的 `slots`。每槽只列 `slot_id`、`journal_status`、可用日志的 `journal_counts` 和 `claim_state`。结果不含身份、执行条目、凭据、路径或原始载荷。

`claim_state` 中，`request_recorded` 仅证明原领取已保存，`response_recorded` 仅证明已记录响应，`complete` 只表示本机领取检查点完成。其余状态区分缺失、不可安全读取、超出上限和内容无效。各文件分别观察，`complete` 表示本次读取覆盖范围。当前进程、租约、服务器确认和业务完成由对应权威另行核对。需要原运行、节点、尝试和 fence 时，按编号单独查看原槽位。

每次最多检查 128 个槽位，读取预算合计 64 MiB，单个日志和领取记录分别以 16 MiB 和 2 MiB 为限。有效根目录下的维护模式允许没有槽位。槽位日志损坏或存在不可安全读取的入口时保留可用行，并标记不完整。退出码 `0` 表示摘要读取完整，`1` 表示缺失、失败或不完整，`2` 表示参数冲突。

### 从资源池节点打开诊断

池所属组织从已授权节点列表打开“本机诊断”，输入原状态目录并选择 PowerShell 或 Bash。界面生成总览命令，由操作者在目标机器上运行。可选预览只接受本合同定义的总览 JSON，按已验证字段显示槽位和计数，提供原槽位命令。摘要来源由操作者核对。更换节点、组织、状态目录或关闭弹窗会清除预览，输入和摘要保留在当前页面内。

## 唯一输出范围

标准输出是 JSON：

| 字段 | 合同 |
| --- | --- |
| `schema_version` | 固定为 1，与产品版本分开管理 |
| `component` | 固定为 `local_execution_journal` |
| `status` | `missing / available / invalid / unreadable / over_limit` |
| `limit` | 固定为 100，仅限制输出条目数量 |
| `complete` | 仅在文件可读取且全部条目已列出时为 true |
| `counts` | 整个已解析文件的白名单计数 |
| `items` | 最多 100 个条目，只输出下面列出的安全元数据 |

`counts` 包含 `entries`、`pending_results`、`acknowledged_results`、`unknown_results`、`pending_progress`、`unresolved_inbox`、`pending_artifacts`、`pending_candidates`、`truncated_progress`。`unknown_results` 仅统计尚未本地确认且已记录 `outcome_unknown` 的结果；`unresolved_inbox` 表示没有结果或制品／候选检查点的本地收件项，不证明进程仍在运行。

每个条目只含摘要引用 `entry_ref`、安全的 `run_id/node_id`、`attempt/fence`、是否已记录／确认结果、原 `outcome_unknown` 标记、待回写进度数量、进度截断标记、待制品／候选检查点标记及 `identifiers_omitted`。引用由私有条目键计算摘要；它不是操作标识、凭据或恢复授权。标识只允许有界字母、数字及 `_ . : -`；不安全标识省略，并明确标记。条目排序确定，超过 100 项时 `complete:false`，计数仍覆盖完整文件。

以下字段永不输出：私有条目原键、Worker 凭据、操作 key、控制面 URL、本地路径、命令和参数、Assignment 配置、原始 Result、Error、Output、Progress.Message、ToolEvent、模型和工作区／制品／候选载荷。读取或解析错误只提供上述受控状态，不输出原始 OS 路径、JSON 片段或载荷。

## 读取和一致性

只接受实际目录及常规文件；拒绝目录／文件符号链接和非普通文件。通过根目录句柄限定读取，比较打开前文件与实际句柄的身份，最多读取 16 MiB。缺失目录或文件返回 `missing`，不自动初始化；超过上限返回 `over_limit`，不将截断内容当作有效摘要。损坏、重复条目键或额外 JSON 内容返回 `invalid`，不输出部分计数。

读取不调用 `OpenJournal`：该运行时入口会取得写锁、为未完成收件项记录未知结果并保存，不能复用为只读诊断。诊断不争抢活动 Runner 的锁，读取原子替换前或后的单文件快照；无法稳定打开时明确返回 `unreadable`。

输出只证明本地单个执行日志中的记录。`acked` 是本地保存标记，不是对当前服务器回执的重新查询。摘要不证明租约有效、执行器健康、工具可用、操作确实执行／未执行、当前预算或服务器接受状态。不读取 `claim.json`、`pool-claim.json`、工具库存或工作区读写 sidecar，也不声称覆盖这些独立状态。

当前私有 Runner 在原回写未确认时停止新领取；资源池普通回写错误阻塞对应槽位，撤权等致命错误会停止整机。现有日志没有持久化完整拒绝原因，因此本命令不把 pending 伪装成“已隔离”或推断具体 HTTP 拒绝。

## 退出与后续操作

`available` 返回 0；缺失、不可读、无效、超过上限返回 1 并仍输出安全 JSON；模式冲突返回 2。诊断说明及帮助提供中文和英文，机器字段保持稳定。

本入口没有 drop、ack、修复、清空、重新派发或自动重跑操作。发现未知结果或待确认回写时保留原私有状态，使用当前已有的原操作核对与授权恢复流程。新增修改型恢复能力须另立权限、不可变证据与副作用合同。
