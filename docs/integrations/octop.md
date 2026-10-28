# Octop 接入与项目协作操作指南

状态：suma 1.0.0／T-MODEL-118 当前操作说明。责任：平台集成、应用工具、模型网关与工作台。项目 MCP 与受控 Harness 开发执行是两个独立方向，权限和凭据不能互换。MCP 原阶段证据见[项目接入报告](../quality/octop-integration-verification.md)，当前共同模型网关及 ACP 执行的真实范围见[本轮报告](../quality/model-gateway-verification.md)；本文不预先宣布验证通过。

## 接入方向与可用范围

项目接入方向使用 **Octop 作为 MCP 客户端，速码作为项目 MCP 服务端**。复用官方 Go MCP SDK v1.8.0 的 Streamable HTTP 服务、现有项目身份、领域事务及唯一 Go DAG；在同一开发工具接入面板选择 Codex 或 Octop，查看对应的配置片段。

Octop 可以用当前有效的项目 API 凭证读取任务交付与分页用例证据，并复用已有草稿工具。这里没有新增批准、派发、测试结果写入或部署工具。Agent 凭证仍只开放原四项读取，不因为客户端叫 Octop 而扩大权限。

**开发执行入口**另见 [Octop Harness / ACP 架构](../architecture/octop-execution.md)、[统一模型网关](../architecture/model-gateway.md)与[执行合同](../../contracts/octop-harness-acp.md)。受控 Harness SDK 接入独立工作树、Go 六工具、检查、封存及同 SHA 独立测试，原生编码与 Octop 共用模型选择、Binding、Rate 和预算。Go 独占真实供应商密钥、HTTP 与模型请求账本；它不启动完整 Octop CLI／控制台，也不继承个人专家、记忆、办公或渠道。

### 受控开发执行 SOP

1. **准备可信运行包。** 在管理员保护的 Python 环境中使用 [runtime 锁定依赖与构建说明](../../packages/octop-runtime/README.md)生成 `.pyz` 及同目录 manifest。核对解释器、包和 manifest 的真实文件；版本清单不等于第三方全部字节、签名或操作系统沙箱证明，跨平台须分别验证。
2. **配置运行环境。** 在执行工具接入向导选择 **Octop Harness / ACP**，只填写绝对 Python、`.pyz`、同目录 manifest 路径及 1–16 个固定检查 ID。复制生成的本地配置供 Runner 管理员审阅合并；不在此处填写供应商、API URL、API key 或模型参数，也不导出 env／env_from。
3. **配置模型允许列表。** 使用现有 Runner `--model-deployments` 文件，按 [统一模型合同](../../contracts/model-execution.md)提供公开部署及仅 Go 可读的完整端点、凭据引用；真实密钥留在 Runner 管理范围，不传给编码 SDK。Pool 按当前项目执行范围配置同一合同。
4. **探测并登记。** 运行只读 describe-tools，核对 Harness／ACP 版本、共同模型协议及工作区能力，再登记并明确启用准确档案版本。未安装、SDK 版本不符、入口变化或能力缺失应保持不可用，不能退回普通 command；探测成功不代表厂商认证、账单或业务成功。
5. **选择共同模型。** 在任务／专家计划选择工具，使用同一模型面板明确选择真实部署版本、请求次数、整个 attempt 的总输出额度及可选预算。上限来自当前模型选项；未定价保持未定价。输入按 context_window × max_steps 保守预留，不是精确 tokenizer 或供应商金额保证。
6. **审阅并启动原计划。** 确认唯一生产节点与计划使用相同完整 Binding、原费率、预算与工具摘要，再明确 Start。部署或计划依据变化需重新预演；已开始节点保持原定义。不把直接更换模型当作恢复原操作。
7. **执行与交付分开核对。** Go 模型网关保持真实 token SSE，终态／usage 后缀在回执持久化后释放；SDK 只持有短期 attempt loopback 凭据。代码由 Go 六工具修改、固定检查并封存；在同 SHA 上独立测试，再完成人工验收和基线交接。
8. **处理取消、失联与重启。** 分别查看取消意图、ACP 停止原因、Go 进程停止、网关关闭和请求状态。相同 completed step／同正文仅可返原缓存；未知或缺缓存不得重发供应商请求。重启保留已知 usage 与未解 hold，不再次发 prompt、不复用个人 session，也不将取消记零。
9. **查看真实账务。** 模型事实卡仅说明原绑定、请求与用量覆盖，不能代替账本 settled／hold。沿既有用量页核对结算和剩余冻结。代码失败可能已产生费用，模型成功也可能仍缺完整用量；依实际事实分别处置。

运行时配置形状如下，路径仅为需管理员替换的示例；模型部署另行管理：

```json
{
  "octop-harness-develop": {
    "adapter": "octop",
    "protocol": "octop-harness-acp-v1",
    "executable": "C:/managed-python/python.exe",
    "args": ["D:/tools/suma/octop-runtime.pyz"],
    "input": "octop-acp-jsonl",
    "read_only": false,
    "model_execution_protocol": "suma-model-execution-v1",
    "native_workspace": {
      "checks": ["formal-check"],
      "runtime_manifest": "D:/tools/suma/octop-runtime.manifest.json"
    }
  }
}
```

只读规划／知识／分诊等 purpose、OpenCode 固定模型和 Codex app-server 保持其独立合同，本向导不声称所有开发工具已经共享模型网关。完整厂商账户、物理多机、安装与生产验收仍按当前报告分别记录。

| 应用工具 | MCP 工具名 | 输入及用途 |
| --- | --- | --- |
| `project.task_delivery` | `suma_project_task_delivery` | `{task_id}`：读取当前任务、确切运行、测试节点摘要、正式验收事实、团队及受保护问题关联 |
| `project.quality_execution` | `suma_project_quality_execution` | `{task_id,run_id,limit?,after_case?}`：读取已持久化运行的固定计划及逐例质量事实；limit省略为10，显式值为1–20 |

两项新增工具只给具有当前项目读取权限的真实 API 身份。项目和组织来自端点与认证上下文，参数中不接受另一个 `project_id`。完整可用目录以当前连接的 `tools/list` 为准；目录声明、签发凭证、复制配置、连通探测和真实工具执行分别记录。

## 一次接通：管理员与使用者 SOP

1. **选定项目与账户。** 在速码切到目标组织和项目，打开现有项目令牌／开发工具接入区域，选择 Octop。核对项目端点及模板中的原用户、组织标识；切换账户或组织后重新获取片段。这两个标识是误用保护，不是授权。
2. **签发专用凭证。** 由有权管理员为该用途签发独立、可撤销、有到期时间的项目 API 凭证。读取仍受当前角色约束；复用草稿写工具还需要当前项目写权限。需要读交付／逐例质量时不能用只提供原四读的 Agent 凭证替代。不要复用 Runner 凭证、浏览器 Cookie 或个人登录密码。
3. **核对实际访问地址。** 模板端点为 `/api/v1/mcp/projects/{project_id}`，源自可信服务公开地址。远程部署使用 HTTPS；明确同主机时可用回环 HTTP。Octop 在容器或另一台机器运行时，`127.0.0.1` 指它自己，必须使用双方真实可达且经管理员确认的地址，不能通过修改不可信 Host 猜测入口。
4. **复制无秘密片段。** 速码不自动连接 Octop、不替用户 PUT 外部配置、不安装 Octop，也不把新签发秘密自动拼入片段、URL、浏览器存储或剪贴板。按以下结构在 Octop 的私有凭据编辑器中填入真实令牌；不要把令牌发给模型、贴进聊天、需求、Git、截图或工单。
5. **合并现有配置。** 先读取或查看 Octop 该用户已有的自定义 MCP 服务器，保留全部其他 server，仅添加／更新此项目的 `suma_<project_id>` 条目。官方保存接口会替换整份 `servers` 文档，不能用一个条目的片段覆盖现有连接器。保持 `shared:false`、`default_open:false`，再由使用者为明确场景选择该连接。
6. **先探测，再调用。** 在 Octop 的连接器界面执行其测试，或由授权操作方使用下文的 inline probe。之后实际列出工具、读取项目概览，再调用目标任务交付；有质量计划的已绑定运行再读取用例页。探测成功不能代替一次真实读取、写权限核验、模型认证或业务完成。
7. **核对正式记录。** 在速码任务、质量和问题页面打开返回的真实 ID，确认当前依据、运行和用例状态。需要创建草稿时沿既有工具提交明确内容与稳定操作标识，再回速码核对版本；发布、正式验收和生产操作继续使用各自人工门禁。

### Octop 配置形状

以下是占位示例；界面生成的片段会固定当前项目、账户、组织和语言。`Authorization` 的值是 **HTTP 字面值**，不使用未经 Octop 证明的环境变量插值。替换仅发生在 Octop 的私有凭据编辑器中。

```json
{
  "servers": {
    "suma_PROJECT_ID": {
      "transport": "streamable_http",
      "url": "https://suma.example/api/v1/mcp/projects/PROJECT_ID",
      "headers": {
        "Authorization": "Bearer <PROJECT_API_TOKEN>",
        "X-Suma-Expected-User": "USER_ID",
        "X-Suma-Expected-Organization": "ORGANIZATION_ID",
        "Accept-Language": "zh-CN"
      },
      "display_name": "suma",
      "enabled": true,
      "default_open": false,
      "shared": false
    }
  }
}
```

Octop 当前[官方实现](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/src/octop/infra/connectors/custom_mcp.py#L169)接受上述 Streamable HTTP 字段；HTTP 使用 `headers`，`env` 属于 stdio 配置。其适配器会补 `Accept: application/json, text/event-stream`，并将连接交给已有 MCP 客户端。若条目已配置 OAuth token，Octop 会用它覆盖 Authorization；不要把普通速码 Bearer 凭证错误包装成 OAuth。中文和英文仅改变受控说明与诊断，URL、工具名、业务内容、标识与证据摘要保持原值。

### Octop 官方保存与探测合同

| 操作 | 合同与注意事项 |
| --- | --- |
| 查看配置 | `GET /api/connectors/custom-mcp` 返回 `{servers}`，作用域是 Octop 当前用户 |
| 保存配置 | `PUT /api/connectors/custom-mcp` 接受 `{servers:{...}}`，替换整份文档并重载相关 Agent；这是使用者在 Octop 侧的明确管理动作 |
| 保存前单条探测 | `POST /api/connectors/custom-mcp/test`，正文 `{ "server": <单条服务器配置> }`；`server` 内是上例 `suma_PROJECT_ID` 对应的值，不是整个包装 |
| 已保存条目探测 | 同一 POST 正文 `{ "name": "suma_PROJECT_ID" }` |
| 部分开关 | `PATCH /api/connectors/custom-mcp/servers/{server_name}` 仅处理 enabled／default_open／shared，不是完整新增配置接口 |

这些接口由 [Octop 官方路由](https://github.com/TencentCloud/Octop/blob/890111b7879665840414ca28ab168b05b0db0921/src/octop/api/routers/connectors.py#L644)提供。速码页面只生成供人工合并的片段，不向上述接口发送凭据或配置。Octop 将自定义配置放在加密凭据文档中，但其普通 API 投影并不隐藏所有 headers／env；具备该用户管理权限的人仍可能读取这些值。加密存储不能替代最小权限、隔离账户、到期与撤销。

## 读取、分页与质量判断

先用 `suma_project_task_delivery` 的 `task_id` 取得正式任务摘要。返回的 `run_id` 是核对用例的真实选择依据，不用模型猜测 ID。没有已绑定运行或没有计划时，明确表达范围不足，不制造“全部测试通过”。

调用示例的 `arguments`：

```json
{ "task_id": "TASK_ID", "run_id": "RUN_ID", "limit": 10 }
```

工具返回 `ApplicationQualityExecutionPage`，保留计划修订、计划／来源摘要、实际代码提交、运行状态、当前依据、全量状态计数及本页用例。需要下一页时，保持同一 `task_id`／`run_id`／limit，将响应的 `next_case` 原样作为 `after_case`；它是固定发布用例顺序中的真实 case key，不是页码、offset或模型生成的标识。非法 key 拒绝，不自动回到第一页。

必须分别解释以下事实：

- `complete` 只表示当前翻页已经读到用例末尾，不表示用例通过或业务验收。
- `state`、`basis_current` 和全量计数来自质量领域。历史失败不会因后来成功、计划退役或模型总结而消失；`source_changed`／`run_changed`／`retired` 等不可写成当前可验收。
- 每例最多返回3个证据引用。`evidence_total` 表示实际引用数，`evidence_complete:false` 表示本页省略了更多引用，不能把3条当成全部。需要完整原件时由有权限人员到速码质量页按需查看。
- 返回中没有原 XML、日志、命令、环境变量、文件路径或秘密。模型摘要可以引用正式报告 ID、代码 SHA、节点、attempt、fence与用例状态，不能猜写未返回的断言细节。
- 自动化 passed、正式 Review、业务任务完成及生产发布是不同事实。新增读取没有进行批准、测试重跑或任务派发的副作用。

## AI 协作与异常恢复 SOP

| 情况 | 处理 |
| --- | --- |
| 需要了解当前交付 | 读交付摘要→读所选确切运行的质量页→以真实 ID／版本／当前性整理结论；只带必要范围进入上下文 |
| 需要后续修复需求 | 先引用问题／失败用例的正式来源，再使用已有需求草稿工具；明确待审，不能通过总结自动发布或验收 |
| 草稿写入超时／断线 | 保存原工具名、完整正文和原 `idempotency_key`；先核对原操作，必要时以原输入重放，不能换 key 重复创建。设计保存还要保持原 expected versions |
| 读取断线 | 可重新读取同一项目／任务／运行；新响应仍须重新核验依据与分页标识，不把缓存结果当新事实 |
| 401／403／撤权 | 停止使用旧事实和秘密；由当前授权方核对凭据、到期、角色与项目，不换工具或改主体头绕过拒绝 |
| 旧依据／无效游标 | 保留当前选择并说明失效原因，重新读取原任务／计划后明确选择新的运行或第一页；不将旧页拼到另一个计划中 |
| 维护／未配置／结果未知 | 按实际状态表达；不将配置存在标为健康，不无上限重试，不自动执行安装、生产变更或高风险操作 |

凭据撤销在速码原凭据管理入口完成；随后读取也应被当前授权拒绝。清除 Octop 侧已保存条目是另一项外部管理动作，不由本向导自动执行。团队共享应另行设计每个主体与项目的权限，不复制含令牌的 server 条目来扩大访问。

## English quick start

Select the exact Suma organization and project, then choose **Octop** in the existing developer connection panel. Issue a dedicated, expiring and revocable project **API** credential with the required current permissions. The two delivery/quality reads are not exposed to the four-tool Agent scope.

Manually merge the generated `servers.suma_<project_id>` entry into the current Octop user's custom MCP document. Preserve other entries; the official PUT replaces the document. Enter the literal Bearer value only in Octop's private credential editor. Keep `shared` and `default_open` false, and use `Accept-Language: en-US` for English controls. The fixed expected-user/organization headers do not grant authority.

Probe one inline `server`, list tools and perform a real task read. Use `suma_project_quality_execution` with the exact task/run IDs, `limit` 1–20, and only the returned `next_case` as the next `after_case`. Page completeness and the three-reference evidence cap are separate from passing tests and human acceptance. Draft writes reuse original operation keys after an uncertain result. Stop on revocation; do not switch identities or tools to bypass it. A successful project MCP read does not verify ACP coding, provider identity or production acceptance; current evidence for those scopes is recorded separately.

### English controlled coding quick start

Prepare the administrator-controlled Python executable, `.pyz` bundle, adjacent manifest and fixed check IDs. The Octop runtime template contains `suma-model-execution-v1` and **no provider/model/key settings or env forwarding**. Configure actual model deployments and credential references in Runner `model_deployments`; provider secrets stay in Go.

Probe the real runtime, import its inventory and explicitly enable the exact tool revision. In task or expert preview, use the shared model panel to select the deployment revision, maximum requests, **total attempt output allowance** and budget. Input reservation is the declared context window times maximum requests; it is not exact tokenization or a vendor billing guarantee. Review the frozen binding/rate and start that original plan.

Go alone sends provider HTTP and records requests. Real SSE content remains streamed; its terminal/usage suffix waits for durable receipts. An identical completed step can return an available original cache without a second upstream request. Unknown outcomes or missing cache require reconciliation. Restart and cancellation preserve known usage and unresolved holds; ACP stop, process stop, gateway closure, ledger settlement and source acceptance stay separate.

Use the existing ledger for settlement and holds, then follow same-SHA independent tests, human acceptance and baseline handoff. Read-only purposes, OpenCode and Codex retain their specific contracts. Actual SDK, provider, multi-host, platform-installation and production evidence belongs in the [current verification report](../quality/model-gateway-verification.md).

## 相关设计与验收

[项目 MCP 架构](../architecture/octop-integration.md)、[当前 ACP 执行](../architecture/octop-execution.md)、[统一模型网关](../architecture/model-gateway.md)、[分享规划采用](shared-roadmap-adoption.md)、[质量证据架构](../architecture/quality-evidence.md)、[原 MCP 合同](../../contracts/application-tools-mcp.md)。厂商实际实例未安装、未验；源码、官方 SDK 协议调用、Octop 实际客户端和生产集成必须在[专项验收](../quality/octop-integration-verification.md)中分别记录。
