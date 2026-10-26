# T-P4-020 原生编码阶段验证

状态：T-P4-020限定本地纵向已验证，完整P0–P7继续实施。Owner：主集成。关联：[PRD](../product/native-coding.md)、[架构与AI SOP](../agents/native-coding.md)、[交互规范](../design/native-coding.md)、[合同](../../contracts/native-coding.md)。020主预览已恢复正常，原数据与原登录保持。

已交付源码：六项attempt工具、官方Go／TS MCP、单文件依赖bundle与摘要、操作回执、开发检查、宿主封存、正式验收补充门禁及紧凑中英事实界面。此列表不表示所有实际验收已经完成。

当前必要切面：Workspace模块11顶层／24事件通过；随后路径策略只复验3组。Seal完整域切面10顶层／27事件通过，原Windows symlink权限跳过已保留，随后真实junction替代和3组partial恢复分别通过。TS原切面43通过；追加工作区专用诊断后只验证21项及重建bundle，旧46语义golden保持，目录现50。前端44项／3文件、类型、3207双语词条通过。各切面有重叠，不累加。

Root宿主集成3项通过，覆盖.mjs实际库存、已知无变更封存、取消不能未封存成功及native摘要先绑定raw evidence hash。独立复核发现并修复.mjs探测漏改和取消／正常退出竞争，两处均不能以模型completed作为成功。

工具链仍Go1.26.10；SDK Go1.8.0与TS2.4.0。当前TS bundle 967987字节、SHA256 `d63d596e9bf379e10cec0a9dd9416abfa5478ebec1f15375c021fb962f334b69`；Node内置外置导入限定crypto/module/readline。原始日志与当前source/构建输入在忽略目录 `.cache/e2e/native-coding-stage-020`，不进入Git。

完整Go集成门禁初次退出1，所列失败均已修复并定向通过；未重复未受影响的完整切面。下述实际旅程与GUI是限定本地协议夹具的独立证据，各切面分别记录。真实厂商模型、完整操作系统沙箱、目标分支合并、跨平台安装、SSO／A2A、分布式存储及规模目标仍单独验收。

集成门禁初次完整Go切面：1637通过事件、0测试skip；失败为两个生成合同测试仍固定46个native键，以及Seal属性夹具把新增受保护.gitattributes作为模型可写效果。原始退出1和全部失败保留；两处计数按新50键定向通过，原46内容golden仍保持。属性夹具改为baseline commit／prepare前置，普通源码效果不含Git策略文件，未放宽生产保护。仅复验 `TestNativeWorkspaceSealGitAttributesAndIgnoredFilesRemainExplicit`：1顶层／4子例（LF、CRLF、external-filter、accepted-ignored），共5个通过事件、0失败／0跳过、exit0；不是5个顶层。回执为 `.cache/mcp019-deps/20261026T025710376Z.{command.json,stdout.txt,stderr.txt}`。没有重跑全Go。

实际助手前置阶段已保留三次失败：公开HTTP别名不符合可信端点规则、Record成员user_id应在data中、helper形参label与业务label冲突；均未执行模型或代码任务。失败控制面退出和原数据单独留存；修助手不算产品能力通过，也不修改生产逻辑来迁就夹具。下一次先静态核对payload，优先恢复同一已初始化fixture，避免重复创建和秘密旋转。

实际第4次已进入原生attempt：run `746dca4b5c67cba4d95964050d385719` 在模型前以 `workspace_transport` 已知失败；0模型请求、无源码文件效果，现场已确认干净，旧运行时和该失败记录保留。该次未完成开发、检查、封存、独立测试或GUI评审，不计为后续成功运行。

真实跨SDK最小探针随后确认原因：Go初始化200、initialized202、GET405、tools/list200，官方TS2.4.0为 `connected=true/catalog=false`；wire路径 `result.tools.2.inputSchema.required` 为null，SDK校验 `invalid_type`，路径为 `tools/2/inputSchema/required`。RED回执 `.cache/mcp019-deps/20261026T030548786Z.{command.json,stdout.txt,stderr.txt}` 保留，测试退出1；探针仅初始化和读取目录，不调用工具、模型或实际fixture。

修复仅在Go `nativeWorkspaceSchemas` 的object helper：无required项时省略字段，有项时才写数组；不再把nil slice发为 `required:null`。TS解析、SDK、依赖和bundle均未放宽或重建。永久跨SDK回归保留在 `native_workspace_interop_test.go`；同探针GREEN为 `connected=true/catalog=true`、null路径与校验issues均空。随后原Go目录的2025-11-25／2026-07-28双协议同次通过，合计2顶层／4通过事件、0失败／0跳过、exit0；回执 `.cache/mcp019-deps/20261026T030614594Z.{command.json,stdout.txt,stderr.txt}`。该切面仅确认目录互操作，不代替实际开发、独立测试或GUI评审。

修复后在同一SQLite fixture执行新run `f1f312e091ed9a7f6781f1b0eb7e9a73`，状态succeeded；旧失败run独立保留。本地确定性OpenAI兼容协议夹具完成5轮请求、9次工具调用，覆盖6类工作区工具，形成4项实际文件效果、generation4；固定开发检查四条断言先失败4项、修复后通过4项。模型边界是本地协议夹具，不是真实厂商模型；文件、固定检查、Git封存及bundle均实际执行。候选为 `fd172cf7cbdc6ac18e6b24f3869170f592d5e4db`，B启动前缺该对象，实际导入同一SHA后完成正式独立4测试、0失败／0错误。不能把开发自测替代这次独立测试。

B助手构造启动argv时曾因 `origin` 未定义触发NameError，失败回执保留；随后只恢复B，在同一DB／同一run继续，A及模型均未重启、凭据未重签发，候选SHA保持，不称一次无失败通过。A／B累计34条进展及2条结果均已ACK；两个Runner排空后graceful退出0。`actual/result.local.json` 的23项harness检查均为true，浏览器评审前任务仍为in_progress v3，尚非accepted。

实际证据位于 `.cache/e2e/native-coding-stage-020/actual/`。`native-source-proof-summary.local.json` 固定原生raw SHA256 `c9871e5d34cf966e3d38fc7da9109da087b34d88c0af66bab9ceacb91b3f7075` 与原工作区回执SHA256 `f8d9a7a51ed290f3c999107a22c4077a117c0f5a8b19a4b05e5ea3149f9937d3`，并保留ledger、封存回执摘要及开发／正式测试来源，不能以界面投影重算替换原件。

GUI前次会话的一次CDP调用阻塞3697秒；随后控制面正常退出，页面缓存与断线状态不能证明当时仍有live服务。失败截图、阻塞与断线记录保留，仅支持当时观察范围，不能作为最终成功证据。

Root随后完成GUI-only恢复，并对新live服务重新读取：0条alerts；B正式test页面的候选SHA与原候选一致，显示4项正式测试。新live英文深色与中文390px页面、Enter展开原始依据／检查输出均已实际验证；26项原始值与英文逐项精确一致，原文焦点及选中run／attempt保持，390px无页面横向溢出。英文Review表单已明确提交，rationale注明本地fixture及非业务／生产验收；该评审由Codex操作测试owner完成。任务变为accepted v4，中文刷新后保持。

本次GUI正证为 `browser/verification.local.json`、`native-en-dark-live.png`、`native-zh-390-live.png`、`task-accepted-zh-live.png`，取代此前缓存截图作为限定范围成功依据；旧失败记录不删除。`临时16` 已清除，metrics已关闭。上述结果只证明本次夹具、候选、测试与测试账号评审，不代表业务负责人签收、真实厂商模型、完整平台或生产验收。最终关闭和主预览更新证据见下文。

实际环境最终关闭：GUI控制面120836与holder232836均通过原生句柄确认退出0，未强制终止；SQLite integrity为ok。最终只读摘要16项均通过：旧已知失败运行和新成功运行／证据保持，模型trace未变化，仍为5次fixture请求／9次工具调用；任务只发生预期的accepted v4及唯一测试Owner Review。原始依据留在忽略目录 `actual/final-actual-summary.local.json`，未创建新的Runner、模型执行或凭据。主预览最终状态见下文。

主预览最终已恢复020 revision2正常模式：控制面234308、holder236712；147表完整schema一致，143表全行内容一致，另4张既有协调表只发生相同主键下的sequence单调递增。18条authority、0 Grant及原失败1／成功2运行保持；原账号在单独临时页直接读取知识空间，未重新登录或发放凭据，用户原标签页未操作。旧程序／网页／WAL感知备份保留。

首次正常升级后的Python别名验证因系统DNS `getaddrinfo11001` 失败，保护流程正常关闭新控制面并保持新版本维护冻结；原冻结holder按既定状态退出1，控制面退出0，分别记录。随后仅在同库同程序上前向恢复，以数字loopback连接配合原HTTP Host验证两个入口HTML／资产字节，未改机器DNS或恢复旧数据库。新正常实例启动前已持久记录写入边界，使用独立停止协议；不能把该辅助验证失败隐去或记作所有进程退出0。

最终vet与生成合同-check通过，274 Schema／174路径／1518引用；3207双语词条已检查。隔离构建及现有未变输入复用，不重复全套。精简可提交证据见[机器摘要](native-coding-summary.json)，原始日志／数据库／截图／bundle／秘密只留忽略目录。已完成的范围是本页列明的本地原生编码闭环；未宣称真实厂商模型、目标分支合并、生产及完整平台交付。
