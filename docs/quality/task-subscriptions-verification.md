# 当前人员任务关注：限定验证

阶段：**T-MULTICA-FOLLOW-154**；2026-10-30；suma 1.0.0。当前状态为 **Windows／SQLite／软件离屏实际交互限定通过**。完整平台58项限定实现／34项待交付／92项不变。[合同](../../contracts/task-subscriptions.md)、[产品](../product/task-subscriptions.md)、[架构](../architecture/task-subscriptions.md)、[交互与AI SOP](../design/task-subscriptions.md)、[English](../en/task-subscriptions.md)及[机器摘要](task-subscriptions-summary.json)共同记录范围。

本阶段交付当前账号自己的任务关注、原操作恢复，以及正式讨论消息向当前有效关注者的同事务通知；复用原个人收件箱准确消息定位。关注不改变任务负责人、派发、模型授权、已读状态或任务结论。

## 运行来源

实际候选以已提交153的 `ba007d2eeab8452361b3977f0357234e5c7dfcb9` 精确归档为基础，叠加独立152 v2准确源码及共享入口16行增量快照，再合入冻结154。154输入42个文件，152输入42个文件，重叠去重后合成79个输入；没有混入其他启动逻辑或TaskConversationKnowledge未提交WIP。未变源码由准确基线归档提供。

合成源码摘要为 `05fc56e46395b823adbda0da320e4eee3481cc219e6b3b8fe913c839993a3043`，算法是按路径排序的 `{path,sha256}` 数组紧凑JSON的SHA256。`synthesis.json`描述文件自身摘要为 `fbd34ad3b76c51e0e9589f09c2d32bc0a195ad45ffc05c594dd3696a1106058a`。79个候选文件逐项摘要与描述一致；实际control二进制为 `479ec1773d71610762c3960cc91263fe56de973322ab39dc143ea8bae0a5a262`，Web入口为 `ec7a484881f432372be2658a704203e95d7f9a74473e2d368c77cc3a84e9db8d`。

来源档案为 `.cache/workbuddy-152/root-integration/synthesis.json`、`stage152-source.json`及 `.cache/e2e/follow154/browser/candidate.json`。这些摘要描述当时编译及实际运行的输入；后续文档状态和入口收口另行映射，不反写历史描述文件。

## 检查分组与计数

| 分组 | 实际结果及去重口径 | 现有证据 |
| --- | --- | --- |
| Go领域及原回归 | 最初35个不同顶层：11个154新用例、24个原回归，另9个命名子场景；批处理新增2个，阶段共37个不同顶层／9子场景。增量5项中的3项及末次加强Mixed的1项均为复验，不再累加；不是最终源码下37项一次全跑 | `.cache/follow154-backend-evidence.json`、`follow154-batch-evidence.json`及对应JSONL |
| 合同生成器 | 3个不同顶层：2新、1原回归；共享message定位描述排序修正后仅复验原来那1项，包输出0.320秒，计数仍为3 | `.cache/follow154/root-go/*.command.json`及同名stdout |
| Web限定行为 | 7文件59项，其中12项新用例；模型3、按钮7、收件箱模型1、收件箱交互1，其余为必要原回归 | `.cache/follow154/ui/focused.log`及15文件`source-hashes.json` |
| 类型与双语 | 原限定类型检查通过；7627对语言键、零生产硬编码。合成候选再次检查，类型41684毫秒、双语2114毫秒，均exit0 | `.cache/follow154/ui/{types,i18n}.log`、`web-follow154-checks.json`及`follow154-i18n.stdout.txt` |
| 隔离Web构建 | 合成候选构建52476毫秒，exit0；保留大于500kB chunk的Vite提示，不把构建通过当性能验收 | `.cache/workbuddy-152/root-integration/follow154-build.{stdout,stderr}.txt` |
| 实际GUI／HTTP | 首轮16项不同检查，3张实际图片，renderer异常0，模型调用／执行运行均0 | [安全回执](../../dist/quality/task-subscriptions-154/receipt.json)、[交付清单](../../dist/quality/task-subscriptions-154/artifact-manifest.json) |

Go领域覆盖首次纯读、viewer、本人与凭据类型／跨项目隔离、CAS／noop／上限、原操作重放后的当前权限、当前全部有效关注者、精准提及优先、人／API自发消息排除、最终assistant完整来源、通知与原消息／回复回滚、重复事件和损坏来源。批处理Mixed用例包含两个任务的5个通知、人／API与两个正式managed assistant最终回复；独立损坏一个人工来源及一个assistant backing后，其余3个仍完整。这是领域测试，不冒充真实GUI或商业模型验收。

生成器stdout为778个model schemas／368个API paths；OpenAPI JSON实际包含779个components，其中1个为既有通用schema。与此前打印775／JSON776及366 paths相比，准确增量只有3个DTO、2个路由；既有收件箱2个操作描述、kind枚举及message_id描述共4处语义字段更新，没有删除旧通知来源。

## 已执行命令与环境

以下均为现有执行记录，不因本文收口重跑。Go使用Windows Go1.26.10、CGO=0、离线模块／构建缓存和进程局部D盘临时目录；精确可执行程序及生成器argv／退出码保存在`root-go/*.command.json`。领域命令来自执行代理原工具记录，输出原件为对应JSONL：

```text
go test ./internal/platform -run '^(TestTaskSubscription.*|TestNotificationInbox.*|TestTaskConversationMentions.*|TestTaskConversationReplyManagedTurnAndOriginalReceipt|TestTaskConversationReplyStaleCompletionAndCancellation|TestTaskHandoffInboxCreationAndOriginalReceipt|TestTaskHandoffInboxProjectionFailureRollsBackWholeSource|TestTaskReviewRequestsProjectionFailureRollsBackPendingReceiptAndSequence)$' -count=1 -timeout=90s -json
go test ./internal/platform -run '^(TestTaskSubscriptionPageHydratesAtMost51SourcesWithOneBatchRead|TestTaskSubscriptionMixedPageDoesNotCrossHydrateDamagedSource|TestTaskSubscriptionHumanProjectionMentionPriorityAndPrivateSource|TestTaskSubscriptionAssistantFinalBindingAndAtomicDelivery|TestTaskSubscriptionDuplicateProjectionAndRecipientWithdrawal)$' -count=1 -timeout=60s -json
go test ./internal/platform -run '^TestTaskSubscriptionMixedPageDoesNotCrossHydrateDamagedSource$' -count=1 -timeout=45s -json
go test ./cmd/suma-contracts -run '^(TestTaskSubscription|TestNotificationInboxContract)' -count=1 -json -timeout=60s
go test ./cmd/suma-contracts -run '^TestTaskSubscriptionContractRetains' -count=1 -json -timeout=60s
go run ./cmd/suma-contracts
go run ./cmd/suma-contracts --check
npm test -- --run src/task-subscription-model.test.ts src/TaskSubscriptionControl.test.tsx src/notification-inbox-model.test.ts src/NotificationInbox.test.tsx src/TaskConversation.test.tsx src/TaskConversationMentions.test.tsx src/TaskConversationKnowledgeFlow.test.tsx
node .cache/e2e/follow154/browser/verify.cjs
```

合成候选实际执行的`tsc --noEmit`、`check-i18n.mjs`及`vite build --outDir .../web-follow154 --configLoader runner`完整argv、cwd和耗时见`web-follow154-checks.json`。前端既有未知写恢复及固定资料回归产生React act提示；59项均通过，提示未隐去，也不据此称零测试告警。

实际GUI命令exit0，工具输出标识`68c47c`，元数据后补记录在 `.cache/e2e/follow154/browser/command-receipt.json`，`runtime-summary.json`保留原字节恢复与Electron正常关闭事实；原stdout／stderr未落盘，字段为null，没有重建原日志。Electron子进程、profile、session、缓存、crash及日志明确置于D盘fixture；顶层Node／Playwright命令没有显式覆盖继承TEMP／TMP，执行时值未留档，后查宿主为C盘Temp，因此不声称全部GUI临时数据均在D盘，也不声称宿主未知临时产物已经清理。原3份运行脚本及摘要已保存至`browser/executed-v1/manifest.json`，随后仅修正未来fixture启动器在加载Playwright前设D盘临时目录；没有重跑或改写原运行回执／图片。

## 实际Windows交互

独立SQLite服务与Electron35.7.5生产隔离preload运行于自有localhost。正式setup、组织邀请／真实接受及viewer授权建立两个正常账号，规范人员资源与两个任务均由正式HTTP创建，直接数据库写入0。窗口使用软件离屏，renderer没有Node；不等于可见原生窗口或安装包验收。

viewer键盘明确关注得到v1，另一账号正式提交原消息后准确收到一个task_followed；打开原消息并聚焦没有自动标为已读。明确取消保留旧通知来源，后续消息不再通知；重新关注后的精准提及只生成task_mentioned。实际共3条消息、2个通知。

第4个偏好命令在真实上游提交200后中断给页面的回执。当前GET即使符合目标，也没有解除原操作未知；用户明确核对保留原body／key，5次传输仅对应4个命令，最终v4，没有重复递增。切换第二任务没有安装另一份偏好或发送新命令；当前HTTP撤权后原来源拒绝且重开的页面不含受保护讨论。所有变更均在原任务与当前viewer身份内。

首轮实际旅程没有UI失败或renderer异常。全程执行运行0、模型调用0；最终assistant只由上述领域测试验证，本旅程没有伪造assistant消息或请求模型。三张实际图片已目视且字节数／SHA256与交付清单一致：

| 实际图片 | 观察范围 | SHA256 |
| --- | --- | --- |
| [中文1366关注及原消息](../../dist/quality/task-subscriptions-154/followed-zh-1366.png) | 简洁已关注按钮、准确原消息 | `d12ce600327cf4cc61ec86cff007df7409a7046b1e5525c38db9fb7326494c5f` |
| [英文390收件箱](../../dist/quality/task-subscriptions-154/inbox-en-390.png) | 提及与关注两条未读通知，打开与已读分开 | `56feb875b77993f780043df20bdb67ad8b63df20666ed4f5e78ffd3f05d945ce` |
| [英文390原操作恢复后](../../dist/quality/task-subscriptions-154/recovered-en-390.png) | 取消状态与可见键盘焦点 | `1b3860def278246e599d4255f33c80e091aef00073abae2a355c005e205e45ac` |

## 保留的红绿事实

新增真实SQLite driver检查最初exit1／0.902秒，报告`reverse=51 message=51`；50项加1个游标哨兵需要102条逐项来源SQL。随后改为最多51个ID的一次私有反向／偏好／消息join，人／API不再逐消息查询；新增检查与相关复验通过。**102→1只指该来源读取，不是全部授权／收件箱API查询，更不是assistant backing查询数。** assistant按task分组、消息去重后沿原完整metadata检查，只有来源校验失败时才有界拆分以隔离坏项。红阶段仅保存在原工具输出，没有磁盘原日志；绿阶段原JSONL保留。

生成器共享message_id描述曾被后续mention合同覆盖；顺序修正后加强原用例，仅复验该1项并重新生成。该修复不新增用例计数。上述真实失败与修复没有导致重发GUI操作或重建实际旅程。

## 交付与剩余边界

只交付安全回执、产物清单和3张合成业务图片；不交付账号、cookie、邀请token、数据库、原请求、操作日志或其他私有状态。安全回执及3图的字节数／SHA256均已独立核对；本报告整理未重跑测试、构建或GUI。

本阶段限Windows、独立SQLite、软件离屏的代表性交互。真实厂商／商业模型、其他数据库服务器、macOS／Linux、可见OS窗口、安装包、物理多机、规模及生产未验；本阶段不使用系统目录选择器，也不沿用153的OS存储证据充当154验收。完整通知通道、邮件、自动关注规则和平台余项继续原计划，58／34／92不变。

## 独立阶段提交

为避免152的独立窄屏验收阻塞154交付，本阶段以`ba007d2`加154自身代码单独提交。154核心源码与上述真实运行输入逐项相同；五处共享文件仅移除152独立初始化／HTTP／生成器接线。独立合同为767个声明模型、768个Schema组件、358条路径，递归核对为原合成合同的完整子集。独立TypeScript检查通过（32262毫秒），原个人CAS及两项合同检查共3项复验通过，不增加测试数量；没有重复完整行为、GUI或Vite构建。原合成源码、control、Web及16项实际回执完整保留，不把独立编译冒充另一轮界面旅程。文本行尾按仓库规范归一化，文档状态收口另记，不反写原运行摘要。
