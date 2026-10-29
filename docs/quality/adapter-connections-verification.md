# 统一工具接入目录限定验证

阶段：T-ADAPTERS-139；产品1.0.0；最终核验2026-10-30，状态`scoped_verified`。源码集成基线 `86fb65e1168bd84f92aa8d758204e34b4d04f6a3`。本阶段只说明工具接入目录及现有配置流程的接续，不把此前Inbox、共享视图或厂商运行证据重复计入。机器摘要见[同阶段摘要](adapter-connections-summary.json)。

## 交付内容和权威边界

提供当前项目授权下的只读 `/adapter-catalog`，按工具入口区分速码调用工具和工具调用速码。目录包含20个明确入口、源码实现状态、能力支持状态、来源及限制；界面提供可搜索目录和已有Runner配置／客户端导出／手动指南的准确接点。目录使用默认收起的原生details，保留选择，优先显示日常工具配置。正式字段见[合同](../../contracts/adapter-catalog.md)，交互和架构见[设计](../design/adapter-connections.md)。

目录implemented不是已安装、已连接、已启用或可派发；配置入口存在也不是新执行适配完成。WorkBuddy云服务保留planned，不以CodeBuddy CLI代替；MCP客户端配置导出不证明反向执行。未知能力明确待核验，实时设备信息仍由原tool-discovery提供。

复审收紧了两类易误判能力：停止请求／终态观察不等于OS与准确attempt的停止证明，host_stop_proof静态及Profile均保持unverified；candidate_capture源码条件路径仍需工作树、制品配置、成功执行及干净新提交，可写Profile不能单独证明候选可交付，只读Profile不支持捕获。unsupported仅针对速码当前入口，不扩大为厂商全产品声明。

无新增依赖。复用现有Go领域、React组件、原Profile校验及固定工具合同；本地Multica `2083970b51c03c0773188636c24a96acd09ad4c3`仅用于行为参考，本阶段没有复制其代码、样式或品牌。沿[既有开发工具矩阵](../integrations/development-tool-matrix.md)记录厂商差异。

## 已完成的针对性检查

- 后端9个独立顶层用例通过：6个目录／Profile用例与3个平台读取用例，覆盖方向与WorkBuddy区分、唯一且新建的闭合目录、恢复／账单／交互不夸大、Profile验证与收窄、条件证据、当前项目权限、机器身份隔离、闭合HTTP查询／no-store、实际路由与注销后拒绝，以及Harness运行时指南方向。原命令为 `.cache/toolchains/go1.26.10/go/bin/go.exe test ./internal/toolcontract ./internal/platform -run '^TestAdapter(Catalog|Profile)' -count=1`；最后新增指南回归后仅重跑toolcontract同一筛选，通过0.163秒，日志 `.cache/shared-views/adapter-runtime-guide-tests.log`。子用例和重跑不重复累加。Go本地化完整包亦通过0.320秒。
- 前端5文件43项通过：AdapterConnections、AdapterConnectionsIntegration、ToolWorkspace、ToolSetupGuide、DeveloperClientConnections。实际命令为 `node node_modules/vitest/vitest.mjs run src/AdapterConnections.test.tsx src/AdapterConnectionsIntegration.test.tsx src/ToolWorkspace.test.tsx src/ToolSetupGuide.test.tsx src/DeveloperClientConnections.test.tsx --maxWorkers=3 --minWorkers=1`，工作目录apps/web，9.31秒；包含目录展开／收起及配置接续。最终限制文案调整后重跑前两个文件11项，通过6.29秒；这11项包含在43项内，不相加。结果保存在执行会话输出，未为了生成JSON重复测试。
- 完整Web类型检查 `node node_modules/typescript/bin/tsc --noEmit` 通过；在apps/web运行 `node scripts/check-i18n.mjs` 通过，7156对中英文键、生产硬编码0。键数不是全平台国际化完成率。
- 完整合同生成器包 `go test ./cmd/suma-contracts -count=1` 通过（0.961秒）；`go run ./cmd/suma-contracts` 及 `go run ./cmd/suma-contracts -check` 通过：730个Go模型、338个API路径。OpenAPI计入通用Error时为731个schemas。
- 生成前后语义比较只新增 `/adapter-catalog` 与AdapterCapability、AdapterCatalogPage、AdapterDescriptor三个schema。全部已有路径和schema不变；TypeScript移除3个新增interface后与原文件逐字节相同。MCP、运行时和本地化等其他生成产物未变化。日志及比较收据在 `.cache/adapter-catalog/contracts/`。

环境为Windows、仓库本地Go1.26.10、Node22.16.0。Go使用进程局部GOTOOLCHAIN=local、GOWORK=off、GOENV=off、CGO_ENABLED=0、GOPROXY=off，模块及构建／临时缓存均在仓库 `.cache/`。不改用户环境，不提交依赖、凭据或样例数据库。

## 浏览器和构建状态

2026-10-29的最终浏览器复核曾被origin自动审批因审核模型容量不足明确拒绝，要求停止、不换途径；当时按要求停止，保留pending_final_browser，未将拒绝计作产品失败或通过。2026-10-30用户明确授权恢复浏览器验证后，集成者完成25项实际检查，阶段现为scoped_verified。

最终检查使用内置浏览器、真实控制面API及独立SQLite数据，入口http://127.0.0.1:43959。检查覆盖键盘展开／关闭、搜索空态、WorkBuddy planned且无执行动作、CodeBuddy独立入口、准确Harness／Codex配置、无实际库存禁保存、反向客户端选择与无密钥配置、收起后保留选择，以及中文／英文、桌面／390px、浅色／深色代表性状态。25项逐条记录见本机收据`dist/quality/adapter-139/browser-final-summary.json`；摘要保留计数及截图SHA256。两处自动化定位调整属于脚本修正，不计为产品缺陷。

实际发现390px详情标题被固定页头遮挡，已将scroll-margin-top从16px改为88px并复验。修复前截图保留，最终截图为final-cn-desktop-light.jpg、final-cn-mobile-light.jpg、final-en-mobile-dark.jpg，位于`dist/quality/adapter-139/`。截图和原始日志不提交，不能把本次限定旅程扩大成全平台界面验收。

修复后的隔离生产构建为`node node_modules/vite/bin/vite.js build --outDir ../../.cache/adapter-139/web-acceptance`，exit0、53.28秒；保留既有大于500kB的chunk建议。43959保留已核对的suma-control-verified.exe，只切换自有静态输出目录。该控制程序先前以Go1.26.10和已提交控制入口overlay构建通过；原`.cache/adapter-139/build-verified-summary.json`继续记录其构建与HTTP20项目录／Harness限制核对。该构建来自当前合成前端工作区，包含App及140／141／142在途源；使用既有139后端同范围API完成25项目录交互验收，不能作为其他页面／新API的完整联调证据，也不是仅139源码的封闭构建。

43项独立UI、9项后端、类型／7156语言键与730模型／338路径的原有效证据按未变输入复用，11项文案复验仍包含在43内；本次新增25项GUI，不重复累加旧测试。当前提交候选以00a0b0c为基线，只增加本阶段路径和3个schema，原路径／schema原始字节与语义、原TS内容保持一致，见`.cache/adapter-139/staged-candidates/manifest.json`。acceptance-before／after.json记录本次构建前后5个选定139源文件摘要一致（4个TSX及CSS）；这不表示与旧web-verified产物一致。产物摘要来自实际文件，不声明完整可重现编译输入证明。

## 未覆盖

最终暂存检查发现4个新增Web文件存在混合行尾；仅统一为LF及单个文件结束换行，未改变代码内容或交互。无需据此重复既有功能检查；最终暂存差异单独检查。

没有调用厂商模型、登录商业工具、远程安装、创建真实厂商任务或部署生产。新增Cursor／Claude／Qoder／WorkBuddy云执行、完整会话恢复、供应方账单、真实设备安装、其他数据库、物理多机和生产验收仍在原实施计划。此阶段不改变完整58限定／34待交付／92总项，不重跑无关全平台套件。
