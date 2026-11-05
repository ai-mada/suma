# 添加工具快捷选择验证

T-TOOL-QUICK-CHOICES-241 · 速码 1.0.0 · 2026-11-05。

| 检查 | 结果 |
| --- | --- |
| Windows、Node 22.16.0，Web 目录运行 `node node_modules/vitest/vitest.mjs run LocalToolsScan ToolQuickConnect QuickConnectIntegration QoderQuickConnectCoordination ToolProfileReplacementIntegration ToolWorkspace-experience` | 12 文件、48 用例通过。覆盖本机快捷入口、过期回退、多配置明确选择、单份已有配置复用、原操作锁和替换流程 |
| `node apps/web/node_modules/typescript/bin/tsc --project apps/web/tsconfig.json`、`node apps/web/scripts/check-i18n.mjs` | Web 全量类型检查及 10,334 对词条检查通过 |
| 隔离 Vite 构建、Playwright 驱动 Chrome | 中文亮色 1366×768、英文深色 390×844。使用真实弹窗组件和合成数据验证 Codex 两种入口、键盘选择、配置读写标识及单份已有配置的查看或启用入口。详情默认折叠，无横向溢出。两组均无写入、无控制台错误或警告 |

真实安装、厂商授权与模型运行按对应合同验收。浏览器构建使用隔离目录。
