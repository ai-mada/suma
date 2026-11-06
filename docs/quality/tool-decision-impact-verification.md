# T-TOOL-DECISION-IMPACT 阶段验证

2026-11-06，Windows amd64，Node 22.16.0，Vitest 3.0.5，Chrome 156.0.8078.80，Go 1.26.10 与隔离 SQLite。设计见[停用与移除确认](../design/tool-management.md#停用与移除确认)。

- 在 `apps/web` 执行 `node ./node_modules/vitest/vitest.mjs run src/ToolProfileDecision.test.tsx src/ToolWorkspaceManagement.test.tsx src/ToolWorkspaceQuickReuse.test.tsx src/ToolProfileDoctor.test.tsx src/tool-operation-ownership.test.tsx --maxWorkers 2 --minWorkers 2`。首轮 31 项通过。快捷接入用例仍指向早期列表内按钮，改用现有入口选择器后单独复验通过。共 32 个不同用例。覆盖当前来源、过期、版本冲突重新确认、提交时撤权和原操作恢复。Doctor 原用例输出 React `act` 警告，断言通过。
- 在 `apps/web` 执行 `node ./node_modules/typescript/bin/tsc --noEmit` 与 `node scripts/check-i18n.mjs`。类型与 10,768 个语言键通过。
- 运行 `& .cache/tool-decision-impact/run.ps1`。实际 Web 工作台、HTTP 和 SQLite 联调通过。英文亮色桌面验证诊断失败后重试、读取准确活动节点引用以及取消后零写入。中文暗色 390px 验证真实并发变更后的 409、重新读取第 3 版、旧确认清除、用户再次确认后移除至第 4 版并可查历史。
- 实际联调记录 29 次服务端 GET，其中诊断 6 次。浏览器两次 POST 分别返回 409 和 200，只产生一次移除。另一次持久操作是夹具模拟的并发停用。运行和事件增量均为 0，原运行与节点保持运行中。首轮浏览器验收的移动端历史定位步骤已修正为先关闭详情再返回列表。最终中英文截图和回执保存在忽略目录 `.cache/tool-decision-impact`。
- 提交拒绝后隐藏旧影响和恢复权限后重新确认，由后续定向测试核验。供应方进程状态、物理多节点和生产环境按原交付矩阵独立验收。
