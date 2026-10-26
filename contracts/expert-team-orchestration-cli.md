# 专家组交付计划 CLI 合同（T-P4-029）

`suma team-plan` 使用现有控制面 API 管理同一个 `ExpertTeamPlan`，遵循
[专家组自动推进合同](expert-team-orchestration.md)。以下以当前
[命令实现](../internal/cli/expert_plans.go)和[全局入口](../internal/cli/cli.go)
为准；不新增本地调度器、凭据接口或自动验收入口。

## 全局选项

全局选项必须放在 `team-plan` **之前**，子命令选项放在相应子命令及 ID 后。

| 选项 | 默认值／语义 |
| --- | --- |
| `--control ORIGIN` | `http://127.0.0.1:4310`；仅 HTTP(S) origin，不含账号、查询、片段或路径（允许末尾 `/`） |
| `--project ID` | 所有计划 API 请求必需；同时绑定查询和写入正文的 `project_id` |
| `--token TOKEN` | 默认读取当前进程 `SUMA_TOKEN`；优先通过既有安全凭据流程提供该环境变量 |
| `--expected-user ID` | 默认 `SUMA_EXPECTED_USER`；浏览器写入的原账号视图，32 位小写十六进制 ID |
| `--expected-organization ID` | 默认 `SUMA_EXPECTED_ORGANIZATION`；浏览器写入的原组织视图，同样为 32 位小写十六进制；设置时必须同时提供原账号 |
| `--timeout DURATION` | 每次请求默认 `15s`，必须大于零 |
| `--language zh-CN\|en-US` | 显示语言；优先级为显式选项、`SUMA_LANGUAGE`、`LC_ALL`／`LC_MESSAGES`／`LANG`、`zh-CN`；接受既有语言别名 |
| `--help` / `--version` | 全局帮助／版本；`--version` 不请求 API |

项目范围 API 凭据不需要浏览器视图选项。浏览器写入沿用当前服务端的账号、组织
视图检查；CLI 发送 `X-Suma-Expected-User`／`X-Suma-Expected-Organization`，
不会自动获取新身份替换原视图。这些选项及 `--confirm` 都不授予权限。

示例中的 ID、版本和文件名须替换为当前项目事实；凭据不写入请求文件。

```text
suma --control http://127.0.0.1:4310 --project PROJECT_ID --language zh-CN team-plan --help
suma --project PROJECT_ID team-plan list --limit 20
suma --project PROJECT_ID team-plan get PLAN_ID
```

## 命令及固定端点

以下路径均以 `/api/v1` 为前缀。CLI 为请求添加 `project_id`；正文可以省略该字段，
如提供则必须与 `--project` 完全相同。

| 命令 | HTTP |
| --- | --- |
| `team-plan list [--limit N] [--before NEXT_CURSOR]` | `GET /expert-team-plans` |
| `team-plan get ID` | `GET /expert-team-plans/{id}` |
| `team-plan revisions ID [--limit N] [--before NEXT_CURSOR]` | `GET /expert-team-plans/{id}/revisions` |
| `team-plan revisions ID REVISION` | `GET /expert-team-plans/{id}/revisions/{revision}` |
| `team-plan create --file request.json --confirm` | `POST /expert-team-plans` |
| `team-plan start ID --file request.json --confirm` | `POST /expert-team-plans/{id}/start` |
| `team-plan command ID --file request.json --confirm` | `POST /expert-team-plans/{id}/commands` |

分页默认 `limit=20`，范围 `1..100`；`--before` 使用上一页原样返回的
`next_cursor`。单个修订号为规范十进制正整数，不能与分页选项同时使用。
`team-plan help` 和 `team-plan --help` 均提供中英文命令帮助。

## 类型化写入示例

所有写入同时需要 `--file` 与显式 `--confirm`；`--file -` 从标准输入读取。
文件须为单个 UTF-8 JSON 对象，大小不超过 4 MiB；拒绝重复键、未知字段、无效
Unicode、过深嵌套及附加 JSON。版本使用 JSON 整数，例如 `2`，不能使用
`2.0`、`2e0` 或字符串。服务端继续校验版本范围、状态和完整业务依据。

### 创建（create.json）

```json
{
  "team_id": "TEAM_ID",
  "expected_configuration_version": 2,
  "items": [
    {"task_id": "TASK_A_ID", "expected_task_version": 1, "role_key": "developer"},
    {"task_id": "TASK_B_ID", "expected_task_version": 1, "role_key": "developer"}
  ],
  "idempotency_key": "delivery-plan-create-001"
}
```

```text
suma --project PROJECT_ID team-plan create --file create.json --confirm
```

`items` 为 1–16 个唯一任务；职责来自指定版本的已发布配置。`role_key` 最多
40 字节，以小写字母开头，其余可含小写字母、数字、下划线、连字符。任务依赖
由服务端从当前任务冻结，不接受额外的 `depends_on` 输入。创建只形成待启动
计划，不因 `--confirm` 自动启动或接受任务。

### 启动（start.json）

```json
{
  "expected_version": 1,
  "acknowledge_dispatch": true,
  "idempotency_key": "delivery-plan-start-001"
}
```

```text
suma --project PROJECT_ID team-plan start PLAN_ID --file start.json --confirm
```

使用创建／读取返回的计划 `version`。启动只派发当前合格项并激活后续推进；
后继等待正式人工验收精确的本计划前置 run，运行成功或测试通过均不能替代验收。

### 管理命令（pause.json）

```json
{
  "expected_version": 2,
  "action": "pause",
  "reason": "等待负责人核对尚未启动的工作",
  "acknowledge": true,
  "idempotency_key": "delivery-plan-pause-001"
}
```

```text
suma --project PROJECT_ID team-plan command PLAN_ID --file pause.json --confirm
```

`action` 仅允许 `pause`、`resume`、`stop`、`revise`，均需非空 `reason` 和
`acknowledge=true`。前三种动作禁止携带 `items` 或
`expected_configuration_version`。暂停阻止新派发，活动 run 继续；显式恢复
重核待启动来源并以当前实际凭据更新激活。停止计划对未开始项终止推进，不声称
活动进程已经停止；原 run 的取消／核对仍走其原有入口。

修订示例（revise.json）：

```json
{
  "expected_version": 3,
  "action": "revise",
  "reason": "明确更新尚未启动任务的来源版本",
  "acknowledge": true,
  "expected_configuration_version": 2,
  "items": [
    {"task_id": "TASK_B_ID", "expected_task_version": 2, "role_key": "developer"}
  ],
  "idempotency_key": "delivery-plan-revise-001"
}
```

修订仍使用 `team-plan command PLAN_ID --file revise.json --confirm`。仅
`ready`／`paused` 计划可修订已有、尚未启动的任务；已启动项不得出现在修订
`items` 中，其来源和 run 保持固定。配置版本改变时，必须覆盖全部尚未启动项。
以当前 `allowed_actions` 和服务端校验为准；示例版本并非保证连续发生的序列。

## CAS、幂等、权限与输出

写前读取当前配置、任务或计划版本；`expected_*_version` 是比较后写入（CAS）
依据，不能用客户端猜测值覆盖。后台进度变化也可能更新计划版本。冲突后重新
读取并明确决定新操作，不能静默改版本重试。

每个新意图提供独立、稳定的 `idempotency_key`；CLI 要求长度 8–128 UTF-8
字节且不含 NUL／回车／换行，服务端仍校验。此命令族要求显式键，不使用通用
客户端的自动生成键路径。超时或断连可能已经提交：先读取原计划；如需恢复原
操作，保留原键、原正文、目标项目及原身份视图。CLI 不自动重试或刷新身份；
服务端重放仍重核当前权限，同一键不能代表另一个意图。

当前项目读取／写入权限、账号／组织、凭据有效性、冻结来源及执行路由均由服务端
核验。创建、启动、管理不替代专家配置的浏览器发布批准或任务的正式人工验收。
凭据过期／撤销后不再自动派发，需授权主体显式恢复；无自动接受、强制重派或
修改已启动来源的开关。

成功输出服务端 JSON；用户正文、机器状态、ID、哈希及原始证据不按显示语言改写。
错误输出到 stderr。退出码：`0` 成功，`1` API／传输失败，`2` 参数或本地输入
错误。产品命令帮助和本命令诊断支持中英，不据此宣称全部底层 CLI 诊断已完成
多语言验收。本文件为实现合同说明，本次文档补充未新增运行或测试证据。
