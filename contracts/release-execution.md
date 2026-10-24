# 发布执行合同 v1

状态：**v1 协议已由根代理确认用于并行实施，2026-10-24；实现及验收仍分阶段进行。** 本文不能作为接口已上线、证明已存在或部署已完成的证据。首次设计任务只新增本文；后续实现由根代理另行授权分配，不运行外部目标或付费模型。

本文扩展 [基础合同](implementation-contract.md)、[治理与执行合同](governance-planning.md) 和 [工作区证据合同](../docs/integrations/execution-workspaces.md)。目标是把真实制品、构建执行、制品测试、发布授权、目标副作用及 HTTP 健康事实串联起来。业务验收、发布审批、部署完成和健康通过分别保存，互不替代。

## 1. 当前事实、选择与首轮范围

### 设计起点（P5 实现前）

- Runner 已有 attempt/fence/worker 租约、持久 journal、真实进度及结果重放、固定提交的隔离 Git 工作树和本机证据。候选交接不传送跨机器 Git 对象。
- `suma_artifacts` 当前只有元数据登记；现有 release 登记拒绝将手填状态直接写为 published/deployed。登记、`source_ref`、文件路径和人工描述都不是实际上传或执行证明。
- 开发者可发布任意合法 `external_workflow`。当前通用适配器把流程输入与管理员配置的静态插件凭证发送给提供方；编译器检查输入形状，不负责发布授权。现有 GET 依赖 POST 返回的远端 ID，POST ACK 丢失后缺少按稳定操作标识找回的完整路径。
- 图中的 approval、DAG 的普通 reconcile 成功及 `compensatable` 标记不能独立授权部署、证明远端已成功或授权回滚。

后续已实施的本机协议和验证边界见 [提供方接入说明](../docs/integrations/release-provider.md)；以上保留为设计依据，不代表新实现仍缺少同名能力。

### 决策

| 方案 | 取舍 | 本轮选择 |
|---|---|---|
| 在通用 external_workflow 输入中附加审批标记 | 开发者能构造输入，不能建立权限边界 | 拒绝 |
| 管理员在 Runner 上配置全权部署脚本 | 易启动，但难以限制环境、代数、重放和恢复 | 不作为发布提供方合同 |
| 控制面生成专用执行绑定，提供方在线验证授权，持久化操作并按环境 fencing | 有额外状态和恢复接口，可阻止任意图绕过批准 | 采用 |

首实现为 **本机静态站点提供方 `local_static_site_v1`**：真实文件制品、真实本地 HTTP 服务、真实内容摘要健康检查和明确的历史制品回滚。不依赖 Docker，不运行上传内容、安装脚本、任意命令或健康脚本；不连接外部发布目标、厂商 CLI、模型或付费服务。测试只使用独立 `.cache/e2e/release-*` fixture。

首轮不包含 S3、签名证书、可信硬件证明、供应链公证、云 CI 身份、容器/Helm、灰度流量、跨环境推广、跨机器 Git 传送、多候选合并、自动回滚或真实生产服务。现有 external_workflow v1 保持其原有行为；具有部署权限的提供方不得挂到其无授权的 v1 `/runs` 接口。

## 2. 身份、权限与不可破坏的约束

1. **当前执行身份**是服务端持久记录中的 tenant、project、run、node、attempt、fence、worker 和有效租约。客户端重复这些字段不构成身份认证。
2. 发布证明使用管理员登记的、绑定单个 worker/attester 的独立执行凭证。现有共享项目 Runner token 仍可执行旧任务，但不能被当作可信发布 attester 或部署权限。提供方识别 Runner 的传输凭证也必须绑定 worker；仅用于识别调用方，不能单独授权副作用。
3. 发布计划、部署授权和环境绑定由控制面生成并保存在专用表，不能从 `Workflow.Config`、普通 record.data、任意 `input` 或用户指定的 `source_ref` 导入。开发者可提出计划；首轮批准、启动、撤销和回滚只允许当前有权的浏览器项目 admin 或组织 owner/admin。普通 developer、reviewer 及 API/Agent/Runner token 不能批准发布。环境配置由管理员管理。
4. 批准绑定不可变计划版本和摘要、制品、证明、目标环境及其策略版本、预期 generation、服务端预分配的唯一执行 run/node。审批时重新核验实际权限和证据；图中存在 approval 不是这里的审批记录。
5. 提供方使用自己的独立服务身份向**管理员固定的控制面地址**核验 ticket。不得采用请求里的 issuer、URL、环境根目录、控制面地址或凭证。静态插件 token、离线 ticket 签名或 ticket ID 单独都不足以开始部署。
6. 制品必须实际收到、按真实字节校验并封存，构建和 package-test 必须有实际进程事实，并覆盖同一制品摘要及固定源码版本。`passed:true`、退出码手填、源提交字符串或指向报告的链接均不能产生证明。
7. 对环境的副作用在本机提供方按 generation 串行 CAS。未知结果保持环境屏障；租约到期、取消 ACK 或本地 Runner 退出不释放该屏障。
8. 迟到的旧 attempt/generation 结果只追加历史审计，不能提升当前部署、覆盖新 active 指针、转成当前健康或创建新的发布证明。回滚是新授权、新操作、新 generation。

信任边界：首轮信任管理员管理的 Runner、构建/测试命令 profile、制品服务和提供方。协议校验来自这些组件的真实观测及实际制品字节，不能证明已被攻破的管理员主机诚实；不能将其描述成第三方 CI 签名证明或可复现构建认证。

## 3. 通用线协议与持久化

- API 基础路径 `/api/v1`；JSON 使用 snake_case，复用 `{error:{code,message,details?}}`。下列接口是 v1 实施目标，具体可用状态以实现验证为准。租户只来自认证；项目对象逐一校验范围。
- `operation_id` 是 8–128 字符稳定标识。业务写入继续用 `idempotency_key` 和 `expected_version`；执行子协议用 `operation_id`。同作用域、同身份、同完整语义内容重复返回原回执；同标识不同内容返回 409。并发重复只能执行一次。
- SHA-256 字段为 64 位小写十六进制；commit 为当前合同支持的完整 Git object ID；时间为 UTC RFC3339，实际进程观测可保留纳秒格式并注明时钟来源。generation/版本为非负安全整数，generation 不允许客户端选择跳号。
- 未定义字段拒绝；必填字段不能为 null。`unknown` 必须显式表示，不能省略后按失败或成功解释。日志有界脱敏，不含 ticket 凭证、会话、环境秘密或绝对用户路径。
- 规范化以 `internal/releasecontract` 的具体函数为准：manifest/profile 使用固定类型声明字段顺序的 Go `encoding/json` compact UTF-8，manifest files 先按路径排序；发布 plan/request/observation 使用 `CanonicalDigest`，以 `UseNumber` 解码再规范 JSON 编码，将对象键序归一，保持数据库 JSON 往返稳定。整数十进制，不接受浮点计数、重复 JSON key、NaN 或非规范路径。服务端重新编码计算摘要。传输时间、重试次数、临时凭证不计入操作语义摘要。
- 服务端至少持久化：上传会话/接收文件、不可变制品 manifest、attestation、计划版本、独立审批、执行绑定、授权记录、环境 generation/屏障、提供方操作观测和回执。不得用可编辑业务 record.data 作为这些事实的唯一来源。
- 操作、证明和授权回执与对应状态/事件同事务保存。首轮不自动清理发布证据、提供方操作记录或被历史引用的制品；将来 GC 必须保留拒绝 operation ID 重用的 tombstone。上传额度按项目预检并在接收时计费，不能只信 Content-Length。

## 4. Runner 制品采集与实际上传

### 4.1 固定采集边界

管理员给许可命令增加不可由流程覆盖的 profile，例如 `site-build-v1`：固定可执行文件/参数摘要、`artifact_output:".cache/release/site"`、产物类型 `static_site_v1` 和限额；build/package_test 种类从 artifact 配置推导，不由图填入。控制面登记 profile ID、摘要、受信 attester 和策略版本。声明 profile 是配置事实；执行及上传完成前不产生能力已验证或构建成功。

Runner 仅在当前隔离 attempt 的命令和子进程全部结束后采集其固定输出目录。源码从固定、干净候选 C 开始；源码 HEAD/状态及来源 proof 单独保存。输出目录位于本次 worktree 的忽略目录，不能写用户原 checkout。缺少隔离、进程停止未确认、源码被改动、输出不完整或路径安全检查失败时，不能产生成功证明。

采集和物化只允许普通文件。拒绝绝对路径、`..`、空段、反斜线、Windows ADS/设备名、大小写折叠冲突、符号链接、junction/reparse point、硬链接和保留段 `.suma-release`。首轮路径使用共享 `ValidPath` 的便携 ASCII 子集：字母、数字、点、下划线、`@`、`+`、短横线；目录分隔符固定 `/`，总长不超过 240 字节、单段不超过 128 字节，禁止尾点。打开文件时核验文件身份和路径链；读前后大小/身份/摘要变化即拒绝，不能只做一次 realpath 后任其被替换。无法确认普通独占文件时失败，不跟随链接。

首轮默认上限：128 文件、单文件 16 MiB、总计 64 MiB、manifest 1 MiB；部署管理员可降低，不能由工作流提高。超限保留明确失败。没有 archive 解压路径，避免将 ZIP 路径或链接当作可信文件。采用 **manifest + 内容寻址 blob**：

```json
{
  "schema_version": 1,
  "kind": "static_site_v1",
  "files": [
    {"path": "index.html", "size_bytes": 128, "sha256": "<64-lowercase-hex>"},
    {"path": "assets/app.css", "size_bytes": 64, "sha256": "<64-lowercase-hex>"}
  ]
}
```

`files` 按路径字节序排列；制品 digest 是上述规范化 manifest 的摘要，包含每个真实文件摘要和长度。相同字节可去重，访问权限仍按 tenant/project 判定；知道摘要不授予读取权限。封存后 manifest 和文件不可原地覆盖。

### 4.2 上传 wire

`POST /runner/artifact-uploads`，绑定 worker 的发布执行凭证：

```json
{
  "project_id": "project-1",
  "assignment": {"run_id":"build-run-1","node_id":"build","attempt":1,"fence":1,"worker_id":"builder-1"},
  "operation_id": "artifact-build-run-1-build-1-1",
  "profile_id": "site-build-v1",
  "profile_sha256": "<registered-profile-digest>",
  "manifest": {"schema_version":1,"kind":"static_site_v1","files":[{"path":"index.html","size_bytes":128,"sha256":"<blob-digest>"}]}
}
```

```json
{"upload_id":"upload-1","state":"receiving","manifest_sha256":"<manifest-digest>","missing_blobs":["<blob-digest>"]}
```

- `PUT /runner/artifact-uploads/{upload_id}/blobs/{sha256}?project_id=...`：原始字节、精确 Content-Length、`X-Suma-Operation-ID`；服务端从上传会话恢复绑定 assignment，核验当前 worker 身份和租约，流式限额并计算真实摘要。先写专用临时文件再持久封存，失败不能留下 ready blob。完整 blob 重传内容相同返回原回执；首轮不支持分片 Range 上传。
- `GET /runner/artifact-uploads/{upload_id}?project_id=...`：返回接收状态及缺失 blob，绝不返回秘密。可供原 worker 重启后查询；租约失效时仅查询，不能接收新内容或 seal。
- `POST /runner/artifact-uploads/{upload_id}/seal`：`{project_id,assignment,operation_id}`。重新校验全部存储字节、长度、manifest 和范围，返回 `{artifact_id,manifest_sha256,state:"sealed",size_bytes,file_count}`。seal 表示字节存在且不可变，**不表示构建或测试已通过**。
- `GET /artifacts/{artifact_id}?project_id=...` 及受权 blob 下载：读取持久 manifest/字节；提供方只能读取授权计划绑定的制品，不接受用户提供的下载 URL、本机路径或 `source_ref`。

Runner 在发送前保存 upload ID、manifest/文件摘要、固定 assignment 和 operation ID；重启先 inspect 缺失内容，复用已持久 blob，不重新运行构建。最终结果仍遵守不可覆盖约束。服务端已保存的同内容 ACK 可在终态后重放；终态后新增内容、改变 seal 或为失效 attempt 创建证明均拒绝。

## 5. 构建与 package-test attestation

### 5.1 证明形成与最小 profile

构建证明必须同时满足：当前受信 worker 身份；管理员固定 build profile；真实进程退出 0 且进程树已结束；固定源码 C 与 workspace evidence；本次输出已实际采集/上传/seal；manifest 与所采集字节一致。失败或 outcome_unknown 可以保留观测，不能产生可发布的成功证明。

最小管理员 `CommandSpec` 仅增加 `ArtifactOutput string`（JSON `artifact_output`，build 输出相对目录）及 `ArtifactInput bool`（JSON `artifact_input`，package-test 消费真实包）。二者不能同时配置；`artifact_input:true` 必须同时 `test_evidence:true,read_only:false`。profile ID 为许可命令 alias；profile SHA 由共享规范化函数覆盖固定可执行文件、参数、环境引用、测试/隔离标记及 artifact 配置，控制面管理员登记相同摘要。秘密本身不纳入可读 profile 或日志，也不从图覆盖。每个 attempt 最多一个输出制品或一次 package-test。

固定 `Env` 配置值与 `EnvFrom` 名称在本机参与 profile 散列；只发送最终 SHA，实际秘密值不发送到控制面或提供方。

package-test 必须是明确 test 节点，`config.artifact_from` 明确指向本流程静态构建祖先；不是标题或任意 artifact ID。Runner 在准备工作树前用当前 assignment 解析服务端绑定，控制面首次解析时持久冻结 artifact、manifest digest、构建尝试及源提交 C。Runner 从制品服务下载并校验这些实际字节，物化到本次隔离 attempt 的专用忽略目录，通过管理员固定的 `SUMA_ARTIFACT_DIR` 环境变量提供给命令；输入目录在测试前后重新计算 manifest，变化即失败。

**package-test 工作树也必须固定 C**：Runner 仅接受解析返回的服务端绑定，显式验证本机管理员仓库已有 C object，且 adminBase 是其祖先，再以 C 建立独立工作树；缺 object 明确失败，不 fetch，不默默退回 B。`artifact_from` 不被伪装成既有 `candidate_from`；两种来源同时存在时其提交与来源约束必须一致，否则拒绝。构建可以直接使用管理员基线 C，不要求产生与 base 不同的新提交。仅测试源码、重新构建另一包或读取一个报告链接不能代替 package-test。

首轮静态 fixture 的 package-test 使用本机临时 HTTP 预览实际物化的包，以真实 GET 断言状态和内容，进程及端口由 Runner 管理并确认结束。地址、端口分配和测试 profile 来自管理员配置；流程不能传任意探测 URL。package-test 不等于部署目标健康，二者各有观测。

### 5.2 冻结输入与 Result wire

`POST /runner/artifacts/resolve` 使用发布执行凭证和 `{project_id,assignment,operation_id,source_node_id}`。`source_node_id` 必须等于固定 spec 的 `config.artifact_from`，由服务端验证真实祖先及其当前成功构建尝试，不能信任请求自报的来源。首次解析与当前 test attempt 的绑定同事务落盘；重复同内容返回原绑定，来源已失效或内容冲突拒绝。

```json
{
  "artifact_id": "artifact-1",
  "manifest_sha256": "<manifest-digest>",
  "source_commit": "<full-C>",
  "build_run_id": "build-run-1",
  "build_node_id": "build",
  "build_attempt": 1,
  "build_fence": 1,
  "manifest": {"schema_version":1,"kind":"static_site_v1","files":[{"path":"index.html","size_bytes":128,"sha256":"<blob-digest>"}]}
}
```

这就是共享 `ArtifactBinding` DTO；所有字段必填。`GET /runner/artifacts/{artifact_id}/blobs/{sha256}?project_id=...` 仅允许当前已冻结输入的受信 worker 下载相应真实字节；manifest 列表外对象、跨项目及失效新下载拒绝。提供方下载另用其绑定计划的独立权限，不能借 Runner 路由读取任意项目。

不新增 `/runner/attestations`，不新增客户端可写 passed/conclusion。Result 只增加单个可选 `artifact_evidence *ArtifactEvidence`：

```json
{
  "run_id":"build-run-1","node_id":"package-test","attempt":1,"fence":1,"worker_id":"tester-1",
  "operation_id":"result-build-run-1-package-test-1-1",
  "exit_code":0,"stopped":false,"outcome_unknown":false,"output":"<actual bounded redacted output>",
  "workspace_evidence":{"base_commit":"<full-C>","head_commit":"<full-C>","clean":true,"test_command":true,"evidence_sha256":"<actual workspace proof>","state":"completed"},
  "artifact_evidence": {
    "kind":"package_test",
    "artifact_id":"artifact-1",
    "manifest_sha256":"<manifest-digest>",
    "source_commit":"<full-C>",
    "profile_id":"site-package-test-v1",
    "profile_sha256":"<registered-profile-digest>"
  }
}
```

`ArtifactEvidence` 恰为上述六个必填字段；kind 仅 `build|package_test`。build 使用 `kind:"build"`、build profile 和本次刚封存制品。WorkspaceEvidence 示例省略了现有合同其他必需摘要；真实请求仍须完整提供，不放宽验证。实际退出码/未知/停止来自真实 Result，进程树和源码/差异来自工作区证据；记录观察时间时只声明实际已记录的精度，不新增伪造时钟事实。

控制面 `CompleteBound` 在接受最终结果的**同一调度事务**核对：当前受信 worker/token 绑定、完整成功 Result、管理员登记的 profile ID/SHA、workspace evidence、实际封存字节、源提交 C；package-test 另核对本次 test assignment 的冻结 ArtifactBinding 和实际输入校验。随后保存最终 dag_attempt/result，并直接派生 build/package-test `verified` 证明；任一步失败不得留下 verified 证明或部分成功。唯一证明身份是 run/node/attempt/fence + kind + artifact digest。同 ID 最终结果仍不可覆盖，丢 ACK 重放只返回原回执。

服务端可读证明包含稳定 `id,kind,state,attester_id,run_id,node_id,attempt,fence,worker_id,profile_id,profile_sha256,policy_version,source_commit,workspace_evidence_sha256,artifact_id,manifest_sha256,verified_at`，不是客户端自填通过记录。源码测试既有证据不自动升级为 package-test；手动上传可 stored/sealed，但没有当前成功构建尝试与上述绑定就没有 build 证明，不能进入正式 release。generic reconcile 标 succeeded 也不能生成或提升证明。

## 6. 不可变发布候选、计划、审批与唯一执行绑定

### 6.0 发布候选

独立 `POST /release-candidates` 允许当前有写权限的用户提出真实已完成的交付组合：

```json
{
  "project_id":"project-1","name":"fixture candidate C","artifact_id":"artifact-1",
  "build_attestation_id":"attestation-build-1",
  "package_test_attestation_id":"attestation-package-test-1",
  "tasks":[{"id":"task-1","expected_version":3}],
  "idempotency_key":"candidate-C-1"
}
```

服务端核对实际 seal、CompleteBound 派生的真实 build/package-test proof、同一 artifact digest/source commit、同一真实执行 run、实际当前成功尝试和仍有效的 accepted task/version/basis，生成不可变 `ReleaseCandidate`：`{id,project_id,name,artifact_id,artifact_sha256,source_commit,build_attestation_id,package_test_attestation_id,accepted_task_basis_sha256,build_proof,package_test_proof,tasks:[],created_at}`。其中 `artifact_sha256` 是实际 manifest digest，task basis 保留 task version、run、review 和摘要；提交者不能自填这些依据。候选不可 PATCH；重放不能改写其内容。创建及详情返回 `{candidate,eligibility,blockers}`，后续源任务/权限/证明发生变化时只更新当前可用性视图，保留历史内容不变。候选不是部署授权；未绑定环境时没有 generation 或票据。

### 6.1 环境和计划

管理员环境绑定保存 `environment_id,provider_id,site_id,policy_version,generation,active_generation,active_artifact_id,barrier_operation_id`。`generation` 是已预约的最高写入 fencing 代数，即使操作未激活或已失败也不回退；`active_generation` 是提供方实测的实际活跃代数，两者可能不同。删除或重建环境不能重用旧身份或重置 generation。

`POST /release-plans`：当前有权的开发者可提出 `{project_id,idempotency_key,action:"deploy"|"rollback",candidate_id,environment_id,expected_environment_generation,rollback_source_operation_id?}`。控制面重新读取实际候选、artifact、verified 证明、业务验收依据和环境策略，冻结以下计划，不接收自报摘要代替读取：

```json
{
  "id": "plan-1",
  "revision": 1,
  "sha256": "<server-calculated-plan-digest>",
  "version": 1,
  "candidate_id": "candidate-1",
  "action": "deploy",
  "environment_id": "local-preview",
  "provider_id": "local-static-1",
  "site_id": "fixture-site",
  "environment_policy_version": 1,
  "expected_generation": 17,
  "generation": 18,
  "artifact_id": "artifact-1",
  "artifact_sha256": "<manifest-digest>",
  "source_commit": "<full-C>",
  "build_attestation_id": "attestation-build-1",
  "package_test_attestation_id": "attestation-package-test-1",
  "accepted_task_basis_sha256": "<server-frozen-acceptance-basis>",
  "reserved_execution": {"run_id":"server-reserved-release-run-1","node_id":"deploy"},
  "health": {"checks":[{"path":"/","expected_status":200,"expected_file":"index.html","expected_sha256":"<actual-index-blob-digest>"}],"timeout_ms":2000,"attempts":3},
  "rollback_source_operation_id": null
}
```

示例中的占位 SHA 必须替换为实际服务端值。health 固定策略来自管理员；manifest 中对应文件实际摘要决定 expected_sha256。root/path/URL/脚本不能由提交者设置。计划改变即创建新 revision、摘要和审批，旧审批不跟随。

`POST /release-plans/{id}/approve`：`{project_id,expected_version,idempotency_key,plan_revision,plan_sha256,decision:"approved"|"rejected",reason}`，仅前述有权浏览器管理员。持久保存 actor、权限依据、plan digest 和时间，不用图 approval 的状态代替。是否必须不同审批人由环境策略固定；本机 fixture 可显式设置不要求第二人，不能推断为生产审批策略。

`expected_version` 指此 plan 的当前并发决策版本，不是候选版本或 release 旧登记版本；`plan_revision` 指不可变内容版本。首轮 revision=1，内容改变创建新 plan，不原地改 revision；批准/撤销/开始等状态推进递增 version。审批无效原因由服务端分别返回，如 `candidate_basis_changed,task_acceptance_invalidated,profile_revoked,approver_revoked,environment_policy_changed,environment_generation_changed,operation_unresolved`，不删除原审批审计。

`POST /release-plans/{id}/execute`：同样要求管理员、稳定幂等标识和精确版本。在一事务中重核审批/证据/权限与环境策略，CAS `expected_generation`，预约 `generation=G+1` 和环境屏障，创建唯一服务端管理的保留 run/node 和发布绑定。普通 `/runs`、用户图或复制这些 ID 都不能创建该绑定；未审批、旧版本、活动屏障或环境代数变化返回 409/403，不派发。

### 6.2 Claim 后短期 ticket

调度器 Claim 管理执行节点后，控制面才签发短期授权，绑定此次 attempt/fence/worker/租约。`operation_id` 由控制面在首次派发前生成并持久化；重试网络、Runner 重启、ticket 续期和远端 inspect 都使用原 operation ID。新 attempt 需要新操作和重新核对环境屏障；不能把未知旧副作用当成 not_started。

拟向 Assignment 增加服务端生成的 `release_execution *ReleaseExecution`，与可编辑 `config` 分离；其 DTO 仅含下面列出的非秘密字段：

```json
{
  "run_id":"server-reserved-release-run-1","node_id":"deploy","attempt":1,"fence":1,"worker_id":"release-worker-1",
  "type":"external_workflow",
  "release_execution": {
    "protocol":"suma.release.v1",
    "operation_id":"release-operation-1",
    "plan_id":"plan-1","plan_revision":1,"plan_sha256":"<plan-digest>",
    "environment_id":"local-preview","expected_generation":17,"generation":18,
    "provider_id":"local-static-1","artifact_id":"artifact-1","artifact_sha256":"<manifest-digest>"
  }
}
```

Runner 先保存上述非秘密执行绑定及语义摘要，再单独调用 `POST /runner/release-authorizations`，请求 DTO `ReleaseAuthorizationRequest` 为 `{project_id,assignment,operation_id}`。控制面仅对真实服务器管理的当前执行返回 DTO `ReleaseAuthorization`：

```json
{"authorization_id":"authorization-1","authorization_ticket":"<short-lived-opaque-secret>","expires_at":"2026-10-24T01:02:00Z","operation_id":"release-operation-1","request_sha256":"<server-calculated-request-digest>"}
```

**authorization_ticket 是随机、短期 opaque secret，仅在内存中领取并传递**；不得放入 Assignment、Config、journal、operation intent、日志或普通读取响应。控制面只存 ticket hash。默认有效 120 秒，策略上限 300 秒；过期不自动取消已受理操作。provider/worker 凭证也只来自进程秘密配置。`authorization_id` 可公开用于核对，知道它不授予消费权限。

同一路由再次领取或续期只服务于原 operation/current assignment：重新核验审批、证据、租约、屏障及绑定，生成新的临时 ticket，不分配新副作用。此秘密领取接口不会通过稳定回执重新暴露旧 ticket；响应丢失可再次领取并撤销旧的未消费 ticket，非秘密 operation 身份保持不变。已消费 ticket 绑定的同 operation 授权记录不被续期覆盖。审批撤销、源依据变化、失效 attempt 或环境漂移均拒绝领取。旧 worker 仍可通过非秘密 operation ID 只读 inspect。

## 7. 部署提供方 v2 wire 与在线授权

提供方端点位于管理员固定 origin，协议命名为 `suma.release.v1`，路由使用 `/v2` 以与旧通用 `/runs` 隔离。请求使用绑定 worker 的独立传输身份，不从 JSON 相信 worker 身份。

`POST /v2/deployments`：

```json
{
  "protocol":"suma.release.v1",
  "operation_id":"release-operation-1",
  "assignment":{"run_id":"server-reserved-release-run-1","node_id":"deploy","attempt":1,"fence":1,"worker_id":"release-worker-1"},
  "authorization_ticket":"<short-lived-opaque-secret>",
  "plan_id":"plan-1","plan_revision":1,"plan_sha256":"<plan-digest>",
  "environment_id":"local-preview","expected_generation":17,"generation":18,
  "artifact_id":"artifact-1","artifact_sha256":"<manifest-digest>"
}
```

提供方先落盘**不含 ticket** 的接收 intent，再以独立 provider 身份调用固定控制面 `POST /provider/deployment-authorizations/consume`。请求 DTO `ConsumeDeploymentAuthorization` 为 `{provider_id,authorization_ticket,operation_id,assignment,caller_worker_id,request_sha256}`；`caller_worker_id` 来自提供方验证过的连接身份。控制面在事务中从真实记录核对：

- provider audience、环境权限、票据有效期、未撤销审批和当前实际成员权限；
- 服务端管理的唯一 run/node，当前 worker、attempt/fence、有效租约和未请求停止；
- 精确 plan revision/hash、实际制品及两个 verified attestation、仍有效的业务验收依据和环境策略版本；
- 环境预约的 operation/generation、expected_generation 和请求语义摘要。

消费保存不含秘密的不可变授权回执与消费状态。响应 DTO `DeploymentAuthorizationGrant` 为 `{authorization_id,operation_id,provider_id,assignment,plan,request_sha256,state:"authorized"}`，其中 plan 由控制面读取；提供方不把调用者的输入升级成可信计划。同操作同绑定重复返回原回执；换 provider、worker、attempt、plan 或内容一律拒绝。不能在线核验则不开始新副作用。提供方消费成功后丢弃 ticket；仅持久保存非秘密 grant。

`POST /provider/deployment-authorizations/inspect` 使用 provider 独立身份与 `InspectDeploymentAuthorization{operation_id,assignment,request_sha256}`，只读取该 provider 精确绑定的历史已 consumed `DeploymentAuthorizationGrant`；未 consumed 返回 404，不创建/续期授权，不要求旧 lease 仍活着。提供方 consume 成功但 ACK 丢失、并在 grant 落盘前进程崩溃时，用此接口找回历史事实；begin-effect 仍重新核验当前租约和全部依据。重启检查确认未消费时，intent 以 `received + error.code:authorization_required` 等待仍 current 的 Runner 单独领取新 ticket 后重发原 operation。

已存在操作的同内容重复 POST 首先返回持久操作状态，不重新消费 ticket、不重新激活。ticket/到期时间不计入语义摘要，续期不会造成重复部署；其他绑定必须一致。不同内容复用 operation ID 返回 `idempotency_payload_conflict`。尚未授权的 intent 不能因存在记录而被当作 authorized；重启必须在线重新取得/消费有效授权，不能从磁盘恢复 secret。

受理后响应 HTTP 202；终态重放响应 200。**HTTP 2xx 只表示有这个操作记录，state 决定实际结果。** 示例状态由持久操作与 active 指针观察生成，不是回显请求：

```json
{
  "protocol":"suma.release.v1",
  "operation_id":"release-operation-1",
  "request_sha256":"<request-digest>",
  "provider_id":"local-static-1",
  "plan_id":"plan-1","plan_revision":1,"plan_sha256":"<plan-digest>",
  "environment_id":"local-preview","generation":18,
  "state":"succeeded",
  "effect":{"state":"applied","previous_active_generation":17,"active_generation":18,"active_artifact_sha256":"<manifest-digest>","observed_at":"2026-10-24T01:00:05.1234567Z"},
  "health":{"state":"passed","generation":18,"artifact_sha256":"<manifest-digest>","checks":[{"path":"/","status_code":200,"body_sha256":"<index-blob-digest>","observed_at":"2026-10-24T01:00:05.1234567Z"}]},
  "evidence_sha256":"<provider-observation-digest>",
  "error":null,
  "version":4
}
```

状态枚举：`received,authorized,staging,applying,checking_health,succeeded,failed,cancelled,reconciling`。effect 为 `not_started,applied,unknown`；health 为 `not_checked,passed,failed,unknown`。`succeeded` 必须 effect=applied 且本代 health=passed；失败但已激活必须如实保留 effect=applied，不能伪装未部署。健康失败不是停止证明。

- `GET /v2/operations/{operation_id}`：按稳定 ID inspect，无需 POST 返回的远端 ID；用受限 worker/provider-reconciler 身份，只返回其 tenant/project/environment 的操作。旧 attempt 可查询自己的历史操作，但不能借查询产生任何写入。
- `POST /v2/operations/{operation_id}/cancel`：带独立稳定取消 operation ID 和精确绑定，仅请求停止，返回 202。后续 GET 才能证明 cancelled/not_started；已激活时返回实际状态，不能自动回滚。停止后的 health 观测仍可能需要恢复。
- 提供方向固定控制面 `POST /provider/deployment-observations` 报告有版本的结构化状态与证据，身份限于自己的操作。控制面保存不可变历史并核对 provider、request、plan、generation、artifact 与 proof，不接受只写 `succeeded`。Runner 最终结果引用 provider operation/evidence；回执及 DAG 当前结果应原子衔接。generic reconcile reason 不能替代这些观测。

## 8. 本机静态站点实际副作用与 generation 屏障

提供方只有管理员指定的专属 `.cache` 根目录、固定 site ID 到回环监听地址/端口映射及自己的本地 SQLite 状态。请求不能改变根、监听 host、端口、命令或外部 URL。未知 site 和非本机目标拒绝。提供方服务端点有独立认证，展示站点只读，不接受 cookie 驱动的管理写入。

1. 从可信计划取得 artifact ID/digest，通过固定制品服务读取真实字节，全部重新计算 SHA-256。安全物化至 `<provider-root>/staging/<operation_id>`，路径/链接/限额与采集端同等严格。提供方不执行任何上传文件。
2. 内容稳定后持久保存只读 revision 目录及 manifest，且校验成功才进入 ready。根目录与状态库由提供方独占管理；发现外部修改、状态库丢失或无法确认单写者时进入 reconciling，不能重置 generation 后继续。
3. 在实际 active 切换前，调用 `POST /provider/deployment-authorizations/begin-effect`，请求 DTO `BeginDeploymentEffect` 为 `{authorization_id,operation_id,assignment,request_sha256,generation}`。同 operation 是稳定幂等作用域，重查当前 assignment/未停止、批准/证据/策略和环境屏障。响应 DTO `DeploymentEffectGrant` 为 `{effect_grant_id,operation_id,generation,state:"granted"}`，是持久的 exclusive effect grant；此后控制面即使租约过期也不能直接把环境分配给新操作。
4. 提供方用本地 SQLite 写事务，在同一事务内检查 operation、已授权 generation 及 `fence_generation`，CAS 更新 active revision、active generation 和副作用回执。静态 HTTP 请求从同一 active 指针快照选取已完整物化的只读目录。目录就绪先于指针事务；不依赖跨平台覆盖目录 rename 的原子性，不会出现半包切换。
5. 真实 HTTP health 访问管理员固定回环 origin，拒绝重定向，逐项核对 HTTP 状态、内容 hash 与 active generation/artifact 标记。保留的 `/.suma-release` 端点从同一 active 指针快照返回 generation/digest；既要核对标记，也要读取计划要求的真实内容，不能只探测一个永远 200 的进程端点。单项响应大小受实际 manifest 限制，不运行自定义脚本。
6. 持久化 health 明细和操作终态，再报告控制面。只有完整匹配的本代观测才能把发布投影为 `deployed_healthy`；已激活但健康失败为 `deployed_unhealthy`，观测缺失为 `reconciling`。普通 release PATCH 不能写这些事实。

### 不能省略的竞态规则

一次在线校验存在检查后失效窗口；**短期 ticket 本身不解决跨进程原子性**。控制面在 effect grant 发出后保持环境独占屏障，直到提供方通过 inspect 证明终态，或完成明确的 fencing 握手。授权请求超时也按可能已 grant 处理，先查询原授权回执。

需要放弃未知操作时，由有权管理员发起服务器管理的 fence 操作：控制面持久预约新的 fencing generation；提供方在环境串行写事务下完成所有可能的旧提交核对，持久推进 `fence_generation`，保证旧操作之后不能再切 active，并返回实际 active 指针及该 fence 的回执。**未收到且复核持久 fence ACK 前，不释放屏障、不启动后继部署。** 提供方不可达时宁可保持未知占用。外部文件、网络和控制面不是同一事务，不能宣称 exactly-once；本合同保证操作幂等、可观察副作用和严格串行 fencing。

环境 `generation` 消耗后不回退；未激活的代数也可能需要 fence 同步到提供方。后续计划必须以当前最高已预约 generation 为 expected_generation，而非旧 active_generation。旧 operation 的迟到 staging/health/结果可以留历史，但最后激活 CAS 必须拒绝过期 fence；旧成功不能覆盖当前部署投影。

## 9. 丢 ACK、重启、取消与恢复矩阵

Runner 在首次 POST 前持久化固定 assignment、operation ID 及用于重算相同请求摘要的不可变执行绑定；不得先发请求再补 journal。提供方也先持久记录非秘密 intent 和实际 request_sha256，再核验/下载/申请 effect grant。两端重启均先按 operation ID inspect，不重新执行任意图或换新 ID 掩盖未知结果。

| 故障点 | 必须恢复的行为 | 不能做的事 |
|---|---|---|
| 上传/seal 成功但 ACK 丢失 | 查 upload 会话并以原 ID/内容重放 | 重跑 build、把字节存在当 build passed |
| Result/证明已同事务入库但 ACK 丢失 | 重放原 Result/artifact_evidence，查询原回执 | 创建第二证明、覆盖最终结果 |
| 部署 POST 前 Runner 崩溃 | inspect 原 operation；404 仅说明当前未找到提供方记录，还需控制面授权/屏障记录核对 | 直接认为没发生并使用新 operation |
| POST 已受理、ACK 丢失 | GET 原 operation，恢复其持久状态与实际指针 | 依赖远端 ID、再次开启部署 |
| Provider 在 active 事务前崩溃 | 检查 intent、effect grant、immutable revision 和指针；未提交事务不可当 applied | 只凭目录存在判部署成功 |
| active 已提交、health/终态 ACK 丢失 | 从持久 active 和操作回执恢复，再真实观测本代 HTTP health；内容必须仍匹配 | 重新切换、伪造先前 health 时间 |
| 当前 assignment 已失效但远端仍运行 | 只读恢复旧操作，保留环境屏障；授权恢复器核对 provider 状态 | 旧 worker 新增副作用、泛用 reconcile 强制成功 |
| ticket 过期或审批撤销 | 未 consume 的过期 ticket 拒绝；已 consume 后 begin-effect 不再看 ticket 到期，但重核当前租约/审批/候选/屏障；已 grant 后撤销则请求停止/检查，保持屏障直至结果或 fencing 已知 | 假定撤销能撤回已发生的副作用 |
| cancel ACK 丢失或 Runner 已停止 | inspect 原操作，确定 not_started/cancelled/applied/unknown | 把取消请求、进程退出当远端停止 |
| 新 generation 已 fencing 后旧请求迟到 | 返回旧持久结果或 stale_generation；不再激活、不更新当前 health | 旧回滚/补偿覆盖新版本 |

已知终态失败且 effect=not_started，可以在服务端确认无副作用并完成必要 fence 同步后释放屏障；重试仍需当前有效计划/批准及新 attempt 的明确授权。effect=unknown 必须保持 reconciling，不能由业务状态覆盖。

## 10. 真实回滚合同

回滚是 `action:"rollback"` 的新不可变计划，绑定同一环境的历史成功 provider operation、确切 artifact ID/digest、原构建/package-test 证明和当前环境 generation；有权管理员重新批准、生成唯一执行绑定及 ticket。`rollback_source_operation_id` 必填且服务端核验同 tenant/project/environment。

提供方复核历史真实制品仍存在且内容一致，以新 generation 原子切 active 到该历史 revision，并进行同样的真实 HTTP 内容健康检查。回滚后 artifact 可以等于旧版本，但 generation 必须增大；不能将旧部署记录改成当前成功。没有已验证历史目标、旧制品丢失/损坏、环境已变化或旧操作仍未知时拒绝。第一版不自动回滚；DAG 补偿模板或上传内容中的 undo 命令不授予回滚权。

## 11. HTTP 错误、状态展示和兼容

| 条件 | HTTP / code | 客户端处理 |
|---|---|---|
| JSON/重复 key/边界无效 | 400 `invalid_request` | 修正内容；不自动重试写入 |
| 未认证或错误受限身份 | 401 / 403 `forbidden` | 不派发、不探测未授权对象 |
| 授权不存在/过期/受众不符 | 403 `authorization_invalid` / `authorization_expired` | 当前身份可尝试原操作续期；不能换绑定 |
| 旧 attempt、plan/审批/策略变化 | 409 `stale_assignment` / `release_basis_changed` | 刷新真实状态，保留原 journal |
| generation 或屏障冲突 | 409 `environment_generation_conflict` / `environment_reconciling` | inspect；新计划需重审 |
| 同 ID 不同语义内容 | 409 `idempotency_payload_conflict` | 保留冲突，不生成新 ID 绕过 |
| 路径、大小或实际摘要错误 | 400 `unsafe_artifact` / 413 `artifact_limit_exceeded` / 409 `artifact_digest_mismatch` | 保留失败及已收范围，不 seal |
| 已受理但未有确定结果 | 202 + operation state | GET 原 operation；不标成功/失败 |
| 提供方不可达或无法核验 | 503 `provider_unavailable` | 无新副作用；已发操作保持 unknown/屏障 |

面向用户分别展示：字节已封存、构建证明、包测试证明、计划审批、远端操作、实际 active 版本、HTTP health 和未知结果。`configured/available/verified` 分别报告：配置完整不等于连接成功，连接成功不等于经过端到端发布/回滚验证。阶段先增加新字段和路由，旧 Result、普通 Runner、普通 external_workflow 不被隐式升级；没有 v2 支持时能力为 unavailable，部署提供方拒绝协议降级。

### 工作台只读资源与恢复动作

浏览器不直接调用提供方，不持有 ticket/provider 凭证。制品和证明只读；写入口是启动已配置的受信构建/包测试，并在实际完成后登记候选，没有“填写 passed”或人工上传后自动通过的入口。

| 控制面路由 | 内容或操作 |
|---|---|
| GET `/artifacts`、`/artifacts/{id}` | 实际 manifest/字节状态、digest、大小和关联 build/package-test proof；未证明须明确显示 |
| GET `/attestations`、`/attestations/{id}` | CompleteBound 派生的不可变证明及当前可用性，不提供客户端写证明接口 |
| GET `/release-candidates`、`/release-candidates/{id}` | 不可变候选内容、当前 eligibility/blockers 和依据范围 |
| GET `/environments`、`/environments/{id}` | 管理员绑定、最高预约 generation、实际 active_generation/digest、屏障及当前健康；秘密不返回 |
| GET `/release-plans`、`/release-plans/{id}` | 不可变计划内容、revision/hash、可变 version、审批与失效原因、保留执行身份及当前 actions |
| GET `/deployments`、`/deployments/{operation_id}` | 控制面保存的精确绑定、提供方状态/观测、effect/health、历史与当前代数的关系、最近核对时间 |
| POST `/deployments/{operation_id}/reconcile` | 有权管理员用精确 expected_version 和稳定 idempotency_key 核对已认证提供方观测；不能提交 outcome/健康结论，不重新部署 |
| POST `/deployments/{operation_id}/rollback-preview` | 仅生成历史已健康目标的回滚计划输入；仍需新计划审批和执行 |

浏览器部署控制资源的精确 DTO、状态和路线以配套 [release-control.md](release-control.md) 为准：`eligibility` 为当前布尔，`blockers:[{code,message}]`，`allowed_actions` 列当前身份和状态允许动作，提交时仍重核。`/environments`、`/release-plans`、`/deployments` 首轮返回真实完整项目范围 `{items:[],total}`，不伪报分页；制品/候选列表提供 `limit,cursor,next_cursor`，按 `created_at DESC,id ASC` 稳定排序，同时间游标使用 `id > last_id`，明确 total 的授权筛选范围与实际加载覆盖。任何当前 eligibility 查询不能改写历史 proof/approval。未提供动作不能被 UI 推断为成功。

来源引用使用真实 run/node/attempt/fence、artifact/attestation/candidate/plan/operation ID；自动问题回流仅引用这些对象及实际 failure/unknown 事实，不能将某个旧 generation 的失败错误地记为当前健康。首轮未实施的列表/动作必须显示未提供或 unavailable，不做本地假状态。

## 12. 必须先写的测试与正式证据

所有测试目录、数据库、HTTP 端口、Git 仓库和提供方根均为任务自建 fixture，临时环境只在进程内设置。不得读取用户正式目录、运行外部目标或调用模型。测试数据库必须真实持久化并重开；关键竞争用两个独立连接/进程，不能仅用内存 mock 证明恢复。

| 测试组 | 必须观察的结果 |
|---|---|
| 实际制品 | Runner 真实固定构建输出→上传真实字节→服务端重算摘要→seal；错 hash/少字节/超限/manifest 冲突不能封存；跨项目和猜 digest 不能读取 |
| 路径与竞态 | traversal、绝对路径、大小写冲突、Windows ADS/设备名、symlink/junction/hardlink、采集中换文件均失败；原 checkout 与专属根外文件不变 |
| 构建证明 | 手填 passed、source_ref、非受信 worker、错误 profile、源码不匹配、exit 非 0、进程树未知、尚未 seal 均不能得到 verified；Complete 事务失败不留下半个证明 |
| package-test | 对上传后的实际包执行真实测试；换包/测试期间改包/只测源码/另一次构建的 digest/错误 source C 均拒绝；同一个函数的多个断言不能伪报多个测试用例 |
| 上传和证明恢复 | 在每个持久边界后断开 HTTP 或杀进程，原 ID inspect/replay 收敛；构建/测试命令的实际执行计数保持 1；终态后不能新增观测或篡改最终结果 |
| 开发者绕过 | 普通 developer 创建任意 external_workflow、伪造 approval/config/input/ticket/assignment、复制另一授权 ID、仅持有旧静态插件 token，都不能触发提供方写入；实际 site 和 generation 保持不变 |
| 精确授权 | ticket 的 tenant/project/provider/environment/plan/run/node/attempt/fence/worker/artifact/策略任一不符、过期、撤权、无在线核验均拒绝；批准与消费之间的权限/证据变化重新检查 |
| 并发部署 | 同 operation 相同内容并发 POST 只产生一个 active 切换；改内容 409；同环境不同计划仅一个 generation 预约；不同环境可各自运行 |
| POST ACK 丢失 | 提供方真实受理并故意丢掉响应，Runner 重开 journal 仅以 operation ID 找回，不能再次部署；没有远端 ID 仍可恢复 |
| generation 竞态 | 授权后、begin-effect 前撤销被拒绝；grant 后失联保持屏障；旧操作延迟到新 fence ACK 后不能切 active；fence ACK 丢失需 inspect，不能静默越过 |
| 进程/服务崩溃 | 覆盖 intent、授权消费、revision 就绪、effect grant、active 事务提交、health 持久化的前后崩溃；从真实文件/库/HTTP 恢复，未知不冒充失败或成功 |
| 健康与失败 | 实际 GET 固定站点，错误内容的 200、错误 generation、重定向、超时均不能通过；已激活但 health 失败显示 deployed_unhealthy，保留实际 active 事实 |
| 真实回滚 | 部署真实包 A 并健康→部署不同包 B 并健康→新授权回滚 A；HTTP 字节分别符合 A/B/A，generation 单调增加，旧 B 的迟到结果不能覆盖 A |
| 取消/通用核对 | cancel ACK 和本地停止不足以宣告 cancelled；泛用 DAG reconcile/补偿不能创建部署证明或获取回滚权；provider 失联时环境保持 reconciling |
| 审计与隔离 | HTTP/日志/journal/证据不含凭证；作用域错误不泄露对象；部署历史与当前投影分离；无外部网络或 paid-model 调用 |

正式证据必须保留：源码/配置/profile 摘要、真实本机工具版本、原始测试结果、实际上传 manifest/文件摘要、build/package-test 与 provider 观测、每次 operation/generation 的事件顺序、故障注入点、重启后复核、站点 HTTP 状态及内容摘要、原 checkout/根外文件不变的检查。截图辅助理解，不能代替上述事实。第一轮只验收该静态 fixture；未执行的 Docker、远端发布、签名、生产健康或规模测试保持未验证。

## 13. 冻结与实施交接

v1 已确认：根代理负责 `internal/releasecontract/artifact.go` 的共享 DTO/规范摘要、受信 worker/绑定 token/profile 登记、实际制品接收封存、CompleteBound 派生证明和 release-candidate；治理代理负责独立 release-plan/environment/授权/屏障与 `internal/releasecontract/deployment.go`；执行代理负责 Runner 制品协议/显式 C 工作树、provider v2 和独立 `cmd/suma-deployer`；工作台代理负责真实状态的发布工作台。新增 shared DTO 的声明顺序及规范函数是实际散列实现依据，双方不得各自复制另一份不一致的编码规则。

任何删减在线核验、实际字节、package-test、环境屏障或按 operation inspect 的调整必须重新评审。可分步上线只读资源或拒绝未实现操作；不能在中间状态暴露绕过授权的部署入口。

冻结后建议分三条实现工作流：控制面（对象/权限/事务/证明/授权/观测）、Runner（采集/上传/实际 attestation/持久恢复）、本机提供方（不可变站点/active 事务/HTTP health/回滚/fencing）。前端只消费这些真实状态，不提供可写 passed/deployed 快捷字段。先完成失败测试与真实最小闭环，再决定后续外部 CI、制品签名和目标环境集成。
