# 独立资源池 Runner 与多项目执行

T-COLLABORATIVE-TEST-BINDINGS-223：[协作测试配置与工具自动核对](../product/collaborative-test-bindings.md)／[架构与AI SOP](../architecture/collaborative-test-bindings.md)／[交互](../design/collaborative-test-bindings.md)／[限定验证](../quality/collaborative-test-bindings-verification.md)。讨论、推荐采用及团队计划保留准确独测配置；工具按原live开关可见到期自动核对，后台刷新保留原请求，失败明确重试。真实同提交开发／独测和中英文代表组件分别核验；完整31项／P0–P7继续。

T-ACCEPTED-TASK-BASELINE-222：[已验收任务交给后续开发](../product/accepted-task-baseline.md)／[架构与AI SOP](../architecture/accepted-task-baseline.md)／[交互](../design/accepted-task-baseline.md)／[限定验证](../quality/accepted-task-baseline-verification.md)。普通智能体候选经准确独测与Review后，可明确采用为未开始任务的基线，沿原broker和独立工作树继续开发；不继承旧验收、不自动开始。完整31项／P0–P7继续。

T-QODER-TOOL-PREPARATION-221：[Qoder配置准备与AI SOP](../product/qoder-tool-preparation.md)／[交互设计](../design/qoder-tool-preparation.md)／[限定验证](../quality/qoder-tool-preparation-verification.md)。明确模型和本机信息沿原ACP准备器创建新文件，再采用准确普通字段或共享本机引用；手工JSON按需展开，来源切换可靠隔离。准备不运行Qoder或模型，真实发现／登记／启用分别核验。完整31项／P0–P7继续。

T-LOCAL-TOOL-PREPARATION-220：[通用CLI准备与AI SOP](../product/local-tool-preparation.md)／[交互设计](../design/local-tool-preparation.md)／[限定验证](../quality/local-tool-preparation-verification.md)。四种通用CLI使用Shell与全新文件两字段，复用原发现／准备命令；手工JSON按需展开，明确采用路径接续普通字段或共享引用。复制、被动探测、文件准备、登记和模型执行分别记录。完整31项／P0–P7继续。

T-SHARED-TOOL-ONBOARDING-219：[共享工具接入与 AI SOP](../product/shared-tool-onboarding.md)／[交互设计](../design/shared-tool-onboarding.md)／[限定验证](../quality/shared-tool-onboarding-verification.md)。共享安装检测后的准备使用准确Pool身份，保留工具、节点和项目返回原Quick；新接入不制造旧档案。明确授权失效撤下旧表单，到期保留输入；跨租户离线配置与项目读取权分离。完整31项／P0–P7继续。

T-TEAM-WORKSTATION-RECOVERY-218：[共享节点恢复与 AI SOP](../product/team-workstation-recovery.md)／[交互设计](../design/team-workstation-recovery.md)／[限定验证](../quality/team-workstation-recovery-verification.md)。工具诊断可进入准确资源池、节点和项目配置，再返回原工具换新；核对到期保留输入，来源失效阻止迟到配置。桌面默认留下，实际关闭后才进入原收尾。中英文代表界面及原生窗口已限定验证；真实远程修复、完整31项／P0–P7继续。

T217补充[准确节点连接准备](../product/tool-recovery-navigation.md)：普通项目节点与共享Pool分别核对授权；原未知检查和写入恢复优先，Guide新报告只交回原Quick。复制、准备、登记和启用分别处理，不因打开指南授予任务或执行能力。

T216补充[开发与独立测试明确派发](../product/test-dispatch-bindings.md)：同Pool不同槽位不等于不同Worker，原计划和最终路由固定准确测试工具；每个producer需另一Worker对当前来源／attempt／fence／证据SHA的同提交测试。等待、配置可用、实际执行及人工验收分别记录，取消后的SQLite恢复见[决定](../adr/sqlite-cancellation.md)。

T-COMMAND-COMPOSITION-215补充[已有命令配置组合](../product/command-composition.md)：在目标机器离线新建完整允许清单后，再把路径用于原项目／Pool配置。组合和采用路径都不启动节点、不读系统凭据、不增加Grant或工具资格，原配对、检测、登记与启用继续独立。

状态：T-P4-006 实现与本机专项验证；最终集成、交付与实际平台旅程以阶段验收为准。负责人：Runtime／主集成。合同：[共享资源池](../../contracts/execution-pools.md)、[Core 接口](../../contracts/execution-pool-core.md)。实现：`internal/runner/pool_*.go`、`internal/platform/pool_runner_*.go`、`cmd/suma-runner/pool.go`。

## 身份与范围

T-POOL-RUNNER-PAIRING-208已补充[安全机器配对](../../contracts/pool-runner-pairing.md)：原Pool节点页创建待办，机器把token保存到独立系统凭据库，管理员核对完整指纹后批准；浏览器不复制或保存token。`--pair-pool-runner ID --control URL --pool POOL`仅准备身份并退出；后续显式启动使用`--pool-identity FILE --pool-config FILE --control URL`，先核准公开范围再读取系统秘密。准确过程和实际Windows系统库证据见[专项报告](../quality/pool-runner-pairing-verification.md)；它不代表节点在线、跨平台安装或业务执行。

资源池节点使用独立机器凭据，只绑定登记的pool／runner。推荐从配对的系统身份读取，也可在原手工模式显式提供`SUMA_POOL_RUNNER_TOKEN`；身份模式不向环境令牌回退。服务端生成 `pool-<runner_id>` 作为 worker 标识；每个项目仍需请求与池拥有方批准的双边 grant。旧 `SUMA_RUNNER_TOKEN`、项目 Agent／API token 和浏览器 cookie 都不能代替资源池机器凭证。

机器 session 返回登记槽位上限和已批准范围，只是配置提示。每次领取仍在当前组织、凭证和 grant 校验后由控制面选择项目，并原子提交份额、资源、attempt 与回执。客户端不轮询修改一个全局 ProjectID，也不提交权重或权限来扩大范围。

派发固定 tenant／project、pool／runner／slot、grant 及版本、credential 及版本、dispatch、run／node、attempt／fence。心跳、进展、结果与停止事实均带同一范围；外层范围、回执核心身份和数据库中的原派发必须一致。进程启动前再对原派发发送一次心跳，授权或取消状态变化时不创建子进程，不把“未启动”伪称实际停止成功。

## 本地配置与运行

T214已接通[交付引用与原验收](../product/task-delivery-navigation.md)：原TaskDelivery正式引用可以到达准确团队／问题／需求，版本及Run定位仅打开原Review；原同提交独测与批准权威不变。当前消费者不新增执行动作，实际范围见[验证](../quality/task-delivery-navigation-verification.md)。

T213补齐[准确工具到项目任务](../product/tool-task-handoff.md)：Pool或普通工具已启用且真实可用时，可从详情选择原项目任务；重读准确工具／任务，再在原派发中明确采用。交接pin仅在内存，实际模型预算、当前路由、代码候选和同提交独立测试仍按原预览／Start及质量门禁；见[限定验证](../quality/tool-task-handoff-verification.md)。

T212提供[连接向导与单次检查](../product/pool-connection.md)：准确准备预览可复制 `--check-pool-connection --control URL --pool-config FILE --pool-identity FILE`，只验证原身份与当前Session；维护／任务前台命令分别明确复制，复制不启动。原读取接口分别显示登记、凭据、历史联系与库存来源，准确项目入口接续原工具Doctor。凭据／Grant／库存和任务资格仍由原执行链重核，实际边界见[限定验证](../quality/pool-connection-verification.md)。

T211补充[本机准备诊断](../product/pool-preparation-diagnostics.md)：合法准备模式失败在stderr输出七字段安全JSON，CLI固定双语原因与项目行／字段可回贴原表单定位。报告仅关联输入／目标，不代表实时文件或节点健康；写入未确认需核对原文件。成功六字段及原执行／授权保持。

配置是本地私有 JSON，不能加入 Git。以下仅演示结构；ID、命令可执行文件和目录需要替换为实际登记与安装内容。

T209提供[多项目配置表单](../product/pool-config-preparation.md)：在准确Pool节点行选择“准备项目配置”，选已加载的批准项目或手工范围，只填写目标机器路径。复制的命令通过私有标准输入调用`suma-runner --prepare-pool-config NEWFILE`，本机读取已有commands／model_deployments／external文件并验证原配置后独占创建全新文件。它不启动节点、不自动发现或启用工具；后续身份／Grant／库存检查仍沿本手册。变更配置时使用新文件和新状态目录，保留旧Journal并先核对未决操作。详见[合同](../../contracts/pool-config-preparation.md)。

```json
{
  "schema_version": 1,
  "pool_id": "registered-pool-id",
  "runner_id": "registered-runner-id",
  "slots": 2,
  "poll_interval_ms": 2000,
  "heartbeat_interval_ms": 5000,
  "scopes": [
    {
      "tenant_id": "approved-tenant-a",
      "project_id": "approved-project-a",
      "workspace": "./workspace-a",
      "commands": {
        "inspect": { "executable": "python", "args": ["--version"], "read_only": true }
      }
    },
    {
      "tenant_id": "approved-tenant-b",
      "project_id": "approved-project-b",
      "workspace": "./workspace-b",
      "commands": {
        "inspect": { "executable": "python", "args": ["--version"], "read_only": true }
      }
    }
  ]
}
```

```text
suma-runner --control https://control.example --pool-config local.pool.json --state .cache/runner --allow-exec
suma-runner --pool-config local.pool.json --describe-tools --allow-exec
```

运行模式需要独立机器环境凭证；第二条仅作本机工具探测，不需要凭证、不领取任务或调用模型。探测输出 `scopes[].tenant_id/project_id/items`，与执行使用相同配置，不输出目录、环境值或命令参数。它描述程序与协议观察，不代表模型认证、测试或生产验收。

配置支持 1–128 个真实本地槽位，不能超过服务端当前登记上限；最多 256 个本地范围。poll 可设 100–60000 毫秒，heartbeat 可设 100–10000 毫秒，缺失或零使用 2000／5000 毫秒默认值。库存按 30 秒刷新；单次网络失败关闭新领取资格，不凭库存失败取消已在执行的任务。

每个 scope 固定 workspace、repository、base_commit、worktrees 和 commands；目录解析为真实绝对路径后参与恢复摘要。不同项目不能共用进程工作目录。写命令沿用已有的固定 Git 基准与独立 worktree 协议，并把完整资源池范围加入 attempt 目录摘要。

原生 Suma 命令自动固定 `SUMA_PROJECT_ID` 为该 scope；明确不同的值或从可变父环境导入项目号会被拒绝。机器凭证不会自动继承到子进程，也不能在 pool 命令的 Env／EnvFrom 中转发；原生 Agent 如需业务只读工具，必须另有该项目的 Agent 凭证，不能把 pool token 改名当作 Agent token。

`read_only` 是管理员登记的执行约定，现阶段不是操作系统强制沙箱。获准接入资源池的节点运营者仍是受信任执行方；不把裸机进程称为不可信代码或多租户操作系统隔离方案。

## 持久化与并发

状态位于指定目录的 `pool/` 子目录，进程持独占状态锁。顶层 identity 保存控制地址摘要、pool／runner、凭证代际和解析后本地配置摘要，不保存 token。身份、凭证代际或执行配置变化需要明确的新状态目录，不能猜测如何处理旧目录中的未完成操作。

每个槽位有独立 journal 和 pool-claim checkpoint；每个 scope 有独立工具库存操作记录。槽位可以并行运行真实子进程，各自保存原派发，不共享一个可变当前项目。journal key 与工作目录 key 包含完整 pool／scope／dispatch 身份。

| 中断边界 | 恢复方式 |
|---|---|
| 请求已落盘、ACK 丢失 | 相同 slot／operation／逐项目能力集合重新请求，不新建 attempt |
| 响应已保存、inbox 未保存 | 刷新原操作当前派发状态，不按本地缓存直接启动 |
| inbox 已保存、开始前或执行中重启 | 保存 unknown，先核对原执行；不盲重启进程 |
| 结果已保存、ACK 丢失 | 恢复 inbox 中的固定范围后重放原结果；不能从结果的公共序列化字段猜 scope |
| closed／reconciling 响应 | 记录观察并结束该次领取，不执行 |

未完成领取遇到 grant 撤回或范围变化仍保留原 operation 和请求，不换范围掩盖可能已发生的派发；该槽位等待核对，其它槽位可以继续。机器撤销／过期被当前心跳或回写拒绝时，执行停止并保留原 outbox；不自动用新凭证接管旧派发。取消依赖真实进程停止后的 `stopped`／`outcome_unknown` 结果，不用删除登记伪造停止事实。

## 机器接口与工具资格

独立 `/api/v1/pool-runner/*` 在人类会话认证之前路由，仅接受 Bearer 机器凭证。session 为 GET；claim、tool-inventory、heartbeat、progress、results 为 POST，严格拒绝重复／未知字段、额外 JSON 内容和超过 2 MiB 的请求。

工具库存按实际 tenant／project／worker 保存，稳定 operation 先授权再重放。固定 profile 仍要求人类启用人的当前审批权限、可用清单、60 秒观察有效期和精确配置／程序／协议摘要；pool 登记不会代替这些事实。池暂停会明确显示“资源池已暂停，停止新派发”，不会伪称工具未安装或凭证撤销。

本阶段支持普通 command、test 和已按项目授权的 agent；托管运行仅放行明确的 TaskDispatch 开发范围。生产部署、外部工作流以及制品导入／发布需要独立 pool 能力协议，目前明确拒绝，不借用旧项目 Runner 的发布资格。进展和最终工具证据只写入原派发项目；失败经既有持久投影回流同项目问题池，重复回执不创建重复问题。

诊断只输出代码、槽位及必要范围标识，不记录 token、prompt、命令秘密、私有环境或请求体。控制服务、当前授权和 CLI 错误分别可定位，不把网络失联猜测成未执行。

## 验证边界

专项验证分别覆盖：真实本机 HTTP 范围回写与 journal 冲突 RED→GREEN；结果重启丢 scope RED→GREEN；两个实际子进程互等启动以证明双槽并行及独立 cwd；实际心跳取消后停止 OS 子进程；真实 SQLite／平台 HTTP 的双组织授权、派发与稳定回执、伪造范围拒绝、TTL 和审批撤销、原项目失败去重；实际构建 CLI 的无凭证逐范围探测与模式混用拒绝。

协议用例中的进程和回执夹具分别标识，不把人工构造的成功回执写成模型或业务测试验收。本阶段没有厂商付费模型、OceanBase／PostgreSQL 实例、生产多机规模、安装升级或不可信进程沙箱验收。原始过程材料仅保留在忽略的 `.cache/e2e/pool-stage/`，Git 只提交实现、必要测试和精简文档。
