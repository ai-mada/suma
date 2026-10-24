# 验证报告与历史基线

## 当前 T-P4-002：持久自动化与运行事件

本机 cron/人工触发、固定版本派发、实际构建与包测试、修订历史、授权 SSE、终态与重启续传已验证。最终 Go 329 个通过事件、1 个 Windows 目录链接场景因权限跳过；JS 138项、类型、网页构建和合同检查通过。完整命令、原失败及修复、当前交付字节和范围见[自动化验收报告](automation-verification.md)。以下 P5 记录保持原历史切面。

## 2026-10-24 P5 制品、发布与问题回流

状态：本机纵向功能验证通过，完整产品继续实施。负责人：主集成代理及并行控制面、执行、工作台代理；阶段 T-P5-001。真实环境为 Windows amd64、Go1.24.5、Node22.16、pnpm11.25、SQLite和独立loopback静态提供方。

- 最终全套 `go test -json ./... -count=1` 和 `go vet ./...` 通过：281个测试通过事件，包含父/子测试和进程helper；8个受测包通过，4个命令包仅编译。不是281个独立产品场景。见[最终Go记录](evidence/go-tests-release.jsonl)和[汇总](evidence/go-tests-release-summary.json)。
- JS/TS包测试98项：web89、agent3、Lance3、desktop3；独立部署测试4项。`pnpm typecheck`、最终网页构建、107模型/96路由漂移检查通过。Monaco/ELK仍有大分块提示，未据此宣称低配大图性能达标。
- 原始执行协议与崩溃/丢ACK/恢复范围见[执行收据](evidence/release-execution-verification.json)；发布控制及真实A/B/A、丢Runner结果后观测恢复见[控制面验证](release-control-verification.md)。[独立审核](release-authority-review.md)另含真实双连接撤权三路RED→GREEN、Worker范围隔离及managed恢复拒绝路径。

### 真实浏览器闭环

1. 任务A与B各自经历开发C、固定C构建、上传实际1198字节包、下载包后HTTP内容测试、人工验收与不可变候选。原始检出保持管理员基准B且干净。
2. 计划预览固定候选、任务依据、策略、目标和最高预约代数；独立批准后只由受保护DAG取得短时在线授权。
3. 实际发布A（代数1）健康200；B（代数2）已切换但健康路径真实404，页面保留活跃B事实，生成问题并回流需求。
4. 独立新计划、批准和操作回退A（代数3）；故障和需求不因回退消失。
5. 实测发现provider与generic失败各建问题，修复为同operation统一身份。最终代码再发布B（代数4），健康404，正式库只有1个关联问题；刷新重复回流仍只有1个需求。随后独立回退A（代数5），实际HTTP字节与A的manifest相符。
6. 需求可直接返回来源问题和原发布操作；问题可打开精确运行证据。390/768/1366/1920实际视口下发布详情无页面横向溢出；窄屏审阅弹层Escape曾误关详情，修复后只关闭弹层、保留record并回到原审阅按钮。最终页面未产生新的console error。这是部分页面走查，不是全部五旅程/浏览器/WCAG矩阵。

正式库只读快照、两份真实blob重算、5次操作、7次运行、独立provider当前代数、原检出与实际程序摘要见[机器可读旅程](evidence/release-browser-execution.json)。修复前重复问题作为历史证据保留；修复后结论只对应代数4。截图与精确范围见[证据索引](evidence/README.md)。

### 本轮交付边界

新增独立 `suma-deployer` 与六目标四程序运行包，详见[构建记录](../deployment/release-runtime-bundles.md)。跨平台编译不等于其他操作系统运行或安装验收。原Windows桌面/portable/NSIS和上一批18文件均保持原切面，不能声称已含本轮P5。

首轮仅本机静态站点与本地制品目录；未验收S3、OceanBase/PostgreSQL集群、Docker/K8s/生产/灰度、CI签名、厂商付费模型和目标负载。profile绑定管理员配置摘要，不是可执行文件字节的供应链证明。完整待办保留在[实施台账](../delivery/implementation-plan.md)。

## 2026-10-24 治理、规划与候选历史基线

当前治理、计划、隔离工作树和候选交接已完成本机真实纵向验证。全套 Go 测试 205 项（含子测试）和 vet 通过；JS/TS 包测试 74 项（界面 65、智能体 3、Lance 3、桌面 3）及独立部署测试 4 项通过。类型、最终浏览器生产构建、49 个模型与 64 条 API 路径的合同漂移检查均通过。

浏览器实际完成组织创建/切换与项目隔离、真实账号资源和团队、迭代/排期/基线差异/容量、固定候选开发与两个 Runner 的测试、失败后新尝试和人工验收。390/768/1366/1920 宽度已做部分页面走查；390 排期完整表单、键盘弹层关闭回焦点和容量周刷新直链已实际验证。这不代表四种宽度下全部五条旅程或完整 WCAG 审计通过。

原始过程与边界见[候选交接](../scheduler/candidate-handoff.md)、[独立审查](candidate-authority-review.md)和[证据索引](evidence/README.md)。

### 当前 Windows 产物

- `dist/desktop/Suma Setup 0.1.0.exe`：实际 NSIS 构建成功，111,358,656 字节，SHA-256 `bc856dde2b53cfdd4afbe05a71aeeff4d8e2275dbb697227fd24ef852171b4b7`。
- `dist/desktop/Suma 0.1.0.exe`：实际 portable 构建成功，111,128,909 字节，SHA-256 `258d5cfbd995606780c41fc655d6d1ffac87384b26226ce13686d7f451780506`。
- `dist/desktop/win-unpacked/`：381 个文件、397,312,961 字节。网页 306 个文件与冻结构建逐个匹配，asar 运行源码与当前桌面源文件匹配；当前入口为 `index-B_twO93P.js`。
- `dist/runner/suma-runner.exe`：已替换为实际候选交接验证所用的 Windows Runner，摘要见[Runner 收据](../agents/evidence/runner-sha256.json)。

目录内真实控制服务和网页已在 loopback 启动，并由浏览器重新读取同一验收与容量数据。安装器、portable、目录主程序及控制服务均未签名；未执行安装/升级/卸载或原生 Electron GUI 验收。当前结果不替代 macOS/Linux 实机验证。

[打包收据](../deployment/evidence/windows-package-receipt.json) · [目录逐文件清单](../deployment/evidence/windows-unpacked-sha256.json) · [控制服务摘要](../deployment/evidence/windows-control-sha256.json) · [网页入口摘要](../deployment/evidence/windows-web-index-sha256.json)

### 六平台 Go 运行时

控制面、Runner 和 CLI 已分别为 Windows/Linux/macOS 的 amd64、arm64 实际编译，共 18 个程序；宿主读取目标 GOOS/GOARCH、工具链和链接模块并逐文件计算 SHA-256。构建前后 Go 源码摘要一致：`bcbd66b6732f635d8850ad9b3aa80d6d1ea3df40ed4f9a617f0313c7062e70a8`。路径空格、失败返回非零且不发布旧文件、父环境保持不变已做独立探针。

这批产物固定为当前治理、计划与候选交接版本。后续发布功能源码需要新构建；跨平台编译不表示已经在 Linux/macOS 或 Windows arm64 启动、安装和验收。见[运行时说明](../deployment/runtime-bundles.md)、[真实构建收据](../deployment/evidence/runtime-build-manifest.json)和[链接模块清单](../deployment/evidence/runtime-linked-modules.json)。

## 2026-10-23 历史基线

日期：2026-10-23。状态：当前开发版本集成检查通过，完整平台验收未完成。负责人：主集成代理。关联：[实施台账](../delivery/implementation-plan.md)、[能力矩阵](../product/capability-matrix.md)。环境：Windows amd64、Go1.24.5、Node22.16.0、pnpm11.25.0。

## 已实际执行

- `go test ./... -count=1`：108 个通过用例（含子测试，103 个顶层测试）；`go vet ./...`通过。[汇总](evidence/go-tests-summary.json)
- 覆盖 SQLite 持久 DAG：编译、条件/汇合、并行、动态展开、资源预留、子流程、补偿、fence、断线核对、重复回执、重启和运行摘要分页。
- 平台真实 SQLite/API：身份/项目/凭证范围、原子任务绑定、批次回滚、多级依赖与需求变更失效、补偿与验收原子提交、丢失回执重放、文件写入日志及崩溃恢复、全范围 SQL 检索、索引失败隔离和持久失败问题投影。
- `pnpm typecheck`、`pnpm test`、`pnpm build`通过。JS/TS 包测试38项：界面29、原生智能体3、真实 Lance 索引3、桌面生命周期3；额外部署测试4项通过。Lance 测试使用真实本地原生索引，模型向量响应为本地 fixture。
- 固定锁文件安装通过；pnpm11 的 CI/本机默认设置不一致已定位并显式固定，普通脚本不再隐式重装。见[复现说明](../deployment/build-reproducibility.md)。
- Compose 基础/覆盖组合配置校验通过；入口秘密文件与知识服务监听测试通过。没有实际启动容器或 Helm 集群。
- Windows Runner 实际进程树停止测试通过；原生模型协议与 HTTP 工作流桥接使用本地受控 fixture。没有调用生产模型或第三方智能体账户。

## 真实浏览器与 Runner 旅程

浏览器访问独立的 `.cache/e2e` SQLite 数据，操作与正式 `.suma` 数据分开；两个真实 Runner 进程使用本地 Go 许可命令。

1. 页面创建需求，编辑拆解并提交两个任务，刷新后读取正式记录。
2. 页面发布流程v1，真实运行暴露 Go 缓存环境缺失；失败证据和问题均保存。未声明幂等的节点保持禁止重试。
3. 执行节点显式配置缓存/临时目录，页面修订并发布v2；两个独立测试节点分配到两个 Runner。go test 从13:00:41Z至43Z，go vet 从13:00:42Z至44Z，存在真实执行重叠。
4. 页面核对成功测试，提交人工验收；任务v4状态accepted，绑定成功运行`52cf864262e75a3cd79da16eca27ee3f`。
5. 页面从首轮真实失败问题转为需求，刷新后重复回流；仍保持1条问题和共2条需求，未产生重复修复需求。
6. 最终界面重新加载后保留project/view/sub/run/node；检查浅/深主题及执行输出，截图保存在下方。控制台曾有旧构建切换造成的懒加载404（12:31Z）；修复错误边界、旧资源保留与明确重载后，最终重载未产生新错误。

[结构化旅程记录](evidence/browser-journey.json) · [真实验收截图](evidence/browser-task-accepted.png) · [最终浅色运行视图](evidence/browser-run-succeeded.png) · [最终深色运行视图](evidence/browser-run-dark.png)

这验证了当前纵向闭环，没有覆盖独立 Git 工作树、完整设计编辑、所有审批/失联恢复页面和真实发布环境。原方案五条完整旅程尚未全部完成；未验证节点继续保留在实施台账中。

## 当时生成的产物

2026-10-23 曾生成 CLI、Runner 和未签名 Windows 目录。它们后来经过本轮重建，当前文件与摘要以上方“当前 Windows 产物”为准。Monaco/ELK 仍有大分块提示，目标大图与低配客户端性能验收尚未完成。

## 未验证

真实OceanBase CE4.4.2 LTS BP3/PostgreSQL集群、完整备份迁移恢复、1000执行槽位/30分钟负载、macOS/Linux实机安装、正式Windows安装升级、生产模型和外部工具账户、支付商户、渠道、生产发布及长期恢复演练。浏览器本轮为当前窄屏面板截图走查；390/768/1366/1920完整矩阵、WCAG审计和跨浏览器矩阵未完成。

## 证据与复现

公开可复核的摘要、截图与清单进入docs/quality/evidence和docs/deployment/evidence；原始过程日志、测试数据库及执行节点日志保留在仓库忽略的.cache中。没有删除用户唯一文件，也没有将运行凭证写入交付文档。GitHub参考基线、依赖版本和许可文件单独留有摘要；见[第三方清单](../delivery/third-party.md)。
