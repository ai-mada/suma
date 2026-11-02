# 已验收任务基线交接：限定验证

速码1.0.0 · T-ACCEPTED-TASK-BASELINE-222 · 2026-11-03。负责人：主集成。基线87a9bc3ea85e5a55f2d02a6e1e07388574e874e3。[合同](../../contracts/accepted-task-baseline.md)／[PRD与AI SOP](../product/accepted-task-baseline.md)／[架构](../architecture/accepted-task-baseline.md)／[证据索引](accepted-task-baseline-summary.json)。完整31项／P0–P7及58limited／34pending／92总账继续。

## 交付行为

原TaskCodeBaseline增加第三种普通agent已验收来源，按任务与准确producer选择，仍复用正式Review、同SHA且不同Worker的测试证明、原双CAS／操作回执、代码broker及独立工作树。Source保存原完整执行tuple与bundle，不接受浏览器自授Review或提交证明；来源变化阻止新动作，原完成事实保持。合成与终端专门来源保留，不新增表、HTTP路径、生产SDK、dispatcher或CLI命令。

| 检查 | 结果及范围 |
|---|---|
| 定向检查 | 17个不同顶层检查通过：Go11／Web6，11新／6直接旧，0Go跳过。原未知body/key恢复、多producer、跨项目、当前权限、双CAS和准确来源覆盖；未选项不计覆盖。 |
| 类型与合同 | 完整Web TSC exit0；生成999个模型Schema／452条API路径及TS类型。原POST三origin presence／整数／闭形规则与OpenAPI oneOf一致。 |
| 中英文 | 全量9873对词条及占位符／复数／标记检查通过，生产硬编码0。其后仅3源CRLF→LF、局部CSS token及一项成对文案纠正，不重复无变化类型／行为检查。 |
| 代表界面 | 中文1366浅色：明确任务＋步骤、原因、确认、真实一次POST双CAS绑定；英文390深色重读准确基线而不重绑；源退回后当前目录不可用于新绑定。三组通过，主要控件44px且无页面横向溢出；checkbox原生13px、标签触达与控件高度分别记录。 |
| 真实执行 | 隔离真实HTTP／SQLite／Git与当前Go测试宿主内原runner.Worker／Execute；两位开发Worker、两位独测Worker。没有生成不被消费的独立suma-runner CLI，也不称外部厂商执行。 |

## 真实代码交接与准确拒绝

目标仓库在M产生前从管理员锚点创建，明确没有M对象。源A由普通agent产出M=299cf4ae660b73d9a03a663086513e0b6e8ee4f1，独立测试和隔离Reviewer沿原服务验收后，源版本3由真实界面固定到未开始B（目标版本1／绑定版本0→目标版本2）。

B在新开发Worker通过原受保护HTTP代码broker获取一次bundle，导入M后产出N=2cff3d65a40d8a1f58867904b73b415663e9a079，真实Git确认N^=M。另一测试Worker针对准确N运行JUnit，隔离测试Reviewer复核通过；不是真实客户签收。两个运行的结果与进度journal待确认均为0。

上游Review退回后，A当前业务状态为blocked、版本4，原verified_run_id被清除；B已完成的任务／运行事实保留。原实际宿主的手工负例遗漏账户上下文，只返回428 view_context_required，**该428不作为来源领域门禁证据**。

随后仅对保留SQLite的一致校验副本补验：使用原测试账号登录、准确账户／组织头、当前A v4和未开始C v1发起原HTTP请求，得到409／repository_integration_basis／repository_integration.basis。C任务、基线、运行及已完成B事实前后不变，未重新执行Git／Worker／Review／界面。首次补验在复制要求前只向原测试库新增登录会话，因把Review决定rejected误作Task状态而停止；不宣称最初原库字节未变。之后原库前后与副本初始SHA均为06cfd85817e62c4a329477d0ab3b10a5e1c9243a35ef25f3367300e78ad1a4d5，原receipt、Git与图片保持。

## 保留失败与必要续验

Go首次定向检查即11通过，无虚构RED。Web首批模型和两项旧检查通过；三个新组件检查因夹具缺旧binding、transferID不合法而失败，原读校验正确拒绝。仅修夹具并续受影响三项，未放宽产品guard。

真实旅程共3次Go宿主调用：初次独立checkout被Git autocrlf转CRLF，严格字节断言失败；只在自有仓库初始提交加入.gitattributes固定字节策略。第二次已完成M→N，但重复邀请同一测试Reviewer被域409拒绝；仅在夹具首次创建并复用该Actor。第三次正向完整通过。负例另有2次小补验（状态词夹具失败、精确上下文副本通过），原业务旅程没有因此重跑。

界面4次宿主均正常退出、未force：首次发现桌面全局token把原44px压成36px，产品只加局部.composition-task-baseline的44px token；第二次真实绑定后，driver独立request通道缺Electron cookie，改同源renderer读取；第三次复用已绑定checkpoint，因第二Go宿主夹具失败停在后续等待；第四次三组全通过。只有2次隔离Web构建（初版、必要CSS修正），业务逻辑未因夹具故障改变。60203／63941两个已就绪服务端口均核验关闭。

主集成已查看最终CN／EN原图。查看时纠正一项通用标签“拟固定的代码输入／Proposed code input”，仅原成对词条值变化，key、占位符、逻辑和布局不变；图片仍保留实际截取时的原字样与SHA，不冒称重拍。

## 环境与交付边界

默认pwsh与内置浏览器内核资产缺失，改用已有系统PowerShell、官方固定Playwright Core1.56.1隔离验证包及已安装Electron。SDK仅在忽略目录，禁安装脚本／浏览器下载、不改生产依赖或用户配置。

经清单、SHA与路径核验，清理5个旧项目生成归档96,824,218字节，以及明确退役SQLite1.36.3的单个生成归档30,847,896字节；源码、工具链、下载依赖及唯一证据保留。为避免重编译和D盘耗尽，4,692个生成缓存逐项校验迁入系统允许tmp下本阶段.cache，436,385,165字节摘要相同。仅任务进程使用该缓存和编译临时目录，不改用户／机器环境；源码、DB、工作区、图片和交付证据仍在D项目忽略目录。原两份未跟踪冲突报告保留不混交。

真实提供方／模型为0，物理多机、OceanBase／PostgreSQL、完整设备安装／SaaS与生产验收未由此阶段证明。推荐、讨论开发和专家团队的本次独测工具选择接线仍为已记录后续项；完整平台继续。正式Git提交只含源码、规范、设计和精简证据索引，不含缓存、日志、私有数据库、二进制或图片。
