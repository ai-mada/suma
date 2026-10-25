# 工具资格、官方图控件与原始值显示验证

状态：T-P4-013 已记录下述限定范围的本地验证，最终源码／自动检查、实际旅程、交付入口字节、关闭克隆及主库对照均留有独立证据。整体交付审计由Root另行独立进行，本报告不记录Git提交状态。当前正式状态由`memory/current_state.json`及[交付状态](../delivery/status.md)管理。依据为[平台显示合同](../../contracts/platform-display.md)、[原始值合同](../../contracts/ui-literal-values.md)、[平台显示设计](../design/platform-display.md)、[原始值设计](../design/ui-literal-values.md)及[机器摘要](platform-display-summary.json)。完整目标继续以[实施计划](../delivery/implementation-plan.md)为准。

Scope (English): Source-bound read-only tool eligibility display, official React Flow localization and ten approved static UI templates with literal raw-value insertion. Automated checks, historical tool observations, final browser observations, provider execution, engineering acceptance and production readiness remain distinct. Full P0–P7 and all-language-surface work remain active.

## 实现与显示边界

16类`display.tool.*`身份在既有授权事务内由具体资格生产者构造；Go权威目录生成客户端规范来源、类型与OpenAPI。显示前同时证明版本、批准键、精确参数合同及同对象完整原始reason。原始Reason、外部Observation、Available、权限、派发、预算、请求哈希和历史回执保持原语义。未知／无效／迟到目录及超界外部文字回退完整原文。旧回执可忽略坏的可选hint，真实业务JSON损坏仍报错，不重算当下资格或改写回执。通用诊断的按键及原文反查均排除批准键和未来`display.tool.*`域；显示身份不构成执行或验收资格。

React Flow固定官方`@xyflow/react`12.12.0与`@xyflow/system`0.0.83，使用公开`ariaLabelConfig`全部11项、`domAttributes`、Handle属性及React事件捕获。title与ARIA同步本地化，保留署名及编辑／只读差异。移动播报绑定实际键盘事件节点和官方移动前坐标，不取代检查器选择。独立审计确认安装的516＋186＝702文件等于留存官方tarball，MIT许可及React19公开peer范围匹配；这是离线固定包一致性证据，不是在线供应链或完整无障碍验收。

新`literalText`／`formatLiteralValues`封闭于合同十个自有静态键：八个已确认raw-name入口及两个Handle方向。公开`getResource`只读当前语言受控模板，校验精确字符串参数和完整占位符后仅扫描模板一次；插入的`{{id}}`、`{{state}}`、`$t(...)`、`$&`、`$$`、引号、标签形文字及Unicode均为普通React文本／ARIA数据。非法键／形状／资源回退可用原始值，不进入服务器／原生／display诊断域。Handle缺模板时保留本地化通用端点名及原始name／ID，不宣称仍有方向说明；资源恢复后同组件立即重显完整用途。全局`tr`、i18next配置、业务选项值、命令、授权及写请求语义未改变。

## 自动检查的精确切面

以下为已保存原始输出和独立只读审计确认的结果。本次文档整理未重跑测试、构建、服务或浏览器。

| 切面 | 已记录结果 | 来源和限制 |
|---|---|---|
| Go最终完整运行 | 1,250个Test pass事件，0失败／测试跳过；test／vet退出0；359输入保持 | 16包通过，另1无测试命令包；含子测试和进程helper。后续前端修正未改Go，最终审计重新核对359输入，不是新跑Go |
| 最终Web | 784项／55文件，0失败／待定；225输入前后及审计时一致 | 107为嵌套suite数，不是文件数；新完整Web与工作区类型检查退出0 |
| 非Web继承 | 原生37＋桌面7＋检索3＝47，失败／取消／跳过／待定均0 | 没有重跑；38项非Web／manifest输入与原始留存切面逐字节一致，日志哈希与归档回执相符；不证明桌面安装 |
| 语言目录 | 前端每语言2,850键；后端801唯一显式定义，含display16及native46 | zh-CN／en-US；前端门禁0候选／覆盖硬编码。后端801为当前源码静态复核，不冒称新增实时HTTP读取或全部语言面完成 |
| 契约 | 217领域模型＋Error＝218 Schema；153路径；1,242引用 | 生成文件保持；最终Go含生成产物一致性测试；独立生成器`-check`退出0的362输入回执仍独立保留 |
| 最终Web构建 | 退出0；225输入保持；HTML／入口JS／CSS摘要匹配回执 | `emptyOutDir:false`保留旧忽略资源，不是干净部署归档；chunk提示不构成性能验收 |
| 运行包 | 200生产输入、6目标×4程序＝24文件，独立来源／字节审计通过 | 后续Go与24文件字节未变；36个模块、8,441提取文件核对，8版本缺可选`.info`已记录；无目标系统安装／在线认证结论 |

保存命令包括`go test -p 1 -json ./... -count=1 -timeout=6m`、`go vet ./...`、`pnpm --filter @suma/web test --reporter=json`、`pnpm typecheck`、早期非Web递归测试及`pnpm build`。测试缓存与临时目录为进程局部仓库内配置，未改机器或用户环境。独立审计重解析JSONL／JSON和哈希输入／产物，不等于重新运行这些命令。

| 历史Web切面 | 保留用途 |
|---|---|
| 721／53文件／222输入 | 原显示实现最终检查及真实工具资格旧队列；当时前端2,848键 |
| 723／53文件／222输入 | 实际图验收发现Handle用途／移动前坐标问题后的第一轮修正；当时前端2,850键；不是最终raw-safe结果 |
| 730／53文件／222输入 | Handle原始占位符修正后的完整切面；后续八家族literal修正之前 |
| 784／55文件／225输入 | 当前literal最终切面；相对730仅7个已有输入变化、3新增，215项旧输入保持 |

旧695／697、定向87／119／186、独立17／3和88／5以及早期Go1,217通过／2失败各自保留，不相加为当前全套数量。原独立全局`tr`校准源码SHA-256 `8ffacb5d2adee7fd34ae0e1d2d38fdcbdbb57113a5a2284f863cc5222ff9428c`不变，迁移前后均16 RED；它刻意暴露旧API边界，不是当前消费者门禁。新API的同16场景为GREEN，八个实际批准调用点另有精确AST门禁。不能把旧guard改别名后冒称同一测试转绿。

初始三个P2（可选hint回执解码、真实焦点节点／检查器错配、通用诊断绕过）均定向复核为ADDRESSED。实际图观察后的Handle原始值P2另有独立17项／3文件复核为ADDRESSED，后续12文件literal固定补丁独立88项／5文件、`PASS_BOUNDED_REVIEW`、0发现；最终审计串联各生产源码哈希。literal的五个原DOM确认键有真实组件证据，TaskDispatch／fixed rule／Release environment三个为精确API样例、实际调用表达式和AST链接，未升级为恶意原文真实服务／浏览器证明。三处受编译器限制ID候选、顺序及单参数负例不算已确认缺陷。

## 运行字节与旧队列工具旅程

运行批次`20261024T184316196Z-b44afa9c`，200生产输入规范清单SHA-256 `54f29f1f23e23815d3a11a7be054a8cc046fd31e5e4965f69d3319f0cf68c0b1`。Windows控制面SHA-256 `cbda383d1b9fc45f5163dae0b3cb2e804fa2a638fdb88f315d8253b1dae78501`，Runner为`43fc5a2be1a31e18dfb55d65665065971916c3a561c021a8b32695122016c460`。最终入口`index-bVwTmtOy.js` SHA-256 `aaed6f254b9f96e7e3b60d551d03564b2bac518166ed8ad22f8e0989d09594a2`；HTML为`decef68c36b7115899ce35a112f3bc491b97d2ddf3b38da3c82b5d901c3c6a9d`，CSS为`9b48ac0ca5c3fa955b494f204c9f4fb04e9917f796e353d9acdd4affc55284a5`。

最初服务／主预览意外退出的原因仍未知，保存原缺失进程证据后用明确既有隔离库恢复，没有重新初始化。工具旅程对应721旧队列：本机Node v22.16.0仅实际执行固定`--version`，describe为安装／配置及库存证据，不是厂商／模型或研发任务验收。浏览器四次POST按expected v1／2／3／4执行enable／pause／enable／pause，持久历史响应分别v2／3／4／5，均HTTP200。四份原历史响应保留，未用最新GET替换；HTTP正文仅比原SQL response TEXT多JSON编码器的一个LF，二者各保留自身原始哈希。

| 双语详情／列表四GET的独立切面 | version／原因 | 空领取前后／实际派发 |
|---|---|---|
| 创建及恢复paused | v1／profile_paused | 0→0／0 |
| 已启用但无库存 | v2／inventory_missing | 0→0／0 |
| 真实Runner新鲜库存 | v2／dispatchable | 460→460／0 |
| 停止前新鲜库存 | v4／dispatchable | 1,236→1,236／0 |
| Runner停止后真实时钟过期 | v4／inventory_stale | 1,240→1,240／0 |
| 最终暂停 | v5／profile_paused | 1,240→1,240／0 |

每个读取窗口全部129表严格相等，raw reason与来源身份跨语言一致；不是“没有dag_dispatches”。每条空领取的SQL TEXT恰为四字节`null`，固定作用域、空run／node、typed attempt／fence／policy零、唯一receipt／operation及摘要形状均核对。旧`enabled-fresh-001`因错误假设零dispatch而失败，原失败保留，未算通过。最终关闭库129表／1,352行／完整性正常、paused v5、1,240空领取／0真实派发；workflow／run／node／attempt／event／reservation／共享池为空。stale GET没有捕获时间戳；随后浏览器Date证实库存年龄178秒，不由文件mtime重构精确60秒跨越时点。

独立审计仅以只读／immutable／query_only打开27个备份，重算3,483表实例、39,142行实例及334,735 TEXT单元；这些含重复快照，不是独立业务数量。旧Runner和控制面在空闲库观测、当前SQL及身份复核后强制停止；证明到完成约3.312／3.331秒，Runner间新增3条严格空领取及单调治理序列，既有行保持。不是SQL／停止原子事务、并发写者排除或优雅停止。

工具响应式证据仅采用`actual/ui-tool-aligned-*`16格：390／768／1366／1920×两语言×light／dark，reducedMotion开启、DOM scrollWidth等于clientWidth、原reason展开。旧raw-CDP／过渡截图无效。JPEG实际尺寸分别375×865、753×882、1351×890、1905×893，不等于请求的900高viewport。390侧栏完全移出屏幕，其余桌面侧栏可见。该矩阵属于721历史网页，不能改称784当前bundle验收。

## 最终图与既有历史记录浏览器观察

最终图在隔离旧工具库的独立克隆上用当前入口进行，未启Runner或模型。图为未发布的两noop节点／一依赖草稿。16格`ui-final/graph-matrix.json`覆盖390／768／1366／1920×zh-CN／en-US×light／dark，实际宽度对应client375／753／1351／1905，全部scrollWidth等于clientWidth。节点／边标签、Handle输入／输出用途、controls title／ARIA及工作流草稿保持；原名`Alpha {{id}} {{state}} $t(flow.zoom_in) $& 用户`及Beta原文完整保留，未把模板形字符当指令。viewport在该矩阵保持。light矩阵节点Alpha有焦点；dark操作后焦点在主题按钮，不宣称所有格都保持节点焦点。

真实Ctrl＋Enter多选时焦点为Alpha（`node-32ce5f`），检查器仍为Beta（`node-0cce6e`），两个选择同时存在。中文ArrowDown播报Alpha移动前(12,12)，后为(12,17)；英文ArrowRight播报移动前(12,17)，后为(17,17)。播报中的坐标明确为starting coordinates。Space后DOM记录Alpha为active且检查器选择Alpha；Escape后记录无active节点、检查器仍保留Beta草稿。两键均有实际记录，不能由此推断所有快捷键或屏幕阅读器已验收。切语言后的旧live文本可留到下一实际移动事件，不把上一事件误称新语言重播。

主预览另以既有协议夹具run／node／attempt1只读读取6条持久事实（seq10／11／12／23／24／25、watermark25）。双语切换前后展开raw字节相同、viewport相同、同一summary保有焦点，仅说明文字本地化；刷新后精确URL和attempt保持。390显示完整执行节点列表2节点；1366与390所查页面scrollWidth等于clientWidth，保存的控制台0问题。它是旧记录的显示回归，不是新任务执行、测试或研发人工验收；历史只读图与编辑草稿图证据分别保留。

主预览最终窄屏稳定视觉证据仅采用`main-aligned-narrow-zh.*`：官方viewport390、reducedMotion true，侧栏x=-235完整移出屏幕，scrollWidth／clientWidth均375，并保留完整执行节点列表2节点。较早`main-narrow-en.jpg`捕获在侧栏x约-135的过渡中，原图保留但排除最终稳定视觉证明；对应DOM、精确URL及scroll／client事实仍有效。该截图校正不新增执行或改写原历史事实。

最终只读观察已确认两个端口共8个未认证health／HTML／JS／CSS GET均200，六份入口响应摘要匹配784构建；只含入口资源，不承诺所有动态chunk。首轮快照：clone129表／1,353行，仅新增一个session、paused v5／1,240空领取／0真实派发；main129表／7,690行有治理计数变化，原报告如实为`DEVIATION_OBSERVED`，不能写全部129表不变。clone终态002与关闭后003均完整性正常、全部129表摘要相同，执行／工作流／池／reservation计数保持零。

Root关闭自有clone标签后，通过保留native handle强制停止该子控制面。宿主会话exit0；子控制面PID188884回执为exit1、exit_confirmed true、force_stop_requested true、kill_error为nil、wait_error为exit status 1、graceful false。独立003复核启动／意图／停止的精确PID、二进制、数据库及保留句柄身份链；不能把宿主0写作子进程0或优雅停止。

最新main004为129表／7,690行／完整性正常，仅`suma_governance_locks`的既有协调序列变化，其他128表的schema、typed rows及逻辑摘要保持。两行序列分别为基线7529→首轮001为7529→最终004为7529（Δ0），以及8069→8181→8571（总Δ502、001后Δ390）。审计结论为`PASS_EXPECTED_COUNTER_ONLY_OBSERVATION`，不抹去首轮全部129表不变断言失败，也不独立归因每次增量到具体请求。003在09:32:31.485–31.600 UTC观察，004随后在09:32:31.635–32.083 UTC观察；各库、先前HTTP、空闲与停止证据没有共同原子锁。主预览继续运行，后续读取仍可能增加协调序列；本轮仅停止自有克隆，未将主预览表述为已停止。

## 证据与未验收范围

所有原始截图、数据库、HTTP、凭证、进程与日志仅留在忽略目录`.cache/e2e/platform-display-stage-013/`，不得进入Git。主要索引为`review/literal-final-validation-audit.{json,md}`、`review/literal-values-review.{json,md}`、`review/actual-tools-evidence-audit.{json,md}`、`review/actual-ui-fix-rereview.{json,md}`、`review/runtime-audit.{json,md}`、`literal-complete-ui/*-verification.json`、`ui-final/graph-matrix.json`、原文／焦点、`final-readonly-001-report.json`、`final-readonly-002-clone-terminal-report.json`、`final-readonly-003-closed-report.json`、`final-readonly-004-main-final-report.json`及`host-stopped.local.json`。精简机器摘要只保留必要计数、边界和哈希索引，不复制秘密、原始进程指令或数据库。

持久NodeRun／Runner／Result原因来源、受保护系统生成记录、Monaco官方NLS及语言服务、所有CLI平台／桌面／安装器／通知／导出／渠道、统一当前凭证／身份SSO／RBAC、真实厂商模型及工具、MCP／A2A、PostgreSQL／OceanBase分布式、规模、签名／部署／计费和完整P0–P7继续实施。literal八家族修复不证明全部字符串字段、第三方内部控件或完整国际化；当前凭证权限的下阶段提案仍未实现。完整WCAG／屏幕阅读器、图规模性能、目标系统安装、业务负责人签收与生产验收均未通过本阶段建立。
