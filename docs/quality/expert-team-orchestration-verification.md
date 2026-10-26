# 专家团队依赖编排：限定验证报告

阶段：T-P4-029；产品版本：1.0.0；整理日期：2026-10-27。
状态：本阶段所列功能已完成必要检查、同机实际流程、中英界面及主预览更新；完整 P0–P7 平台继续实施。
依据：[领域合同](../../contracts/expert-team-orchestration.md)、[CLI 合同](../../contracts/expert-team-orchestration-cli.md)、[机器摘要](expert-team-orchestration-summary.json)。

## 范围与事实边界

直接增强既有 `ExpertTeamPlan`：固定 1–16 个真实任务及依赖、来源修订、职责和执行依据；明确启动后，
服务端仅在前置任务针对本计划准确 run 获正式验收时派发后继。Go DAG 仍是唯一执行派发器。
未开始项不预建未来执行授权、不占机器槽位，业务依赖也不自动组合 Git 候选。

暂停／停止只阻止后续派发，已有运行继续使用原取消和核对入口；修订只改变明确选择的未开始项。
晚派发逐次核对实际原凭据、当前权限和来源，凭据失效需当前有权主体明确恢复。
源码、定向检查、本机实际进程、厂商与跨物理机器／生产验收分别记录。

本次实际环境为**同一 Windows 主机上的真实 Control、两个独立 Runner 及自有 Node CLI 协议夹具**。
Harness／CodeBuddy 生产适配链路被复用，但没有据此宣称安装或调用真实厂商 CLI／账号／模型。
本阶段固定夹具及实际流程的模型、厂商调用和新增 Agent Token 均为 0。

## 已通过的必要检查

父组、子例、预先单项和修复复验不相加为阶段总数。Go 使用仓库缓存 Go 1.26.10；没有重跑全套或付费模型。
以下路径相对仓库 `.cache/`。

| 切面 | 实际结果与范围 | 收据 |
|---|---|---|
| 新领域五组 | 5 顶层／7 pass 事件：预览无 run，所选未验收前置可固定，成功不代替正式验收，pending-only 修订与历史，外部依赖不隐式扩选 | `e2e/expert-team-orchestration-stage-029/domain/verification.local.json`；`mcp019-deps/20261026T144923673Z.*` |
| 原专家派发受影响组 | 4 顶层／10 pass：全事务回滚、私有／Pool 原 run 重放、禁止单项绕过、来源／配置漂移 | `mcp019-deps/20261026T145052591Z.*` |
| 原单任务派发 | 3 顶层／8 pass：原子绑定、准确依据、到期／质量缺失与撤权门禁 | `mcp019-deps/20261026T145228897Z.*` |
| 并发与撤权重放 | 1 顶层／1 pass：两次并发启动返回相同非空实际引用、没有额外运行；撤权后不能重放 | `mcp019-deps/20261026T145430896Z.*` |
| 推进／凭据／事实投影 | 5 顶层／9 pass，platform vet exit 0：原浏览器／API 范围恢复、准确验收 run、公平有界推进、no-op 不增业务版本、暂停／来源／凭据门禁 | `controller/verification.local.json`；`mcp019-deps/20261026T145412475Z.*`、`145421109Z.*` |
| reconciling 投影修复 | 新增 1 顶层／1 pass；真实 Core unknown 回执，保留原 run 状态及引用，投影 blocked，后续 Tick 不重派 | `controller/progress-recovery.local.json`；GREEN `mcp019-deps/20261026T155017221Z.*` |
| 成熟 CLI 扩展 | 1 顶层／5 pass：固定 `team-plan` list/get/revisions/create/start/command，严格文件 JSON／数字／Unicode，明确确认和原 key，无自动重试或身份刷新 | `cli/verification.local.json`；`mcp019-deps/20261026T150004774Z.*` |
| 历史读取边界 | 1 顶层／1 pass：没有当前不可变来源修订的原件不进入新列表或操作，旧真实运行引用与载荷不变；历史时间取修订行，返回副本不改原件 | `domain/history-boundary-verification.local.json`；GREEN `mcp019-deps/20261026T161630047Z.*` |

表内 `controller/`、`cli/` 是 `e2e/expert-team-orchestration-stage-029/` 的子目录。
领域收据中的先行单项 `144725080Z` 后来已包含在五组 `144923673Z` 中，不重复计数。
Controller 门禁使用真实 Core 完成及 Review 测试夹具，不冒充真实厂商／Git／Runner 子进程验收。

组件 `TeamTaskDispatchDialog.test.tsx` 在 2026-10-26 22:55:04（+08:00）5 项通过；
22:57:30 仅复验最后受影响的身份／403 与命令两项，通过。覆盖依赖预览、一次明确启动、成功仍待验收、
当前打开计划的有界只读刷新、丢响应恢复、范围隔离、暂停／恢复／停止、未开始项修订 CAS 与只读历史。
最终 `tsc --noEmit` session `62993` exit 0；i18n 为 3968 对，新增领域 82 对，语言／占位符／复数／标记检查通过。
这些前端结果来源于代理工具执行输出，**没有独立原始日志文件，不补造 raw capture**；定位信息保存在机器摘要。
后续到期时间的显示条件仅隐藏空／无效日期，按主集成指令未重复测试或类型检查。

`web-reviewed-build.local.log` 包含 Vite 构建完成记录，并保留大于 500 kB chunk 警告。
最终制品使用 `runtime-reviewed` 与 `web-reviewed`，见下述配对及实际结果。

Root 的 HTTP／本地化／合同联查 `145117726Z` 有 34 个 pass 事件，且旧并发测试断言错误地把预览空引用当作实际派发引用，导致该命令 exit 1。
已修改该过时断言，仅复验并发／撤权组 `145430896Z` 通过。未把失败命令改写为整组绿色，也未重复完整联查。
新命令入口、精确整数／重复参数、双语闭合说明和不可变原文投影检查已在该联查中通过；最终生成合同 `--check` exit 0。

## 中间失败与保留事实

- 领域初始 `144053973Z` 是依赖预览尚未实现的预期 RED。`144637182Z` 发现 authority 尚未清理时嵌套读取耗尽 deadline，
  修正为同事务预览投影及最终读取前清理；`145023994Z` 的 fixture 变量重复声明修正后，仅复验受影响组。
- 原组件旧 fixture 缺增强 DTO，首轮 2 fail／1 pass；更新真实依赖读模型后五项通过，没有增加旧协议兼容或放宽来源门禁。
- 初次实际计划 `930af77aec2356aa10544840ab5c1f65` 固定预览成功，但沿用 027 的 CLI 夹具目录限制和同时进入屏障，
  不适合 029 的依赖顺序。根节点因缺少完整终态进入 reconciling，测试节点保持 pending，后继没有派发。
  原未知运行保留，计划未开始工作明确停止；不能把此轮写成成功，也不能归因为业务依赖门禁失效。
- `actual/initial-fixture-findings.local.json` 保留上述原因；`actual/closed.local.json` 记录初轮两个 Runner 和 Control
  均优雅退出 0、无强制结束、integrity=ok、来源清单未变。
- 此轮还暴露真实投影接缝：聚合 run 仍为 running，但准确关联节点已 reconciling，旧投影错误显示 executing。
  修复仅检查同 tenant／project／run 的真实节点，显示 blocked 并保留原 RunState／RunID；新增 RED `154814118Z`、GREEN `155017221Z`，没有重派或改写执行事实。
- `actual-final/fixture-adaptation.local.json` 只修自有夹具：固定 029 所属目录、移除旧同时进入屏障、允许明确修订后的正版本，
  独立四断言测试仅调整目录限制。该适配收据本身**不是最终实际通过证明**。

## 最终实际流程与制品

计划 `7518384b1e958d43cb7fd3c788ef1b08` 通过产品界面发布配置、预览、启动、暂停、修订、恢复和正式验收。
根运行 `c494d922d64881a6954ac6eec8ea38a0` 开发成功并由另一 Runner 导入同一候选做四项正式测试，界面仍显示等待人工验收。
后继没有实际运行；暂停后明确验收根任务仍不派发后继。

随后在界面修改尚未启动的子任务说明，并明确修订计划：子任务固定版本由 2 到 3、来源修订变为 2；
已开始根保持来源修订 1 与同一原运行。恢复第一次因当前事实版本变化收到 409，界面保留原因草稿，
读取最新版本、重新确认后恢复成功。服务端自动派发后继运行 `e974cd71834d2d5066366949f1b41a7f`。
它同样由另一 Runner 导入准确密封候选并独立测试四项，最后由本地测试 owner 在界面明确验收。
最终计划 completed、2／2 正式验收；两次开发、两次正式测试都只有 attempt 1，准确候选 SHA／bundle／回执一致。
修订前未开始项保留一个排他 ID 预留，该 ID 没有 `dag_runs` 行，不能计为第三次执行。

中英文切换保留任务、职责原文、准确运行链接及同一计划 URL；刷新恢复 completed 修订 2。
修订历史只读、不提供当前写操作。最终只读复核还修正了列表混入旧原件与修订时间误用初始时间的问题：
最后构建冷启动回读展示 R1=23:58、R2=次日00:05，当前列表仅包含可用来源修订的计划，旧载荷没有重写。

`actual-final/verification.local.json` 固定 2 次实际运行、8 项正式断言、2 次显式本地测试 owner 验收；
`actual-final/closed.local.json` 记录 Control 与两 Runner 均正常退出 0、无强制结束、完整性 ok、源夹具四文件摘要保持。
最后构建只读回读 Control 也正常退出 0，没有重新登录、创建运行或调用模型。
原始证据只留 `.cache/`，截图、日志、数据库、密钥与二进制不提交。

| 制品 | 摘要或结果 |
|---|---|
| Go 输入 | 378 个；`2171e84a7d8c429d8b9edd59b23e5e3fc9bc4314c1087db4303a08540834346c` |
| 最终 Control | `0c15e37625aa3861871eccbba41302e609822546125a863d07a5fd5b65ef6648` |
| Runner | `b721ecdb9d8169a0ccc5d4025e688bd886405b2a0030f3632c8846bdd6799289` |
| CLI | `20dd031e4813d5aeb5f64feff1c71b50e533c4cb4186a0e26a4750813afb9cfa` |
| Web | 131 文件；入口 `2c15d59f3104c21c3a57f8a8e262fe593356a80a30f0bfaa8bc9fe0d03a25387` |
| 生成合同 | 371 Schema／218 路径；最终 `--check` 通过 |

实际完整执行使用最后历史读取补丁前的 Control `322a86a…4293e`；该补丁只改变原件读取隔离与修订时间展示，
完成定向回归后重建最终 Control，并在同一已完成数据上冷启动回读。没有为这项只读修复重复运行开发或模型。

## 主预览更新与剩余范围

`http://pool.localhost:4324/` 已更新到 029／1.0.0，Control `251820`、holder `199624`，正常模式。
旧 028 Control 与 holder 经原停止协议正常退出；保留原二进制、网页及 WAL 感知备份。
175 个原表的完整 schema 与全部行保持一致，仅新增两张空表和两个索引至 177；
18 条 authority、0 条 Grant、原登录／任务和 1 failed／2 succeeded 执行保持，没有签发新凭据或自动提升旧权限。
维护模式写入口返回 423，正常模式入口及关联资源与固定 131 文件制品一致。
依据 `primary-upgrade/final-verification.local.json`，摘要登记精确证据哈希。

跨物理主机、真实厂商 CLI／模型／计费、生产授权、完整设备／主题／多数据库／规模及 P0–P7 全平台继续实施。
本阶段业务依赖推进不自动合并前置代码；候选集成继续使用独立受控 Git 集成合同。
