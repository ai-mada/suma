# 签名事件入口：GitHub 与 Suma

本页描述 T-P4-003 的接入合同。实现、测试与真实投递范围以专项验收记录为准；配置成功、签名通过、创建触发、运行成功、人工验收和生产部署分别记录。合同见[签名事件入口](../../contracts/automation-ingress.md)与[自动化及持久事件](../../contracts/automation-events.md)。

提供两种独立协议：`github` 接收 GitHub 原生 Webhook，`suma` 接收调用方主动签名的 Git、CI、监控事实。通用发送器示例不代表 Jenkins、Prometheus 或其他厂商已经原生接入。本页不要求创建账户、申请权限或开放公网。

## 建立入口与配置秘密

1. 使用真实组织会话进入项目。管理入口需要当前项目写入及审批权限；API、Runner、Agent 范围凭证不能创建、修改或读取入口管理信息。
2. 准备一条引用固定流程版本的自动化规则，并按既有规则权限启用。入口可以附着人工或 cron 规则；cron 规则的原定时触发仍生效。
3. 在该规则下创建入口，选择 `github` 或 `suma`、事件白名单和必要过滤器。创建后为暂停状态，并绑定当时的规则修订。
4. 使用界面安全随机生成或调用者提供的 **64 个小写十六进制字符**轮换入口秘密。双方 HMAC 使用这 64 个字符的 UTF-8 原文，不能先 hex 解码。每个入口使用独立秘密。
5. 将同一秘密配置到对应发送端的秘密存储；在 Suma 重新核对当前规则版本与修订并明确启用入口。入口地址为控制面 origin 加 `/api/v1/hooks/INGRESS_ID`，地址中没有秘密或查询参数。

秘密明文只用于当次配置，不写入文档、版本库、URL、浏览器存储或命令行参数。服务端保存 AES-256-GCM 密文，浏览器不会在刷新后再次显示原文。若保存结果未知，先用原操作核对元数据及 fingerprint；丢失秘密时读取最新版本，再明确轮换和配置发送端。

轮换会立即使旧秘密失效并暂停入口，没有旧密钥重叠期。先规划发送端切换窗口；新秘密保存后配置发送端，再明确启用。轮换、暂停和撤销会阻止该入口尚未派发的触发，不能取消已经派发的运行。规则修订后，入口必须通过修订操作明确绑定当前规则，再启用；旧投递不会因重新启用而补跑。

## GitHub 原生 Webhook

适用首轮范围是仓库 Webhook。GitHub 创建仓库 Webhook 需要仓库所有者或管理员权限；Suma 的项目审批权限不能代替该权限。仓库管理员可按[GitHub 官方创建步骤](https://docs.github.com/en/webhooks/using-webhooks/creating-webhooks#creating-a-repository-webhook)配置：

| GitHub 设置 | 配置 |
|---|---|
| Payload URL | 已部署控制面的 HTTPS origin 加入口的 `endpoint_path` |
| Content type | `application/json` |
| Secret | 该入口当前的 64 字符秘密原文 |
| Events | 仅订阅需要的 Pushes、Workflow runs；新建 Webhook 的 ping 仅用于连接检查 |
| SSL verification | 保持启用 |

公网 GitHub 不能直接访问本机 `127.0.0.1`。只有在另行授权并准备可达 HTTPS 控制面后，才执行真实账户配置和投递；本机签名请求验证不等于公网可达验收。

Suma 的 `github` 入口必须同时配置 `owner/repo` 与仓库数字 ID。可以从有权访问的[GitHub 仓库元数据 API](https://docs.github.com/en/rest/repos/repos#get-a-repository)读取 `full_name`、`id`；不得从仓库名称猜测 ID。名称比较不区分大小写，数字 ID 必须匹配。配置 ref 时使用完整的 `refs/heads/main` 等值；空 ref 表示所有分支和标签；可使用完整的 `refs/tags/v1.0.0` 精确筛选标签。若需要同时接收 push 与 CI，通常保留 conclusions 为空，或分别建立过滤器不同的入口：push 没有 CI conclusion。

| 入口事件 | 接收与保留的关键事实 | 运行边界 |
|---|---|---|
| `push` | `repository.id/full_name`、`ref`、`after`、`deleted` | 分支或标签删除不启动构建；普通标签push受相同过滤；SHA/ref只作来源证据 |
| `workflow_run.completed` | `action=completed`、仓库、`head_branch`、`head_sha`、运行 ID、`conclusion` | 其他 action 记 ignored；completed 本身不表示成功 |
| `ping` | 连接检查所需的受限上下文 | 永不创建运行 |

实际字段和事件可用性以[GitHub 官方事件合同](https://docs.github.com/en/webhooks/webhook-events-and-payloads)为准。Suma 仅提取合同白名单字段，丢弃其余正文，不保存提交消息、作者邮箱或原始请求。原生 JSON 可以包含 GitHub 的其他字段；通用 Suma JSON 则严格拒绝未知字段。

GitHub 使用 `X-Hub-Signature-256: sha256=<hex>` 对原始正文计算 HMAC-SHA256。Suma 验证原始字节后解析，拒绝缺失签名及 SHA1 降级。代理不能改写正文。[官方验签文档](https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries)提供独立测试向量；“签名正确”说明请求持有对应秘密，不授予修改流程、任意执行或业务读取权限。

`X-GitHub-Delivery`、`X-GitHub-Event` 等头不在正文 HMAC 范围内。Suma 同时持久去重投递 ID 和原始正文 SHA256，防止只更换投递头重放；保守语义为同一入口的同字节正文只触发一次，跨入口修订和重启仍保持。GitHub 原生协议不能证明尚未见过的历史有效正文的新鲜度。不能把它表述为带签名时间戳的协议。

GitHub 官方要求及时响应，其最佳实践写明 “Respond within 10 seconds”。Suma 提交持久回执及触发后返回，运行由后台派发，不等待流程完成。[GitHub 最佳实践](https://docs.github.com/en/webhooks/using-webhooks/best-practices-for-using-webhooks)。

官方明确说明：“GitHub does not automatically redeliver failed deliveries.” 管理员需观察失败，再从 Recent deliveries 或获授权的 API 明确重投；官方通常允许重投过去三天的投递。重投沿用原 `X-GitHub-Delivery`，Suma 恢复同一回执，不生成另一运行。[官方重投说明](https://docs.github.com/en/webhooks/testing-and-troubleshooting-webhooks/redelivering-webhooks)。

## Suma 通用 Git、CI、监控事件

`suma` provider 支持 `git.push`、`ci.completed`、`monitor.firing`、`monitor.resolved`。发送正文为 `AutomationIngressEvent`：

| 字段 | 约束 |
|---|---|
| `schema_version` | 固定为数字 `1` |
| `event` | 上述白名单之一，且已被入口订阅 |
| `external_id` | 必填，最多 128 个安全 ASCII 标识字符 |
| `repository` / `repository_id` | 可选，分别最多 201 字节 / 30 位数字 |
| `ref` | 可选，最多 250 字节；分支过滤使用完整 ref |
| `commit` | 可选；非空时为 40 或 64 个小写十六进制字符 |
| `conclusion` | 可选，最多 40 字节；作为外部报告值精确过滤 |

禁止控制字符、重复字段、未知字段和多个 JSON 值。配置了非空仓库、ref 或 conclusion 过滤器时，事件必须提供可匹配的值；空事件字段不能绕过该过滤器。事件没有 shell 命令、工作流规格、执行仓库地址、下载 URL 或秘密字段。

以下 JSON 是字段示例，ID 和结果均为占位示意。将 CI 示例保存到仓库忽略的 `.cache/ingress/ci-event.json`；将监控示例保存到同目录下的 `monitor-event.json`。不要将临时目录建在盘符根目录。

```json
{
  "schema_version": 1,
  "event": "ci.completed",
  "repository": "example/suma",
  "repository_id": "123456789",
  "ref": "refs/heads/main",
  "commit": "0123456789abcdef0123456789abcdef01234567",
  "conclusion": "failure",
  "external_id": "ci-job-42-attempt-1"
}
```

```json
{
  "schema_version": 1,
  "event": "monitor.firing",
  "external_id": "monitor-api-42-firing"
}
```

监控入口若不关联仓库和分支，应保持相应过滤器为空。恢复事件使用 `monitor.resolved`、该事件自己的稳定 external_id 及 delivery ID；它只是恢复报告，不表示已执行修复或回滚。通用 `git.push` 可按 CI 示例提供仓库、ref、commit，省略 conclusion，并改用对应 event/external_id。

在 CI 或监控任务的秘密存储中，将当前入口秘密注入**当前进程**的 `SUMA_WEBHOOK_SECRET`。下面的命令不包含真实秘密；`--secret-env` 指向环境变量名，CLI 只在内存读取秘密：

```powershell
suma --control http://127.0.0.1:4310 webhook deliver --ingress INGRESS_ID --delivery ci-job-42-attempt-1 --file .cache/ingress/ci-event.json --secret-env SUMA_WEBHOOK_SECRET
suma --control http://127.0.0.1:4310 webhook deliver --ingress MONITOR_INGRESS_ID --delivery monitor-api-42-firing --file .cache/ingress/monitor-event.json --secret-env SUMA_WEBHOOK_SECRET
```

每条命令运行时必须注入对应入口的独立秘密。全局 `--control` 放在子命令前；远程地址使用 HTTPS。该命令调用签名公共入口，不需要 `--project`，不发送浏览器 Cookie 或业务 Bearer。普通 `api` 命令附带业务身份，不能代替签名投递命令。使用包含本阶段 `webhook deliver` 命令的 suma 构建。

CLI 以文件原始字节签名并发送，使用当前 Unix 秒构建协议头：

```text
X-Suma-Delivery: <delivery_id>
X-Suma-Timestamp: <unix_seconds>
X-Suma-Signature-256: sha256=<HMAC-SHA256 hex>
```

签名输入为以下字节连接；`\n` 代表一个 LF，正文后不另补换行：

```text
suma-v1\n<ingress_id>\n<timestamp>\n<delivery_id>\n<raw_body>
```

服务端以数据库时间接受前后 300 秒的时间偏差。结果未知时，保留原文件及 delivery ID，重新调用同一命令；时间戳和签名可以刷新，正文必须逐字节相同，包括空格和换行。同 ID 异正文返回 409；不要通过改 ID 掩盖旧请求。`external_id` 是来源事实标识，持久防重放依据是 delivery ID。Suma 协议允许相同正文配不同有效 delivery ID 表示独立交付，因此发送端必须稳定保存该 ID；它没有 GitHub 的同正文去重语义。

## 回执、限制与核对

| HTTP 结果 | 含义与处理 |
|---|---|
| `202` | 正式提交新回执；读取其 state，不能直接解释为运行成功 |
| `200` | 重复交付恢复原回执，不创建另一触发 |
| `400` | 正文或协议格式无效；修正输入后按明确的新操作处理 |
| `401` | 缺失或无效签名，或不满足签名时间要求；核对配置，不自动降级 |
| `409` | 相同 delivery ID 绑定了不同正文；恢复原始字节并核对历史 |
| `413` | 正文超过平台 2 MiB 上限；不截断或拆成虚假事件 |
| `429` | 达到接收限额；发送方保留原 ID/正文，明确安排重试 |
| `5xx` / 超时 | 结果可能未知；先核对历史，再以原 ID/正文恢复 |

回执 state=`accepted` 表示已创建正式 occurrence；`ignored` 表示签名有效但未匹配或只是连接检查；`blocked` 表示入口、规则、权限或流程门禁不允许。ignored/blocked 也不会在后续重新启用时自动转成运行。在项目内查看交付历史、当时入口/规则修订和原因，并沿 occurrence 关联核对真实 run。无效签名不会写入可被攻击者污染的交付历史；该列表不等于完整攻击审计。

公共入口拒绝 query、Origin、Cookie、Authorization、content-encoding、非 JSON content-type 和重复协议头。每项目最多 50 个入口，每入口每数据库分钟最多 120 条新有效交付；重复回执仍可核对。并发接收有进程上限。GitHub 官方允许更大 payload，但本平台上限仍为 2 MiB；超限会明确失败。[GitHub 正文上限说明](https://docs.github.com/en/webhooks/webhook-events-and-payloads#payload-cap)。

## 部署秘密、许可与验收边界

控制面 `SUMA_INGRESS_MASTER_KEY` 是规范 base64 编码的 32 字节 AES 主密钥，与每个发送端的 64 字符 HMAC 秘密分开。SQLite 未提供环境主密钥时在既有 dataDir 持久生成私有密钥文件；OceanBase/PostgreSQL 要求各控制实例明确配置同一共享主密钥。备份数据库时单独安全备份主密钥；主密钥丢失不能恢复来源秘密，不能通过重启静默换钥。秘密文件和环境配置应仅授予实际控制进程及管理员访问权限。

GitHub 账户/仓库权限、发送端 CI/监控任务的配置权限、Suma 的项目审批权限分别授权。HMAC 秘密只许可向固定入口提交受限事实；签名事件不能读取业务、修改工作流、注入命令、改变实际执行代码版本、越过审批或制造测试证明。工作流仍执行先前批准的固定版本，Git SHA/ref 仅保存在来源证据中；外部 CI 的 success 不会成为 Suma 的真实测试证明，监控恢复也不会成为部署健康证明。

本机签名向量、SQLite 竞争/重启、CLI HTTP 投递及浏览器交付下钻只能证明实际执行到的范围。GitHub 公网投递与真实账户权限、发送端持久重试、生产 CI 身份/证明、OceanBase/PostgreSQL 集群和目标吞吐须独立验收。本次接入文档及示例没有授权创建账户、发送对外消息、开放公网、部署生产或订阅付费服务。
