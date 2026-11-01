# Cursor ACP 开发执行：限定验证

T-CURSOR-ACP-189；suma 1.0.0；2026-11-01。状态：本阶段限定通过，完整平台继续实施。

[产品](../product/cursor-acp-execution.md) · [合同](../../contracts/cursor-acp-execution.md) · [架构与 AI SOP](../architecture/cursor-acp-execution.md) · [交互](../design/cursor-acp-execution.md) · [English](../en/cursor-acp-execution.md) · [摘要与证据索引](cursor-acp-execution-summary.json)

## 实现

- Cursor独立runtime/auth配对加入原ACP共享合同、执行目录与生成Schema，固定模型／observe-only／一次权限沿原控制面，不新增派发器、表或HTTP。
- 原厂Node+index.js完整依赖闭包与原生宿主分别绑定；固定禁自动更新及sandbox参数，最终spawn前再次复核包摘要，拒绝依赖变化。
- 本机只接受明确凭据环境引用并映射CURSOR_API_KEY，使用新私有home／配置／临时目录；发现项目祖先执行设置和固定企业hooks则拒绝，不导入原登录。
- 原成熟acp-go0.13.5 SDK严格适配当前Cursor实际帧与缺省agentInfo，configOptions及准确ACK是模型唯一依据；普通通知有界脱敏，阻塞专有请求明确失败。
- Cursor EOF／不支持请求有界结束，实际Wait、强制停止和unknown分别留证；不重发未知Prompt、不自动恢复旧session、不把强制停止当成功。
- 复用原任务、讨论、专家团队、普通／Pool Worker及候选续作；真实Git候选、同提交独立Node/JUnit与独立审阅账号沿原质量门贯通。
- 原桌面ACP选择器和托管连接支持明确Cursor来源、模型、凭据引用及原操作恢复；编辑器／脚本被动行不升权，正式包扫描只提供配置事实。
- 中英文、390px暗色、键盘和分步连接／暂停登记／独立启用复用既有组件；修复底部Qoder品牌残留，共同ACP事实标题中性化。

## 必要验证

54 个不同顶层检查（36 新、18 个直接受影响旧例）与32个Go子例通过：控制面4、受控完整链1、Runtime22、Web20、桌面4、Root共享／生成合同3；不把重试或子例重复计数。原失败、错误匹配和辅助脚本问题保留，不冒充所有命令首遍通过。全类型通过；8805对中英词条／占位符／复数／markup及0生产硬编码通过。

原完整链只运行一次：准确模型ACK、一次权限、原Worker Git候选 `afa3b5b0b0dc9423debb255352cda9b347f14cc5`，独立Worker真实Node/JUnit base/head同SHA、测试前验收拒绝、之后独立邀请账号验收通过；管理员原草稿保留，两Journal pending=0。此为隔离受控宿主，不是真实Cursor模型开发验收。

一次初始组件构建，源文案修正后另建隔离资产，未覆盖活动预览。GUI四项：中文明确连接／暂停登记／独立Enable回调、英文390px暗色原操作恢复、授权拒绝私有隐藏、当前Cursor品牌只读视口；四个原生宿主正常退出。初始两项复用，最后只读不再派发、连接或登记。Root查看了中文当前页与英文窄屏图。

生成合同仅修改 ACPExecutionCapability 与 ACPModelDiscoveryResult 的约束；0新路径、0新Schema类型、0新业务表。独立共享窄审无确认P1/P2；Runtime完整包最终门及界面品牌P2均已修并定向复验。

## 真实范围与剩余验收

- 真实Windows Cursor 2026.10.22-27dcf5f仅核实版本、help、initialize和未认证session/new拒绝；无真实账号／API key／Prompt／商业模型调用，无动态模型目录或认证后生命周期证明。
- 真实包探测共5次启动：前2次是探测守卫错误拒绝data:嵌入WASM，修正后仅重版本／help及一次握手；ACP EOF后经定向SIGTERM，不能称正常退出。原数据与失败结果均保留，5个PID确认消失。
- -e endpoint与禁自动更新均不等于网络隔离；探测守卫曾阻止启动DNS／fetch／子进程，属于探测用Node API guard，不是产品OS安全认证。
- 完整开发旅程使用经过生产完整包核验的自有Go原生fixture，明确不是Cursor或Node开发宿主；原SDK／HTTP／SQLite／Git与独立真实Node/JUnit是真执行，reviewer为隔离测试账号，不是客户签署。
- GUI使用真实生产ToolQuickConnect／ACP／DesktopToolConnector，父级账号、桥、网络、模型目录与回执是合成；独立Enable只核实父回调，不冒充完整登录App或生产启用。
- 先通过全类型与8805对词条，再仅修正ToolQuickConnect的文案参数和两语言同key；受影响原双例、最终词条检查和隔离构建／只读视口复验，未重复整套类型或业务测试。
- 首GUI保留已通过的连接与恢复两项；第三页被原unknown离开保护阻止，剩余授权例在新宿主补验。首补验脚本错误期待已被父级隐藏的子notice，修正断言后仅重该项。四个GUI进程均退出0、无pageerror。
- 不支持的专有问题／计划请求不会默认同意；持久旧session恢复、真实厂商开发／完整扩展、其他OS安装、多机／跨库／规模和生产验收继续。完整31／P0–P7／58限定／34待交付／92总账保持active。

## 证据与复现

原件、stdout/stderr、包清单、SQLite、Git工作树、图片、构建资产均保留在被忽略的 `.cache/cursor-acp-189/`，摘要逐项记录关键证据SHA256。产品测试使用仓库Go工具链／离线缓存；完整链需显式opt-in，见 `internal/platform/cursor_acp_runtime_test.go`。日常只运行新增／直接受影响case，不重复实际厂商探测、SDK构建、全套或安装矩阵。

Multica参考固定提交 `5a2cc891ac338eed1919aa11a1eef5957cd82cec`，其Cursor采用headless stream-json与yolo；速码独立采用成熟ACP SDK，不复制这些参数、专有源码或包。[采用记录](../integrations/multica-cursor-acp.md)区分来源、实施与证据。
