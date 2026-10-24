# 受控后端诊断国际化阶段验收

状态：T-P4-009本机阶段已验证；完整平台P0–P7实施中。负责人：主集成代理、控制／执行／发布领域代理、覆盖门禁代理与独立审查代理。关联需求：FND-I18N-001。基线为`ebba5868118576934ddec9101b1ec93ffb7e6ce5`。接口依据为[语言与消息合同](../../contracts/localization.md)、[语言偏好合同](../../contracts/locale-preferences.md)，设计见[受控诊断架构](../architecture/localized-diagnostics.md)，完整交付范围仍以[实施计划](../delivery/implementation-plan.md)为准。

本报告分别记录领域源码、显示投影、门禁、固定输入测试、产物一致性和实际界面证据。受控中文错误构造的门禁通过，不代表后端所有诊断、全站双语或生产验收已经完成。原始证据均位于忽略的`.cache/e2e/localization-stage-009/`，下文短路径均相对于该目录。

## 实现范围与计数依据

新增447个完整静态原文及10个参数化键，后端目录由123增至580键；每个定义包含`zh-CN`和`en-US`。领域目录使用具体语义键，不以HTTP通用`invalid_input`、前缀、相似字句或机器错误码替代具体原因。

| 领域 | 新增完整静态原文 | 源码与职责 |
| --- | --- | --- |
| 控制 | 143 | `internal/localization/catalog_control.go`；身份、治理、规划、记录、完成门禁及版本拒绝 |
| 执行 | 158 | `internal/localization/catalog_execution.go`；执行、自动化、工具、派发、调度及事件流校验 |
| 制品与发布 | 146 | `internal/localization/catalog_release.go`；制品依据、候选、人工验收、发布计划、授权、观察、未知副作用及核对 |
| 参数化生产者 | 10键、12处 | `internal/localization/catalog_validation.go`及六个领域生产文件；完整source、稳定键及声明参数共同确定显示依据 |

上一阶段409是受限正则盘点的唯一原文候选，包含6个动态片段候选。重新按AST区分后为403个完整静态原文；纳入原盘点遗漏的`releaseError`／`releaseConflict`消息参数再增加44个，得到447。不能将409与447相减解释成遗漏数量，也不能把动态前缀登记成完整句子。

独立逐项比对在45个生产文件中核对474处原静态字面量表达式：完整中文原文与冻结分配逐字相同，缺失、额外、重复键、重复原文及英文缺失／含汉字项均为0。447是新增唯一完整原文数，474是这些原文在原分配中的出现数；最终AST门禁的546个唯一静态原文包含既有目录，580则还包含参数模板及不在该门禁构造内的目录定义，三者不能互换。

`catalog.go`通过`combineCatalogs`复制组合领域目录，避免共享底层切片被追加改写；初始化验证稳定键、原文歧义及模板合同。没有可变的进程全局语言，没有运行时外部翻译或新增依赖。英文措辞保留原业务条件中的当前权限、固定版本／来源／测试身份、未知副作用、核对屏障以及执行、独立测试、人工验收和实际部署之间的区别。

## 参数化原因与HTTP显示边界

原有12处动态中文诊断位于`automation.go`、`planning.go`、`release_deploy.go`、`release_deploy_plans.go`、`tool_inventory.go`及`tool_profiles.go`。生产者改为显式`invalidDiagnostic(key, source, params)`，仍生成原完整诊断；没有通过字符串切分猜测字段或翻译外部错误明细。

| 稳定键 | 参数 | 规范中文完整模板与出现依据 |
| --- | --- | --- |
| `validation.field_required` | `field: identifier` | `缺少{{.field}}`；规划和发布版本字段各1处 |
| `validation.field_integer` | `field: identifier` | `{{.field}}必须为整数`；规划和发布版本字段各1处 |
| `validation.field_version_integer` | `field: identifier` | `{{.field}}必须为有效版本整数` |
| `validation.field_safe_integer` | `field: identifier` | `{{.field}}必须为安全非负整数` |
| `validation.field_string` | `field: identifier` | `{{.field}}必须为字符串` |
| `validation.schedule_field_required` | `field: identifier` | `排期必须包含完整字段：{{.field}}` |
| `validation.automation_detail` | `detail: text` | `自动化字段或类型无效: {{.detail}}` |
| `validation.release_detail` | `detail: text` | `发布请求存在未知字段或错误类型: {{.detail}}` |
| `validation.tool_inventory_detail` | `detail: text` | `工具探测数据无效：{{.detail}}` |
| `validation.tool_profile_detail` | `detail: text` | `固定工具档案无效：{{.detail}}` |

`internal/platform/diagnostic_error.go`中的`diagnosticAPIError`保存一个精确APIError及私有参数快照；`Error()`返回原文，`Unwrap()`保持原cause，调用方之后修改原参数映射不能重新标记已产生的错误。参数仍受统一类型合同约束，不回显额外、缺失或非法显示参数。

`http.go`的`writeError`先确定真实APIError及传输状态，`localization_http.go`的`localizedAPIError`再投影显示。接受注解必须同时满足：注解cause就是最终选中的同一个APIError；键存在且参数验证成功；以`zh-CN`模板和该参数渲染的完整文本与原`APIError.Message`完全相等。随后才按当前请求语言输出`message_key`、`message_params`、`locale`、`message`及`localized`。语言头、显示参数及目录版本不参与授权、业务正文、幂等摘要或持久回执。

真实RED发现：格式合法但含义无关的`validation.field_required`曾能把另一条完整诊断显示为“expected_version is required.”，掩盖实际原因。`annotation-binding-red.log`保存有效失败；修复增加完整规范source绑定，最终用例覆盖参数快照、不同joined cause、source不匹配、额外参数和非法参数。该修复不能只表述为“参数验证通过”。

注解无效后继续查找精确完整原文；未知完整原文保留原文并标记`localized=false`、`fallback=source_untranslated`，不输出未验证参数。只有原文为空时才允许通用code映射。HTTP状态、机器`code`、`details`中的false／零值／未知结果、组织／项目身份、固定版本、哈希、签名、原操作标识和原始错误不变。JSON解码器与工具返回的`detail`是原始诊断数据，参数模板接入不等于这些明细已翻译。

相关公开入口包括`GET /api/v1/localization/catalog`，鉴权与记录接口，自动化`/api/v1/automations/*`，派发`/api/v1/task-dispatch-plans`，调度`/api/v1/scheduling/policy-previews`，事件流`/api/v1/runs/{id}/events/stream`，以及`/api/v1/release-candidates`、`/api/v1/release-workers`、`/api/v1/release-plans/*`。这些入口沿用原有权限和业务合同；本阶段增加其错误显示依据，不授予额外权限或放宽拒写。

## 有边界的源码覆盖门禁

`internal/localization/platform_coverage_test.go`使用标准Go parser／AST检查`internal/platform`全部83个非测试Go文件，包含各build variant。识别`invalid`、`conflict`、`errf`、标准库`errors.New`／`fmt.Errorf`及重命名导入、位置／具名`APIError.Message`、`releaseError`第3参数和`releaseConflict`第2参数。字面量、括号、加法和词法绑定／跨文件字符串常量须解析为完整原文；动态中文须使用已注册的具体参数键及完全一致的字面参数名。

| 最终聚合指标 | 实际结果 |
| --- | --- |
| 受检生产文件／诊断位置 | 83／658 |
| 静态中文出现／唯一完整原文 | 589／546 |
| 已注解动态出现／唯一参数键 | 12／10 |
| 本门禁内未映射静态中文／未注解动态中文问题 | 0／0 |
| 明确排除的非中文静态／原始动态位置 | 18／39，均不登记为已翻译 |
| 合成解析样例／通过事件 | 39／41；含两个顶层测试，退出0、0失败／跳过 |
| 固定输入 | 105项前后一致：83个平台生产文件、20个localization文件及`go.mod`／`go.sum` |

生产文件范围摘要为`ded9fac911da0f8b20d123400d6f2ed57a7db17357207b2a0c302e88cca6bf33`；最终门禁文件SHA256为`f83191f80dba6c3f5bb8202a4120395313d88756ff2a66ccb9c005a5f62943d2`。证据为`gate/verification.json`、`gate/final-result.json`、`gate/final.log`及前后输入、摘要收据。

门禁有三轮有效RED→GREEN，均保留原日志。首轮34个合成样例让故意接受遗漏的初始检查器失败，覆盖未知静态、片段、模板和错误参数位置；第二轮新增2例证明有效注解不能掩盖未知完整静态原文或前缀；独立审查再发现两类P2：同名build-variant包常量可能被覆盖选择，以及selector／方法名可能错误解析为无关包常量。新增3例先失败，再修为引用歧义常量时明确拒绝、selector和字段声明不冒充常量。最终39例全部通过，审查记录`gate/review-summary.json`关闭这两类P2；该复核只读源码与既有日志，没有声称独立重跑全量。

该门禁不做一般变量数据流或类型检查，不覆盖任意其他wrapper、其他Go包、未进入约定错误构造的展示字段、英文静态文案、外部`err.Error()`、用户内容或保存日志。0门禁问题只证明上述构造中的中文诊断已有明确归属，不能写为“后端国际化全部完成”。

## 并发HTTP与不可变业务证据

新增领域测试通过真实loopback HTTP、生产handler／中间件和临时SQLite执行；同时存在明确标识的合成错误投影用例。固定流程、构建／测试／人工验收和运行输出使用测试fixture，不代表真实厂商Runner、付费模型或业务负责人签收。

| 回归入口与证据 | 实际验证边界 |
| --- | --- |
| 控制域；`control/red-http.log`、`control/green-focused.log` | 24次并发双语请求验证无测试／人工验收的完成拒绝与过期版本更新拒绝，400／409及具体code保持；任务版本、任务／计划／活动／操作数量不变。用户故意填写与目录相同的标题、描述仍保持原文 |
| 执行域；`execution/red.log`、`execution/final-targeted.jsonl` | 24次并发草稿规则触发继续409拒绝，不新增run或occurrence，规则version／revision／state保持；分页、派发、调度和SSE具体400原因、公共Webhook无会话401签名拒绝保持 |
| 原始执行输出 | fixture保存的Runner output／error即使与目录原文相同，HTTP读取和数据库仍逐字保持，原run身份与固定workflow版本不变；未知后缀及details中的零／未知值保留 |
| 发布域；`release/red.log`、`release/final-targeted.log` | 24次并发非法执行节点请求保持请求语言隔离；候选、执行节点与计划校验继续拒写，不新增凭证、回执、运行、部署或授权 |
| 固定发布历史与回执 | 过期计划批准继续409；固定计划SHA256、原审批原因、两条审批历史及operation response原始字节逐项保持。已发生或未知副作用不能变成取消成功 |
| 参数化解码；`dynamic-red.log`、`dynamic-final-green.log` | 12次并发自动化／发布预览严格解码拒绝保持400／invalid_input，原解码明细与参数一致且不回显原请求payload；不新增automation、release plan或run |

三个领域初始RED均在保留预期状态／code的基础上，实际失败于缺失目录、legacy键或`source_untranslated`断言。发布另有`release/semantic-red.log`，先复现三处英文约束不够准确：人工验收属于同一次运行、已接受的副作用不能获得新授权、provider不可达时环境仍保留占用；修订英文后通过，未改中文或业务判断。

## 自动化切面与独立审查

各定向结果与最终完整结果分开记录，不重复相加。`review-backend.json`分别记录旧46个事件的领域复验、最终14个事件的注解边界增审，以及新41个事件的AST门禁增审；三次切面不重复累加为全量。门禁另有独立子审查及最终完整Go覆盖。

| 检查 | 实际结果与输入依据 |
| --- | --- |
| 领域定向 | 控制新增4个顶层测试、8通过事件；执行新增7个顶层测试，最终含相关回归27通过事件；发布新增8个顶层测试，另有7个既有回归／13事件。原命令、环境和日志分别留在各领域目录 |
| 后端独立审查旧切面 | 规范source加固前，21个顶层测试／46通过事件、2包、0失败／跳过，24个选定输入不变；`review/targeted.jsonl`与`review/input-manifest.json` |
| 后端独立审查最终边界切面 | cause及规范source绑定后，3个顶层测试／14通过事件、1包、0失败／跳过，2个选定输入不变；`review/boundary-targeted.jsonl`与对应manifest。46和14不是同次最终60事件 |
| 最终完整Go与vet | 12:58:51–13:03:11 UTC，`go test -p 1 -json ./... -count=1 -timeout=6m`退出0，904通过事件、0失败、0测试跳过；`go vet ./...`退出0。315输入前后一致，`source_unchanged_since_start=true`，见`go-verification.json`；11个测试包通过、4个无测试命令包编译，后者不计作测试跳过 |
| 最终完整Web／类型／非Web | 13:01:33–13:02:17 UTC，Web461项／41文件、0失败／待测；工作区类型退出0；桌面7、原生智能体20、知识3共30项通过，0失败／跳过。193输入前后一致，见`web-verification.json`、`web-tests.json`及对应日志 |
| 网页目录与合同 | 两语言各2798键，语言／插值／复数／标签检查通过；旧模块候选0、覆盖硬编码0，含义仍以008范围为准。`contracts-check.log`确认212模型、152路径及TypeScript wire types保持一致 |
| 最终生产Web构建 | 13:02:44–13:03:43 UTC退出0，最终193输入不变。入口`index-BY49gZhR.js` SHA256 `8035c3e60993c448149d49f480380cd1cd4cc68abbb34503c43258d6a94f6a31`；CSS `index-DUAGU50N.css` SHA256 `b13cc5714628648f00aff39083b3625a41deca5b01d28a48945e80e31d2322ed`，见`web-build-verification.json` |

首轮完整Go虽为901通过事件、0失败／测试跳过、vet0，但运行期间门禁测试源经独立审查后变化，315输入的`source_unchanged_since_start=false`；必须保留为`pre-gate-review-go-verification.json`的旧切面，不能用它证明最终源码。最终904事件来自重新冻结后的完整运行，没有与旧901叠加。最终原始Go JSONL摘要为`0c3099cec4e051d120936125c19e07d515a71e65b3de401e39492c3d9cdf1635`。

登录辅助入口样式修改前，旧193输入的Web461／非Web30、类型和构建也通过；`before-auth-spacing-*`保留该切面。上表结果来自CSS修订后的重新验证。`emptyOutDir:false`仍可能保留旧忽略资产，当前HTML入口收据不等于干净部署包；Monaco／workflow大块告警保留，不代表性能已验收。

## 运行产物与实际HTTP读回

六目标批次`20261024T125200490Z-8392a126`包含Windows／Linux／macOS各amd64、arm64，每目标控制面、Runner、Deployer、CLI四程序，共24文件。184个非测试运行源码与`go.mod`／`go.sum`输入聚合摘要为`01aed16b911f629330b2812b77b5a814d7c5bd845291e911d886384df7fe4eb5`，当前文件、保存source清单、主清单与内嵌清单记录一致，0不匹配。

独立只读审计`runtime-audit.json`读取24个实际PE32+／ELF64／Mach-O64文件头、架构、长度、SHA256及宿主Go buildinfo，核对36个链接模块并集和目标归属。再次重算本地8,441个模块目录文件、36个原ZIP及go.mod的h1，与本地ziphash、`go.sum`和pin一致；无replacement、无目录链接、未联网核实sumdb。原构建收据位于`dist/runtime/builds/20261024T125200490Z-8392a126/`。

| Windows amd64产物 | SHA256与实际边界 |
| --- | --- |
| 控制面 | `a6cd75fe375e4891e0f8b0522ee48bfeffd251326a4699222d2edf54543f540d`；已核对原预览进程后激活于`127.0.0.1:4324`，`control-launch.local.json`记录新PID与实际路径 |
| Runner | `5e920312e5a5edbf1efadf5c4e753ad0bb49cf1c5981fa89bcb9fa040195bf33`；本阶段未重启Runner，未新增厂商执行 |

源码摘要保存在清单中，并非二进制内签名的源码证明。产物一致性与交叉编译不等于各目标系统实际安装、启动、签名、远端部署或生产验收。门禁文件属于测试源码，不进入运行二进制，故184运行输入与315测试输入分别核对。

`live-readback.json`记录对实际重建控制面的6个双语请求：登录错误各1次401，自动化／发布预览严格解码错误各2次400；具体code、key、params、`Content-Language`及`localized=true`一致。公开目录实际读回580键，版本`2f9f004046903ef34534423b6426e505f4bc2d4b7cbb120a2825d1b1ecfd0881`。该读回0成功业务变更、0厂商执行，测试登录与非法预览不冒充真实生产操作。

初始读回脚本只发送`Accept-Language: zh-CN`，却得到已认证账户保存偏好所选的英文。`live-readback-initial-mismatch.json`保留这一观察；这是脚本忽略既定“手动→账户→组织→请求”优先级的假设错误，不能登记成生产缺陷。显式请求选择通过`X-Suma-Language`表达后，最终6请求通过；没有修改账户／组织偏好来迎合脚本。

## 最终实际界面与数据保持

实际浏览器在独立本地origin以测试输入触发一次登录拒绝，先显示英文具体错误，再切换中文，同一错误立即显示“邮箱或密码不正确”。`login-denial-en-full.ax.txt`与`login-denial-zh-full.ax.txt`保存完整AX，字段已脱敏；该结果验证已有错误按目录重显，不靠重复登录写入更新文字。初轮截图为`screenshots/login-denial-en-default.jpg`。

实际英文界面同时发现“创建空间”和“接受组织邀请”两个辅助入口挤在一行；主集成只在`apps/web/src/style.css`对`.auth-card>.text-button`增加分行、至少44px高度及长文本换行。该修改已进入上表最终193输入与新Web构建，初轮旧截图作为问题依据保留，最终效果使用新构建的DOM和截图另外验证。

| 最终实际回验事项 | 当前登记 |
| --- | --- |
| 新入口与CSS实际加载 | 实际DOM加载`index-BY49gZhR.js`与`index-DUAGU50N.css`，对应上表最终构建；`ui-facts.json`与`login-final-en-full.ax.txt`、`screenshots/login-final-en-1280.jpg`记录新版本 |
| 登录拒绝中英文即时重显 | 最终构建以不存在的测试邮箱发起401拒绝，保留同一错误，切中文／英文立即显示具体原因，输入邮箱保持；不是重新登录取得新错误。实际使用公开580键目录，浏览器诊断捕获0条警告／错误 |
| 实际视口、辅助入口尺寸与溢出 | 本轮只验证1280×720和390×844；最终中英文两辅助按钮均实测44px高，分行且不重叠，页面scrollWidth不超过innerWidth。390窄屏Tab从创建入口到接受邀请，长译文保持可操作；未复验768／1366／1920或全部主题。截图API可裁去滚动条，JPEG像素不替代CSS视口 |
| 原预览库逐表摘要与保护表 | 当前两次实际一致SQLite备份均integrity_check=ok，129表逐表行内容摘要全部相同，包括62张执行／制品保护表；无新增业务写入、执行或厂商调用。实际新阶段结果记录在before／after-ui快照与摘要，未复用008结论 |

本阶段没有声称完成全部页面／主题／键盘／语言交叉矩阵、所有授权拒绝界面、完整WCAG或全旅程验收。HTTP用例的全表计数、固定回执与原始证据保持属于各自临时fixture；必须与实际预览库的前后核对分开登记。

独立复核发现`ui-facts.json`有一项`emailRetained=false`，原值完整保留。补采只在新表单填写合成邮箱，不输入密码、不提交：`email-probe.json`显示英文切换前和中文切换后，DOM观察值均为`<redacted>`，因此与已知文字比较的断言本身无效。独立审查逐张查看`email-probe-before-en.jpg`与`email-probe-after-zh.jpg`，确认同一邮箱文字和输入焦点实际保持。这里区分无效采样与单独的视觉证据，不将原false静默改成通过，也不因工具脱敏修改产品。

## 可复现入口与保存规则

在仓库根目录使用Go1.24.5 Windows amd64、本地锁定Node／pnpm依赖。缓存、临时目录及遥测覆盖仅对当前进程生效，保存在仓库`.cache/`，不修改机器或用户环境。原始命令与各进程具体临时路径以阶段收据为准；下面列出对应复核入口，不把再次运行视为既有证据。

```powershell
$taskRepository = (Get-Location).Path
$env:GOTOOLCHAIN = 'local'
$env:GOPROXY = 'off'
$env:GOMAXPROCS = '2'
$env:GOCACHE = Join-Path $taskRepository '.cache/go-build'
$env:GOTMPDIR = Join-Path $taskRepository '.cache/go-tmp'
$env:TEMP = Join-Path $taskRepository '.cache/tp9'
$env:TMP = $env:TEMP
go test -p 1 ./internal/localization -run '^TestPlatformDiagnostic' -count=1 -v
go test -p 1 ./internal/platform -run '^TestParameterized' -count=1 -v
go test -p 1 -json ./... -count=1 -timeout=6m
go vet ./...
pnpm --filter @suma/web test --reporter=json
pnpm typecheck
pnpm --filter '!@suma/web' -r --if-present test
pnpm build
```

领域独立命令为`go test -p 1 -count=1 ./internal/localization ./internal/platform -run '^TestControl' -v`、执行目录`summary.json`中的精确回归selector、发布`README.md`中的`TestReleaseCatalog|TestReleaseDiagnostic`命令。运行构建入口为`scripts/build-runtime.ps1 -All`，逐目标命令、工具链、环境与模块缓存来源在对应build清单中；新构建产生新批次，不能覆盖本报告批次。

原始日志、完整盘点、输入摘要、截图、数据库、凭证、依赖缓存及运行二进制保存在本地忽略目录和既有`dist/runtime/`产物目录，不进入源码提交。未执行scratch清理或删除历史证据。源码保留原style.css一致CRLF以减少无关变更，新增Go及文档为UTF-8无BOM／LF；Git使用单命令`core.whitespace=cr-at-eol`识别原换行并检查其余空白，不修改仓库或全局配置。正式Git交付保留必要源码、合同／设计、阶段状态和[精简验收摘要](backend-diagnostics-summary.json)；主集成在实际证据闭合后统一本地提交，不push、不发布。

## 剩余范围与完整平台目标

本阶段关闭约定平台错误构造内的中文归属缺口；18处非中文静态和39处原始动态诊断、其他包、任意wrapper、展示字段与外部错误仍需分类核实。Runner CLI、本地配置诊断、原生智能体全部人类提示、Monaco内置NLS／语言服务诊断、ReactFlow库内提示、系统原生控件、通知／邮件、安装器、外部渠道及完整双语文档继续实施。用户文件、模型输出、原始工具日志、终端内容与历史不可变证据始终保留原文。

完整P0–P7继续：真实厂商模型与专业工具SDK、OceanBase／PostgreSQL分布式验证、企业SSO与自定义RBAC、版本化插件／技能／MCP／A2A、模型网关、协作与离线、媒体／Office／会议语音、生产传输、告警与微信／钉钉／企微渠道、正式安装签名、远端／容器部署、支付账单、规模与故障注入，以及完整需求到执行／独立测试／人工验收／部署与问题回流旅程均未因本阶段缩减。0门禁问题、局部浏览器结果和24个交叉编译文件都不构成全平台或生产完成声明。
