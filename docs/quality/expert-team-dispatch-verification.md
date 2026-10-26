# 专家配置与协同派发：限定本地验证

阶段：T-P4-027；产品版本：1.0.0；日期：2026-10-26。
当前状态：源码、必要定向检查、配对构建、一次本地 Runner／CLI 协议旅程、代表性中英 GUI 及主预览前向升级已限定本地验证。实际执行位于同一 Windows 主机的两个独立 Runner 进程和仓库；不代表两台物理主机或远端分布式验收。完整 P0–P7 继续实施。

## 范围与实现事实

本阶段提供版本化项目专家配置，以及 1–16 个独立任务的一次原子派发。配置绑定现有团队成员、资源、固定工作流节点、已批准工具和经审阅知识，不新增执行权限。发布、任务质量、人工验收和发布上线仍各自独立。

预览复用现有任务派发构建器；启动在同一事务重核当前权限、配置、来源、角色与路由，创建各任务自己的 run、绑定和回执。任一项失败全部回滚。批次内选择前置任务不能代替其已验收条件，批次所属单项计划也不能从原单任务入口绕过启动。

Core 新增 `CreateManagedRunsBound`：固定有序请求、最多 10000 个顶层节点、每个 run 独立 family root，复用原操作回执表。重放复核当前权限并恢复原有序 run，跳过绑定和最终提交回调；不增加队列或领取前的机器资源占用。旧单 run API 保持原行为。

源码主要位于 `internal/platform/expert_team*.go`、`task_dispatch_build.go`、`task_dispatch_direct.go`、`internal/dag/managed_batch.go`、`cmd/suma-contracts/expert_teams.go` 及对应 Web 配置／派发入口。精确构建输入见下述 346 项清单；冻结语义见 [合同](../../contracts/expert-team-dispatch.md)，机器摘要见 [验证摘要](expert-team-dispatch-summary.json)。

## 必要检查与收据

以下为不同切面。父组、子例和失败项复验不相加为阶段总用例数。Go 使用仓库缓存 Go 1.26.10 与 `run-go019.ps1`，未跑全套、厂商模型或新增依赖。表内路径均相对仓库 `.cache/`。

| 切面 | 已验证事实 | 精确收据 |
|---|---|---|
| Core 批量事务 | 私有／Pool 路由、独立 family、当前授权重放、有序原 run、限额与完整回执、全事务回滚、双连接恢复；4 顶层／21 pass 事件 | `mcp019-deps/20261026T122039559Z.*`；`e2e/expert-team-dispatch-stage-027/core/verification.local.json` |
| 领域七组 | 配置历史／退役／API 权限，预览回滚和已验收前置门禁，私有／Pool 独立 run 与重放，来源漂移，禁止绕过单项启动，并发及撤权重放，角色身份不替代执行权限 | `mcp019-deps/20261026T121910828Z.*` 中 6 顶层通过；余下失败 fixture 修正后 `121942453Z.*` 单组通过 |
| 绑定回滚与原单派发 | 新增 bind-rollback 子例；原单任务原子绑定／回执恢复、双连接单 run 恢复、不可达路由三组通过 | `mcp019-deps/20261026T122047719Z.*`，本次选择共 8 pass 事件 |
| HTTP 合同 | 严格请求、整数版本词法、当前 scope、版本与确认、原子批次启动和回执 | `mcp019-deps/20261026T122400892Z.*`，1 顶层通过 |
| 后端本地化 | 平台诊断覆盖、规范模板语法与目录快照通过；不代表所有 CLI 原始诊断已双语闭合 | `mcp019-deps/20261026T122637897Z.*`，2 顶层通过 |
| 生成合同 | 333 个领域模型加独立 Error，共 334 个 OpenAPI components.schemas；201 条 API 路径与 TypeScript wire types 一致 | `mcp019-deps/20261026T122639478Z.*`，`suma-contracts -check` exit 0 |
| 静态检查 | DAG；platform、suma-control、suma-contracts | `mcp019-deps/20261026T122235454Z.*`、`122518351Z.*`，均 exit 0 |
| Web 构建 | 隔离输出 127 文件，保留 chunk 大小警告；构建成功不替代实际 GUI 检查 | `e2e/expert-team-dispatch-stage-027/verification/web-build.log`、`web-artifacts.local.json` |
| Web 类型与本地化 | 修正 App 条件分支的 JSX 闭括号后 tsc 通过；3789 组中英键，覆盖模块硬编码 0 | `e2e/expert-team-dispatch-stage-027/verification/root-web-checks.local.json`；来源为 Root exec 输出，无独立 raw capture |

Core 精简收据 SHA256 为 `4add6b3ef63d9450540e47cae0890e58e578f8156cddab8a0a307d0146eefb68`。各命令收据与输出的 SHA、精确选择器和组合关系保存在机器摘要，不把 exit 1 的中间轮写成全组通过。

Web 类型与本地化摘要 SHA256 为 `4983ccc5c05eab30d5cf05bb8200afa6856a350be9085a036e1c4fa6ac0918d5`：初始 exec `2c6213` exit 2，修正 App 条件分支后 session `19793` 的类型和 i18n 检查 exit 0，没有重跑。此摘要明确来自工具执行输出，不伪称独立命令捕获。后述实际 GUI 已覆盖分享／刷新恢复原批次和代表性中英操作，不能用键数代替完整界面或设备验收。

## 配对构建

`e2e/expert-team-dispatch-stage-027/runtime/build-facts.local.json` SHA256 为 `382e7358de40a51109bd88fce5c4d26e64534f8ac9739e1d3cb65a27cf26e09f`。
本轮 346 项源输入摘要为 `54b56b2ffcfd501fe794b8afbbc59efa8970cca2538ee1381f7448ab3c980dc5`；原清单 `runtime/source-before.local.json` SHA256 为 `6d5a0701d6b169a44e9d69c80d07bd5c17c2ab16eb1411d22ca7bf77644364c7`。

- Control：`134124f9433afb1a6b8356ee66c25577b4bdb268d3f574331fcaf0c748ab7e21`；构建命令 `mcp019-deps/20261026T122737014Z.*`，exit 0。
- Runner：`936a5ea205fdb9d5e29022687943019ac7330963502447f3fffe83eb8bcf89bb`；构建命令 `mcp019-deps/20261026T122913806Z.*`，exit 0。
- 初版 Web：127 文件；入口 `1d2e547f4a0300625b526ef7476755ebc0ac030d4af9f7d52cd5499d7ae6002e`；逐资产清单 `verification/web-artifacts.local.json` SHA256 为 `507e3a4ea4b4ec0b8b22225c6186dd53943d68469a4f0edd3b8b16015222f3f2`。
- 最终 `web-final`：127 文件；入口 `45ecfe8da547b315b2c913a8b9e8e5adb8cdcce6bf6778b240774ad6ea5b193a`。仅一行到期时间显示增加本地时／分；`verification/web-final-build.log` 构建 exit 0，逐资产清单为 `verification/web-artifacts.actual.local.json`。没有为该显示调整重复类型检查、测试或模型执行。

Runner／CLI 生产协议沿用现有合同；此次本地 Harness／CodeBuddy 协议夹具不需要原生模型 bundle。构建清单记录 model_calls=0，这仅是构建事实，不预先宣称实际执行旅程结果。

## 实际协议、GUI 与主预览

`actual/verification.local.json` SHA256 为 `fe8288af4f0a4e57956911f85f602d392c05b99f2bacac2791eba31d21e90414`，35 项检查全部为 true。数据来自 026 闭合验收库，源库 SHA `a30aa78b114bd855c1bc4b5726e17c05a9931ce318e4bca1ebdbb649c16dae8e` 保持，主预览未参与测试写入。

实际是同一 Windows 主机上的两个本地独立 Runner、两份独立仓库及两个真实并发 CLI 子进程。Harness／CodeBuddy 边界由自有 Node 协议夹具提供，没有安装或调用厂商 CLI／模型。每项实际生成 Git 候选，经密封 bundle 跨 Runner 传递，以同一候选 SHA 正式执行 4 个断言，共 8 个实际断言；4 个 attempt 各执行一次并 ACK。原收据内 `cross_machine` 检查名仅描述此跨 Runner 传递，不扩张为跨物理主机证明。

- 批次：`d0c515c4189c03da89709e530847b368`；固定配置草稿 v1 → 明确发布 v2。
- CodeBuddy run：`4fa55fd688be983bcac51fffd4bfc793`；Harness run：`a6d926ec2ef2e97d814148656de6dbc9`；均为 succeeded。
- 模型／厂商调用／新 AgentToken 均为 0。原任务、run、知识及 026 的 40 tokens／57 微元账本原行摘要保持；原仓库不变。
- Runner A 232728、B 220848 及 holder 248212／exec 49150 正常退出 0，无强制结束；helper 未停止 Root Control。

`browser/verification.local.json` SHA256 为 `defffb9af37925106403daab57226285dbf050933715aad1a0c26d8174700de1`。Root 在 GUI 编辑固定角色、保存草稿并明确发布，选择两项任务及各自角色，固定预览后一次明确启动；中英标签、原 run 链接、刷新／分享恢复原批次及正式测试证据均有代表性检查。截图及 SHA 留在该收据。

运行成功不等于任务验收：Root 以本地测试负责人身份，仅明确验收 CodeBuddy 任务 `0c8c659951b564a3fbee7bfb2acb3586`，状态 accepted v3；Harness 任务 `8616cc66c13f55bd9708f4b6ba67c571` 保持 in_progress v2。脚本不执行人工 Review；此为独立验收门禁的本地验证，不是业务或生产批准。

初始隔离 UI Control 254700／holder 250112／exec 10840，最终 UI Control 251748／holder 214636／exec 61402，均正常退出 0、无 force；最终复核 clone 来源保持。最终一行时间显示改动已用 `web-final` 只读 GUI 复核，没有重复 CLI 执行。

主预览 `http://pool.localhost:4324/` 已前向升级为 027：normal Control 255376／holder 247864／exec 63413 保持运行，旧 026 exec 2716 正常退出 0。`primary-upgrade/final-verification.local.json` SHA256 为 `b20cf94ece63b4c3433a01451f58f2872e810b22434ae674ef6cf02d63683ff1`：164 张原表 schema／全部行精确保持，新增 4 张空表和 2 个索引至 168 表，integrity=ok。18 authority／0 Grant、原登录与 1 failed／2 succeeded 原 run 保持，未重发登录；回环地址和浏览器别名的 HTML 字节及引用资产配对。Control 与 346 项源输入摘要不变，采用最终 127 文件 Web；隔离测试团队／任务没有导入主库。

## 中间失败与解释

- 领域初轮 `121743088Z`、`121833395Z` 保留 exit 1：旧 `SaveTeamMember` 的 map 输入要求 `expected_version` 为 float64，fixture 传入 int 0；修正 fixture 后七组中的六组通过。
- `121910828Z` 余下一组失败由前置任务 fixture 使用 `[]string`、被既有 `stringsAt` 忽略造成；改为实际 JSON 形态 `[]any` 后只复验该组，`121942453Z` 通过。没有放宽业务已验收门禁。
- Core 初始接口 RED 和并发 fixture 的 SQLite BUSY 收据保留。并发 fixture 补上真实 Service 已有的同事务 authority-before 写协调锁后通过；没有引入自动重试或改写旧事务／调度语义。
- Web 初始 JSX 闭括号错误修正后构建成功；失败执行记录与最终成功构建分开保留。实际 setup 首轮把保留的 `execution_slots` 错放入 `resource_limits`，API 正确返回 400；helper 改用 `concurrency_limit=2` 后以同库、原操作键恢复，已成功的 observe 配置未重复写入。原失败保留在 `actual/setup-001-*`，没有用新库、账号或 CLI 重跑掩盖。

## 仍未覆盖

真实厂商模型／账号／计费、跨数据库与分布式规模、完整设备／主题矩阵和全 CLI 本地化未验收。本阶段没有实现专家自主互相交接／综合结论、候选评审自动替代人工验收、真实目标分支 CAS 合并或生产发布。完整平台目标继续实施。
