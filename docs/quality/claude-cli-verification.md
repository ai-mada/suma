# Claude Code CLI：定向验证与待验边界

版本1.0.0；阶段T-CLAUDE-CLI-158；日期2026-10-30。状态：协议、SQLite领域、公共合同及组件定向检查通过，冻结组合UI构建已通过（42.50秒），控制面和Runner分别完成Windows编译。GUI、正式Worker.Run／HTTP旅程及真实厂商验证均为0；不作为完整平台或生产验收。关联[执行合同](../../contracts/claude-cli.md)与[机器摘要](claude-cli-summary.json)。完整58 limited／34 pending／92总项不变。

## 本阶段行为

新增`claude-code-cli`／`claude`／`claude-jsonl-v1`前台执行入口，固定已审阅的2.1.314版本及实际version／help门禁。它与CodeBuddy、Claude MCP客户端及仍planned的Claude SDK入口分开。固定模型只作为管理员CLI配置和执行时确认事实，不映射平台模型选择、参数硬控或模型网关。

复用现有Worker、DAG／Pool、Profile、journal、隔离工作树、候选传输、独立测试和人工验收，不新增第二个job或状态机。Claude配置在计划、原节点、当前库存及执行证据之间准确匹配；配置漂移在写工作树前拒绝。组织或项目任一enforce预算均阻止预览、Start、claim及账本Check／Reserve；observe只保留未定价、partial／unreported事实，不填零或认证厂商账单。

权限沿管理员明确配置；任务和模型不能替换参数、环境、MCP、权限策略或任意resume。safe-mode／restricted及目录限制不代表完整OS隔离，managed policy仍优先，内建文件限制不能充当Bash的OS沙箱证明。未知、后台、排队、交互或未确认终态不能冒充整体完成。新提交、clean工作树和独立候选测试仍是各自事实，Claude不自认证验收。

## 检查计数与范围

| 检查 | 不同用例或实际结果 | 范围 |
| --- | --- | --- |
| 新增Go顶层 | 15：后端8、Runner6、root公共合同1 | 按不同顶层去重，分批相关检查，不是一次完整套件全绿 |
| 已有Go回归 | 4：后端相关旧回归2、已有本地化检查2 | 单列，不计入15个新增；重复公共合同检查不再计数 |
| 后端子场景 | 11个命名子场景 | 两级预算各阶段拒绝6、配置漂移5；不加到Go顶层计数 |
| Node | 1个新增用例 | 桌面入口检查；native Go用例内的Node候选测试不另重复计数 |
| Web | 4个不同新增用例 | 组件／模型检查，不是浏览器GUI验收；重复运行不增加数量 |
| 公共合同生成 | 退出0；当前总量785个schema、370条路径 | 这是生成物总量，不是本阶段新增数量 |
| 完整类型 | 初次退出2，夹具补齐已有必需字段后最终退出0 | 没有放宽API；最终类型检查不代替组合Vite构建 |
| 国际化 | 7705对词条、生产硬编码0 | 静态参数／词条检查，不代表已目视中英文界面 |
| Runner构建 | 1次成功 | 二进制构建，不是启动正式Worker.Run旅程 |
| 组合UI构建 | 退出0，42.50秒 | 冻结源9ac0d457…6403d；index SHA见摘要；不等于GUI通过 |
| 控制面构建 | 1次成功 | 来自同一冻结源，不包含其他启动修复WIP；仅编译 |

后端为Windows／SQLite领域定向测试，使用受控数据与测试装配，未开展真实HTTP服务或正式Worker.Run旅程。其8个新增顶层与11个子场景不能替代实际CLI、HTTP或生产运行证据。未运行完整历史套件或重复无关旧检查。

## 原生受控协议执行

`TestClaudeCLI158NativeWorktreeAndIndependentCandidateTest`最终单项在明确`-timeout=45s`下退出0，包耗时9.152秒。它调用正式`Runner.Execute`并运行自有原生协议可执行文件，覆盖version／help、固定模型漂移零工作树执行、隔离的真实提交、准确候选的独立Node测试、未知结果和Windows自有进程取消。

该文件模拟已审阅的公开CLI帧，未调用真实Claude。它不是`Worker.Run`，没有正式领取／心跳／结果HTTP旅程，没有厂商登录或模型调用。Windows进程停止事实也不外推到其他操作系统。

本机查找记录覆盖59个绝对PATH目录及明确的常见安装位置，未发现Claude；未执行真实Claude的version命令、安装、登录或厂商请求。实际GUI检查0、正式Worker.Run／HTTP检查0、真实厂商调用0。

## 中间失败与修正

- 后端初次缺少新接口的红阶段仅有实际工具输出，未落盘的日志不补造。随后一次领域组合在测试夹具中保留事务后读取计数，耗尽90秒；修正为先关闭事务。另一Pool夹具把空流程域的首版写为version3，改为正确version1。只复验受影响组，没有放宽产品权限或预算规则。
- Runner早期夹具有未定义helper及未trim的commit；真实漂移回归先发现错误固定模型可以进入工作树，随后在创建可写工作树前增加固定配置门禁。该修正与后面的夹具修正分别保留。
- 一次六顶层影子组合耗尽默认Go时限，原日志记有`ran too long (11m0s)`；该次不算通过，也不据此认定产品死锁已定位。之后原生单项受明确时限验证，不把单项通过改写为六顶层组合全绿。
- 中间独立测试夹具复用了已耗用的执行上下文，并出现工作树根未被Git忽略的错误。按阶段独立限时、调整夹具有界性后，仅复验受影响原生用例，最终9.152秒通过。
- 集成类型检查初次退出2：新options夹具漏了已有必需的`codex_supported`／`codex_reason`。只补明确的false／空字符串，最终退出0；不是新增API字段，也没有因此重复计数四个Web用例。

## 来源与可追溯性

本报告整合三份ignored本地记录：`.cache/claude-cli-158/backend-evidence.json`、`runner-ui-evidence.json`和`root-integration-evidence.json`。它们分别记录后端16文件源组SHA256 `a417ad4cbd95e94ff3b7be0773b8e1908193e08f0fe3bbb506e19541f40fd41b`、Runner／UI 35文件源组SHA256 `f4a20ffc62af8174451ec7683e91004da710a3bbbd12bc2d1587c614e3560568`及root集成结果；两个源组的组合算法不同，不伪造统一已验收候选摘要。具体输入哈希见机器摘要。

Go使用本地1.26.10、离线模块缓存、CGO_ENABLED=0及进程局部D盘`.cache`临时目录。影子Go-overlay仅改变编译器视图，不等于主树源码读取、生成器或完整接口验收；后续公共合同生成与类型／词条结果由root记录单列。Node／Web原始磁盘日志在该记录中为空，保留实际工具回执引用，不伪造完整argv或不存在的日志。

最终原生单项metadata为`.cache/claude-cli-158/go/20261030T123517969Z.command.json`，对应stdout／stderr同名文件；命令范围是`go test -overlay … ./internal/runner -run '^TestClaudeCLI158NativeWorktreeAndIndependentCandidateTest$' -count=1 -timeout=45s`。Runner产物为`.cache/claude-cli-158/bin/suma-runner.exe`，SHA256 `44a2826536195f291576cafdd0924365c145f72d0af5bde7eb110c09e64e5bad`。本报告编写没有新增测试、构建、克隆或Git操作。

## 待验范围

真实浏览器中英／窄屏／键盘检查、正式Worker.Run与HTTP链、真实Claude安装／登录／供应方模型、其他数据库、跨平台进程控制、多物理机、安装包和生产继续待验。既有工作树／候选／JUnit能力复用不等于这些完整真实旅程已经验收；不宣称硬预算、远端停止、人工验收或发布完成。Cloud ACP设计保持未来未实现状态。完整P0–P7及58／34／92范围继续保留。

最终组合源SHA256 `9ac0d4578851ddcb37d6aca19b967adab12c5b9c6b715aa0c14118eb4cc6403d`，界面index SHA256 `735e0bd788b217a9831272562980066ef1f780f7e35f575fd36d63bb3068fdd1`；控制面SHA256 `f5f83b56b4ca362e3119ecce7ccaa1beb21998310fdbabeca9b625ac01da5097`。最终完整类型在共享技能生命周期修正后再次退出0；未重复上述Go／Node／Web套件。Go控制面、Runner与Web为各自目标的一次构建。
