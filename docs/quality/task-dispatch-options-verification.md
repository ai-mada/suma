# 工具推荐与文件保存恢复：限定验证

状态：T-P4-034／1.0.0；源码、定向检查、隔离实际界面、15项最终冷态核对及主预览保持验证通过。完整P0–P7与需求1–22保持实施中，非生产验收。

关联：[PRD](../product/task-dispatch-options.md)、[架构](../architecture/intelligent-capability-selection-notes.md)、[交互](../design/task-dispatch-options.md)、[AI SOP](../operations/task-dispatch-options.md)、[合同](../../contracts/task-dispatch-options.md)、[English](../en/task-dispatch-options.md)、[精简摘要](task-dispatch-options-summary.json)。负责人：平台集成。

## 完成行为

- 浏览器与CLI共享当前授权的工具选项服务。读取固定任务、流程版本、节点和执行路由，复用现有工具库存及正式派发编译器；游标绑定完整工程依据。
- 派发弹窗按需展开推荐，显示本页实际兼容与受阻原因；采用只填写原工具选择，保留真实版本及摘要。原预览、Start、Claim、独立测试和人工验收继续各自核验。
- IDE保存回执须匹配准确路径、提交内容和UTF-8 SHA-256。无效200仍保留原操作；较晚403／409不解释更早未知结果；恢复成功后保留后来输入的新草稿。

## 必要自动检查

环境：Windows、本地Go1.26.10、已安装Vite／Vitest3.0.5／TypeScript／Monaco。没有安装新依赖。

| 范围 | 记录与结果 |
| --- | --- |
| 领域 | 3项初始分组及最终依据游标分组通过；覆盖只读、准确任务／流程／节点版本、暂停、路由／流程不兼容、分页及无新增计划／运行／用量 |
| HTTP／CLI | `TestTaskDispatchOptionsHTTPCurrentScopedReadAndExactQuery`、`TestTaskDispatchOptionsCLIScopedRead`通过；严格参数、重复／未知字段、版本与项目范围、伪造游标、无效CLI不请求后端 |
| 工具界面 | 4项定向检查通过，含恶意／错范围响应、分页、权限、迟到、语言切换和真实新档案v3覆盖列表同ID旧v2 |
| 保存 | 6项受影响检查通过；原请求／原调用者、后续拒绝、较新草稿、恢复后新key、精确CRLF／Unicode字节、缺失hash能力及原冲突语义 |
| 合同／静态 | 同源OpenAPI与TypeScript生成及`--check`通过；受影响Go vet、最终TypeScript通过；4363条中英文配对和占位符／复数／markup检查通过 |
| 构建 | Go四程序各构建一次；439项生产Go／原生TS输入摘要`d6c8dc4bc4ce931d03189be252eaa578c3cc30be510819abacbed6f9fbd0450d`。前端一次构建，351项输入摘要`a4d855f0143f80f127b5b5b66f8c15639307a82983727ea761ef85eaf07bd893`，135项输出；构建前后源码及活动033静态产物不变 |
| 复用 | 原生SDK11项直接输入及161项实际闭包输入逐字节相同，复用已验证产物；没有重复模型调用、历史来源开发、正式候选测试或完整测试套件 |

定向命令及退出码：`.cache/mcp019-deps/20261027T002549147Z.command.json`、`20261027T002823571Z.command.json`、`20261027T002921214Z.command.json`、`20261027T003122896Z.command.json`、`20261027T003123739Z.command.json`。界面工具输出转录（非重跑）：`.cache/e2e/task-dispatch-options-stage-034/verification/ui-agent-focused-checks.local.json`；保存检查：`.cache/e2e/ide-save-recovery-stage-034/verification.local.json`。

## 实际界面与文件

使用已闭合033数据的独立SQLite副本、原测试会话、独立Runner A库存；没有新登录、凭证、模型服务或正式任务测试。Control在59208，回执故障代理仅监听loopback59209，且只准许本阶段拥有的`ide-proof.txt`。

1. 新待办任务`8b16500641bbee62b1f29623eb37d989`选择流程`bfe12bf2e48238d6724c56d0e12560ae@1`与`code`节点。实际返回14项工具；A当前兼容，B及其余离线／不匹配工具显示真实原因。
2. 中文及英文展示页内推荐、观测时间、路由、模型固定、费用未知及正式门禁说明。采用后原工具选择为`4134de1028e7f0278d813629f7553e41@2`，摘要`406f503c788e731eb220f0279492e5868a9683ebbdb43ce0036f838958363267`。
3. 明确预览产生计划`0eba4f3983263b7812b69d335a20e1a8@1`，刷新保持准确记录；预留编号`fa870725060dea28d79e7652b4641654`不是正式运行。没有点击Start或人工验收。
4. 在真实界面绑定本阶段专属工作区，真实Monaco编辑并保存一次。代理仅在后端真实提交后，将准确200回执替换成错误文件200；界面显示中英文错误与恢复入口，未显示保存成功。
5. 继续输入v2草稿，再恢复原保存；v2仍保持脏状态。最后明确保存v2，界面中英文显示已保存。实际三次HTTP请求准确对应两项持久文件操作；原恢复key／body相同，新保存使用新key与v1真实hash。

界面证据：`.cache/e2e/task-dispatch-options-stage-034/browser/verification.local.json`及对应截图。HTTP／文件持久核对与进程闭合记录在同阶段`actual/`、`ide-actual/`中，摘要只收必要事实，缓存、数据库、日志、凭证、截图和二进制不入Git。

## 失败记录与剩余边界

首次Control因本地`SUMA_PUBLIC_URL`配置不满足既有校验而退出1；原数据与隔离数据库保持，尚无业务写入。保留原失败证据后，仅修正本地验证配置并明确重新启动，没有修改产品校验或重建产物。

当前推荐是本页按稳定ID排列的首个兼容工具，不是跨页最优策略；可写工具模型固定、费用未知，不证明额度充足或机器空闲。原Start／Claim路径复用既有证据，本阶段没有重新执行它们。保存恢复限定当前浏览器会话；完整PTY／LSP／DAP、编辑器内置菜单的中文化、跨关闭恢复、真实厂商、物理多机、分布式数据库与规模验收继续实施。参考项目的全能力差距见[矩阵](../product/platform-capability-delivery-matrix.md)。

## 最终闭合与主预览

隔离Control239192、Runner238644正常退出0，代理正常退出且Root工具回报0；持有进程因核对脚本将合法流程发布误算作执行而退出1，原始`closed.local.json`的false保留。最终`final-reviewed-closure.local.json`在已闭合数据库上以immutable读取，15项全通过：仅新增1项合法workflow-publish操作回执，未创建任何运行／节点／尝试／执行事件／用量；原业务选中行及schema、来源033全部文件保持，2项真实文件操作／新key／基线hash和最终内容准确。没有为修正证据而重放功能旅程。

一次来源只读连接曾新建空WAL和SHM；确认所有来源连接关闭、主数据库摘要不变且原清单无这些文件后，仅将本次自建辅助文件连同摘要保全移走。最终来源数据库仍为`5d55064183aa76dbb4d7bfc551cc42223eddd5b5ebaaef86226565a26c3c83ff`且辅助文件状态准确恢复；未改原件或回写数据库。

主预览034正常运行；033旧服务正常退出，186表完整schema与行在升级前后准确保持，无新schema。原会话中英／键盘／刷新核对后，182张非锁表保持，4张内部锁表34行仅sequence各增加28；18authority／0Grant／18session及原1failed／2succeeded不变。135本地资产摘要准确，实际抽查页面入口与JS两项，未重复抓取全部资产。

原主预览已验收v3任务的选项读取正确拒绝新开发范围，中英文明确提示经变更需求建立新范围；没有新增计划或运行。正向推荐／采用／预览在隔离副本独立证明。主预览Control248172／holder211636保持正常，地址`http://pool.localhost:4324/`；只读验收不是生产部署批准。

最终证据及摘要：`.cache/e2e/task-dispatch-options-stage-034/actual/final-reviewed-closure.local.json` SHA`ea1f1a8fc8396c67efcc5d2fa68463cca35bfc27be722ddcaf23793ea17214d6`；`primary-upgrade/warm-verification.local.json` SHA`7bd4f528f76076f3bfbf4d46e60843c6c93f7080c635a54d1ff59fdb70ca0a2a`。完整必要机器摘要见[summary](task-dispatch-options-summary.json)，全部临时证据留在忽略目录。
