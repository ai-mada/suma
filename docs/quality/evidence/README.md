# 当前交付证据索引

状态：本机证据，2026-10-24；负责人：平台集成。证据只适用于[验证报告](../verification.md)明确的范围，完整目标见[实施台账](../../delivery/implementation-plan.md)。

## 当前 T-P4-004 固定工具派发与实际质量闭环

| 证据 | 内容 |
|---|---|
| [最终Go512](tool-go-post-review-tests.jsonl)、[源码/环境](tool-go-post-review-verification.json) | 0失败/0测试跳过、vet通过、189输入前后相同；初轮479与原始RED分别保留 |
| [最终工作区](tool-workspace-final.json)、[212项测试](tool-workspace-final-tests.txt)、[类型](tool-workspace-final-types.txt)、[构建](tool-web-final-build.txt) | web198、native8、desktop3、knowledge3，105输入前后相同；旧203/208/211切面原字节保存 |
| [初组三旅程](tool-browser-execution.json)、[最终第4次停止](tool-post-review-database.json)、[交付程序](tool-post-review-runtime.json) | 固定需求任务、隔离候选、独立2测试、人工验收、真实失败/重复回流、实际停止；4runs/8nodes/5attempts，原35事件及验收回流未改写，工具paused v5 |
| [运行时专项](../tool-runtime-verification.md)、[CLI只读探测](tool-codex-probe/README.md)、[官方usage形状](tool-codex-usage/README.md) | 111域内/原生8/真实控制面三授权用例分别取证；本机Codex0.164仅help/version，无真实厂商模型执行 |
| [工具UI](../tool-ui-verification.md)、[窄屏节点列表](../tool-ui-responsive-supplement.md)、[真实React导航](../tool-ui-navigation-verification.md)、[完整点击盒](../tool-ui-hitbox-verification.md) | 组件切面与实际浏览器分开，保留原失败、源码和锁文件范围 |
| [浏览器观察](tool-browser-visual.json)、[原图摘要](tool-visual-files.json)、[390](tool-final-390.png)、[768](tool-final-768.png)、[1366](tool-final-1366.png)、[1920](tool-final-1920.png)、[深色](tool-final-dark-1366.png) | 最终M_630Zj3资产、实际CSS宽度、键盘和默认鼠标点击、返回、主题；原生PNG的1.5 backing像素与CSS宽度分别记录，无图片编辑 |
| [根归档](tool-root-archive.json)、[最终Go归档](tool-post-go-archive.json)、[实际停止归档](tool-post-live-archive.json)、[最终JS/视觉归档](tool-final-archive.json) | 原始日志和历史失败不覆盖；不归档账号、token、配置、数据库、外部登录文件或运行二进制 |
| [当前24程序](../../deployment/tool-stop-runtime-bundles.md)、[首批历史24程序](../../deployment/tool-runtime-bundles.md) | 最终051829绑定512测试/116运行输入，Windows实际激活复验；旧045219和更早批次保持独立切面，跨编译不等于目标OS安装 |

完整阶段范围及未完成能力见[工具验收](../tool-execution-verification.md)。测试模型端仅使用明确标注的loopback fixture；实际Git、进程、SQL、独立测试和浏览器操作有各自证据，不作为真实模型编码、生产或整个P0–P7完成结论。

## T-P4-003历史：签名来源与交付

| 证据 | 内容 |
|---|---|
| [最终 Go](ingress-go-tests.jsonl)、[汇总与源码](ingress-go-verification.json)、[入口子集](ingress-platform-subset.jsonl) | 全套385个通过事件、0失败、0测试跳过，9个受测包及4个命令编译，vet通过；子集46事件含23顶层 |
| [JS173项](ingress-js-tests.txt)、[类型](ingress-typecheck.txt)、[最终消费者12项](ingress-ui-final-consumers.txt)、[构建](ingress-web-build.txt) | web164及另三包各3，最终暂停提示文案后再执行消费者及构建；浏览器加载index-DqUuHkug |
| [真实本机旅程](ingress-browser-execution.json)、[协议检查](ingress-protocol-execution.json) | 18请求、7交付、3成功运行9节点、6证明3原件；暂停r2保留r1，最终交付控制与CLI重启恢复原回执 |
| [部署映射](ingress-deployment/archive-map.json)、[测试档案](ingress-test-archive.json) | 部署8项、6种Compose、values与脚本验证；原日志和失败记录原字节归档，未运行Helm或真实集群 |
| [390首次配置](ingress-initial-key-390.jpg)、[390入口](ingress-entry-390.jpg)、[768交付](ingress-tablet-768.jpg)、[1366最终定位](ingress-desktop-final-1366.jpg) | 实际DOM宽度检查，首次设置只打开提示未生成密钥，手机Escape回焦点 |
| [1366深色](ingress-dark-1366.jpg)、[1920交付](ingress-deliveries-1920.jpg)、[1920受限上下文](ingress-context-1920.jpg)、[真实运行](ingress-run-1366.jpg) | 深浅主题、旧修订交付和实际构建下钻；[视觉范围](ingress-browser-visual.json)、[原图摘要](ingress-visual-files.json) |
| [原失败](ingress-go-pretag-access-denied.jsonl)、[说明](ingress-go-pretag-access-denied-summary.json)、[独立审查](ingress-final-review.json) | 旧store.test.exe权限失败未作为验收；最终宿主串行全套通过，独立只读审查无阶段阻断项 |
| [当前四程序](../../deployment/ingress-runtime-bundles.md)、[历史后续复核](ingress-historical-integration-check.json) | 033010批次24文件绑定101源码；实际Windows控制运行，跨编译与目标OS实跑分开；两份当前部署文档预期变化与历史原件分开 |

截图均为未编辑原始JPEG；`ingress-desktop-1366.jpg`为完善粘滞定位前原图，手机入口图为弹窗过渡前页面；实际格式和尺寸见原图摘要。真实厂商／公网投递、生产验收和完整矩阵仍未完成，范围以[签名入口验收](../ingress-verification.md)为准。秘密、数据库和缓存不进入提交。

## T-P4-002 持久自动化与事件

| 证据 | 内容 |
|---|---|
| [最终Go记录](automation-go-tests.jsonl)、[汇总与源码](automation-go-verification.json) | 串行包全套退出0，329个通过事件、1个Windows目录链接权限场景跳过、9个受测包，vet通过 |
| [JS138项](automation-js-tests.txt)、[类型](automation-typecheck.txt)、[网页构建](automation-web-build.txt) | web129及其余三个包各3；当前网页index-DpCWI2E_，大分块性能仍待验收 |
| [实际旅程](automation-browser-execution.json) | UI创建/预演/启用/人工及cron触发；2运行、4真实证明、2原件重新摘要；r1历史与暂停r2分开 |
| [同页面重启](automation-browser-restart.json)、[视觉范围](automation-browser-visual.json) | Chrome重启后15事件无重复，选中build与画布变换不变；正常轮换、持久历史153..167及空下一页 |
| [390首屏](automation-light-390.jpg)、[390触发](automation-triggers-390.jpg)、[768](automation-light-768.jpg)、[1366深色](automation-dark-1366.jpg)、[1920浅色](automation-light-1920.jpg) | 实际DOM宽度、部分页面无横向溢出；不是完整视觉/可访问性矩阵 |
| [修订和触发](automation-history-1920.jpg)、[运行画布](automation-run-1366.jpg)、[终态对象](automation-terminal-history.jpg) | 不可变修订、实际节点与候选/测试、公开终态空对象；[原始捕获摘要](automation-visual-files.json)保留实际JPEG格式与尺寸 |
| [构建争用失败原文](automation-go-contention.jsonl)、[诊断](automation-go-contention-summary.json)、[真实Worker夹具修正](automation-fixture-heartbeat-receipt.json) | 原失败不擦除；生产30秒租约不放宽，夹具复用Worker.Run，短租约真实HTTP409到通过的对照 |
| [夹具原始记录清单](automation-fixture-heartbeat-files.json)、[末轮独立审查](automation-final-review.json) | 六份原始测试记录逐字节摘要；SSE、真实Worker生命周期和历史文档来源复核，无本阶段提交阻断项 |
| [当前运行包](../../deployment/automation-runtime-bundles.md) | 最后023250批次24文件与93份运行源码绑定；先前两个P4切面及P5文件保留历史，交叉编译不等于外系统实跑 |

普通Chrome为持续连接与重启验收浏览器；IAB保留上下文但曾出现重连提示。截图原始字节未编辑，最初扩展名为PNG的原生JPEG已按真实格式更名并保留摘要。运行数据库、凭证、控制面应用日志、缓存及二进制不纳入源码提交。完整范围见[专项验收](../automation-verification.md)。

## P5 真实制品与发布

| 证据 | 内容 |
|---|---|
| [最终Go记录](go-tests-release.jsonl)、[汇总](go-tests-release-summary.json) | 281个通过事件（含子测试/helper）、8个受测包；命令包只编译 |
| [真实发布旅程](release-browser-execution.json) | 实际2份blob与证明、5个部署操作、7个运行、provider代数5、原检出干净；修复后问题与需求各1个 |
| [包测试](release-b-package-test.png)、[制品证明](release-artifact-proofs.png)、[计划审阅](release-plan-review.png) | 真实固定C/下载字节/人工门禁 |
| [B健康异常](release-b-final-unhealthy.png)、[实际B站点](release-b-site.png)、[需求来源](release-requirement-lineage.png) | 真实404与已切换事实分开；需求可返回原问题/发布 |
| [回退批准](release-rollback-approved.png)、[最终宽屏](release-final-1920.png)、[实际A站点](release-rollback-a-site.png) | 独立新计划/批准/操作；A恢复，原故障保留 |
| [390](release-final-390.png)、[768](release-final-768.png)、[1366](release-final-1366.png)、[1920浅色](release-final-light-1920.png)、[范围与键盘记录](release-browser-visual.json) | 部分发布页面实际宽度及弹层Escape回焦点；非完整可访问性验收 |
| [执行协议回执](release-execution-verification.json)、[原始事件](release-execution-tests.jsonl) | 实际上传/包测试、Provider崩溃重启、ACK丢失及固定操作恢复，范围由收据限定 |
| [发布控制回执](release-control-review.json)、[独立审核](release-authority-review.json) | 实际SQLite并发、精确授权、原unknown保留、当前租约/only-managed/撤权与重放边界 |

宿主捕获、过渡帧及IAB重复合成截图已按[捕获收据](visual-capture-receipt.json)保存摘要，原件归档到忽略的任务诊断目录，不作为最终视觉验收。最终四宽度PNG使用独立Chrome标签页在布局稳定后捕获，记录实际innerWidth；390审阅弹层及键盘另有IAB实际验证。这些是部分页面证据，不能替代完整视觉矩阵。

## 治理、计划与候选历史切面

| 证据 | 内容 |
|---|---|
| [完整 Go 汇总](go-tests-wave2-summary.json)、[原始结果](go-tests-wave2.jsonl) | 205项通过、0失败/跳过；包括子测试 |
| [真实候选执行](candidate-browser-execution.json) | SQLite正式状态、4次尝试、2个Runner、固定提交、journal与文件摘要核对 |
| [实际候选测试页面](candidate-test-1920.png)、[人工验收](candidate-human-acceptance.png) | 来源attempt2、测试C而非B、正式任务accepted |
| [首轮真实失败](candidate-runner-first-failure.png) | Windows环境缺失的失败事实保留；不计作验证通过 |
| [Windows环境修复](runner-windows-environment-execute.json) | 同一许可配置经过真实Execute完成编辑、测试、vet |
| [治理与候选独立审查](governance-planning-review.json) | 撤权竞态、严格版本/字段、未知效果、来源冻结等复现与修复 |
| [团队身份](governance-team.png)、[组织隔离](governance-isolated-organization.png) | 实际页面创建/切换、真实账号团队归属、空组织无旧项目数据 |
| [基线差异](planning-baseline-comparison.png)、[容量](planning-capacity.png)、[迭代完成](planning-iteration-completed.png) | 实际业务数据及门禁；容量是估算分配而非实际工时 |
| [390页面](planning-390.png)、[390完整排期表单](planning-form-390.png)、[768页面](planning-768.png)、[1366页面](planning-1366.png)、[1920运行](candidate-test-1920.png) | 部分页面实际尺寸走查，不代表全部旅程矩阵 |
| [新计划深色主题](planning-capacity-dark-1920.png) | 容量、未知值及范围说明的实际深色页面 |
| [打包后实际工作台](packaged-control-candidate-1920.png) | Windows 目录包中的控制面与网页实际启动，恢复已验收候选；不等同 Electron 原生 GUI 验收 |
| [六平台构建收据](../../deployment/evidence/runtime-build-manifest.json)、[源码切面](../../deployment/evidence/runtime-source-manifest.json)、[链接模块](../../deployment/evidence/runtime-linked-modules.json) | Windows/Linux/macOS × amd64/arm64 共 18 个 Go 程序实际编译和目标信息检查；未在其他操作系统执行 |

## 2026-10-23 历史纵向基线

| 证据 | 内容 |
|---|---|
| [Go 测试摘要](go-tests-summary.json) | 108个用例、5个受测包及vet结果 |
| [实际旅程](browser-journey.json) | 双Runner真实执行、人工验收、问题与去重需求回流 |
| [人工验收页面](browser-task-accepted.png) | 页面提交后正式任务accepted |
| [最终浅色运行页面](browser-run-succeeded.png) | 固定v2执行、两个成功测试节点 |
| [最终深色运行页面](browser-run-dark.png) | 深色主题、节点与图例可读性 |
| [源码与文档清单](source-sha256.json) | 非忽略交付文件逐个SHA256；不含自身 |
| [Windows桌面收据](../../deployment/evidence/windows-package-receipt.json) | 实际打包结果，未签名、无GUI与安装结论 |
| [Windows逐文件清单](../../deployment/evidence/windows-unpacked-sha256.json) | 整个桌面目录内容摘要 |
| [第三方元数据](../../delivery/third-party-dependencies.json) | 依赖、许可文件及锁文件摘要 |

使用 PowerShell7 执行 `scripts/snapshot-delivery.ps1` 可以在源码或文档变化后重新生成交付清单。构建二进制、目录包有独立收据；重新构建后必须重新采集。截图源于真实浏览器，本轮没有生成图替代产品运行结果。
