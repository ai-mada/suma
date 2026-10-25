# T-P4-013 平台显示诊断与图控件实施计划

> 执行方式：使用subagent-driven-development分工实现、独立任务复核和最终集成复核；由主集成完成验证后本地提交。

目标：让当前工具资格在各入口即时切换中英文，保持原始依据，并用正式React Flow接口覆盖图内部控件／辅助说明。

架构：Go权威目录定义受控source-bound显示身份，控制面在原受权事务内构造只读字段；浏览器以生成的来源合同核对raw reason再渲染。图控件使用固定官方版本的公开本地化接口。方案不修改持久执行原因或可编辑业务正文来冒充显示；它们继续作为必做的后续纵向任务。

技术：Go／encoding/json／现有localization模板、React19／TypeScript／i18next、React Flow12.12.0、Vite／Vitest。规范：[冻结合同](../../contracts/platform-display.md)。完整范围：[总实施计划](implementation-plan.md)。

## 全局约束

- 原始`reason`、工具Observation、执行／业务状态、原件、事件、幂等请求及历史回执不因语言变化而改写。
- 显示身份不授予权限，不改变Available、派发、预算、测试、重试或验收。
- 元数据与同对象的原始`reason`绑定；未证明、未知版本／键或参数错误时保留原文。
- 仅由Go权威目录生成批准的16条display.tool来源定义；不创建第二套手工规范原句。
- 工具资格链路与图控件均支持zh-CN／en-US，保留主题、草稿、披露及仍存活的焦点。
- worker不提交／push／派生代理；Root只提交必要源码、合同、设计及精简验收。凭证、缓存、原库、日志、截图和运行二进制留在ignored目录。
- 原批准P0–P7范围保持；持久执行原因、生成记录来源、Monaco、CLI／桌面／安装／渠道及SSO等继续实施，不据此声明完整平台完成。

## 审查重点

1. 外部原因恰好是平台模板或键：只能作为参数／原文，不获得新身份；覆盖后端和真实前端消费者。
2. 同一原文配错key、未知版本、null／多余参数、Unicode或长原因：完整证明失败回退原文，业务状态不变。
3. HTTP中英并发与目录迟到：原文／原回执不变，切语言0额外GET／POST；披露和草稿保留。
4. 共享池凭证／授权变化与旧操作重放：资格由现有事务重新核验，不能因新增hint放宽许可或升级历史响应。
5. React Flow真实内部ARIA／title及动态移动播报：使用官方类型和真实组件验证，保留键盘语义／署名／精确选择，不靠mock掩盖英文。

## Task 1：Go显示合同与资格生产者

文件：新增`internal/displaycontract/`、`internal/localization/catalog_display.go`及对应测试；修改`internal/localization/catalog.go`／`render_impl.go`、`internal/platform/tool_types.go`、`tool_profiles.go`、`localization_projection.go`及必要的`pool_runner_tools.go`，新增资格HTTP／来源测试。仅拥有Go领域文件，不修改Root生成器／前端。

接口：输出冻结合同中的DisplayDiagnostic、SourceDefinition、BindToolReason、ValidateToolReason、SourceDefinitions及ToolProfile.reason_diagnostic；消费现有localization与原授权事务。参数映射和可变定义按值复制。

- [x] 写入有意义的失败测试：16个具体资格分支、raw reason跨语言不变、范围内外参数、伪造hint及旧回执。
- [x] 运行对应Go测试，保存实际RED及输入摘要；不把编译缺类型当作全部行为RED。
- [x] 实现闭合目录／完整source证明、可信生产者附加字段及原文投影保护；保持资格判定及权限语义。
- [x] 同一输入运行覆盖测试及vet，保存真实命令、完整结果、源码摘要、自查和限制至本阶段ignored报告。
- [x] Root按任务差异、合同及报告独立复核；问题交原实现者修复并定向复核，之后才完成此任务。

## Task 2：真实消费者与官方图控件本地化

文件：新增`apps/web/src/display-diagnostic.ts`及测试、`flow-localization.ts`及真实组件测试；修改`ToolFacts.tsx`、`ToolBindingStatus.tsx`、`tool-model.ts`、必要的`ToolWorkspace.tsx`／`Workflow.tsx`及语言目录；升级`apps/web/package.json`与`pnpm-lock.yaml`。不修改Go、Root生成器或生成文件。

接口：消费Root生成的`DisplayDiagnostic`与`packages/contracts/src/display-sources.ts`中的`displaySourceDefinitions`；输出纯renderDisplayDiagnostic及统一资格显示。生成类型未就绪期间可实现并记录真实行为RED，等待Root生成后完成类型验证，不能另写第二份规范目录。

- [x] 在现有消费者先证明英文raw资格泄漏／无身份保护及图内部英文的具体失败；保存原切面。
- [x] 读取固定包官方metadata与源码类型；仅从官方registry安装固定12.12.0、禁用生命周期脚本、使用仓库内缓存。记录锁差异、完整性及许可，保留旧失败证据。
- [x] 实现source-bound renderer，接入全部上述资格入口并保持raw披露、外部参数、未知fallback、目录迟到、locale无请求及焦点／草稿。
- [x] 接入该固定版本全部public ariaLabelConfig；保持现有自有label、真实编辑／执行语义、布局／选择／viewport及署名。
- [x] 覆盖消费者、恶意／无效／迟到数据与真实ReactFlow／Controls DOM，运行对应测试、类型及i18n门禁；不以mock空图作为库内部验收。
- [x] Root生成任务审查包，独立复核后完成；全尺寸／真实浏览器仍由Task 3验收。

## Task 3：生成、集成、实际验证与交付

Root拥有`cmd/suma-contracts/`、生成类型／OpenAPI／display-sources.ts、本合同、设计／交付文档和memory。生成器从Go来源确定排序，检查缺项／重复key／模板与漂移。Go／Web稳定切面后运行合适完整测试、类型、合同、构建与六目标程序检查，再做全变更独立审查。

- [x] 等待Task 1正式类型，先写生成器失败测试，再实现同源生成／check。将Task 2所需真实输出交接，不交手工stub。
- [x] 冻结最终输入，完成完整测试及独立审查／修复；保留初版、修订与最终切面。
- [x] 用独立本机账户／项目／工具档案真实验证暂停→探测→启用→资格读取→暂停及失效说明；配置、程序探测、模型与研发验收分别记录。不得调用付费模型或伪造工具成功。
- [x] 在真实图页面检查中英文内部按钮／描述／播报、键盘、draft、精确attempt、主题及390／768／1366／1920；保存真实DOM尺寸和截图。保持浏览器默认尺寸与原预览数据，停止本阶段自有排空进程。
- [x] 核对原始响应／回执、业务／执行计数、数据库完整性与原数据，更新设计及机器摘要和未覆盖清单。
- [x] 完成独立交付审计、逐文件必要性与摘要核对；本地阶段commit沿用用户授权，由主集成完成。随后继续完整目标。

## 后续必做纵向链路

持久NodeRun／Runner.Result显示身份必须贯通生产、脱敏、Journal、HTTP／pool、DAG、attempt、重放及宽窄视图，不能只映射英文字符串。生成Issue／需求须有普通API不可伪造的来源sidecar和逐字段撤销规则，不能信任可写data.origin。Monaco必须先证明正式ESM/NLS、各语言服务及无损草稿切换。CLI、安装器、通知、导出和渠道的剩余说明，以及当前凭证统一守卫／SSO和全部原平台能力保持开放，按各自合同实现与验收。


## 实际验收追加：原始名称不重插值

2026-10-25：实际图界面发现端点用途及起点坐标问题，独立组件复核又发现名称占位符重插值。限定同类排查确认8个raw-name入口，另有编译器ID约束和单参数／顺序负例，不冒称全部tr调用均有问题。以[UI原始值合同](../../contracts/ui-literal-values.md)为新增实施基线，在现有SDK公开接口上统一纯格式化；不修改全局翻译配置或业务状态。先保留730项／53文件切面，再实现全部8确认入口、同一失败回归GREEN、独立复核及最终网页／真实图验收。原固定Go359测试输入、runtime200生产输入不变时继承对应证据并重新核对，不虚称重跑。该追加不缩减完整平台目标。
