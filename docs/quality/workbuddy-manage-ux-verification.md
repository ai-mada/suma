# WorkBuddy 连接管理交互验证

T-WORKBUDDY-MANAGE-UX，2026-11-05，Windows amd64，Node 22.16.0。见[交互设计](../design/workbuddy-connections.md)。

| 检查 | 结果 |
| --- | --- |
| 在 `apps/web` 运行 `node ./node_modules/vitest/vitest.mjs run src/WorkBuddyConnections.test.tsx src/workbuddy-connection-presentation.test.ts` | 41 项通过。两项回归先复现缺少快捷授权和刷新结果未知时的通用回执，再验证仅沿用当前允许的原权限、精确读取原连接、结果仍未知时保留状态。 |
| 在 `apps/web` 运行 `node ./node_modules/typescript/bin/tsc --noEmit` 与 `node scripts/check-i18n.mjs` | 类型检查及 10350 个语言键通过。 |
| 在 `apps/web` 运行 `node ./node_modules/vite/bin/vite.js build --logLevel warn --outDir D:/codes/ai-dev-os/suma/.cache/workbuddy-manage-ux/dist-final` | 隔离生产构建通过，输出包含 Vite 的分包体积提示。 |
| Edge 与 Playwright，隔离组件，1366px 中文浅色及 390px 英文深色 | 桌面用键盘展开管理，核对访问到期、刷新返回未知和准确连接读取。移动端初始只显示快捷权限选择，打开调整后焦点进入准确勾选项，取消未写入，快捷继续只有一次授权准备请求。两组主要操作至少 44px，无横向溢出和页面错误。 |

截图 SHA-256 为 `17c5689e6b22b9a4a90780720a4723747f3d86a8189acf096bc888992fb324f5`（桌面刷新结果）和 `52da5f52c3f8ad73b3635c58208110de181f42a042e8d8d183754e2dc322cd2f`（移动端重新授权）。代表性页面使用本地测试响应。真实 WorkBuddy 应用和账号联调在部署配置完成后验收。
