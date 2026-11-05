# 添加工具配置核对验证

T-TOOL-QUICK-REVIEW-240 · 速码 1.0.0 · 2026-11-05。

| 检查 | 结果 |
| --- | --- |
| Windows、Node 22.16.0，在 Web 目录执行 `node node_modules/vitest/vitest.mjs run LocalToolsScan ToolQuickConnect QuickConnectIntegration QoderQuickConnectCoordination ToolProfileReplacementIntegration ManagedToolPreparationPanel ToolWorkspace-experience RunnerPairingWorkspace ToolWorkspaceReportFeedback` | 15 文件、61 用例通过。覆盖自动选择与草稿、名称表单、已有配置复用、跨节点替换、节点接入和原操作恢复。节点接入测试使用其固定数据对应的时间 |
| `node apps/web/node_modules/typescript/bin/tsc --project apps/web/tsconfig.json` 与 `node apps/web/scripts/check-i18n.mjs` | Web 全量类型检查通过，10,327 对语言词条检查通过 |
| 隔离 Vite 构建和 Playwright 驱动 Chrome | 使用实际添加工具弹窗组件及合成扫描、库存、匹配和保存回执。中文亮色 1366×768 和英文深色 390×844 都在首屏显示保存操作，无横向溢出。验证节点详情展开、节点检测打开和收起、过期库存重读、回车保存与暂停回执焦点。两条路径各一次保存请求，控制台无错误或警告 |

浏览器证据限于上述合成场景。真实桌面安装、厂商授权及模型运行按对应合同验证。隔离构建使用任务临时目录。
