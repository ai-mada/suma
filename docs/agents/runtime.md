# Agent Runtime 与 Runner

## 可执行边界

`packages/agent-runtime` 是独立 TypeScript 内核；`cmd/suma-runner` 是 Go 执行器。控制服务负责身份、项目授权、DAG 调度和租约；Runner 只通过出站 HTTP 领取任务、续约和回传结果。Runner 不导入 DAG 实现。

内核通过 OpenAI-compatible `chat/completions` 接口调用**用户配置的**模型。没有配置模型时输出 `missing_model`，不会生成演示回答或把任务标成成功。当前原生工具为控制服务授权的项目查询工具；原生内核没有本地文件编辑、终端或任意 HTTP 工具。需要执行代码任务时，使用 Runner 本机允许的 CLI 命令。

## 原生内核

构建：`pnpm --filter @suma/agent-runtime build`。

配置仅从进程环境读取：

| 变量 | 用途 |
| --- | --- |
| `SUMA_MODEL` | 显式模型名，必填 |
| `SUMA_MODEL_BASE_URL` | 兼容接口基址，例如 `https://provider.example/v1`，必填 |
| `SUMA_MODEL_API_KEY` | 服务商密钥；本地免密服务可省略 |
| `SUMA_CONTROL_URL` | Suma 控制服务基址；省略时不提供工具 |
| `SUMA_AGENT_TOKEN` | 独立 agent scope、绑定当前项目的 Bearer token；不使用 Runner 凭证 |
| `SUMA_PROJECT_ID` | 工具所操作的项目 |

启动 `node packages/agent-runtime/dist/src/cli.js`，向 stdin 每行写一个请求：

```json
{"goal":"检查当前项目的阻塞任务","max_steps":8,"max_tokens":8192,"request_id":"inspection-1"}
```

stdout 为 JSONL 事件：`started`、`usage`、`tool_started`、`tool_completed`、`completed`、`error`。`{"cancel":true}` 中止正在执行的请求；SIGINT/SIGTERM 也触发 AbortSignal。单进程同一时间只运行一个请求，第二个请求返回 `busy`。发生错误时 CLI 退出状态为非零。

`node packages/agent-runtime/dist/src/cli.js --version` 与 `--help` 只输出版本／协议说明，不创建模型请求或读取项目工具。Runner 的本地 Suma 协议探测使用这两个入口。

默认最多 8 次模型调用，最大 16 次；每轮最多 8 个工具调用。输出 token 默认预算 8192，上限 32768；服务商未返回 usage 时事件会标明 `usage_reported:false`，此时无法核实实际 token 消耗，仍受轮数、单次 completion token 和总时间限制。默认总超时 120 秒。单个上游响应限制 2 MiB，工具结果限制 64 KiB。所有远程地址要求 HTTPS，loopback 允许 HTTP；不跟随重定向。

工具列表从 `GET /api/v1/agent/tools?project_id=...` 取得；调用通过 `POST /api/v1/agent/tools/invoke`，携带项目、工具名、JSON 参数和幂等键。模型请求的工具必须出现在当前授权目录中。服务端仍负责参数校验和 RBAC；模型不能新增工具或指定命令执行器。

控制面名称（如`project.overview`）与模型provider名称分开：模型工具名使用确定的`tool_`加原名称SHA-256前32个十六进制字符，满足[Chat FunctionDefinition名称合同](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)。单次目录建立严格反向Map，拒绝重复canonical名称及alias冲突；模型直接返回canonical或未知alias都不会invoke。assistant历史保存provider别名，控制面请求与`tool_started/tool_completed`仍记录真实canonical名称，权限与只读工具范围不变。

## Go Runner

```powershell
go build -o .cache/bin/suma-runner.exe ./cmd/suma-runner
# 将执行节点登记返回的、绑定 worker-local-1 的 Runner 凭证放入当前进程环境 SUMA_RUNNER_TOKEN。
# 原生内核如需项目工具，另行创建 agent scope 凭证放入 SUMA_AGENT_TOKEN；两者不能互换。
.cache/bin/suma-runner.exe --control http://127.0.0.1:4310 --project PROJECT_ID --worker worker-local-1 --workspace D:/work/project --state .cache/runner-local-1 --commands commands.local.json --allow-exec
```

`commands.local.json` 由本机操作者维护，不接受模型或服务器提供的可执行路径和参数。示例：

```json
{
  "unit-tests": {"executable":"go","args":["test","./..."]},
  "native-agent": {
    "executable":"node",
    "adapter":"suma",
    "protocol":"suma-jsonl-v1",
    "read_only":true,
    "args":["D:/path/to/suma/packages/agent-runtime/dist/src/cli.js"],
    "input":"agent-jsonl",
    "env_from":["SUMA_MODEL","SUMA_MODEL_BASE_URL","SUMA_MODEL_API_KEY","SUMA_CONTROL_URL","SUMA_AGENT_TOKEN","SUMA_PROJECT_ID"]
  }
}
```

工作流 command/test 节点使用 `command:["unit-tests"]`；agent 节点使用 `command:["native-agent"]` 与 `config:{"goal":"..."}`。agent 规范的 `input` 可为 `agent-jsonl` 或 `goal`；后者向外部 CLI 的 stdin 提供纯文本目标。节点不能追加参数。`env_from` 只转发明确允许的环境变量；密钥不写入工作流和命令规范。默认仅传递 PATH、系统目录、用户目录、临时目录和语言环境。

`--allow-exec` 和允许目录同时存在才会运行。工作目录固定为本机配置并经真实路径解析的 workspace，不接受远程路径覆盖。**这是受信命令的启动控制，不是操作系统文件沙箱**；被允许的命令拥有 Runner 用户的权限。非 `read_only` 命令要求管理员明确设置 `--repository` 与不可变 `--base-commit`，每次 attempt/fence 使用独立 Git worktree；`read_only` 表示使用固定共享目录，不是操作系统强制只读。候选交接与真实代码／测试证据见[固定候选](../scheduler/candidate-handoff.md)。

本机 `--describe-tools`、执行节点登记、库存 TTL、固定工具摘要和结构化协议见[工具执行运行时](tool-execution-runtime.md)。普通项目 Runner token 无法上报库存；需要使用现有执行节点登记产生的绑定凭证。未启用执行、未安装或协议不支持的本地命令不声明 `command:<id>`，不会消耗对应任务 attempt。原生 Suma 和 Codex 子进程不会转发 `SUMA_RUNNER_TOKEN`，项目工具使用独立 `SUMA_AGENT_TOKEN`。

### 持久化与故障处理

每个 `--state` 目录只允许一个存活进程持有。领取到的 attempt 在执行前写入 `journal.json`；结果先写入 outbox，再回传控制服务，成功确认后持久化 ack。文件通过临时文件、文件 fsync、原子替换更新；Unix 同步父目录，Windows 使用 write-through MoveFileEx。已确认记录保留用于去重，当前没有自动压缩。

所有上报携带 run/node/attempt/fence/worker 和固定的 `operation_id`。网络失败和非成功 HTTP 响应保留同一结果并重试，先清空 outbox 才领取新任务。永久拒绝也保留证据并阻止新领取，需操作者检查授权、租约与协调状态；Runner 不擅自删除记录。

重启时，任何已有 inbox 而没有结果的 attempt 都转换为 `outcome_unknown:true`、`stopped:false`，进入人工协调，不会自动重跑。即使进程被终止，也无法推断先前是否已产生外部副作用。幂等节点的后续调度由控制服务决定，Runner 不自行重试命令。

心跳携带完整 attempt fence；心跳返回取消请求或出现续约错误时中止执行。超时默认 10 分钟，上限 24 小时。Windows 在挂起状态创建进程，先加入禁止脱离的 Job Object，再恢复执行；取消时终止整个 job，查询活动进程数为零后才声明 `stopped:true`。进程正常退出也关闭 job。POSIX 当前只支持进程组信号，无法证明已经脱离进程组的后代被终止，因此超时/取消返回 `outcome_unknown:true`，要求协调；尚未提供 Linux cgroup 容器隔离。

普通文本命令的 stdout/stderr 经管道逐块捕获，合计默认保留 128 KiB；已知凭据、常见授权字段及控制字符脱敏，有界进度先持久保存再通过 Runner progress 回写。结构化协议单独解析 stdout JSONL，仅选择白名单事实；stderr 作为独立有界脱敏诊断，不能伪造成功终态。会话标识是只读来源，不会自动生成 resume 命令。不要让命令主动打印未知的业务密钥。

## 已执行验证

Windows 上的 Go 测试覆盖：崩溃恢复不重跑、独占 journal、回执失败后重放同一幂等键、Bearer 认证、命令 opt-in 与允许目录、原生 agent 输入、输出截断脱敏、超时杀死子进程。TypeScript 协议测试使用本地兼容接口 fixture，覆盖无模型、未知工具拒绝和轮数/usage 边界。真实付费模型与外部 CLI 任务未执行，不能据此声明其任务结果已验证。
