# 节点凭证与 WorkBuddy 授权管理验证

T-TOOL-IDENTITY-MANAGEMENT，2026-11-05。[设计](../design/tool-identity-management.md)。

- Windows，`apps/web` 运行 `npm.cmd run test -- src/ToolWorkerIdentityList.test.tsx src/tool-worker-identity-presentation.test.ts src/WorkBuddyConnections.test.tsx src/workbuddy-connection-presentation.test.ts`，42 项通过。增加行内配置后单独复验节点测试，3 项通过，计数不重复。
- `npm.cmd run test -- src/ToolWorkspace-recovery-navigation.test.tsx src/ToolInspectionTarget.test.tsx src/ToolWorkspace-experience.test.tsx`，原恢复、诊断定位和工具页面体验 7 项通过。本阶段共 49 项不同用例。
- `npm.cmd run typecheck` 通过。`npm.cmd run check:i18n` 通过，10,017 对词条，生产界面硬编码 0。
- 项目根目录运行 `node tests/integration/tool-identity-management/build.cjs`，输出到任务忽略目录。使用既有 Playwright Core 和本机 Chrome 运行 `node tests/integration/tool-identity-management/driver.cjs`，进程级设置 `SUMA_PLAYWRIGHT_MODULE`、`SUMA_BROWSER_EXECUTABLE`、`TEMP` 与 `TMP`。
- 生产组件与合成回环 HTTP 的两组浏览器旅程通过。中文浅色 1366px 验证筛选、过期限制、撤销回执及所选节点的配置库存读取。英文深色 390px 验证权限复用、明确确认、原操作记录与唯一 POST 一致、回执后进入折叠历史。目标控件至少 44px，两个页面无横向溢出，23 个请求中 2 个为夹具写入，浏览器与端口已关闭。
- 早期 Electron 尝试在页面加载前遇到 GPU 子进程错误。Chrome 初次受限启动同样未到页面。隔离 Chrome 随后运行，修正夹具对读取头和目录字段的假设后两组通过。原失败回执和成功截图分别保留。

最终回执 `.cache/tool-identity-management/visual/run-5JUcnX/result.json`，SHA256 `11443734fd659ce3b93bcdf864768136fc668195428b9f731a1710b78285edae`。本阶段沿原授权、撤销和断开合同验证界面交互。普通节点凭证轮换和真实 WorkBuddy 供应商授权仍需独立验收。完整平台原 31 项和 P0–P7 继续推进。
