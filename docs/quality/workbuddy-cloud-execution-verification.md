# WorkBuddy云任务控制面与工作区验证

阶段T-WORKBUDDY-CLOUD-152；2026-10-30；速码1.0.0。控制面与工作区已挂入原运行节点，26项实际Go服务／浏览器交互和3项最终CSS布局检查通过，最终中英截图已目视。范围不等于完整云Runner、真实供应方或生产验收。[合同](../../contracts/workbuddy-cloud-execution.md)、[架构](../architecture/workbuddy-cloud-execution.md)、[交互](../design/workbuddy-cloud-task.md)。

## 已完成的实际检查

| 范围 | 当前结果 |
| --- | --- |
| 当前领域及共享入口集成 | `go test ./internal/platform -run '^TestWorkBuddyCloud' -count=1 -timeout=90s -v`：16个顶层、12个子场景，13.827s通过 |
| Runner窄控制面客户端 | 5个顶层、9个子场景，0.395s通过；原项目／Pool身份、原task观察绑定、无重定向／重试及脱敏 |
| 共享Go类型与校验 | 5个不同顶层通过；原4项0.387s及新增准确节点／人工观察项0.176s，重复运行不加数量 |
| 完整合同生成器 | `go test ./cmd/suma-contracts -count=1 -timeout=90s -v`：62个顶层、5个子场景，1.920s通过；含2个152新用例 |
| 统一合同 | 实际生成及`--check`通过：775模型、366路径；11个新模型、10个真实入口。递归比较全部既有JSON键／数组／值，无旧合同语义变化 |
| Web工作区与当前来源 | 34个不同交互／模型用例通过：原面板9、父容器7、模型6、恢复新增2、原运行页挂载8、记录刷新2；受影响重复检查不另计数 |
| 双语及类型 | 最终共享工作树7611对语言键、占位符／复数／标记及生产源码0硬编码检查、TypeScript类型检查通过；该语言数量含并行153，不能写成fc4隔离候选数量 |

以上共88个不同Go顶层，其中28项为152新增、60项为已有生成器回归；同一测试的先前定向运行不重复计数。未运行平台无关全套，也未宣称race通过。UI为真实JSdom组件与API调用桩，不替代浏览器连接实际Go服务的验证。

Web检查覆盖准确节点及本人连接读取、只采用固定摘要与明确期限、委派结果丢失后重开以原body／key核对、不会创建第二个逻辑委派、跨scope迟到响应退休、当前拒绝清除正文、存储失败不外发和未知操作继续锁定。模型检查曾实际复现“准确task但错误attempt回执被误接收”，现核完整Assignment及原连接绑定。版本冲突的明确只读恢复会要求重新确认；它不解除未知外部作用锁。授权到期与最近观察均显示具体时间。

最终独立检查另发现供应方合法RFC3339时区偏移被页面误当必须UTC的问题。新增场景先失败后通过，6项受影响模型检查0.019s通过；合法`+08:00`／`-07:00`与小数秒可读取及核对，非法日期／时区仍拒绝，平台自有UTC时间规则保持。修复后的最终类型检查通过。该新增1项已计入34项，原5模型重复运行不另计数。

原运行页从准确`workflow_id／workflow_version`及固定协议识别节点。独立复现并修正同ID／版本工作流缓存跨项目借用的问题；切项目、切身份、未发布草稿、来源失效不挂载工作区。正常态新增次级“读取当前记录”；共享`run.version`只触发合并的平台读取，保留草稿与未知操作锁，不增加供应方GET或卡片轮询。检查证据为`.cache/workbuddy-152-ui/run-mount-summary.json`及`record-refresh-summary.json`。

实际领域检查使用本机真实OAuth／任务HTTP、SQLite、两个Service与独立数据库句柄，以及原Go调度器和当前机器证明。覆盖同key恢复、不同行为冲突、双副本仅一次POST、prepared未知不重发、固定task_id读取、正常token轮换、当前撤权、旧fence、暂停、scope与grant变化、两级enforce预算、准备事务回滚及撤权后的准确晚到事实保存。

公开Server检查还覆盖：原浏览器视图缺失428、浏览器不能使用机器入口、真实项目机器创建、本人明确观察、撤权后不能取得旧回执；资源池路径通过正式Server Handler保持真实Bearer证明。资源池assignment来自原调度器的准确测试资格，不是公开Pool claim已开放的证据。

共享Service.Initialize在任何新DDL之前拒绝残缺云领域，原表结构与用户行保持不变。版本上限时准确事实仍可读，无法执行的委派动作不会出现。准确节点ID按run100／node150上限处理，真实长节点已验证。

## 环境与证据

Windows、Go1.26.10、CGO_ENABLED=0、GOTOOLCHAIN=local、GOWORK=off、GOENV=off、GOPROXY=off。模块缓存为`.cache/go-mod-runtime`；GOCACHE／GOTMPDIR／TEMP／TMP均位于`.cache/workbuddy-152`且仅进程生效。未修改用户／机器全局环境。

最终当前领域日志为`.cache/workbuddy-152/platform-stage-integration.log`，SHA256为`084df611607f709571f32363f97e97fc79947dc5eae8cb52d25e4401c43c56f3`；源码与其余日志清单分别见该目录的`platform-summary.json`、`root-summary.json`及`contract/summary.json`。初期含Pool夹具失败的日志不作整轮通过证明；其后准确修复和最终16项同次结果替代它。原失败记录保留，不删除。

定向Go／UI检查运行在共享工作树，记录其准确来源。实际页面候选以`fc4cebe057ef246454e2a5be3adad7c04a6a955a`、152准确文件及4处16行共享增量单独构造；Go生成775／366、Web隔离构建通过，排除153／154生产改动。v2构建66秒，产物位于`.cache/workbuddy-152/candidate/out/web-v2`，未覆盖用户活动预览。既有IDE大包提示保留，不宣称已解决包体优化。

## 实际页面与恢复

使用Playwright控制本任务独立、隐藏、软件离屏Electron窗口，真实Go Server／SQLite／OAuth与任务HTTP协议，对接合成供应方。没有前端API桩、真实供应方、模型或用户浏览器。全部TEMP／TMP、profile、日志和截图位于D盘本任务`.cache`。

第四个独立测试作用域完成26项检查：准确原run／node及固定来源、键盘与明确确认门禁、一次委派／一次提交、合法供应方偏移时间、平台读取不触发供应方GET、明确两次观察到pending／completed、完成与验收／未报费用分开、重开读取持久事实、丢委派回执后的原操作保存／恢复、未知提交不重建及无盲目查询、英文390px深色无横溢出、真实当前撤权清除正文和操作。已知和未知场景分别只有1条委派、1条提交及1次供应方POST；已知GET2，未知GET0。原委派正文／key恢复前后SHA256均为`461f3fe3708588c292fb2982103e9c19adc429abf09f8a09410c4f3f4794bb1f`。

恢复过程如实保留宿主边界：隐藏Electron首次未提供离开确认，`page.reload`超时。保存其原sessionStorage后，在同测试作用域的新独立窗口恢复捕获的原记录；仅在夹具中明确接受离开，再执行真实完整reload及页面“核对原操作”，原正文逐字一致，没有新逻辑委派或第二次云POST。不把会话记录的夹具恢复视为原生弹窗或跨设备自动同步已验。

夹具撤权辅助入口曾返回409；随后独立真实管理员登录，使用公开`project-access`接口和当前版本确认原成员active v1→removed v2→active v3→removed v4→active v5。页面读取后移除受保护正文，最终恢复v5有正式回执。没有直接改数据库制造撤权。此前独立作用域的失败日志、请求journal及图片保留；第二任务无领取的根因是默认并发1且首个DAG仍占槽，第四作用域经真实管理员策略预览／提交明确配置2槽，不改生产默认值、不伪造DAG完成。

26项交互及原5图记录在`.cache/workbuddy-152/browser/receipt.json`；最终fixture停止并Go进程exit0，见`browser-fixture/final.json`。fixture的Go PASS仅证明服务按要求退出，不另算业务通过数。专用fixture控制器在明确委派后调用原DAG准确领取与152创建，它不是公开Runner云执行已启用的证据。

## 最终可读性核对

Root目视原5图，发现`.pane-section dl`选择器优先级高于云任务事实容器，将侧栏和手机覆盖成65px／剩余宽度双列。仅调整云组件CSS作用域和按容器宽度排布，固定标签12px、值14px、行高1.65；没有改变TS／Go业务行为。v3独立构建77秒通过，原v2及修复前CSS快照保留。

新独立样式作用域连接同一真实Go协议夹具，用真实页面产生1条委派、1条提交、1次POST和2次原任务GET；未知场景0POST。3项CSS核对分别为桌面窄侧栏单列与正常字号、英文390px深色单列且无横溢出、英文标签单词不碎裂，不重复计为26项业务检查。实际侧栏列宽228.667px、手机302px；标签12px／值14px。Root目视最终两图，供应方ID、状态、观察时间、未报费用和下一步操作均可读。

最终CSS SHA256为`4ad45cb11f3685524e903a9ce2e3bfae44009a9f76b4858e8db6240cfb5c7ad9`；证据为`.cache/workbuddy-152/browser-style/receipt.json`、`style-fixture/final.json`、`runtime-final-inputs.json`与`.cache/workbuddy-152-ui/facts-css-build/summary.json`。最终图片归档于`dist/quality/workbuddy-cloud-152/`，原失败图和私有操作记录保留在任务缓存、不提交凭据或原始journal。两个验证服务及本任务隐藏窗口均正常退出，用户原预览和数据未改动。

## 仍需完成

提交回执由集成者记录。完整云DAG公开领取／Execute、持久ACP进度与权限协作、初始云任务停止证明、可信制品及固定候选、独立测试和人工验收继续实施；不以本阶段REST创建替代完整能力。真实供应方、多台物理Runner、原生弹窗、设备重启、安装包及生产仍待验。

## 与已交付154合成

152在已提交任务关注`c622da9`之后独立交付。四处共享接线仅增加16行，保留153本机连接及154关注初始化／接口／生成器；当前合成合同为778个声明模型、779个Schema组件、368条路径。递归核对154全部旧合同字段无变化，新增11个模型及10条入口。合成Go编译、类型／7627语言目录及构建证据复用既有准确输入，152最终仅CSS再由独立v3构建和3项真实布局验证确认，不重复26项流程。完整v2／v3资产清单保留在[安全运行清单](../../dist/quality/workbuddy-cloud-152/runtime-inputs.json)，源码摘要只保存其数量及SHA，避免把重复构建目录索引当源码提交。原运行来源及失败记录保持不变；完整云DAG等待办不因合成提交关闭。
