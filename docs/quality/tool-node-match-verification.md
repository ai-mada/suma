# 添加工具的可用节点查找验证

T-TOOL-NODE-MATCH-273 · 速码 1.0.0 · 2026-11-06。

| 检查 | 实际结果 |
| --- | --- |
| Windows、Node 22.16.0，在 `apps/web` 运行 `node node_modules/vitest/vitest.mjs run --no-cache ToolQuickConnect LocalToolsScanQuickConnect QuickConnectIntegration QoderQuickConnectCoordination ToolProfileReplacementIntegration` | 10 个文件、37 项通过。覆盖唯一匹配、部分读取失败、晚到结果无法覆盖手动选择，以及原本机接入、已有配置复用、替换与保存恢复 |
| 在 `apps/web` 运行 `node node_modules/typescript/bin/tsc --noEmit --pretty false`，在仓库根目录运行 `node apps/web/scripts/check-i18n.mjs` | Web 全量类型检查和 10,882 对词条通过 |
| Playwright 驱动 Windows Chrome，使用隔离 Vite 构建加载实际添加工具组件 | 中文桌面 1366×768 核对部分失败、明确选择与暂停保存，英文深色窄屏 390×844 核对唯一匹配、预填名称与暂停保存。两条路径都验证键盘启动查找、选中后重新读取、保存前零写入和零渲染异常 |

浏览器使用受控的双节点库存和保存回执。真实 Windows 桌面的 9 个入口检测及保存恢复见 [上一阶段验证](tool-quick-recovery-verification.md)。本阶段的真实多节点联机及完整发布验收待实际环境核对。

浏览器截图、结果和校验摘要位于忽略目录 `dist/quality/tool-node-match-273/`。
