# WorkBuddy 账号连接交互验证

T-WORKBUDDY-CONNECTION-UX，2026-11-05，Windows amd64，Node 22.16.0。见[交互设计](../design/workbuddy-connections.md)。

| 检查 | 结果 |
| --- | --- |
| 在 `apps/web` 运行 `node ./node_modules/vitest/vitest.mjs run src/WorkBuddyConnections.test.tsx src/workbuddy-connection-presentation.test.ts` | 39 项通过。新增用例先复现首次进入看不到权限的断点，再验证显式选权限、单次发起、官方入口焦点及隐藏重复待授权记录。桌面分支验证返回列表只切换本地视图。 |
| 在 `apps/web` 运行 `node ./node_modules/typescript/bin/tsc --noEmit` 和 `node scripts/check-i18n.mjs` | 类型检查与 10327 个语言键通过。 |
| 在 `apps/web` 运行 `node ./node_modules/vite/bin/vite.js build --logLevel warn --outDir D:/codes/ai-dev-os/suma/.cache/workbuddy-connection-ui/dist-final` | 隔离生产构建通过，输出包含 Vite 的分包体积提示。 |
| Edge 与 Playwright，隔离组件，1366px 中文浅色及 390px 英文深色 | 键盘直接选权限，继续后焦点进入官方入口，返回后核对同一连接，重新授权焦点进入表单。390px 用例也覆盖管理员配置说明。每组一次授权发起和一次精确连接读取。主要操作高度至少 44px，无横向溢出和页面错误。 |

浏览器截图摘要为 `7069d8ac49e2f5381a93cc7bb8e51b6e92259375c3fe283be986d6bbd68da9f6`（首次连接）和 `c741ff0366a989ea1e3cc00cbb67641a5298741bf6eff937eb05733eeb067b28`（移动端确认）。使用速码现有主题和控件规范。环境无法获取 12ui 工具包，视觉核对使用本地浏览器渲染。

供应方页面及连接响应使用本地测试替身，真实 WorkBuddy 应用和账号联调在部署配置完成后验收。
