# CLI 批量调整任务

`suma` 提供 `task-priority` 和 `task-status`。先从项目任务读取准确 ID 和版本，一次明确选择 1 至 16 项。使用当前用户签发的项目 API 凭证，命令从 `SUMA_TOKEN` 读取。使用浏览器会话时，同时提供原会话的账号和组织视图。两种身份均须保有当前项目写权限。

```powershell
suma --project $env:SUMA_PROJECT_ID task-priority preview --file priority-preview.json
suma --project $env:SUMA_PROJECT_ID task-priority apply --file priority-apply.json
suma --project $env:SUMA_PROJECT_ID task-status preview --file selection.json
suma --project $env:SUMA_PROJECT_ID task-status apply --file status-apply.json
```

`--file -` 从标准输入读取。正文可以省略 `project_id`，填写时须与 `--project` 一致。保留同一批任务的顺序和原版本。

| 命令 | 请求字段 | 输出 |
| --- | --- | --- |
| `task-priority preview` | `tasks`、`priority` | 当前版本、原优先级、目标优先级和是否变化 |
| `task-priority apply` | 上述字段与 `idempotency_key` | 原操作回执，含原版本、新版本及变更结果 |
| `task-status preview` | `tasks` | 每项原状态、版本与可编辑资格 |
| `task-status apply` | `tasks`、`status`、`idempotency_key` | 原操作回执，含原状态、最终状态及版本 |

`tasks` 是唯一明确对象的列表，例如 `[{"task_id":"实际任务ID","expected_version":3}]`。优先级支持 `urgent`、`high`、`medium`、`low`。计划状态支持 `draft`、`todo`、`in_progress`、`blocked`、`cancelled`。状态预览返回 `editable`、`workflow_status`、`active_run`、`review_required` 或 `unverified_run`。预览只读取当前资格，提交再核对当前权限、原版本和适用业务条件。

提交需要调用者为确认过的正文提供 8 至 128 字节的稳定操作编号。若提交响应中断，用同一文件和原编号重新运行 `apply`。收到明确版本冲突时重新读取任务并预览，确认新正文后使用新的操作编号。CLI 输出 JSON，退出码 0 表示已核对结果，1 表示服务或传输问题，2 表示用法或输入错误。

Agent 只读令牌和 Runner 凭证不具备批量写授权。工具和智能体可在明确委派下通过用户签发的项目 API 凭证调用。批准、执行与验收由各自原流程管理。更多字段见[优先级合同](../../contracts/task-priority-changes.md)和[计划状态合同](../../contracts/task-planning-status-changes.md)。
