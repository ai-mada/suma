# 《完善速码平台架构》设计与实现整合记录

状态：当前 1.0.0 的 T-INTEGRATION-111 整合记录。负责域：主集成、任务域、交互与合同文档。参考对话：《完善速码平台架构》（`chatgpt-conversation://6ae163e6-0700-83e9-bf86-623690b54884`）。

## 来源与获取状态

参考对话描述了独立的 0.1/0.2 研发协作实现，使用 Python/FastAPI、SQLAlchemy 和 Alembic。对话明确说明当时没有挂载用户 Windows 目标项目，也没有成功将源码推送到目标远端。因此这些描述不是当前 Windows 项目已合并、已验证或具有相同 Git 历史的证据。

| 来源项 | 已知内容 | 当前可作证据的范围 |
| --- | --- | --- |
| 对话正文 | 需求、两轮交付说明、团队分配、派发诊断、依赖草稿及 Worker 恢复设计 | 设计来源与能力对照；主集成已读取对话 |
| 0.2 交付库文件与本机包 | `libfile_15f66beecf248191a1d806eea36b7214`；用户提供 `C:\Users\Administrator\Downloads\suma-0.2.0-dev-3f272a3.zip` | 已核验、检查条目并隔离展开，已实际阅读下表中的源文件 |
| 0.1 交付库文件 | `libfile_14ce216e11b0819188306a0c550d3a87` | 历史描述来源，尚非当前验收证据 |
| 包中 bundle 的分支 | `refs/heads/dev` 指向 `3f272a3925f2bfa192143753f423ffaa1b3615ea` | 分支清单及 `git bundle verify` 均成功，包含完整 SHA-1 历史；独立来源历史，不当作当前分支祖先或已 cherry-pick |
| 对话中的环境路径 | `/workspace/scratch/82973a87168f/suma` | 来源当时的独立环境，不能当作本机 `D:\codes\ai-dev-os\suma` |

源包 SHA-256 为 `a46db2758a5b467522870edcb2ba45bb719725c0766c732c566392d6536dbf79`，801382 字节。条目检查覆盖 144 个条目、120 个文件、解压总量 1797789 字节，拒绝路径穿越、绝对路径、链接、重复名称与大小异常。只读展开位置为项目忽略目录 `.cache/e2e/integration-111/reference-source/`，不提交源包、bundle、patch 或展开副本。

`suma-dev.bundle` 为 332398 字节，SHA-256 `b368be34c1b2e330e79b0f7be442d3a50ae36cbb04aeda82eba692003a248472`；`suma-v0.2.patch` 为 521588 字节，SHA-256 `11ef68993963ff12b8967622868ed981a96525aa84f726e3afe6f51c4f59773e`。本轮审查源包中的设计及实现，再适配当前领域合同；不执行旧包的初始化、迁移、安装或开发服务指令，也不将两个控制面混合部署。包内未发现独立 LICENSE/NOTICE 文件；当前采用方式为设计审查与当前架构下的重新实现，没有把旧 Python/React 文件直接拷入正式源码。

## 实际阅读的来源文件

路径相对源包根目录；摘要绑定本轮实际读取的文件字节，便于源包不入库时复核来源。

| 文件 | 字节数 | SHA-256 |
| --- | ---: | --- |
| `DELIVERY.md` | 2083 | `7c1a3311dd3e89f4f0ae3c8c5f6f1e89c107808ee807f4764360a31b425b3c52` |
| `suma/docs/V0_2_COLLABORATION.md` | 12905 | `df64811e52a77a11dd37f314d260216b8786b4cdc9c2b92929cd7fa90af167e4` |
| `suma/apps/api/suma_api/services.py` | 26463 | `e1da7328d90be57bfc2ccecbf6f253c0f609062c894d5dab5e7c65358b4c7ceb` |
| `suma/apps/api/suma_api/planning.py` | 12116 | `28d63088ea22a3c1546bd0802355d7539fe5a90d73b381eb8106211408df3ee2` |
| `suma/apps/api/suma_api/routers.py` | 29246 | `8bd6d9918541b84570f78a4551514b8fa1cdefa34f84773d8e8ae791bd8d6bf7` |
| `suma/apps/api/suma_api/schemas.py` | 9244 | `a05cbb31db468dbf914459d99736868a59d1da4214d182cb3f51b31c3407a3e5` |
| `suma/apps/web/src/pages/Dag.tsx` | 24273 | `b59b45faf44de4dc06092a4d2bca3723c8ec3fb3c4e7a409570cba63bde96f2c` |
| `suma/apps/web/src/core/dependencies.ts` | 1427 | `fbf29859386e1b04e3e3abd2261184381a232a1c82478b64271c9f721ffd96d5` |
| `suma/apps/cli/src/suma_cli/worker.py` | 18324 | `f514bb8c5c3b6d1f444ea5e60379ee0dee69a03e2db267c47d0fb750c7b82ced` |
| `suma/apps/cli/src/suma_cli/transport.py` | 12055 | `bb33fafce2479c0bfd6cc50820e501c62633dbc862769de666403e22faa6c7a8` |
| `suma/apps/cli/src/suma_cli/preflight.py` | 5134 | `f059afa4788f8bdb71994fed519a6ccbbce4c18fa69b59939c514e3fdc1dda90` |

## 逐项整合清单

| 来源能力 | 当前正式领域与采用方式 | 当前边界与证据入口 |
| --- | --- | --- |
| 负责人、团队、指定 Worker 分开管理 | 复用[任务责任交接](../../contracts/task-handoff.md)、项目成员与团队、执行路由；业务责任与机器执行身份分别建模 | 人员资源 `member.id` 与真实账户 `user_id` 不混用；团队关系不扩张项目权限。沿用[交接验证](../quality/task-handoff-verification.md) |
| 派发前检查与等待原因 | 复用[当前工具推荐](../../contracts/task-dispatch-options.md)、固定派发预览、真实工具观测、路由和调度准入 | 读取或采用推荐不启动任务、不预留预算；真实容量、工具版本、固定代码和当前权限由执行入口再次核验 |
| 任务依赖查看、草稿、批量保存与多人冲突 | T-INTEGRATION-111 增加[完整业务任务图与原子编辑](../../contracts/task-dependencies.md)，以每任务版本 CAS 代替旧接口的冗余原依赖列表 | 最多读取 1000 个任务完整图；最多保存 100 个脏任务；同项目全图事务校验、不可变 ACK 后独立刷新；当前证据由本阶段记录 |
| Worker 本地 outbox、断线回传与错误隔离 | 复用[Runner 持久 journal](../agents/runtime.md)、[资源池独立槽位](../agents/execution-pool-runner.md)、[调度与用量](../agents/scheduling-usage.md) | inbox 执行前落盘、outbox 上传前落盘、原事件确认后保存；保留原 key/fence。拒绝或不确定事件不静默删除；不能用旧租约自动重跑策略绕过副作用核对 |
| CLI/工具配置与实际预检 | 复用[工具执行运行时](../agents/tool-execution-runtime.md)与[团队 CLI 合同](../../contracts/team-cli-runtime.md) | 使用真实版本、入口与配置摘要、Git 根和独立工作树。声明能力或可执行文件存在不等于真实模型执行和验收 |
| 测试、人工验收与失败问题回流 | 复用[验收规则](../quality/acceptance.md)、[失败诊断与问题回流](../../contracts/failure-diagnosis.md)及现有候选合成/独立测试链 | 验收绑定当前需求、任务、代码和真实执行证据；问题回流生成可审阅草稿，不自动关闭问题或代替人工验收。沿用[失败诊断验证](../quality/failure-diagnosis-verification.md) |
| 一般任务状态、团队计划及开发进度展示 | 复用现有任务、[团队执行工作台](../../contracts/team-execution-workbench.md)及专家团队计划 | 展示真实等待、执行、待验收、已验收和阻塞状态；不把某工具退出成功或图完成当成业务验收 |

以上是能力映射与当前实现落点，不是对来源 83 个文件或完整 25 类平台范围的合并声明。已有领域继续沿用原验证；本轮集中填补任务依赖编辑缺口，避免把相同能力维护为两个服务或两份状态。

## C201～C208 源码对照与适配决定

| 来源需求及已读实现 | 当前整合决定 | 当前落点 |
| --- | --- | --- |
| C201 项目团队；C202 责任与执行目标。`planning.py` 的 `assignment_blockers` 将有效项目权限、团队和指定 Worker 分开核验 | 保留业务责任与执行目标分离原则；采用当前组织/项目权限与正式交接，拒绝把来源直接用户 ID 复制为当前资源 ID | `internal/platform/task_handoff.go`、`task_handoff_basis.go`、现有治理与执行路由 |
| C203 可解释派发。`planning.py:158` 的 `task_readiness` 分开 dispatch/execution blockers，读取不预占预算；在线能力来自注册和心跳，源文档明确不证明本机仓库或模型登录 | 复用当前只读工具推荐、固定预览与真实执行准入，不新增第二份 readiness 状态或乐观容量数字 | `internal/platform/task_dispatch_options.go`、`task_dispatch_options_http.go` 与调度域 |
| C204 原子依赖。`services.py:89` Kahn 全图校验，`:111` 租户锁/项目授权后批量修改；`schemas.py:61` 使用 `expected_depends_on`，最多 1000 个改动、每任务 100 条依赖 | 采用全图、原子、草稿与冲突原则；重构为原任务版本 CAS、最多 100 个真实改动、重复显式拒绝、服务端无新增前驱数量限制；沿当前任务域使验收资格失效，不照搬旧状态冻结或旧 HTTP 422 接口 | `internal/platform/task_dependencies.go`；[当前合同](../../contracts/task-dependencies.md) |
| C204 交互。`Dag.tsx`/`dependencies.ts` 提供图连接、表单替代、集合比较、局部草稿、离开保护；旧保存把响应任务直接合入缓存后清草稿 | 共享图/表草稿与等价入口；当前显式处理原操作未知结果，原回执不可变，独立 GET 后再核验；冲突保留草稿，用户确认才基于最新图重建脏任务 | 当前任务依赖组件与中英操作文档；本阶段 UI 证据单列 |
| C205 领取范围。`planning.py:142` 的 `claim_filters` 仅允许项目及能力缩小授权；`worker.py` 在 `worker/claim` 发送本地项目和实际能力 | 复用当前 Runner 项目凭证、当前工具清单、执行池槽位和路由合同；保持机器配置不能扩权 | `internal/runner/worker.go`、`pool_worker.go`、`claim_checkpoint.go` |
| C206 故障隔离。`transport.py` 的 `quarantine` 保留整 run 原事件和首拒绝原因；`Reporter.flush` 对 401/403 停止、永久拒绝隔离，`worker.py` 的 replay 不重启子进程 | 采用原事件、拒绝可诊断及执行隔离原则；当前私有 Runner 原 journal 拒绝后阻止继续领取，资源池按槽位隔离，不能宣称二者都具备旧包整 run 隔离界面 | `internal/runner/journal.go`、`worker.go`、`pool_worker.go`；后续改善检查入口需独立需求与证据 |
| C207 无副作用预检。`preflight.py` 验证本地执行开关、凭证存在、Git/base_ref、状态目录、可执行入口及测试策略，明确模型认证未知 | 复用当前真实版本/help 观测、入口/配置哈希及已授权工作树校验；预检不调用付费模型，不能把可执行文件存在当作完整接通 | [工具运行时](../agents/tool-execution-runtime.md)、`internal/runner/tool_inventory.go`、`workspace.go` |
| C208 Alembic 升级与旧数据保全 | 不移植旧数据库栈、0001/0002 历史链或兼容入口；遵从当前 1.0.0 初始化。存储备份/迁移能力继续由当前存储域规划与真实数据库验收负责 | `Service.Initialize` 与当前存储合同；不引入 Alembic 依赖 |

来源 `services.py:449` 的验收核对 commit、artifact 与成功测试，`:500` 的问题提升按 `promoted_requirement_id` 幂等生成需求草稿。当前复用更严格的固定版本/独立测试/正式验收、失败诊断和问题 intake，不削弱为“工具退出成功即可验收”。源码审查同时确认来源 `services.py:241` 的过期租约恢复可以直接创建新 attempt；当前明确不采用这一路径，未知外部副作用继续先核对。

## 不直接移植的设计

1. 不移植 Python/FastAPI/SQLAlchemy/Alembic 服务、旧表升级链或旧产品版本。当前 Go 模块化控制面、TypeScript/React 工作台与 `Service.Initialize` 的 1.0.0 初始化模型继续作为唯一实现。
2. 不引入旧接口别名、自动字段迁移、双写旧数据或兼容租约状态。业务操作、审批、预算、租约、outbox 和审计沿用当前事务边界。
3. 不依据租约过期、失联或回执丢失直接重跑可能已发生副作用的操作。先恢复原操作并核对实际状态；只有已证明未执行或具备现有幂等合同的操作才可重试。
4. 不以参考包原验证数字替代当前环境证据，不把示例 Worker/CLI 声明当作真实外部平台完整能力，不借用旧提交号证明当前分支已发布。
5. 不让图编辑启动任务或绕过验收。保存依赖改变当前任务版本及验收资格，原运行快照与历史证据保持独立可追溯。

## 可追溯交付要求

来源条目按“对话或源文件 → 来源摘要/哈希 → 当前合同 → 实现文件 → 当前证据”登记。新增合同与 Go DTO 生成 OpenAPI/TypeScript；英文操作说明和精确双语服务端诊断一起维护。主集成完成后补记本阶段提交、必要检查和实际 UI 证据，并明确真实外部平台、OceanBase/分布式及生产环境仍未验证的项目。源包的历史验证数字仅说明来源，不纳入本轮当前代码验收计数。

阶段提交仅包含源码、合同、必要文档和经筛选的证据，不提交源包副本、缓存、临时数据库、日志洪流或构建目录。当前文档不新增一套完整 PRD；产品总范围与既有计划继续由正式能力矩阵维护。
