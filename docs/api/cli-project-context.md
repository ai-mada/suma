# CLI 项目知识与需求设计 / Governed project context

状态：T-P4-017 实施；本页描述 CLI 消费既有 API 的行为。领域权限、版本、来源核对和原子事务由服务端负责，CLI 不发行或提升身份。关联：[知识合同](../../contracts/project-knowledge.md)、[需求设计合同](../../contracts/requirement-design.md)、[当前身份合同](../../contracts/current-credential-authority.md)。

## 参数与输出 / Invocation and output

全局参数放在命令前。凭证通过进程环境 `SUMA_TOKEN` 提供；不要写进文件或命令历史。浏览器身份写入须携带操作时捕获的账户和组织，使用 `--expected-user`／`--expected-organization` 或对应的 `SUMA_EXPECTED_USER`／`SUMA_EXPECTED_ORGANIZATION`。这些头只约束原页面，不授予权限。项目 API 凭证具有服务端固定范围，可以编辑草稿，不能发布、退役或采用；Agent／Runner 不获得管理或批准能力。

Global flags precede the command. Use the process environment `SUMA_TOKEN` for credentials. Browser writes use the captured account and organization through the original-view flags or environment variables; those headers never grant authority. Scoped project API credentials may edit drafts when currently authorized, but cannot approve publication, retirement or adoption. The CLI never fetches a replacement identity or retries a request automatically.

```powershell
suma --language zh-CN --help
suma --language en-US knowledge --help
suma --language en-US requirement design --help
suma --control http://127.0.0.1:4310 --project PROJECT_ID --language zh-CN knowledge list
```

JSON 输出保留用户正文、来源快照、机器码和数值字面量；语言只影响本工具提示和 HTTP `Accept-Language`。远端诊断保留服务端返回的消息和本地化元数据。退出码 0／1／2 分别表示成功、API或传输失败、用法错误。

Responses retain user text, source snapshots, machine codes and numeric lexemes. Language controls CLI help/errors and `Accept-Language`; it does not translate stored content or remote diagnostics. Exit codes are 0 for success, 1 for API/transport failures and 2 for usage errors.

## 命令 / Commands

下表中的每个命令前都须添加全局 `--project PROJECT_ID`，可同时提供 `--control`、语言和原页面身份。所有历史读取一次只返回一页，默认20、最多100；使用服务端 `next_cursor` 原值读取下一页，不把不同项目或筛选的游标混用。

Every command below requires global `--project PROJECT_ID`. Histories return one page per invocation, default 20 and maximum 100. Reuse the exact `next_cursor` with the same project and filters.

| Command | HTTP | 用途 / Purpose |
|---|---|---|
| `knowledge list [--kind memory\|spec\|skill] [--state draft\|published\|retired] [--query TEXT] [--limit N] [--cursor VALUE]` | GET `/project-knowledge` | 筛选知识 / Filter knowledge |
| `knowledge show ID` | GET `/project-knowledge/{id}` | 当前草稿与发布 / Current draft and publication |
| `knowledge history ID [--limit N] [--cursor VALUE]` | GET `/project-knowledge/{id}/revisions` | 不可变修订 / Immutable revisions |
| `knowledge create --file FILE` | POST `/project-knowledge` | 完整人工草稿 / Full manual draft |
| `knowledge edit ID --file FILE` | PATCH `/project-knowledge/{id}` | 完整替换草稿 / Full draft replacement |
| `knowledge extract --file FILE` | POST `/project-knowledge/extractions` | 有来源的文本提取草稿 / Source-backed extractive draft |
| `knowledge command ID --file FILE` | POST `/project-knowledge/{id}/commands` | publish／retire |
| `requirement design show ID` | GET `/requirements/{id}/design` | 正式需求与设计 / Requirement and design |
| `requirement design save ID --file FILE` | PUT `/requirements/{id}/design` | 完整设计与任务草稿 / Full design and task proposal |
| `requirement design history ID [--limit N] [--cursor VALUE]` | GET `/requirements/{id}/design/revisions` | 设计修订 / Design revisions |
| `requirement design adoptions ID [--limit N] [--cursor VALUE]` | GET `/requirements/{id}/design/adoptions` | 采用记录 / Adoption records |
| `requirement design command ID --file FILE` | POST `/requirements/{id}/design/commands` | publish／retire |
| `requirement design preview ID --file FILE` | POST `/requirements/{id}/design/preview` | 只读完整范围变更预览 / Read-only full-scope change preview |
| `requirement design adopt ID --file FILE` | POST `/requirements/{id}/design/adopt` | 明确采用已审阅计划 / Explicitly adopt the reviewed plan |

`history` 与 `revisions` 同义，`command` 与 `commands` 同义。通用 `api /RESOURCE --method PUT --file FILE` 同样支持 JSON PUT；该入口是 JSON 客户端，不替代 Runner 的二进制制品上传协议。

`history`/`revisions` and `command`/`commands` are aliases. The generic `api` command also supports JSON PUT. It remains a JSON client; Runner binary artifact uploads retain their separate protocol.

所有 JSON 入口接受 `--file -` 从标准输入读取，最多4 MiB，要求有效 UTF-8 且只含一个对象；不静默截断。领域本身更小的正文／提案限制仍由服务端严格核验。`project_id` 可省略，由 `--project` 同时绑定到 URL 和正文；显式提供不同项目会在发送前拒绝。整数以 `json.Number` 保留字面量，CLI 不将小数、指数或大整数四舍五入成合法版本，服务端负责严格整数校验。

All JSON inputs accept `--file -` for bounded stdin: at most 4 MiB, valid UTF-8 and exactly one JSON object. Domain size limits still apply. The CLI binds `--project` into both URL and body, rejects a conflicting body project before sending, and retains number lexemes for strict server validation.

## 工作流 / Legal workflow

1. 读取正式来源与当前版本，再创建／提取／完整编辑草稿。知识 `sources` 只提供 `kind/id/version/quote`；不要提交可信快照、哈希、批准人或权限声明。`skill` 是文本知识，不执行脚本。提取是文本提取，不代表模型摘要。
2. 当前有批准权限的浏览器身份审阅正文与来源后，显式 `command` 发布。编辑已发布对象只生成新草稿，旧发布在明确发布或退役前继续存在。API／Agent／Runner 不能越过人工批准。
3. 需求设计保存须带 `expected_version`、`expected_requirement_version`、产品／体验／技术完整 `content` 和完整 `tasks`。任务 key 在版本间保持稳定；保存不创建正式任务。
4. 发布后，向 preview 提交 `expected_version`、`revision`、`sha256`。审阅响应的 create／update／keep／exclude 全部变更与 `plan_sha256`。preview 是只读 POST，不加 `idempotency_key`，不预留资源。
5. 当前批准者向 adopt 提交同一版本／修订／摘要、审阅后的 `plan_sha256` 及稳定操作标识。采用是独立明确写入，正式任务 ID 由服务端返回；采用不会自动运行开发或完成验收。
6. 每次成功后重新读取当前对象，再准备下一次 CAS；历史成功回执不等于当前版本。退役不会自动停止旧运行。

Read current sources and versions, save a complete draft, then have a currently authorized browser approver review and explicitly publish it. For requirement designs, preview the published revision, review every proposed task change and the full `plan_sha256`, then explicitly adopt that same plan. Saving, publishing, previewing, adopting and starting a run are distinct actions. Reread current state before the next CAS; historical receipts do not establish the next version.

以下是请求正文形状示例。项目由全局参数提供；实际操作使用当前读取到的版本／摘要。跨版本任务 key 应继续使用同一逻辑身份。

Example request bodies follow. Supply the real current versions/hashes and preserve task keys across revisions.

```json
{"kind":"memory","title":"项目约定","summary":"审阅说明","content":"用户原文","sources":[],"expected_version":0,"idempotency_key":"knowledge-create-v1"}
```

```json
{"action":"publish","expected_version":1,"idempotency_key":"knowledge-publish-v1"}
```

```json
{"expected_version":0,"expected_requirement_version":1,"content":{"product":"产品设计","experience":"体验设计","technical":"技术设计"},"tasks":[{"key":"core-task","title":"实施核心能力","description":"明确范围","priority":"medium","owner_id":"","depends_on":[],"acceptance_criteria":["通过当前需求验收"],"covers":[1]}],"idempotency_key":"design-save-v1"}
```

```json
{"action":"publish","expected_version":1,"expected_requirement_version":1,"idempotency_key":"design-publish-v1"}
```

```json
{"expected_version":2,"revision":2,"sha256":"ACTUAL_PUBLISHED_SHA256"}
```

```json
{"expected_version":2,"revision":2,"sha256":"ACTUAL_PUBLISHED_SHA256","plan_sha256":"ACTUAL_REVIEWED_PLAN_SHA256","idempotency_key":"design-adopt-v1"}
```

写入文件已有 `idempotency_key` 时原样保留；缺失时沿用现有 CLI 算法，从控制面地址、项目、方法、路径和正文生成固定内容标识。语言和文件／标准输入的选择不改变标识。建议为需要人工恢复的操作明确保存 key 和原始请求。

Existing `idempotency_key` values are retained. When absent, the existing CLI algorithm derives a deterministic key from the control origin, project, method, path and body. Display language and file-versus-stdin do not change it. Preserve an explicit key and the original request for operations requiring manual recovery.

传输中断的写入结果可能未知；保留原调用者、原页面身份、路径、方法、正文和 key，先核对服务端当前状态。不得遇到401／403／409后自动换身份、更新版本或重发新 key；重新登录或重新签发是独立明确流程。preview 超时只表示读取失败，不标成写入结果未知；其他未知 POST 路径仍按写操作保守处理。CLI 无自动重试、自动发布或自动采用。

An interrupted write may have an unknown outcome. Retain the original caller, captured view, path, method, body and key, and reconcile current server state. A 401/403/409 does not authorize replacing identity, versions or keys. Reauthentication/reissuance is a separate explicit workflow. Preview transport failure is a failed read; unrecognized POST routes retain mutation recovery warnings. No automatic retries, publication or adoption occur.

## 验证边界 / Verification boundary

定向 HTTP 消费者测试覆盖路径、项目双绑定、原页面头、语言、稳定 key、文件／有界标准输入、精确数值、当前API拒绝及只读POST分类；共享 CLI 组复核现有命令。测试使用进程内 HTTP 夹具，不访问主库或启动平台服务；不代表真实重新登录／重签发、厂商模型、发布部署或完整平台通过。原始日志留项目 `.cache/e2e/context-stage-017/`，阶段证据由集成报告登记。

Focused HTTP consumer tests cover routes, project binding, original-view headers, language, stable keys, bounded inputs, exact numbers, server authority and read-only POST classification. They do not claim service migration, real credential reissuance, vendor model execution or production acceptance.
