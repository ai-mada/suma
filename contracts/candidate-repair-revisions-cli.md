# Repair revisions and task execution history / 修复修订与任务运行历史

Status: implemented in 1.0.0; actual journey evidence is tracked separately.
状态：1.0.0源码已实现；实际旅程证据单独记录。

Use the existing authenticated `suma` connection and explicit project scope.
复用已有登录连接，明确指定项目。权限、计划版本、来源和用量规则与浏览器一致。

```sh
suma --project PROJECT candidate-compositions revision-context PLAN
suma --project PROJECT candidate-compositions revise-repair PLAN --file revision.json --confirm
suma --project PROJECT tasks run-history TASK --limit 20
suma --project PROJECT tasks run-history TASK --limit 20 --before CURSOR
```

`revision-context` is read only. Use its current plan/task versions, complete conflict digest and, for a certified startup failure, the recovery digest in the reviewed request file. The file uses the exact [API request](candidate-repair-revisions.md); `project_id` must equal the CLI project. Numeric versions are positive JSON integers. `acknowledge: true`, a nonempty reason and a stable `idempotency_key` are required. `--confirm` is separate from the request acknowledgement.

`revision-context`只读。将返回的当前计划／任务版本、完整冲突摘要以及可信启动失败时的恢复摘要写入经过审阅的请求文件。文件使用准确[API合同](candidate-repair-revisions.md)，`project_id`须等于CLI项目；版本必须为正JSON整数。必须包含`acknowledge: true`、修订理由和稳定的`idempotency_key`，并独立传入`--confirm`。

The response is a **new plan ID for the same task**. Revision does not start a run. Review the returned plan, then use the existing explicit composition Start command. Do not substitute the latest plan ID when recovering an uncertain command: retry the original path, body and operation key to retrieve its original receipt.

返回结果是**同一任务的新计划ID**，修订不启动运行。审阅新计划后使用已有明确Start命令。核对不确定操作时，保持原路径、请求正文和操作编号，不能自动换成最新计划。

`run-history` returns a bounded page with opaque `next_cursor`, current-run markers and accurate plan links. A nullable historical `bound_at` or `task_version` stays null when not recorded. `observed_at` is an observation time, not an invented execution or binding time. Reading does not backfill historical records.

`run-history`返回有界分页、不可自行构造的`next_cursor`、当前运行标识和准确计划链接。未记录的历史`bound_at`／`task_version`保持null；`observed_at`只是观察时刻，不代表虚构的执行或绑定时间。读取不追写历史。

Superseded plans cannot start, resume, retry or provide fresh code inputs. Unknown, stopped or effectful failures require reconciliation under the original run. A successful replacement still needs independent exact-candidate tests and explicit human acceptance before baseline handoff.

被替代计划不能启动、恢复、重试或提供新代码输入。未知、已停止或已发生效果的失败须沿原运行核对。替换后成功也必须通过独立准确候选测试和明确人工验收，才能交接代码基线。
