# 冲突修复与受控恢复：限定本地验证

阶段 T-P4-032；产品版本 1.0.0；日期 2026-10-27。**本阶段限定本地功能验证完成，`stage_complete=true`；准确提交以 Git 历史与本地提交收据为准。**
准确冲突、独立修复任务、同运行显式恢复、独立候选测试／Review、未开始任务基线，以及最终中英冷回读和主预览前向升级均有闭合证据。
完整 P0–P7 继续 `in_progress`；厂商、跨物理机器和生产验收未因此完成。
见[领域合同](../../contracts/candidate-conflict-repair.md)、[机器摘要及收据摘要](candidate-conflict-repair-summary.json)。

## 实际结果

同机 Control 与两个独立 Runner 共创建 **3 个新运行、4 次代码 attempt**：Source X 成功；原合成保留已知真实 Git 冲突及失败；唯一修复运行经明确 retry 后成功。
Source X 与修复候选各有另一 Runner 执行的 4 项正式断言，共 **8 项**，并由本地夹具测试 owner 分别明确 Review，共 **2 次**。运行成功、正式测试和 Review 分别记录。

| 事实 | 最终证据 |
|---|---|
| Source X | `6b7182020da1991602635e3364e057fc`；accepted v3；head `106096d78824765331284508a53472d8085163c0` |
| 原合成 | `composition-87c15c2bb9e23a923694972990f6a2d8`；code attempt1 已知冲突失败，无封存候选；原件与唯一修复回链保持 |
| 独立修复 | `composition-fe7f83a76fc1df5e64ee96d7e83c2956`；attempt1 在工作区／模型前因 `unsupported agent purpose` 失败，原结果 SHA `ef9e490235815aca278482b305572383ac1aa88306e9988811191993427532f1` 保留；同运行显式 retry 的 attempt2／fence2 成功 |
| 精确候选 | head `21cc51598692c6121e7324f382757f45cd0a1e9e`；tree `9c1660c5225d01f8c642a1ce1344e381a5034267`；独立测试只执行一次，再明确 Review，repair task accepted v3 |
| 下游 D | `3efda5d5b61423eb8425b0e47a08068d` v2，protected baseline v1 指向上述 accepted repair／attempt2／fence2；仍 todo、无 run；没有重复 D 开发或测试 |

实际最终收据 `actual/recovered/closed-final.local.json` 为 true、integrity ok：Control 246720、Runner A 253380、B 251820 均正常退出 0，无强停。
原 loopback 模型进程 251676（exec 49659）正常退出 0，**实际 6 次请求、6 次工具调用、0 次失败**，未重启；厂商调用 0。
这是自有本地协议夹具，不是厂商推理质量或账单；usage 为 `unreported`，Token／费用未知，不当作零。

## 定向检查与边界

顶层组、子例及重叠复验不相加为阶段总数。下列收据均位于 `.cache/e2e/candidate-conflict-repair-stage-032/`；命令号位于 `.cache/mcp019-deps/`。
平台／Pool 切面使用真实 HTTP、SQLite、Core、授权及字节事务配合类型化执行夹具；实际 Git／模型旅程另行记录。

| 切面 | 通过范围 | 收据 |
|---|---|---|
| 领域 | 8 个原新增顶层：丰富准确失败、原件保持、创建回滚／并发单任务、当前来源／凭据、reserved task、候选／Review／基线、显式刷新及稳定冲突身份；受影响合成 20 组含重叠旧例 | `domain/verification.local.json`；`domain/stable-conflict-identity.local.json` |
| Pool | 1 顶层／2 子例：完整 grant／credential／dispatch／lease；固定来源字节；错 binding／scope／fence 拒绝；grant 撤回阻止新读；漂移后真实失败／完成事实及单次重放保持 | `domain/pool-transport.local.json` |
| 原生 Git／SDK | 16 个真实来源、2 次冲突、15 个 merge commit、B-only 精确候选测试；CAS／marker／原操作恢复、三来源 clean prefix 重放；官方 MCP 桥类型检查、3 个 SDK 定向例。45 工具调用／46 请求属于该独立 SDK 协议夹具 | `runtime-verification.local.json` |
| 封存边界 | 专用修复 previous 为真实完整 merge prefix；无后续编辑为 `no_change`／0，有编辑为 prefix 的 sealed child。原生 2 顶层＋Pool 1 顶层通过；结构证明仍须 durable exact execution 授权，未绑定证明 403 | `domain/native-seal-seam.local.json`；`runtime-seal-state-delta.local.json` |
| 受管恢复 | 稳定身份保证原 task version 改变仍回到唯一子计划，完整 basis SHA 继续控制新使用。1 HTTP 顶层／5 子例证明同运行 retry、旧 attempt 保留、新 fence 与准确质量；非幂等、未知、漂移及无关发布入口仍拒绝 | `domain/managed-run-commands.local.json` |
| purpose／真实 wire | Go 专用原生 purpose 路由 1 正向＋12 拒绝；消敏实际响应及受影响 UI 14 项通过，strategy／basis／tool 仍严格匹配 | `runtime-purpose-dispatch-delta.local.json`；`verification/web-release-guard-fix.local.json` |
| HTTP／CLI／capability | 初始 HTTP/private 1 顶层、CLI 2 顶层、tool capability 1 顶层；最终 CLI 范围例 `210727273Z` 通过。标题／描述／criteria 上限为 200／16000／16×1000 | `202120377Z`／`202120416Z`／`202120395Z`；`verification/root-contract-delta.local.json` |
| 可选字段 | 省略 priority 在验证和幂等摘要前归一为 medium；非法值 400 且零残留，同 key 显式 medium 重放原件，1 新领域组通过。priority／owner 可省略，显式 null 拒绝；仅受影响原 HTTP 组复验通过 | `domain/optional-priority.local.json`；`215108959Z` |
| 最终构建 | Go 四程序构建及最终合同检查退出 0；web-delivery 类型检查和一次隔离构建通过，4269 对中英词条按相同输入复用通过记录；大 chunk 警告保留 | `runtime-delivery/build-facts.local.json`；`215228642Z`／`215232520Z`／`215233535Z`／`215234705Z`／`215236199Z`；`verification/web-delivery-build.local.json` |

保留的 RED 记录包括 reserved task 直接绑定绕过、未知 workflow 原始 SQL 错误、过期刷新、原生 seal 假定 previous=B、稳定身份丢失、真实 wire guard、purpose 路由与受管命令误拦。
修复各由对应定向例闭合；没有删除失败或把原件改成成功。NLS 两个 closed key 生成失败后明确允许；native snapshot／diagnostic 的旧 53 项断言改为真实 55 项后仅复验相关组。
稳定身份回归前的磁盘满发生于链接，未执行测试；仅 Root 按清单清理编译缓存，审计 `verification/compiler-cache-recovery.local.json` 保留。

## 会计与原事实保持

第一段原始 `actual/closed.local.json` 的 `checks_passed=false` 保留：旧“所有业务行不变”守卫拒绝真实会计聚合变化。
`actual/phase-one-audit.local.json` 精确推导两条原组织／项目 usage account 的 version 15→17、unresolved 1→2，spent 80／reserved 0 保持。
第二段 `actual/resumed/closed-final.local.json` 的 false 和 financial delta null 也保留，当时修复／测试／Review 旅程未完成。

最终审计精确核对同两条原聚合为 **version 15→21、unresolved 1→4**，spent 80／reserved 0 不变。
新增 3 条真实 `unreported` attempt 为 X1、执行前失败的 repair1、repair2，均有一次原结果事实；没有忽略整个账本。
其余原业务行／schema、来源 031 的八文件／辅助／业务、原权限及会话保持；不声称所有旧行从未改变。
第一次准备的旧“六文件”假设在复制／启动前纠正为当前封闭 031 的八文件，来源 DB 摘要 `23901dca66434e9112e3c8564a2eb5806d893ebd18631b0a148d18e99abdca8d`，未沿用历史 85d 起点。

## 产物、冷回读与主预览

实际执行使用 `runtime-resolved` 的 688 项源摘要 `b8a0ae196e46b937c23a440b862d959aafda681c2f48d5156b368c729d53d2f1`。
最终冷回读与主预览使用 **runtime-delivery** 的 688 项源摘要 `3114a23dd25d6ba42df20db1b1bb53ac1f3de48b076d037337a187741b04e148`，
Control `1e1f735a6a06152f9bf0f18d9484c63bf766c7904d7c903e8dd1499c7afe072a`。
差异仅为可选字段默认／null 门禁与生成合同，实际显式 medium／owner 路径不变；1 领域与 1 受影响 HTTP 组证明此差异，未冒称以新 cut 重跑实际模型／Runner／正式测试。
`verification/root-final-cold-review.local.json` 记录两个 cut；完整程序 SHA 见机器摘要。

原生 bundle `1145e77d94e0dec12ee146d1565c77d207700699e8f0d7ab17d57492a6f3f4c0`、manifest `771aa33b8b63d94f77f05cf80e0ad84cd80d2c082214e682aa229a8d5bda7ae7` 保持。
11 个源输入／161 个 closure 输入未变，晚期 Go 修复不重建 JS、不重复 16 来源／SDK 夹具。
最终 **web-delivery** 有 328 项源，摘要 `42a406edb8a3e37899bc6d73c1ffc32e58d4b6348408a6181950ffcbce214c50`；135 个产物与 web-reviewed 逐字一致，
index `8a802f340ba22297b6d555acfa42e850c0752c14c6b67fefb541067d759b7a08`，4269 对词条；生成 TS 变化后执行了一次必要隔离构建。

`final-readback-recovered/out/closed.local.json` 为 true：Control 256088 正常退出 0；无 Runner、模型、新登录／凭据或正式复测，当前全部业务 schema／行及账户完全保持。
中英、键盘展开原证据、刷新、原失败与唯一修复回链、成功 attempt2／fence2、精确测试／Review／D 基线均回读通过；无未捕获错误。

主预览 031 正常退出 0，前向升级到 032：183→184 表，仅新增空 `suma_candidate_composition_repair_sources`，无新增显式索引；**warm 浏览器前原 183 表完整 schema／行均相同**。
最终 normal Control **233736**、holder **212508**、exec **31937** 持续服务；18 authority、0 Grant、原会话及原 1 failed／2 succeeded 运行保持，无重新登录／凭据签发。
warm 中英／刷新后，原全行守卫失败及诊断保留：仅 4 张当前授权锁表的 **34 行 sequence 各 +22**；所有身份列、原 183 表 schema、其余 179 表完整行保持。
没有回写计数器或恢复数据库；不能把 warm 后表述为全部 183 表行未变。精确只读证明见 `primary-upgrade/warm-browser-verification.local.json`。

本阶段保留管理员原 `Idempotent` 与 Core 效果未知门禁；不自动重试、不新建无界修复任务。
完整计划／工具重选修订、更广自动恢复、厂商质量／账单、真实远端、多物理主机、生产及完整 P0–P7 仍待继续交付。
