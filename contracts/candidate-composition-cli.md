# 候选合成与代码基线 CLI 合同（T-P4-031）

沿用 [候选合成合同](candidate-composition.md)、现有控制面客户端和当前凭据，
不新增本地调度器、凭据接口、合并脚本或自动验收入口。实现为
[candidate_composition.go](../internal/cli/candidate_composition.go)。

## 全局选项与权限

全局选项放在 `candidate-compositions` 或 `task-code-baseline` **之前**。

| 选项 | 默认值／语义 |
| --- | --- |
| `--control ORIGIN` | `http://127.0.0.1:4310`；HTTP(S) origin，不含账号、查询、片段或路径，允许末尾 `/` |
| `--project ID` | 全部请求必需；固定请求查询与正文的 `project_id` |
| `--token TOKEN` | 默认当前进程 `SUMA_TOKEN`；建议通过既有凭据流程提供环境变量 |
| `--expected-user ID` | 默认 `SUMA_EXPECTED_USER`；浏览器写入使用原账户视图，32 位小写十六进制 ID |
| `--expected-organization ID` | 默认 `SUMA_EXPECTED_ORGANIZATION`；原组织视图，同样为 32 位小写十六进制；设置时须同时提供账户 |
| `--timeout DURATION` | 每次请求默认 `15s`，须大于零 |
| `--language zh-CN\|en-US` | 显式选项优先于 `SUMA_LANGUAGE`、`LC_ALL`／`LC_MESSAGES`／`LANG`、默认 `zh-CN`；接受现有语言别名 |
| `--help` / `--version` | 全局帮助／版本；版本输出不请求 API |

使用既有项目范围 API 凭据时不伪造浏览器身份。浏览器写入保留原
`X-Suma-Expected-User`／`X-Suma-Expected-Organization` 视图；不会自动刷新身份。
当前授权由服务端重核；`--confirm`、操作键和视图请求头都不授予权限。
正式任务 Review 和目标分支集成仍使用各自既有授权及人工批准流程。

## 命令与固定端点

以下路径均以 `/api/v1` 为前缀。JSON 可省略 `project_id`，CLI 会注入
`--project`；若 JSON 已包含该字段，必须完全一致。CLI 不接收任意候选 SHA、
包路径、封存证明或执行身份，来源事实均由服务端解析。

| 命令 | HTTP |
| --- | --- |
| `candidate-compositions options --task TASK_ID` | `GET /candidate-compositions/options?task_id=TASK_ID` |
| `candidate-compositions list [--task TASK_ID] [--limit N] [--before CURSOR]` | `GET /candidate-compositions` |
| `candidate-compositions get PLAN_ID` | `GET /candidate-compositions/{id}` |
| `candidate-compositions create --file request.json --confirm` | `POST /candidate-compositions` |
| `candidate-compositions start PLAN_ID --file request.json --confirm` | `POST /candidate-compositions/{id}/start` |
| `candidate-compositions command PLAN_ID --file request.json --confirm` | `POST /candidate-compositions/{id}/commands` |
| `task-code-baseline get TASK_ID` | `GET /tasks/{id}/code-baseline` |
| `task-code-baseline bind TASK_ID --file request.json --confirm` | `POST /tasks/{id}/code-baseline` |

分页默认 `limit=20`，接受规范十进制 `1..100`；`--before` 原样使用上一页的
`next_cursor`，最多 4096 字节。`options` 必须指定集成任务；`list` 可不按任务过滤。
两个命令均支持 `help` 和 `--help`，帮助内容支持中英文。

## 类型化写入

所有写入必须同时提供 `--file` 与显式 `--confirm`；`--file -` 从标准输入读取。
文件为最多 4 MiB 的单个 UTF-8 JSON 对象。拒绝重复键、未知或大小写别名字段、
显式 null、无效 Unicode、超过 32 层的嵌套和附加 JSON。版本须为 JSON 整数，
不能使用 `2.0`、`2e0`、`"2"` 或 `-0`；上限为 2147483646。

创建固定预览（`create.json`）：

```json
{
  "task_id": "INTEGRATION_TASK_ID",
  "expected_task_version": 2,
  "workflow_id": "WORKFLOW_ID",
  "workflow_version": 3,
  "producer_node_id": "code",
  "sources": [
    {"task_id": "TASK_A_ID", "expected_task_version": 4, "source_node_id": "code"},
    {"task_id": "TASK_B_ID", "expected_task_version": 5, "source_node_id": "code"}
  ],
  "idempotency_key": "composition-preview-001"
}
```

```text
suma --project PROJECT_ID candidate-compositions options --task INTEGRATION_TASK_ID
suma --project PROJECT_ID candidate-compositions create --file create.json --confirm
```

`sources` 为明确排序的 2–16 个不同来源任务；不自动排序或选择最新分支。
`create` 持久化固定预览，尚不创建运行。审阅返回的来源、工作流、路由与版本后，
使用独立文件明确启动（`start.json`）：

```json
{"expected_version":1,"acknowledge":true,"idempotency_key":"composition-start-001"}
```

```text
suma --project PROJECT_ID candidate-compositions start PLAN_ID --file start.json --confirm
```

启动前可明确撤回预览；`command` 当前仅支持 `dismiss`（`dismiss.json`）：

```json
{"expected_version":1,"action":"dismiss","reason":"重新核对来源顺序","idempotency_key":"composition-dismiss-001"}
```

```text
suma --project PROJECT_ID candidate-compositions command PLAN_ID --file dismiss.json --confirm
```

合成结果经独立测试及有权主体正式验收后，可明确固定给未开始的开发任务
（`bind.json`）。首次 `expected_binding_version` 为 `0`，替换时使用当前绑定版本；
`expected_composition_version` 必须来自最新读取的明确合成计划：

```json
{
  "expected_task_version": 1,
  "expected_binding_version": 0,
  "composition_id": "PLAN_ID",
  "expected_composition_version": 2,
  "acknowledge": true,
  "reason": "下游开发明确继承已验收合成结果",
  "idempotency_key": "task-code-baseline-001"
}
```

```text
suma --project PROJECT_ID task-code-baseline get TASK_C_ID
suma --project PROJECT_ID task-code-baseline bind TASK_C_ID --file bind.json --confirm
```

绑定会更新任务版本并使旧预览失效，不修改已开始运行或历史计划，也不自动派发任务。
其后的显式派发／专家计划未开始项修订再固定该受保护基线。

## CAS、恢复和验收边界

每次写入都要求调用者提供原始、稳定的 8–128 字符 `idempotency_key`，不会默默
生成新键、自动重试或切换身份。结果未知时保留原路径、正文、凭据、视图和操作键，
先读取原计划或基线。遇到 409，重新读取当前事实并显式决定后续操作；不悄悄改版本。
退出码沿用现有客户端：`0` 成功，`1` API／传输失败，`2` 用法错误。

CLI 不调用模型，不把合成／测试成功当作任务验收，不自动更新目标分支，不自动
启动下游开发。冲突与未知结果保留原运行恢复边界，不发起另一轮合成掩盖事实。

English: this CLI preserves current scoped authorization, the original browser view,
exact integer CAS versions, ordered source tasks and caller-supplied operation keys.
Creating a preview does not start a run. Start and baseline binding require explicit
acknowledgement; only a formally accepted composition may become a protected task
baseline. No automatic retry, identity refresh, model call, acceptance, target update
or downstream dispatch is performed.
