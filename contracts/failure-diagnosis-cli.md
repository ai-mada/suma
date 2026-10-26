# 失败分诊 CLI 合同（T-P4-030）

`suma failure-diagnosis` 使用现有控制面客户端，遵循
[固定失败证据、智能分诊与修复需求合同](failure-diagnosis.md)。它不会在本地
另建模型执行器，也不在读取后自动创建诊断、采用建议或重试原运行。

## 身份与全局选项

沿用[现有 CLI 全局选项](expert-team-orchestration-cli.md#全局选项)，全部放在
`failure-diagnosis` 前。每次请求必须有 `--project ID`；`--control` 默认
`http://127.0.0.1:4310`，`--timeout` 默认 `15s`。`--language zh-CN|en-US`
控制帮助和诊断显示，不改变来源正文、模型原件或请求中的 `locale`。

读取可使用当前有权的浏览器或项目 API 凭据。此领域写入只支持当前浏览器会话：
复用既有安全流程提供的 `SUMA_TOKEN`，并提供原 `--expected-user` 或
`SUMA_EXPECTED_USER`。原组织视图可用 `--expected-organization` 或
`SUMA_EXPECTED_ORGANIZATION` 提供，沿用现有可选语义。CLI 不登录、签发凭据或
自动刷新身份；没有凭据或原账号视图时在本地拒绝写入。

`expected-user` 请求头不是浏览器身份或权限证明。即使伪填视图参数，API token
仍不能创建诊断或管理分析；服务端独立核验真实凭据种类、当前账号、组织及项目
权限。`--confirm` 同样不授予权限。

## 命令与端点

下列路径均以 `/api/v1` 为前缀，CLI 自动加入固定 `project_id`。

| 命令 | HTTP |
| --- | --- |
| `failure-diagnosis context --run ID --node ID --attempt N` | `GET /failure-diagnosis/context` |
| `failure-diagnosis list --run ID --node ID --attempt N [--limit N] [--before NEXT_CURSOR]` | `GET /failure-diagnosis/analyses` |
| `failure-diagnosis get ID` | `GET /failure-diagnosis/analyses/{id}` |
| `failure-diagnosis create --file request.json --confirm` | `POST /failure-diagnosis/analyses` |
| `failure-diagnosis command ID --file request.json --confirm` | `POST /failure-diagnosis/analyses/{id}/commands` |

`--run`、`--node`、`--attempt` 精确定位原失败尝试；`get`／`command` 的 ID
则是分析 ID。attempt 必须是规范正整数，不接受 `0`、`01`、`+1`、`1e0`。
列表默认 `limit=20`，范围 `1..100`；`--before` 原样使用同一范围上一页的
`next_cursor`，不自行制造游标。帮助入口为 `failure-diagnosis --help` 或 `help`。

```text
suma --project PROJECT_ID failure-diagnosis context --run FAILED_RUN_ID --node test --attempt 1
suma --project PROJECT_ID failure-diagnosis list --run FAILED_RUN_ID --node test --attempt 1 --limit 20
suma --project PROJECT_ID failure-diagnosis get ANALYSIS_ID
```

context 只读，不创建问题或模型执行。问题尚未投影、节点并非准确已知失败、结果
未知或仍需核对时，以服务端 `eligible`／`can_create`／`blockers` 为准。

## 显式创建与管理

所有写入均需 `--file` 和 `--confirm`。`--file -` 读取标准输入。输入为单个
有界 UTF-8 JSON 对象（最多 4 MiB、32 层），拒绝重复键、未知或大小写别名字段、
显式 null、无效 Unicode 及附加 JSON。数值必须是真实 JSON 整数，不接受字符串、
小数或指数形式。正文可省略 `project_id`；如提供，必须与 `--project` 完全相同。

创建示例（create.json；ID、版本、SHA 均替换为当前 context／工具事实）：

```json
{
  "issue_id": "ISSUE_ID",
  "expected_issue_version": 1,
  "run_id": "FAILED_RUN_ID",
  "node_id": "test",
  "attempt": 1,
  "expected_source_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "goal": "核对本次固定失败证据并提出可审核的修复需求",
  "locale": "zh-CN",
  "tool_profile_id": "TOOL_PROFILE_ID",
  "expected_tool_version": 2,
  "max_steps": 1,
  "max_output_tokens": 4096,
  "acknowledge_model_use": true,
  "idempotency_key": "failure-diagnosis-create-001"
}
```

```text
suma --project PROJECT_ID --expected-user ORIGINAL_USER_ID failure-diagnosis create --file create.json --confirm
```

显式 create 才提交一次受管分诊。固定来源 SHA 不能凭空生成；工具须是当前可用
专用 FailureDiagnosis profile。`max_steps` 可省略，固定为 1；输出预算可省略
默认 4096，范围 `1..8192`。`acknowledge_model_use=true` 不能由 `--confirm`
替代。JSON 中的 goal 最多 2000 UTF-8 字节，不接受调用方提供的证据正文或权限。

人工修订示例（revise.json）：

```json
{
  "expected_version": 2,
  "action": "revise",
  "repair": {
    "title": "修复已核对的失败条件",
    "description": "明确描述需要修复的行为及边界，保留原证据的不确定性。",
    "priority": "medium",
    "acceptance_criteria": ["增加独立回归测试并由有权人员正式验收"]
  },
  "reason": "核对证据后人工修订",
  "idempotency_key": "failure-diagnosis-revise-001"
}
```

```text
suma --project PROJECT_ID --expected-user ORIGINAL_USER_ID failure-diagnosis command ANALYSIS_ID --file revise.json --confirm
```

`repair` 只允许 title（1–200 字节）、description（1–16000 字节）、priority
（`low|medium|high`）及 acceptance_criteria（1–16 条，每条 1–1000 字节）。
人工修订不改写模型 observations、hypotheses 或原始输出。

明确采用示例（adopt.json）：

```json
{
  "expected_version": 3,
  "action": "adopt",
  "reason": "明确创建修复需求草稿",
  "acknowledge_draft": true,
  "idempotency_key": "failure-diagnosis-adopt-001"
}
```

采用仍使用 `command ANALYSIS_ID --file adopt.json --confirm`。`command` 仅支持
`revise|adopt|dismiss`，均须非空 reason（最多 2000 字节）。仅 revise 可携带
repair；adopt 必须 `acknowledge_draft=true`；dismiss 不携带 repair。
采用只创建或回到已有正式问题回流需求，不覆盖已有需求，不表示问题已修复，
不关闭问题、不重跑原节点、不自动验收。响应中的 `adopted_requirement_id`
提供真实需求链接依据；分析 `run_id` 是诊断运行，`source.run_id` 才是原失败运行。

## CAS、幂等及验证边界

写入保留当前 `expected_*_version`、固定来源 SHA 和稳定的
`idempotency_key`（8–128 UTF-8 字节，不含 NUL／回车／换行；服务端继续校验）。
CLI 不自动生成此命令族的操作键。来源漂移或版本冲突后读取当前事实，再明确
决定新意图；不能偷偷替换版本或 SHA。超时可能已经提交，恢复原操作必须保持
原键、原正文、项目及原身份视图；CLI 不自动重试或自动采用。重放仍需当前权限。

成功输出服务端 JSON，原始模型／来源／用户内容不翻译。错误输出到 stderr；
退出码 `0` 成功、`1` API／传输失败、`2` 参数／本地输入错误。中英帮助与本命令
静态诊断沿既有目录提供，不代表全部底层 CLI 诊断已完成多语言验收。

本阶段 CLI 验证只使用本地 HTTP fixture 核对端点、正文、原视图、严格输入和
撤权不重试；不据此声称真实模型、整条产品流程或生产验收。
