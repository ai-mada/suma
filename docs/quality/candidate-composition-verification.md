# 多来源候选合成与开发基线：限定验证报告

阶段：T-P4-031；产品版本：1.0.0；整理日期：2026-10-27。
状态：所列定向检查、同机实际 M→基线→C、最终中英冷回读与主预览前向更新完成；**仅 T-P4-031 限定范围完成**。完整 P0–P7 状态仍为 `in_progress`。
依据：[领域合同](../../contracts/candidate-composition.md)、[CLI 合同](../../contracts/candidate-composition-cli.md)、[机器摘要](candidate-composition-summary.json)。

## 范围与计数

固定 2–16 个当前已验收、已封存且独立测试过的来源，明确集成任务与流程版本；Runner 内置命令合成新候选。
新候选沿原同运行 `candidate_from` 独立测试并由人正式验收，之后才能使用原受控目标交付，或明确绑定下游开发基线。
合成、测试成功、人工验收和目标 ref 事务分别证明，不能相互代替。

顶层组、子例 pass 事件、重叠选择及局部复验分别记录，不累加为阶段总测试数。
平台检查采用真实 SQLite、HTTP、Core、封存字节传输及 Review 事务，但其中 Runner／Git 结果是明确的类型化协议夹具。
实际 Git 由下面独立运行端组证明；同机隔离仓库不能称跨物理主机，任何一组都不代表生产验收。

## 已有必要检查

表内路径相对仓库；manifest 完整 SHA256 与命令定位见机器摘要。

| 切面 | 结果与实际边界 | 收据 |
|---|---|---|
| 领域初版 | 6 个不同顶层组有通过记录：有序来源、预览零运行、并发启动及原操作重放、当前来源／凭据／租约、分页／关闭、精确候选测试和正式验收后的基线 CAS；会话更换只打开原运行 | `domain/verification.local.json`；先 5 组 `175456262Z`，第六组 `175715573Z`；受影响 3 组 `180258555Z`，最终单组 `180407471Z` |
| 普通派发／专家计划／设计 | 新增 3 顶层通过：真实持久 managed spec 携固定基线；旧预览失效；来源漂移禁止新读／审批，保留事实；待执行项必须显式修订，历史和已启动前置不改；正式前置测试及 Review 后才推进；真实已发布设计任务采用精确 durable composition 证明 | `domain/shared-verification.local.json`；`182340461Z` 的前三组 |
| Pool 传输 | 同一组织真实双边授权、typed claim、两个固定来源字节相等；错 tenant／fence／transfer／plan 拒绝；来源变化后拒新读，原已发生失败及重放仍入库，终态旧租约不能续读。未声称跨组织或物理机器 | 同一 `182340461Z` 的第四组 |
| 受影响旧控制面 | 普通派发原操作、专家 pending pause／revise／stop、原设计固定上下文 3 顶层通过；平台 vet exit 0 | `182537497Z`、`182537478Z` |
| 真实 Git 合成 | 3 顶层通过：分叉来源合并、独立 B-only 仓库测试精确 M、继承开发与无净增量完整对象包、冲突不封存、固定提交身份及隔离配置／拒自定义 driver | `runtime/verification.local.json`；`180724241Z` |
| 受影响 Runner | 7 顶层、22 pass 事件、0 skip；两克隆、原 Journal 不重执行、坏包／缺对象拒绝、原生封存、基线不回落管理员 B、专用命令和当前 Pool 输入 | `181148780Z` |
| 原目标 Provider | 继承候选的 bundle anchor→selected base→head 校验及原 prepare／apply 1 顶层通过；这不是整条 UI 交付旅程 | `181121853Z` |
| 共享协议／运行端 vet | 2 顶层通过；有序来源、作用域、基线摘要与继承锚点；4 包 vet exit 0 | `181256723Z`、`181255943Z` |
| CLI | 1 顶层、6 pass 事件、0 skip；精确作用域／整数／确认／当前 key，不自动重试，不冒充浏览器授权 | `cli/verification.local.json`；`181916494Z` |
| Root HTTP／私有下载 | 2 个选定顶层 exit 0：重复／未知输入及整数词法、原 start 重放、不可伪造 protected baseline、CN／EN 错误；真实 private claim 携 typed composition，固定字节下载与错误来源／租约拒绝 | `.cache/mcp019-deps/20261026T182208790Z.*` |
| Root 首绑零版本 CAS | 仅 HTTP 入口顶层复验 exit 0：显式 `expected_binding_version:0` 到达领域检查；缺失、字符串、小数和指数表示拒绝。不把该单项重复计为两个 HTTP 新组 | `.cache/mcp019-deps/20261026T182452131Z.*` |
| 继承基线验收链补验 | 额外 1 顶层／2 子例／3 pass 事件、0 skip：旧创建者注销后，新当前浏览器仍可 Review(C)，accepted C 的仓库资格先正向通过；M 原来源变化后，未验收／已验收 C 的新 Review 及仓库资格拒绝，原 C succeeded／结果 SHA 保持 | `domain/baseline-acceptance-verification.local.json`；`184307354Z` |
| Root 初 cut Go | 平台／本地化／合同生成器／Control 四包 vet、生成合同 `--check` 和三个程序构建 exit 0；保留为初 cut，不充最终发布构建 | `183106399Z`、`183110193Z`、`183111931Z`、`183117159Z`、`183118982Z` |
| Root 第二 cut Go | 继承链修正后，平台 vet、生成合同 `--check`、Control／Runner／CLI／Provider 四个程序构建均 exit 0；保留为第二 cut | `184632921Z`、`184634930Z`、`184638900Z`、`184642498Z`、`184643498Z`、`184644496Z` |
| command／agent 模板补验 | 额外 1 顶层／7 子例／8 pass 事件：两类根代码产出节点及固定 capability 正向；test、非根、条件、额外 capability 拒绝，原 workflow／独立质量节点不改 | `domain/workflow-verification.local.json`；`190144284Z` |
| Root 第三 cut Go | command 模板修正后，四程序构建均 exit 0，Root 记录约 7.6 秒；未重跑旧测试，配对新网页另有下述收据 | `190300534Z`、`190304162Z`、`190305168Z`、`190306114Z` |

除完整 `.cache/mcp019-deps/` 路径外，表内 manifest 位于 `.cache/e2e/candidate-composition-stage-031/`，
短命令号均为同目录外 `.cache/mcp019-deps/20261026T<短命令号>.*`。原始数据库、令牌和运行产物不进入 Git。

## 界面已有结果

以下依据代理工具输出，**没有独立 raw 日志文件，不补造路径**；不等于真实浏览器旅程。

- 01:56:07（+08:00）`CandidateCompositionPanel` 原 4 组先 3 pass／1 fail，另有 1 条未处理错误，tool chunk `d74aa8` exit 1；
  误调用 `action.resume` 后改为 `action.run`，01:56:30 仅恢复组 1 pass／3 skip，`db4cba` exit 0。
- 加入精确基线读回约束后，01:57:57 仅基线组 1 pass／3 skip，`7211c5` exit 0。
  来源变化不得静默换 SHA、必须明确移除再选择的检查，01:59:10 仅该组 1 pass／3 skip，`a04b76` exit 0。
  原 4 个不同组均有 GREEN；没有把这些局部复验描述成同一次全组绿色。
- 02:24:54 新 `TaskCodeBaselinePanel` 1 项及 `CandidateTransfer` 继承 bundle anchor 单项共 2 pass／15 旧例 skip，
  `57437a` exit 0。覆盖折叠零读取、不自动取 latest、双 CAS 409 保稿、未知沿原 key、重挂载及已启动只读。
- 初网页 cut 类型检查 session `78460`、chunk `502698` exit 0。领域 107 对 key；全量 CN／EN 检查 4197 对、legacy／hardcode 均 0，`470d78` exit 0。
  同一已装 Node 执行一次隔离 Vite 构建，session `65372`、最终 chunk `a9a0c1` exit 0；317 个前端／合同／配置源项构建前后逐字节相同。
  构建收据为 `verification/web-build.local.json`，不冒充独立原始工具输出文件；构建通过仍不是发布或浏览器验收。
- command 选择修正后，03:03:24 仅新增选择组 1 pass／4 旧组 skip，`dc1772` exit 0；类型检查 `28711`／`79f9a6` exit 0，词条 4198 对、legacy／hardcode 0，`84dd4f` exit 0。
  `web-release` 单次隔离构建 session `6577`／`a09fa2` exit 0，317 个源项前后相同，保留 chunk 大小警告；收据 `verification/web-release-build.local.json`。
- 实际 App 消费检查随后暴露首 render 出现 3 份 `TaskDesignProvenance`，`14aaa0` RED；为同 sibling 的设计来源与交接面板分别加 `task-design:`／`task-handoff:` key 后，
  `6a5f32` 仅该组 1 pass／13 旧例 skip，v3→v6 及 CN／EN 始终唯一、最新、0 write。中间 `40c43d` 是夹具硬编码 Refresh 未命中，改为实际 `tr(shell.refresh)`；不另称产品缺陷。
  新类型检查 `59460`／`0e6a6e` exit 0；4198 对 i18n 源逐字未变，复用已有检查。`web-polished` 单次构建 `45397`／`44bc72` exit 0，保留 chunk 警告；
  收据 `verification/web-polished-build.local.json`。没有重跑下述两真实运行或八项检查，最终 Control-only 冷回读见末节。

## 保留的 RED 与局部修正

- `175658859Z`：运行端先行编译缺 `bufio`；`175711228Z`：基线拒绝组 RED，暴露不能在无效／未授权基线下静默选管理员 B。
  修正后仅该组 `180220142Z` GREEN，之后列入受影响 Runner 组。
- `180019957Z`：平台复验撞到并行运行端文件尚未写齐，报缺 `compositionClient`／`executeCandidateComposition`，未运行测试。
  等运行端可编译后，只复验受影响 3 组 `180258555Z`；原失败未覆盖。
- `180415575Z`：真实 Git 已产出合成结果，但测试传输 fixture 未实现 candidate-transfer POST，返回 404，命令退出失败；
  修正该 fixture 后仅真实合成组 `180505854Z` GREEN，再执行有隔离策略检查的 3 组 `180724241Z`。没有把失败命令称封存成功。
- Provider `181050078Z` 是夹具请求结构类型错误造成的编译失败；`181102962Z` 的真实继承对象包因旧 header 校验仍以 selected M 为锚点而 blocked。
  修正为 bundle B 及 B→selected→head 两段 lineage 后，仅该 Provider 组 `181121853Z` GREEN。
- `181847549Z`：CLI 首轮帮助与严格入站 RED，带引号的数字被当数字接纳。修正原值类型及整数词法检查、明确中文首绑字段后，`181916494Z` GREEN。
- UI 首轮失败及分组复验见上节；没有补造日志文件或改写为首轮全通过。
- `184107292Z`：新增继承验收链两子例均 RED，普通 C 的基线 M 没有进入审批来源遍历，M 原来源变化后仍错误允许新 Review。
  同一受限 walk 补入 durable run/node 基线、当前 protected binding、M 版本／来源／验收摘要，旧创建者不重鉴权；不套用领取时的 task version +1 规则。
  仅原新增组 `184307354Z` GREEN，既证明合法 accepted C 可交付，也证明来源漂移后拒绝新审批／交付且不改已有事实。
- 真实界面首段使用 command 根节点，UI 将其禁选，旧后端也确实只有 agent predicate；Root 最初判断后端已支持 command 是误判。
  没有把夹具改成 agent 绕过。`190124467Z` 中两个 command 正向子例 RED；仅补共同 command／agent predicate 后，`190144284Z` 原新增组 GREEN。
  旧构建不因此追溯宣称支持 command；UI 同步改为“代码产出节点”，新选择组和隔离网页通过，真实浏览器旅程待补。
- 网页最终构建的包管理器入口在执行脚本前被现有 metadata 拒绝；Root 改用同一已装 Node 直接运行脚本，未改依赖。
  该入口 `99263a` exit 1、`ERR_PNPM_VERIFY_DEPS_BEFORE_RUN`，脚本／Vite 尚未启动；之后同脚本与同一 Vite 正常完成。
  保留大于 500 kB chunk 警告，不以构建绿色宣称全平台性能目标通过。

## 已固定的构建产物

最终第三 cut 后端采用 `runtime-reviewed/build-facts.local.json`，SHA256 为 `b34c46c47ba0c5cd9d476ad8d2bf359b1b9b6d464e2f955590079cd43bd41575`。
旧 `runtime-final` 的 666 项／`6657c1c9…` 初 cut 与事实收据 `7e306e64…` 原样保留，不充最终产物。
`runtime-release` 的 667 项／`3674f32a…` 和收据 `6d74d9e9…` 保留为第二 cut。
下列是构建事实，不是实际旅程／发布回执；Go 版本为 1.26.10。

| 对象 | 固定摘要 |
|---|---|
| 668 个受控源项（含测试，不称 668 个编译文件） | `7932801ef1dec95a54541b6a00123147a5a4f0812714f5e04db8953e290aff3a` |
| Control | `17f95b5a190d3091eb0bd0ab51c38251af24d18805052ee613d548014359e528` |
| Runner | `039d883448c30044315d51617489c606198daf41a9bf46af5a5d6be64d926032` |
| CLI | `09875e7cf7b3649f8169da1eae4498a40131ada2f8e8aaf4278fecef89497d78` |
| Repository Provider | `2b6d0e2b748cf0a0910d720881a19397a5c6b19ef82e12cf611bd55c3329c884` |
| 原生 bundle | `5ab81bdf05e461b73bcc50b32dec92f477b24da288cbf30d27f41f716a5c30d5` |
| 初网页 `web-final/index.html`（135 文件／134 assets，非待更新最终网页） | `19a9d33a548cd2731c59459be93f37cdea910aa997309f8d6503bd0b403a5f93` |
| 第二网页 `web-release/index.html`（135 文件／134 assets，保留历史） | `7fb87c4fef8fc6e98c282bc47818538977275e998433f87cf6df1862be1e6ded` |
| 最终修正网页 `web-polished/index.html`（135 文件／134 assets） | `34ffa34af3a43f08d98ba4f14c0935d6dd5268a198e459e85c6fa23842398e84` |

Runner、CLI、Provider 与第二 cut 字节相同；初网页 135 文件／317 源项／4197 对词条证据保留。
command 修正后的网页收据 SHA256 为 `c14d32c7feac8b1a1e225c8c80a432b14fa1bf033599aa5a1e06b6dea4b7b4f2`；旧网页保持，不能替代新网页或真实旅程验收。
App key 修正后的 `web-polished` 收据为 `d507847f0a46f834b1fc899fe7c3e163cce642484cda9106907f856a47ab608d`，317 源项构建前后相同；前两网页 cut 均保留。
原生 TypeScript 输入逐文件相等，复用 030 原 bundle 与 manifest；没有重复原生 Node 构建、模型请求或厂商调用。

## 实际合成与基线开发

实际首段仅 setup。`actual/setup-evidence/inventory.local.json` 与 `closed.local.json` 固定其现场：
两个本次任务均 v1／todo／run_id 空，composition 为 0、未新增本次派发运行；不是说源库历史运行数为 0。
Control 与两个 Runner 均正常退出 0、无强制停止，数据库完整性 ok；源清单／辅助文件／旧业务行及克隆原非数据库文件保持。
未新增机器／API／Agent 凭据；setup 合法 import／enable 一个工具 profile、创建两个任务与两个固定流程。
setup 前后 browser sessions 为 16→18，来自 helper 与 UI 两次正常本地测试登录，不能写成本阶段从未登录；模型／厂商调用均为 0。
保留同一 DB、会话与 profile 续跑，不重建组织或重新派发；这段正常关闭不是实际合成通过。

修正后复用同一测试库与会话，真实两 Runner 分别承担 A 产出和 B 独立正式测试，位于同一 Windows 主机的两个独立仓库：

| 实际阶段 | 固定事实 |
|---|---|
| 合成 M | 计划 `a191e55babd552421a70d06fc522f949`；运行 `composition-a191e55babd552421a70d06fc522f949`；候选 `9bb999a604953b0c450500a47eed0bf618792188`；B 的 4 项正式检查成功；显式本地测试 owner Review 后集成任务 accepted v3 |
| 明确绑定 | protected binding `dd9335b311ab060b8e7fed03727bbb87` v1，固定目标 task v2；摘要 `2697b0a50ce671caa6638088755b701ff66beff26888a1d1cfb8c49757ca2653`；源为精确已验收 M；C 启动／验收后不可更改基线的提示符合 unstarted 门禁 |
| 开发 C | 派发计划 `94ebcb4d50528aa3dfec564ccba6963a`；运行 `97791b55a2387767f786207dec1267a4`；候选 `d4b9917671a1af165e6707a332cefe19b1ef4baf`；B 的另外 4 项正式检查成功；第二次显式本地测试 owner Review 后 C 任务 accepted v4 |

本次计 **2 个真实运行、8 项新正式检查（M4+C4）、2 次本地测试账号显式 Review**；029 原 8 项不相加，不称真实人类生产验收。
只读 Git 核对证明 M 的有序父为两个固定来源 A／B；C 的父为 M。C 的 selected base 为 M，管理员／对象包锚点仍为
`8cbc4344897335c2915efc9b763ad126f59d3bdd`，两个仓库 checkout 都保持该 B。M／C 的真实封存字节摘要、正式回执与当前 attempt／fence 完整对应。
C 包为 1817 bytes，SHA256 `3dc8a65d5b86f194ebe1a7b9b33b3d4ec67441a07a0d800f258e92100fe25b5f`；Harness 用量为 unreported，没有由缺失用量伪造 0 tokens。

resumed 前后 browser sessions **18→18**，没有重复登录、新 machine／API／Agent 凭据或模型／厂商调用。
原 Control 与两个 Runner 正常退出 0、无 force；关闭后仅用 immutable read-only SQLite 与只读 Git 对象／祖先查询收集，未 checkpoint、复制恢复或重跑测试。
数据库收集前后均为 `85d0ac35811061ea6672c39c3e06b17d4b905a54e0ffc7139af64f38c8f3f49d`，完整性 ok；385 条原业务行及另 418 条应保持的 setup 行逐行摘要复核通过，
仅两条本次 UI 任务行允许正常变化。源六文件／辅助文件及克隆原非数据库文件保持。

实际来源为 `actual/verification.local.json`（SHA256 `c7373bdf63fcbcc6ed955007c4f2489ff6314c1749e65fd2f7320878b60f5c06`）、
`actual/evidence-pins.local.json`（`b4a8ee4124d1ea5212973dd0e114590a082820b1e75e62e9b3d94b3b3c94b10f`）及 `actual/closed-final.local.json`。
本旅程未另启 Provider 做目标 CAS，也未另跑实际冲突任务；原 Provider 与真实冲突定向组证据分别保留，不能充作本次实际步骤。

## 最终冷回读与主预览

Root 使用最终 Control 与 `web-polished` 只启动 Control 冷回读：CN／EN、刷新、深链、键盘展开与焦点保持通过，设计来源始终 1 份、console 0，M／C／基线及 accepted v4 保持。
121 张所选领域与 DAG 表完整保持；回读 Control 正常退出 0，没有 Runner、重复正式测试、新登录、模型或厂商调用。
浏览器收据 `browser/verification.local.json` SHA256 为 `db227d4ad6ab250bb5ddeb37f88156679ef159b34e6dc99d7e71a7b7cab22fa8`；
闭合收据 `final-readback/out/closed.local.json` 为 `b124af03feb372eeb1007861b21ea0afb057d1c0825c68d8c4d74a4a66f6ab58`。
截图／原值文件由浏览器收据逐一 pin，代理临时页已关闭，用户原 tab 5 未动。

主预览已 `normal_ready`，Control `24588`、holder `244588`、执行句柄 `3078`。原 180 张表的完整 schema 与全部行一致，
仅加 3 张空表与 1 个索引至 183；原 18 条 authority、0 条 Grant 及 1 failed／2 succeeded 运行保持。
两个入口返回的网页及链接资源匹配最终制品；无 login、新凭据、grant、模型／厂商调用。这与测试库 setup 的 16→18 两次登录是不同范围。
最终主预览收据 `primary-upgrade/final-verification.local.json` SHA256 为 `01e161bc898148782280fb936f4dbfc954622f209fb52bc56403c1d01e5d9826`。

本报告 `stage_complete=true` 只表示上述 031 定向、同机实际、代表性中英界面与主预览更新范围完成。
真实厂商／远程物理机器、完整数据库／设备／容量矩阵、生产验收和全平台其他 P0–P7 目标继续实施，没有缩减范围。
