# Runner 执行工作区与真实进度

日期：2026-10-23。本文描述已实现的本机命令执行合同；正式部署、业务验收和实际合并仍分别记录。

## 管理员配置

编码命令及会写文件的测试命令在每次尝试独立的 Git worktree 中执行。管理员必须同时提供 `--repository`（现有 Git checkout 的绝对根目录）、`--base-commit`（完整 40/64 位 commit ID）及本机命令许可表。远程工作流不能更换仓库、基线、可执行文件或参数。基线必须在仓库中实际存在，`HEAD`、分支名和缩写 SHA 均不作为固定基线接受。

```json
{
  "test-candidate": {
    "executable": "go",
    "args": ["test", "-mod=readonly", "./..."],
    "test_evidence": true
  },
  "inspect-version": {
    "executable": "git",
    "args": ["--version"],
    "read_only": true
  }
}
```

`read_only:true` 由本机管理员显式声明，仅供只读探测使用共享 `--workspace`。省略该声明的 command、test、agent 命令均要求隔离。`test_evidence:true` 表示管理员将整个许可命令指定为测试；证据只证明该命令在记录的代码版本上实际退出成功，不等于人工验收或生产验证。执行外部 CLI 仍由管理员明确配置；没有自动模型调用、收费调用或隐式供应商回退。

启动示例（把固定 commit 替换为管理员已审查的真实值）：

```powershell
suma-runner --control http://127.0.0.1:4310 --project <project> --worker <worker> --workspace D:\project --repository D:\project --base-commit <full-commit-id> --commands commands.local.json --state D:\project\.cache\runner\state --allow-exec
```

Runner 令牌由当前进程的 `SUMA_RUNNER_TOKEN` 提供。每个 Runner 必须使用独立状态目录及稳定 worker 身份。默认尝试目录为 `<repository>/.cache/runner/worktrees/<attempt-key>/`，可用 `--worktrees` 指定仓库 `.cache` 下独立且被 Git 忽略的子目录。路径不得穿过目录链接；不会在磁盘根创建临时目录。命令 TEMP/TMP/TMPDIR 只对其子进程设置，指向本次缓存目录，不更改机器或用户环境变量。

本机命令只继承明确许可的普通运行环境。Windows 另外保留 SystemDrive、PATHEXT、ComSpec、APPDATA、LOCALAPPDATA、ProgramData 等系统目录与原生命令上下文；环境变量名称按 Windows 规则忽略大小写，同名本机配置覆盖普通默认值，配置中相互冲突的大小写别名会被拒绝。非白名单的父进程变量不会自动传给许可命令；其他变量仍须在本机 `env` 或 `env_from` 中显式配置。

## 隔离与恢复

attempt-key 由 run、node、attempt、fence、worker 的规范化身份确定。`attempt.json` 先记录准备操作，再创建 detached worktree；工作区启动前必须与固定基线一致且干净，源目录中的链接会被拒绝。已有同身份目录会拒绝再次执行，交由人工核对。用户原 checkout 的 HEAD、分支及脏文件不会被切换、重置或覆盖。Git worktree 提供源文件隔离；许可命令仍拥有 Runner 操作系统用户的权限，不是操作系统沙箱。

命令结束后保存 `changes.patch`、`status.txt`、`untracked.json`、`attempt.json` 与 `evidence.json`。证据含固定 base、结束 HEAD、真实退出码、未知结果标记、差异及状态摘要，以及未跟踪普通文件的 SHA-256 清单。补丁包含从固定 base 到结束候选版本的已提交与未提交跟踪文件差异；新增未跟踪文件保留于工作区并记录摘要。控制服务的 `workspace_evidence` 接收版本、状态、摘要及最多 200 条脱敏相对变更路径（每条最多 512 字节，另含真实总数及截断标记），补丁内容保存在本机。证据采集失败时结果保持待核对，不记录成功。

Runner 接收尝试后先保存 journal，再启动命令。进程重启后不会执行 journal 中已有尝试；没有最终回执的尝试报告 `outcome_unknown`，需要原执行身份的核对。最终结果一旦落盘不能覆盖。工作区与证据自动保留，当前没有自动清理或自动合并；清理应先按项目规则保留证据及摘要。

## 明确交接测试候选

每次尝试默认重新从管理员基线创建工作区。编码节点产生的新提交不会自动进入下一节点；继续测试旧基线不能证明新候选已通过测试。需要验证前序候选的 test 节点明确声明真实上游节点：

```json
{
  "id": "verify-candidate",
  "type": "test",
  "command": ["test-candidate"],
  "depends_on": ["code"],
  "config": {"candidate_from": "code"}
}
```

流程设计界面中，先建立 `code → verify-candidate` 的明确依赖，选择 test 节点，在节点配置 JSON 中填写 `{"candidate_from":"code"}`；源码中的对应字段是 `Node.Config["candidate_from"]`。`code` 必须是本工作流真实的静态 command 或 agent 节点 ID，不能填写标题、其他运行 ID、动态 map 子节点或任意 commit。

交接合同只接受本次运行上游节点当前尝试产生的成功、结果已知、干净且已提交的 HEAD 和证据摘要。调度器冻结并派发 `candidate_commit`、`candidate_from`、`candidate_attempt`、`candidate_fence`、`candidate_evidence_sha256` 五项顶层执行身份；配置中的任意 commit 字段不能代替这份身份。多个分支不会自动合并为候选。

Runner 要求这些字段完整、来源与 `config.candidate_from` 一致。test 节点的管理员许可命令须显式 `test_evidence:true` 并使用隔离模式；`read_only:true` 共享模式不能接受候选交接。P5 另外允许 command/agent 构建节点显式交接，但仅限管理员设置合法 `artifact_output` 的构建 profile；构建完成后源码必须仍在该候选且干净，不能把普通编码命令当作此例外。候选完整 commit 必须已存在于本地管理员仓库，且管理员固定基线是其祖先。对象缺失或谱系不符会明确失败，不自动 fetch、merge 或切换用户 checkout。实际制品与包测试交接见 [本机静态发布提供方](release-provider.md)。

当前没有跨 worker 的 Git 对象传输，也没有多个候选的自动聚合；不同 Runner 使用不同本地仓库时，管理员必须先通过另行授权的流程准备相同提交，缺失对象会保留为明确失败。动态 map、跨子运行来源不会被推断为可交接候选。

交接后的 `base_commit` 是实际测试候选 C，`admin_base_commit` 保留管理员边界 B；证据另外记录来源节点、尝试、fence 和原摘要。后续验收必须核对测试覆盖实际交付候选，不能仅因两个节点均退出成功就认定代码版本一致。既有仅测试、没有工作区证据的运行仍只表示既有测试节点执行事实，不表示代码版本已绑定；正式代码验收需要这份来源证据。候选交接的调度、HTTP、真实 Runner 与浏览器结果应分别以最新集成证据为准。

## 持久进度

stdout/stderr 的实际写入产生脱敏输出快照，单条最多 8192 UTF-8 字节，最多记录 1024 条。快照为当前已捕获输出的尾部，可能包含前一条已出现的文字；它不是完成百分比，也不替代最终 `Result.Output`。达到进度条数上限时 journal 标记截断，命令继续执行，最终有界输出仍保留。已配置秘密、常见 token/API key/Bearer 模式、ANSI 转义及控制字符经过清理；跨输出写入拆开的已知秘密前缀也会被隐藏。

每条进度在请求发出前落盘，身份由尝试身份和序号确定。`POST /api/v1/runner/progress` 发送 `{project_id,assignment,operation_id,message}`。服务端在调度事务内验证租户、项目、当前 worker/attempt/fence 及有效租约，同时持久保存 `node_progress` 事件与操作回执。回执丢失时重发原身份和原内容；同身份不同内容冲突，已持久内容可在完成后再次确认，新的完成后进度会被拒绝。心跳独立于进度上传运行，上传失败不会重新执行命令。

待确认进度先于最终结果重放。租约或归属已失效且进度从未入库时，服务端会拒绝该新写入，Runner 保留原 journal，等待核对，不能通过新建尝试掩盖未知结果。最终成功、测试证据、业务验收、发布分别保留其真实含义。

## 已评审合并前检查

管理员准备本地 JSON 请求，使用本次 `workspace_evidence` 中的真实摘要及已审查的版本：

```json
{
  "attempt_key": "<attempt-key>",
  "expected_target_head": "<reviewed-target-commit>",
  "expected_candidate_head": "<tested-candidate-commit>",
  "expected_evidence_sha256": "<evidence-sha256>",
  "reviewed_by": "<local-reviewer>"
}
```

```powershell
suma-runner --repository D:\project --base-commit <full-base-commit> --merge-preflight reviewed.local.json
```

检查要求管理员指定的测试命令实际成功、结果已知、候选已提交、测试后 HEAD 未改变、本地证据摘要一致、候选与目标均干净，且候选包含预期目标 HEAD。目标 HEAD、候选 HEAD 或证据发生改变时必须重新评审。通过后在尝试目录保存稳定摘要命名的检查回执并输出 `ready:true, merged:false`。该操作只检查并记录回执，不执行实际合并、分支更新或发布；后续合并仍需独立授权与目标版本核验。

## 本次验证证据

本机 Windows 上执行 `go test -json ./internal/runner ./internal/dag ./cmd/suma-runner -count=1`，70 项测试通过，0 失败或跳过测试；Runner 命令入口仅编译、没有独立测试用例。相关 `go vet` 通过。缓存和临时目录在仓库 `.cache`，测试使用本机 HTTP 与任务自建 Git 仓库，未调用模型或传输外部用户数据。`CGO_ENABLED=0`，未运行 race detector。

- [原始测试记录](../quality/evidence/runner-workspaces-tests.jsonl)
- [SHA-256 校验记录](../quality/evidence/runner-workspaces-tests.sha256)

原始记录 SHA-256：`16913738f54076a8d14b0e0624fa676af0a37519b4fcfc407083f6969e73cd1d`。这组记录覆盖 Runner 与 DAG 的当次实现验证，不替代后续 HTTP 集成、浏览器、人工验收或生产验证。

候选交接与验收修复后，执行 `go test -json ./internal/runner ./internal/dag ./internal/platform ./cmd/suma-runner -count=1`，166 项测试通过，0 失败或跳过测试；这四个包的 `go vet` 通过。新增覆盖包括 HTTP 权限及进度回执、动态节点能力与当前尝试证据、C/B 候选混用拒绝、来源身份冻结、提前 any_success 等待，以及真实 Git 候选在本机的执行。未把这些 fixture 结果记作正式业务验收。

- [候选集成原始测试记录](../quality/evidence/runner-candidate-integration-tests.jsonl)
- [候选集成 SHA-256 校验记录](../quality/evidence/runner-candidate-integration-tests.sha256)

候选集成记录 SHA-256：`0a9bda84ca4b6613effe1144845ac07f07dbaf8678a12b121df1e0e4e43e1737`。真实 Runner 二进制与浏览器的集成验证由后续交付记录单独说明。

## 浏览器验收专用本机 fixture（2026-10-24）

独立测试仓库位于 `.cache/e2e/candidate-repo`，管理员许可表和启动脚本位于 `.cache/e2e/candidate-runner`。它只用于真实浏览器派发与双 Runner 候选交接验证，不是平台源码副本或正式用户仓库。基线为 `27f4c5bf61c67b33537fbd35a9a37f97f747a76d`：简单 Go 功能及测试在基线下真实通过。

`edit-candidate` 只允许在该 fixture 的独立尝试 worktree 中运行固定管理员脚本，修改折扣函数与明确断言，再使用固定测试作者、固定 fixture 时间、本机空 hooks 目录提交。`test-candidate` 执行真实 `go test -mod=readonly -count=1 -v ./...`；`vet-candidate` 执行真实 `go vet -mod=readonly ./...`，二者均为 `test_evidence:true, read_only:false`。Go 工具链使用已安装本机版本，依赖代理与校验服务关闭，缓存及临时文件在仓库 `.cache`；没有调用第三方智能工具或付费模型。

离线准备演练产生候选 `a90d4b5a3ae388b200a6d9807198b0a87269d35e`，真实断言验证输入 1000 得到 900、100 得到 90、0 与负数得到 0，`go vet` 也通过；原 fixture checkout 仍在干净基线。`.cache/e2e/candidate-runner/preparation-receipt.json` 保存演练用途、退出码及文件摘要，SHA-256 为 `cd7914380a53436326a81da02c1c281b87d2e0eccc6eaf9d1d7d9229185a5cb3`。这项离线演练没有创建平台 workflow/run，也没有启动 Runner，不能作为浏览器验收结果。

`start-runner.ps1` 与 `start-runner-2.ps1` 分别使用 `candidate-browser-runner`、`candidate-browser-runner-2`，以及独立的 `state`、`state-2` journal 目录。当前 Runner 令牌绑定项目与凭证种类，没有绑定单个 worker，因此同一项目令牌可用于这两台本机进程；worker、attempt 与 fence 仍分别校验。令牌值仅存忽略目录，不进入文档或日志。准备阶段保留启动脚本，实际派发、启动和浏览器验收由操作人另行执行并记录。

首轮真实 Runner 运行 `b3aed192e84f83fdee6537fada20516a` 失败，暴露 Windows 最小环境缺少普通系统变量的问题；失败尝试及其 14 个文件的摘要清单保留在忽略目录，没有通过清理或新建假回执覆盖。修复后，通过原许可表调用真实 `Runner.Execute` 完成编辑、固定候选测试及 vet，代码与测试工作区均干净，未再产生字面 `%SystemDrive%` 路径；40 项 Runner 回归通过，0 失败或跳过，相关 `go vet` 通过。这份离线修复验证仍不替代浏览器重试结果。

- [Windows 环境回归原始记录](../quality/evidence/runner-windows-environment-tests.jsonl)，[SHA-256](../quality/evidence/runner-windows-environment-tests.jsonl.sha256)
- [原许可表的 Execute 修复验证](../quality/evidence/runner-windows-environment-execute.json)，[SHA-256](../quality/evidence/runner-windows-environment-execute.json.sha256)

### 真实浏览器重试与人工验收

修复后，在本机 `127.0.0.1:4321` 的真实浏览器中重试同一运行 `b3aed192e84f83fdee6537fada20516a`。编码节点 `node-5d54c2` 的 attempt 2 / fence 2 在独立工作树从基线 B 提交候选 C；测试节点 `node-6440cb` 由 `candidate-browser-runner-2` 执行，vet 节点 `node-2f7d99` 由 `candidate-browser-runner` 执行。两个测试节点都冻结同一来源尝试和证据摘要，实际 `base_commit` / `head_commit` 均为 C，工作树各自独立且干净。真实 Go 测试的一项测试函数中，1000→900、100→90、0→0、负数→0 四项断言通过，vet 真实退出成功。

持久记录中，运行最终为 `succeeded`、版本 10；任务 `8ace9276787acfcb4356e5c156abf81a` 经浏览器人工验收后为 `accepted`、版本 3，评审记录明确保留验收前任务版本 2。首次失败的 attempt 1、原失败事件及 14 个文件均保留，摘要复核一致；没有通过覆盖失败或创建替代成功记录隐藏故障。原 fixture checkout 再次复核仍为干净基线 B，没有合并或发布。

数据库中的 vet 调度执行区间为 `2026-10-23T16:11:32Z–16:11:38Z`，test 为 `16:11:33Z–16:11:39Z`，名义重叠 5 秒。开始、完成和事件时间仅精确到秒，这些区间包括工作树准备、进程执行和证据采集。纳秒格式的 `captured_at` 只表示结束后的证据采集时刻；现有 journal 没有子进程精确开始时间，因此没有推算子进程的精确并行时长。

只读归档从活动 SQLite 的 `mode=ro` 事务读取该运行、固定流程版本、完整事件、尝试、候选记录、任务及评审，并核对两台 Runner journal。4 次尝试的最终回执均已确认，7 条进度均与持久事件和操作回执一致，4 个工作树的证据、补丁、状态与未跟踪清单摘要全部核对一致。归档没有复制活动数据库、认证数据或令牌，绝对工作区前缀已规范化；截图也有摘要记录。

- [真实浏览器候选交接与验收证据](../quality/evidence/candidate-browser-execution.json)，[SHA-256](../quality/evidence/candidate-browser-execution.json.sha256)
- [首次失败截图](../quality/evidence/candidate-runner-first-failure.png)、[固定候选测试截图](../quality/evidence/candidate-test-1920.png)、[人工验收截图](../quality/evidence/candidate-human-acceptance.png)

归档 SHA-256：`facc28c1b966cdf85c9cf2f6375fb9492044cbde75c1a6c3f86f62a882e58b4c`。该结果仅覆盖本机专用 Go/Git fixture 的真实浏览器派发、双 Runner 候选交接和人工验收；两台 Runner 共用本机仓库中的 Git 对象，不代表跨机器对象传输、厂商智能 CLI、付费模型或生产部署验证。
