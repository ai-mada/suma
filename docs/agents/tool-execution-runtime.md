# 工具执行运行时与本机探测

状态：T-P4-004 Runtime源码与域内回归完成；平台实际旅程与最终交付由主集成验收。负责人：Runtime 代理与主集成。合同：[工具执行](../../contracts/tool-execution.md)。源码：`internal/runner/tools.go`、`tool_inventory.go`、`tool_stream.go`、`packages/agent-runtime/src/cli.ts`。

## 配置与只读探测

管理员维护本机命令 allowlist。未填写 `adapter`／`protocol` 的命令归一化为 `command`／`text-v1`；通用命令不猜测 `--version` 参数，版本标记为 `local-command`，只核对原生可执行文件存在、字节摘要及显式执行许可。该标记不表示执行成功。Suma 使用 `suma-jsonl-v1`，Codex 使用 `codex-jsonl-v1`。OpenCode 当前固定标记为不可用：所研究 CLI 没有可证明整个运行完成的 JSONL 终态，`step_finish` 不能当作整体完成。

```powershell
go run ./cmd/suma-runner --describe-tools --commands commands.local.json --allow-exec
```

该模式无需平台 token，不启动 Runner 轮询、模型或任务，输出 `{items,execution_enabled}`。其中每项是非秘密 Observation，可粘贴到平台的工具配置界面；不输出本机可执行路径、参数、环境值或登录文件。可用只表示本地安装与受限协议探测，模型认证、计费、远端健康和真实任务执行分别验证。超过128条、非法命令名／协议、重复键与未知配置字段均明确失败，不截断或静默降级。allowlist在启动读取，文件调整后需重启并按新观察核对工具登记。

Suma 示例：

```json
{"native-agent":{"executable":"node","adapter":"suma","protocol":"suma-jsonl-v1","args":["D:/path/to/suma/packages/agent-runtime/dist/src/cli.js"],"input":"agent-jsonl","read_only":true,"env_from":["SUMA_MODEL","SUMA_MODEL_BASE_URL","SUMA_MODEL_API_KEY","SUMA_CONTROL_URL","SUMA_AGENT_TOKEN","SUMA_PROJECT_ID"]}}
```

Suma profile 要求一个本机绝对路径 JavaScript 入口及 `agent-jsonl` stdin。版本／帮助探测追加 `--version`／`--help`；原生 CLI 在这些入口不会进入模型循环。Codex profile 只接受固定stdin preset：`exec --json --sandbox read-only|workspace-write [--ephemeral] [--color never|auto|always] [--model MODEL] -`；sandbox必须与read_only一致，末尾只能有一个`-`，拒绝字面prompt、resume、cwd/add-dir/config覆盖和危险bypass。探测只调用 `--version` 和 `exec --help`，不启动模型。Windows `.cmd`／`.bat` 包装器不会被自动作为原生程序执行。每次 probe 最多 5 秒、32 KiB 输出，同批最多 4 个并发；非只读命令另只读核对管理员Git根、基准和缓存隔离，不创建工作树或目录。

`profile_sha256` 来自共享 `ProfileDigest`，绑定归一化 CommandSpec（含私有配置值但只输出摘要）与实际可执行文件 SHA-256。Suma 另把唯一绝对路径 JavaScript 入口的字节摘要与 CommandSpec 一起放入私有摘要输入，入口上限16 MiB；同路径／同版本脚本替换也会改变profile。JSON 对象键确定排序，参数数组顺序保持。它不等于签名验证，也不覆盖入口导入的全部模块、解释器依赖等传递供应链。启动前再次计算配置、入口及可执行摘要，与当前库存和 assignment 固定摘要核对；漂移、未安装或不匹配时不启动命令。

## 库存、身份与能力

Runner 必须使用现有执行节点登记产生的 worker 绑定凭证。普通 runner scope 项目 token 不自动获得设备登记身份。启动先恢复 `tool-inventory.json` 未确认的原 operation，服务端重复回执保持原数据库时间；随后创建新的操作报告当前观察。新的报告每 30 秒刷新，平台按数据库时间 60 秒过期。原操作重放不能延长在线 TTL。

报告先经临时文件、fsync 与原子替换持久化再发送 `/runner/tool-inventory`；只有服务端明确返回 `acknowledged:true` 和有效观察时刻才记录 ACK。启动时永久授权错误明确退出；运行中库存请求失败停止新领取，以原operation重传并在恢复后创建新的TTL报告，不因一次库存503而杀死健康进程。当前进程仍由实际心跳／租约／撤权判断是否停止，既有结果outbox仍可上传。库存报告无法获得执行权限，派发仍由控制面检查当前项目授权、工具启用者和固定 profile。

仅可用本机条目声明 `command:<command_id>`；配置了但未安装、禁用执行和不支持协议不会声明。DAG 的 command／agent／test 各自还要满足完整的命令及显式额外 capability。任务使用固定工具 profile，机器执行者与业务任务负责人分别记录。

## 结构化输出与结果

stdout 按 64 KiB 单行、32 层／32768 token 的 JSON 结构上限解析，支持 UTF-8 跨写入分片及多行；拒绝重复键、非法 JSON、无效 ID、超限与冲突终态。只保存共享合同白名单事实，已知秘密和常见凭据先脱敏，原始供应商 JSON 不进入日志重放。工具原始参数、完整命令、任意 payload 和推理内容不进入结构化事实。stderr 独立有界脱敏，不能提供 stdout 协议终态。

| 原协议 | 平台事实 |
|---|---|
| Codex thread.started／turn.started | 会话来源、轮次开始 |
| Codex item.started／updated／completed | 受限活动种类／状态与 ID；不保存原命令、参数或 reasoning |
| Codex agent_message | 完成后的公开消息文本 |
| Codex turn.completed | 一致整体终态与官方 usage 数值；不估算费用 |
| Codex turn.failed／顶层 error | 整体失败；item.error 和 command failed 不擅自推断整个 turn 失败 |
| Suma started／request_started／tool_started／tool_completed／usage／completed／error | 同一受限事件合同；未返回 usage 的模型不伪造用量 |

成功要求进程退出 0 且只有一个受支持的一致成功终态。Codex必须按thread.started→turn.started→terminal顺序；Suma started及后续事实必须匹配本次stdin request_id（assignment的稳定SHA256），缺失／外来ID、重复开始和终态后活动均拒绝。Codex归一化要求input_tokens、cached_input_tokens、output_tokens三个基础计数显式存在；reasoning_output_tokens、cache_write_input_tokens作为可选拆分，缺失时wire不输出，不当作已报告零。所有存在计数必须为0..9007199254740991的安全整数，明确0与缺失分开保存，空usage、缺基础字段、错误类型与溢出均拒绝。Native逐请求记录usage_state，部分小计不称整轮完整用量；详见[本阶段用量与领取恢复](scheduling-usage.md)。模型声称成功不替代进程、固定候选、测试证据和人工验收。缺终态、格式错误、冲突或进程／控制结果不确定保留 unknown；取消仍以实际进程树停止回执为准。`waiting_input` 只表示进程仍在等待的观察，保留执行槽位，不提供未经协议支持的回复或恢复按钮。

这是针对已核实官方协议形状的兼容边界：[rust-v0.100.0的Usage Serialize定义](https://github.com/openai/codex/blob/rust-v0.100.0/codex-rs/exec/src/exec_events.rs#L54-L63)只含三个基础计数；[锁定970111f6定义](https://github.com/openai/codex/blob/970111f6cb62cc02fe36a82ea7d71b37a2889c97/codex-rs/exec/src/exec_events.rs#L54-L68)含五个计数，且其reasoning字段为必需、cache_write带反序列化默认。平台接受上述两种形状，不从缺失拆分推断失败或生成默认用量。T-P4-004历史wire的omitempty曾省略显式零拆分；T-P4-005已改指针保留明确零，不覆盖历史证据或推断旧缺失字段。本机0.164.0-alpha.11.1只做version/help探测，没有观察其真实模型任务输出，不能声称该安装版本只输出三个计数。

typed progress 与原文本 progress 共用 Journal、稳定 ordinal operation ID 和 ACK 重放，绑定 run/node/attempt/fence/worker，最终结果另保存 ToolEvidence。会话 ID 仅供人工定位，不读取外部登录文件、不自动重放／恢复不确定副作用。远端会话恢复、外部桥接重启查询、完整终端／输入中介仍在后续范围。

## 验证边界

新增测试记录了缺失功能、原生凭证混用、入口字节漂移、协议身份／顺序、非法preset与缺失用量计数的真实失败，再验证修复。初次域内 `go test ./internal/runner -json -count=1` 为111个通过事件（含子测试和helper），0失败、0跳过，61.973秒；`go vet ./internal/runner ./cmd/suma-runner`退出0。39份Runtime源文件当时前后摘要一致。Native CLI5项测试及typecheck通过。库存503／恢复测试使用实际200ms子进程，保持心跳与正常结果后才恢复新领取；非法配置和129条配置明确拒绝。

另覆盖配置及入口同路径／同版本漂移、能力选择、JSONL分片与安全字段、缺失／冲突终态、真实模型无关fixture进程、stderr不能伪造成功、取消、typed Journal和库存丢ACK恢复。Native CLI使用本地HTTP fixture核对独立agent凭证及只读help/version，不把这些测试称为已连接真实厂商模型。fixture自报版本明确为`fixture-transport-only`；真实厂商CLI只读探测、平台HTTP权限和实际界面旅程分别记录在主验收报告。

其后TS原生内核补充provider-safe工具名称：确定hash别名、严格反向映射与重复目录拒绝，实际HTTP fixture从3项失败修复到全套8项通过，typecheck/build及CLI只读version/help均退出0。原111个Go测试与39文件收据保留当时切面，TS两文件变化单独留补充摘要；不把旧原生5项称为新别名源码的测试结果。追加Codex三计数兼容修复后，三个Go文件另有新切面：协议解析器与测试发生变化，目标回归从12通过／4失败到16通过／0失败，包含真实本机fixture进程。原111项与旧源码摘要不覆盖该变更；[补充验证](../quality/tool-runtime-verification.md)及主集成后续全套分别记录。
