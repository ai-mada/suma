# 模型管理目录验证

2026-11-06 · T-MODEL-DIRECTORY · Windows amd64、Node 22.16.0、Vitest 3.0.5、Vite 6.1.0。范围为模型管理的项目只读目录。浏览器使用本轮构建和受控 HTTP 响应。

| 检查 | 执行与结果 |
| --- | --- |
| 定向回归 | `node apps/web/node_modules/vitest/vitest.mjs run --root apps/web src/ModelDirectory.test.tsx src/ModelExecutionPanel.test.tsx src/model-execution-model.test.ts src/tool-management-list-model.test.ts src/api-context.test.tsx src/UsageWorkspace.test.tsx`，6 个文件、42 项通过。目录 16 项覆盖四类固定模型、Codex 预算限制、准确费率和目录响应、先核对当前工具再重读、链接恢复、切换竞态及撤权。 |
| 类型与本地化 | `node apps/web/node_modules/typescript/bin/tsc --noEmit -p apps/web/tsconfig.json` 与 `node apps/web/scripts/check-i18n.mjs` 均退出 0，10,823 个成对词条。直接使用本地依赖执行，因 pnpm 的工作区依赖快照检查要求重新安装。 |
| 隔离构建 | `node apps/web/node_modules/vite/bin/vite.js build apps/web --outDir ../../.cache/model-directory-complete/web --emptyOutDir` 退出 0。Vite 报告已有的大包体积提示。 |
| 浏览器 | 本机 Chrome + Playwright 在 `127.0.0.1:5197` 打开本轮构建，检查 1440 px 桌面与 390 px 窄屏。零来源接入入口、精确费率和未定价、搜索清空、来源分页、固定模型链接恢复、浏览器返回、模拟 409 清除旧目录及 v3 重读、中英文切换均通过。页面运行异常 0，控制台仅记录预期的模拟 409。桌面与窄屏截图已人工核看。 |

证据 SHA-256。桌面图 `53cf5ea98c7ddaa0604d3e00a3c4ff0118e0def6d36a3062712ef87622a146bb`，窄屏图 `a6085974e3e793e7579e2ddfe850df3b9f1554bf9e120aef1d048652d0ae328d`，浏览器回执 `2b9a0d0646c0270beba8a2b793e2c582ec92135450b13c91d91d323db1d62273`。

真实节点、厂商账号和真实账单联调待有对应环境时独立验证。本轮 Go 1.26.10 工具链自动下载遇到本机缓存写权限限制，未复跑 Go 用例。当前任务变更为 Web 目录，沿用已有 Go 项目授权和模型协议。
