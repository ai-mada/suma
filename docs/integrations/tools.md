# 工具与外部 Agent 接入

## 授权工具目录

原生内核只使用控制服务通过 `/api/v1/agent/tools` 返回的工具。当前目录提供项目概览、项目任务和项目搜索；服务端依据 Bearer 身份和 `project_id` 授权，工具调用走 `/api/v1/agent/tools/invoke`。项目数据是工具输出，不作为内核系统指令。

工具接口统一返回 JSON，模型不能提供新的执行地址。工具集没有通用 shell、文件系统、浏览器、MCP 动态服务器连接或随意 HTTP 访问。增加能力时应先在控制服务实现授权与输入校验，再注册 schema；不得依据模型传入的字符串动态 dispatch 任意函数。

## CLI 能力探测

`pnpm --filter @suma/agent-runtime build` 后运行 `pnpm --filter @suma/agent-runtime catalog`。该操作只在 PATH 查找候选程序，运行 `--version` 和对应的只读帮助命令；每次最多 5 秒、32 KiB 输出。Windows 优先查找原生 `.exe`；对 `.cmd/.bat` 包装器不自动启用 shell。目录中的 `installed`、`operations`、`verified` 分开表示安装存在、帮助声明的操作和实际完成的只读检查。`execution_enabled:false` 明确表示目录探测没有执行任务。

2026-10-23 本机探测结果：

| 适配器 | 安装状态 | 实际检查 | 可声明的能力 |
| --- | --- | --- | --- |
| Codex | 已安装，`codex-cli 0.163.0-alpha.11.2` | `--version` 与 `exec --help` 成功 | 帮助包含 `--json`、`--sandbox`，可配置 JSONL exec 与 read-only sandbox |
| OpenCode | PATH 未发现 | 未运行 | 无 |
| Hermes | 存在 `hermes.exe` | `--version` 退出 1 | 安装存在，兼容性未验证 |
| dsh | PATH 未发现 | 未运行 | 无；不猜测未知 CLI 的参数或协议 |

这些结果只是本次机器探测，其他机器必须重新探测。没有调用 Codex 模型或运行 CLI 编码任务。Hermes/OpenCode/dsh 未实现经过验证的 headless 任务协议；不能将安装存在描述成可用执行能力。

## 通过 Runner 使用已经核实的 CLI

Runner 的本机命令目录可以显式加入外部工具。以下 Codex 规范使用已读取帮助确认的参数：

```json
{
  "codex-inspect": {
    "executable":"codex",
    "read_only":true,
    "args":["exec","--json","--sandbox","read-only","--ephemeral","-"],
    "input":"goal"
  }
}
```

agent 节点填写 `command:["codex-inspect"]` 与 `config.goal`，Runner 从 stdin 传入目标，不通过 shell 拼接目标。CLI 自己使用本机已有认证；Suma 不读取、迁移或记录外部 CLI 的登录文件。请为生产 Runner 建立独立用户与工作区，确定其已有认证和插件权限。

JSONL 输出目前作为有界、脱敏的节点输出保存，没有将外部 CLI 的事件假定为 Suma 的执行状态。进程退出码决定 Runner 成功/失败；真实任务的业务验收仍需测试证据和评审。Codex worktree、会话恢复、OpenCode/Hermes/dsh 远程会话和第三方 MCP 工具属于后续适配范围，不宣称已经互通。

## 外部工作流 HTTP 桥接

Runner 支持独立的 `external_workflow` 节点。这是 Suma 与管理员提供的工作流桥接服务之间的合同；不是 Temporal 原生 REST API 适配器。Temporal、Camunda 等引擎需要由桥接服务自行使用官方 SDK 接入。没有真实连接这些引擎，当前验证仅覆盖本地 HTTP 合同 fixture。

管理员显式提供 `--external-config external.local.json`：

```json
{"temporal-bridge":{"url":"https://bridge.example/suma","token_env":"SUMA_TEMPORAL_BRIDGE_TOKEN"}}
```

token 从命名环境变量读取；远程地址必须 HTTPS，本机 loopback 可用 HTTP。任务不能提供/覆盖 endpoint，也不能选择 token 变量。未配置插件或未提供 token 时不会 dispatch，Runner 仅声明已配置插件的 `external_workflow:<plugin>` 能力。此 HTTP 能力不要求启用本地 shell 的 `--allow-exec`，管理员配置桥接文件即是显式启用。

节点 `config`：`{"plugin":"temporal-bridge","workflow":"release-flow","input":{"release":"v1"}}`，`command` 必须为空。桥接协议：

| 请求 | 合同 |
| --- | --- |
| `POST /runs` | 请求 `{dispatch_id,workflow,input}`；响应 `{id,state,output?,error?}` |
| `GET /runs/{id}` | 响应同上；state 为 pending/queued/running/succeeded/failed/cancelled |
| `POST /runs/{id}/cancel` | 请求 `{dispatch_id}`，成功 HTTP 状态只代表收到取消请求 |

`dispatch_id` 是 run/node/attempt/fence 的稳定 SHA-256；桥接端必须持久化该幂等键与远程执行 ID 的对应关系。同一个 key 不能产生两次远程执行。Runner 在重启后仍按未知结果协调，不自动重发未完成的任务。

Runner 轮询到 `succeeded` 才成功，`failed` 才失败。超时/取消发送 cancel 后继续查询，仅观察到远程 `cancelled` 才返回 `stopped:true`；确认超时、网络失败、非法状态都返回 `outcome_unknown:true`，要求人工核对远程执行。不会把 HTTP 202 或断开连接当成已经停止。
