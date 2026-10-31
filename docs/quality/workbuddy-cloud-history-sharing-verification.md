# 原云观察明确团队共享：验证记录

阶段：`T-WORKBUDDY-HISTORY-SHARE-165`。产品版本：**1.0.0**。日期：2026-10-31。基线：`d96666983bc3fa444c026b3fe8c9cab67d60407f`。

状态：**定向检查及本机原App共享旅程的11项独立功能检查已核实；夹具失败与同环境续验分别保留。** 各次命令退出与功能结论分别记录，完整平台继续交付。主集成最终文档检查退出0（`29d725`）：481份Markdown本地链接、375条API路径及3464项Schema引用通过。机器记录见[精简摘要](workbuddy-cloud-history-sharing-summary.json)。

提交前又修复一项分页选择P2，当前Web定向累计41项，并增加1条独立纯组件浏览器检查。**原11项App／HTTP旅程、18项安全产物、81项gate与两图均属于分页修复前的40项Web输入；不改写为当前源码重新实跑。** 新组件检查不并入11项业务旅程。

## 1. 本阶段范围

原私有观察的当前 owner 明确选择最多8条受支持非空记录，指定同项目平台任务，预览精确原文、终止状态、`unverified`覆盖及目标任务可读范围，再明确共享为本人 human 讨论消息。正文按原次序用两个换行连接，不自动截断、翻译或改写。已共享副本按当前目标任务权限读取，不授予原私有load权限；原云连接撤销不自动收回副本。

预览没有正式写入或供应方调用。新共享在同一事务核验当前权限、owner、load／task版本及预览摘要，复用现讨论序号、消息、关注通知、固定活动与回执；来源关系失败须整体回滚。同原metadata／key恢复已提交消息，不因目标任务后来前进或旧私有来源失效重复发布。

依据：[冻结合同](../../contracts/workbuddy-cloud-history-sharing.md)、[领域架构与SOP](../architecture/workbuddy-cloud-history-sharing.md)、[交互设计](../design/workbuddy-cloud-history-sharing.md)。自动共享、知识／模型发布、Prompt、供应方助手代言、权限回答、远端停止、任务完成和研发验收均不属于本动作。

## 2. 定向验证与计数

本次使用仓库已有 Go 1.26.10、本地依赖与进程级缓存／临时目录。只复验改动行为及受影响的旧讨论合同；没有运行全 Go 套件、全 Web 套件或完整 Web 构建。以下是去重后的独立用例，不累计重跑、子断言、生成命令或准备性跳过。

| 检查 | 实际记录 | 计数边界 |
|---|---|---|
| 新后端／DTO | platform 9项＋DTO 3项通过 | 12项新用例；后续同HTTP用例不重复计 |
| Root旧讨论回归 | Append／提及／知识／关注10项通过 | 10项旧用例；原stdout非空且有真实测试事件 |
| 公共合同 | 最终3项通过：1新＋2旧 | 首次新1项与最终重叠，不额外计 |
| Go去重合计 | **25项：13新＋12旧** | 不声称一次完整全套通过 |
| Web | model 15＋controller 12＋selection 14，**41项** | 先38项＋两项P2精准GREEN2＝旧40；后验分页新增RED1／GREEN1，原13项selection跳过未重跑。两项旧P2的10项controller跳过记录保持 |
| TypeScript | 分页修复后退出0，工具记录`b32ada`（启动`fb8b7e`） | 修复前`233c8d`（启动`cac97a`）及初次`6fb534`保留历史，不累计为新测试用例 |
| 国际化 | **7823组成对词条，生产硬编码0**，`4cad55` | 只执行一次；两项UI修复未改变locale输入 |
| 合同生成 | 首次`97f483`，最终`455c72` | 两个安全错误遗漏sourceAliases登记后重新生成；命令不是测试用例 |

新后端原命令为 `go test ./internal/workbuddycloudcontract ./internal/platform -run '^Test(CloudHistorySharing|WorkBuddyCloudHistorySharing)' -count=1 -timeout=90s -json`。新范围通过后仅对既有 `TestWorkBuddyCloudHistorySharingHTTP…` 复验中／英400和409的状态、code、message_key、localized、locale及Content-Language；**没有重跑全部9＋3项**。opt-in实际宿主入口当时未启用，跳过不作为实际旅程通过。

新后端覆盖当前真人／owner／项目范围、预览零正式写和零外连、终止版本、受支持选择及已提交前缀、正文边界、来源和任务变化、同事务消息／来源／通知／回执回滚、原操作恢复、目标读者与私有load隔离，以及未知schema在写前拒绝。Web覆盖选择与目标、预览与范围、metadata-only原请求恢复、权限／来源失效清理、消息来源核对、畸形恢复journal、首帧受保护正文与提交中关闭行为。覆盖说明来自定向证据，不替代实际旅程。

## 3. 公开合同变化

最终生成结果为 **796个声明模型、797个OpenAPI Schema、375条路径**。新增5个DTO：`CloudHistorySharePreviewRequest`、`CloudHistoryShareRequest`、`CloudHistorySharePreview`、`CloudHistorySharedEntry`、`CloudHistorySharedObservation`；新增两个POST：`/workbuddy-cloud-history-loads/{id}/share-preview`、`/workbuddy-cloud-history-loads/{id}/shares`。

已有路径变化为0；已有Schema仅 `TaskConversationMessage` 增加可省略、非null的 `cloud_observation`。普通消息请求没有任意来源写入口。原文、原云私有请求摘要、vendor task_id、账号／票据和未选记录不通过安全来源元数据发布。

## 4. 真实失败、修复与审查顺序

- 后端先保留缺符号／未使用import等失败，再通过完整新范围检查。边界RED证实独立正文校验遗漏来源禁止控制字符、选择可越过load已提交前缀，两项均修复。
- 后续HTTP RED先发现使用通用错误code；改成明确400／409后，仍因本地化sourceAliases未登记失败。相关原日志均保留，即使文件名含green／final也不据名判定通过；最终四语言状态断言以 `backend-http-errors-registered.jsonl` 为准。
- 首轮独立后端／UI源码读审没有识别后述两项P2。作者审随后发现：①pending且选择失效时，首帧仍显示旧私有预览；②pending Modal无法收起。两项真实RED2后仅修改对应两文件，精准GREEN2；Root独立复审已关闭。**不把后续发现写成首轮审查已覆盖。**
- 畸形恢复journal的可见处理也有独立RED／GREEN，计入controller的12项，不另增总数。全部旧失败与早期38项记录保留，不回填成最终40项一次运行。
- 原11项业务旅程完成后、提交前作者发现新的分页P2：选择第51条，刷新返回前50条，再选第2条，旧选择正文不在当前页时会传入undefined并崩溃。只改 `WorkBuddyCloudCollaboration.tsx` 与 `CloudHistorySharingSelection.test.tsx`；新增第14条selection用例真实RED1／GREEN1，原13项跳过未重跑，独立复审关闭。最多8项原文字面选择仅在同load／version的受保护临时内存保留，失权／换域清除，不进入journal。原Go25／Web旧40／i18n7823未重跑；仅补最新TSC与下述独立组件浏览器检查。

## 5. 实际验证：11项限定功能核实

Root读取安全汇总及校验回执，并分别查看两张原始截图后，接受**11项独立功能检查：原驱动4项＋同一live环境续验7项**；未重复累计前4项。model、clone和actualPreview纯诊断不计入11项。实际运行使用原 `apps/web` 的按需Vite开发宿主和受控SDK准备，不是完整Web构建、正式桌面包安装或真实厂商模型效果。

本节保留修复前原40项输入的完整历史，三条相关场景未涉及分页可限定复用；后续分页修复仅由新回归和独立组件场景证明，不把旧81项SHA、两图或HTTP结果升级为新输入验证。

### 准备失败、精确来源与独立第二seed

首次隔离seed在正常follow准备中漏了必需的 `?project_id`，返回400；此时App／preview／share均0，受控Create与SDKLoad各调用1次。Go宿主0.41秒退出1；外层PowerShell返回0，但Node回执的exit_code为null，**不能补写成Node退出0或1**。无Vite／浏览器、descriptor或一致快照，原临时数据已由早期清理释放。首轮21项夹具证据文件（含源码、二进制与81项gate）已归档并逐摘要核对，归档清单SHA为 `877370ff66d19e551d65c816dde260a1447f6f6395b6555fa370ba3e8970ff23`。

之后仅修正3项授权输入：opt-in Go夹具复用现 `AuthenticateActor`／`subscriptionHTTP`，缓存PowerShell双流／真实ExitCode包装，Node退出事件回执。其余78项不变，产品实现改动0；没有重跑Go25／Web40／类型／i18n。81项特征／集成／夹具输入的合并摘要由 `cda9d84e5700b352e1d07eb337d1bc8e978fde1ecd546351031f1db7351dffdc` 变为 `b47c861296c03c63a41bf1372a50b256900e6eb1f8642c56a9561d3b86ba30f0`，最终81项在实际执行前后保持；不是整个仓库或全部编译依赖源码盘点。续验14项helper指纹另列 `continuation-inputs.json`，不混入81项。

两次test-host编译均退出0，二进制SHA依次为 `a9a20aa6791e36d2f4dbda4e8e45fbdd2cb6e5142dff4183e8e68ccb39c57e75`、`8bda8df5d3d48faf7d6f708200ddd2087329bf07802c8a5cc1b61aea98dfe3c2`。第二次是**新隔离seed**，再次受控Create／SDKLoad各1，不称作首次准备的原操作恢复。以下全部共享、恢复、读者与刷新检查才共同使用第二次的原数据库、load、两个窗口、metadata及原key。

### 实际共享与同操作恢复

| 事实 | 实际结果 |
|---|---|
| owner与选择 | 原App挂载已保存私有观察；明确选择序号1／2及准确平台任务，unsupported不可选，不自动选第一个任务 |
| 预览 | 精确字面正文、`unverified`和`project_task_readers`范围，默认未同意；正式写入0、准备之后外部调用0 |
| 首次共享 | 服务端201并提交1条消息／1条来源／1条读者通知，夹具按计划中断浏览器响应；journal仅保留metadata |
| 第二／三次传输 | 原metadata／原key重放；夹具 `route.fetch/fulfill`虽带有效JSON，浏览器观测status0，界面保持未知，不产生新实体 |
| 第四次传输 | 先按PID／exe／UTC核对原Node调试对象，仅通过公开 `Page.unroute` 移除唯一夹具路由；正常浏览器POST201，MessageDetail200，saved=true、pending=false、journal=0 |
| 原操作身份 | 四次传输只有一个operation，原始请求正文摘要一致；status0与正常201的响应正文SHA均为 `291614a02e83468176357ad52e6a1d0e651e398f38317fd870f324af3efdf00c`；没有换key盲发 |
| 正式结果 | 1条human消息、1条不可变安全来源、1条reader关注通知及1条关联；通知仍unread1，reader标读和其他写入均0 |
| 私有隔离 | 独立当前viewer可从通知打开共享讨论副本；原owner私有load仍404且不泄露私有文字 |
| 刷新与执行事实 | 刷新仍是同一副本／通知；run=running、node=waiting／`cloud.observation_unconfirmed`、run v3、attempt1／fence1、目标task v1；GUI provider／model／newRun增量均0 |

解除夹具路由没有修改产品代码或私有数据，也没有直接创建新POST；其后的“核对原共享”由原浏览器动作发起。原请求正文及key不进入安全交付；只保留摘要和同一性结论。已共享观察仍未核实，未把供应方文本提升为任务完成、测试或人工验收。

### 退出、快照与视觉边界

第二次主驱动在4项通过后等待MessageDetail超时，Node与外层均退出1，原记录保留。同环境helper完成另外7项及截图后，仅在close-wait调用不存在的 `Browser.waitForEvent` 时退出1；原关闭标记已写入。最终文件系统只读审计退出0，**另列为证据与收口校验，不把前两次exit1改写为一次端到端exit0**。

原生两个窗口均已关闭，存在before-quit／will-quit／exit0；Vite与Go宿主各退出0。主运行时未强制终止：`primary_runtimes_forced=false`。`force_stop_used=true`仅针对阻塞 `inspector.close` 的已核实官方Inspector CLI PID147324，准确exe为Node、创建UTC `2026-10-31T00:39:24.2723340Z`、命令 `inspect -p 162052`；不能声称所有自有进程均优雅退出。记录中的准确PIDs及9229／43993／43994监听均已不存在。

第二次在HTTP关闭后、helper清理前，以参数化 `VACUUM INTO` 保存私有一致快照：2,850,816字节，SHA `759d88dfd068fac1988473107ebee2d0edb82908b205681f1e56dcb14aa33e5b`；原数据库／临时目录随后不存在。快照保留在私有忽略区，未进入安全产物；首次失败未产生快照。

Root分别查看原图：中文1366×960浅色最终态的成功侧栏部分可见，英文390×844深色最终态的来源／覆盖／可读范围说明清晰；各自client宽与scroll宽相同。正常空闲后来源details可Enter开关、焦点已确认，触控目标44px。这是两个代表最终态与一次键盘检查，**不覆盖完整preview截图、全主题、全设备或完整可访问性验收**。

### 后验分页修复：独立纯组件浏览器检查

当前修复的两文件前后摘要由最新 `web-evidence.json.post_actual_fix` 保留；原40项证据另存 `web-evidence-before-pagination.json`，SHA `b9a39bc528968c25cf7d9f90bfcb14c77d5be09ae2ca80fe3eec166e27e5b7d2`。最新Web证据SHA为 `fb64126b577f1573102f56b787d36c5f13d086bcc5e546406cb3bd78e1997306`，不覆盖旧40项来源。

新场景使用真实当前Collaboration组件／context／CSS，页面内55条合成终态记录及受控GET响应。实际浏览器操作“更多 → 选51 → 读取已保存记录返回前50 → 选2”，最终选择 `[2,51]`、两份原文精确、invalid=false、undefined0；受控GET5、POST及其他非GET均0、pageerror0。**这是1条独立组件交互，不是第12条App／HTTP业务旅程**；没有Go服务、账号、Provider、SDK、模型、Runner、数据库或新Run。

初次直接esbuild构建退出0，但未转换 `import.meta.glob`，验证及Node退出1、功能case0；正常关闭并保留58文件hash归档、warning／pageerror／timeout与原输出。随后只改缓存构建文件，使用已安装Vite6.1的单fixture library／iife模式完成正常glob／CSS转换，不构建完整App。第二次build／verify／Node／native／server均退出0、case1；29项gate仅1项缓存变化，其余28相同。新source SHA为 `8f44ab8c19278d8261fc3f51ca566d11596d16f3f8242a8db3b5f6686f0016da`；实际1860模块文件的SHA闭包为 `ead8081e842e247e00fdba4ab3b548b189aecd6577d4feb58f9be043098e8978`，单列而非并入旧81项。记录中8个准确PID均不存在、43997无listener、force_stop_used=false。

Root已查看1366×960组件原图，确认第2／51条保留；截图SHA `aa98502f1e3986cc9d49f5ca5a7152912127ebd586aa28491879717c201bb613`。范围仍是单组件，不扩展为完整App或主题／设备矩阵。15项新安全产物独立放在 `dist/quality/workbuddy-history-share-165/pagination/`，manifest SHA `a9c92c4711a432844490c8a86091c23fceef6e8a57b23be4d360afbed5f4b05e`；原18项清单和 `674d25…` seal保持。新receipt／summary与模块指纹均有各自原始摘要，私有profile、原流与编译JS／CSS payload不进入该安全清单。

## 6. 证据位置与未覆盖范围

本次定向证据均位于忽略目录 `.cache/workbuddy-history-share-165/`：`backend-evidence.json`、`web-evidence.json`、`root-evidence.json`、`public-contract-drift.json`。Root公共合同／旧Append的原命令、非空stdout及摘要由root-evidence引用；原RED、局部GREEN、TSC和i18n日志保留。本页只归纳已存在记录，文档编写期间未启动测试、实例或Git操作。

安全交付位于 `dist/quality/workbuddy-history-share-165/`，18项清单产物的manifest SHA为 `674d25b5708b364d747afc03f81fbb5b221698af5cc9cb5fe7c0eb7d208b46e1`；包含实际汇总、11项校验、两次编译／命令、原驱动与续验回执、恢复／收口证据、来源指纹和两张原图。数据库／快照内容、cookie、raw key、原metadata正文、私有预览、Inspector原流／heap均排除。安全汇总和最终只读审计独立保留失败退出信息，不以截图代替事务事实。

完整31项要求、P0–P7、**58项限定／34项待办／92项总账**保持；`full_platform_complete=false`。本轮未运行全套／全Web构建，未验证真实厂商模型、物理多机、其他数据库／操作系统或生产。25项Go、当前41项Web、修复前11项业务功能及修复后1项纯组件交互各按独立输入／范围解释；文档检查结果仅回填一次，新增事实没有新Markdown链接或接口，不重跑。提交及远端一致性由阶段Git回执单独核对。
