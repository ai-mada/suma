# WorkBuddy 直接授权验证

T-WORKBUDDY-DIRECT-AUTH，2026-11-06，Windows amd64，Node 22.16.0。见[交互设计](../design/workbuddy-connections.md)及[领域合同](../../contracts/workbuddy-connections.md)。

| 检查 | 结果 |
| --- | --- |
| 在 `apps/web` 运行 `node ./node_modules/vitest/vitest.mjs run src/WorkBuddyConnections.test.tsx src/workbuddy-connection-presentation.test.ts` | 46 项通过。直接打开和失败恢复用例先复现旧版缺少直接入口，再核验授权回执与当前权限、未知结果沿原操作恢复、视角切换关闭旧标签页、拦截后聚焦备用入口及跨分页定位原记录。 |
| 在 `apps/web` 运行 `node scripts/check-i18n.mjs` 和 `node ./node_modules/typescript/bin/tsc --noEmit` | 10608 个语言键及类型检查通过。 |
| `node --test apps/desktop/tests/oauth-bridge-main.test.cjs apps/desktop/tests/oauth-window.test.cjs` | 18 项通过。前端直开测试同时核对四个原始身份参数、重复打开保护及重新授权的原记录版本。 |
| 在 `apps/web` 运行 `node ./node_modules/vite/bin/vite.js build --logLevel warn --outDir D:/codes/ai-dev-os/suma/.cache/workbuddy-direct-auth/dist` | 隔离生产构建通过，输出包含 Vite 分包体积提示。 |
| Edge 与 Playwright，隔离组件，1366px 中文浅色及 390px 英文深色 | 桌面浏览器键盘一次发起，等待当前视角复读后导航。移动端模拟拦截，备用入口可继续，断开结果展开原记录。每组一次授权准备写入、一次返回精确读取和一次拦截的供应方导航，供应方页面无 opener，导航请求无 Referer。主要操作至少 44px，无横向溢出及页面错误。 |

截图 SHA-256 为 `ab551a80bd7f0f8ba507341ddcf4932ccc4a4a0c2fc710d34f4345df6659ac56`（首次授权）和 `f5a7871056e32589811e288f935157ec866e0cd69e8a6e32b14a1661e4c29391`（重新授权）。

浏览器使用拦截的供应方页面和测试响应。真实 WorkBuddy 应用和账号联调在部署配置完成后验收。
