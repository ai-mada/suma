# T-WORKBUDDY-DELIVERY-RETURN 阶段验证

2026-11-06，Windows amd64，Go 1.26.10，Node 22.16.0，Chrome 156.0.8078.80。使用隔离 SQLite 控制面和实际 Web 工作台。阶段设计见[云任务工作区](../design/workbuddy-cloud-task.md#返回原任务交付)。

| 检查 | 实际结果 |
| --- | --- |
| `go test ./internal/platform ./internal/workbuddycloudcontract ./cmd/suma-contracts -run 'WorkBuddyCloudNode\|CloudNodeView\|WorkBuddyCloud(Schema\|Route\|Progress)\|TaskDeliveryHTTP\|WorkBuddyTaskDispatchStartAtomic' -count=1 -v` | 16 个顶层测试和 18 个子场景通过。覆盖正式来源、绑定变化、手工节点、来源冲突、团队读取、撤权、项目隔离及现有 CLI 输出。 |
| `go run ./cmd/suma-contracts --check` | 1,085 个模型、490 条 API 路径及 TypeScript 生成类型通过。 |
| Web `node ./node_modules/vitest/vitest.mjs run src/workbuddy-cloud-task-model.test.ts src/WorkBuddyCloudTaskPanel.test.tsx src/WorkBuddyCloudTaskWorkspace.test.tsx src/task-delivery-navigation-model.test.ts src/TaskDeliveryPanel.test.tsx src/record-navigation-dom.test.tsx --maxWorkers 2 --minWorkers 2` | 6 个文件 52 项通过，包含过时记录、读取失败、拒绝、深链精确授权、导航保护和任务版本显示。 |
| Web `node ./node_modules/typescript/bin/tsc --noEmit` 与 `node scripts/check-i18n.mjs` | 类型通过，10,752 个语言键通过。 |
| `& .cache/workbuddy-delivery-return/run.ps1` | Chrome 联调通过。英文亮色桌面用键盘打开原任务交付，返回原运行及节点。中文暗色 390px 触控入口达到 44px，打开面板且无横向溢出。两端均核验实际任务版本。77 次 API 请求均为 GET，持久操作、运行、事件增量均为 0，供应方创建和查询均为 0。 |

首轮浏览器路径通过，目视发现中英文版本占位符未展开。修正插值并增加版本断言后，浏览器和 Web 定向检查均通过。最终截图与验证回执保存在本机忽略目录 `.cache/workbuddy-delivery-return`。

此阶段核验平台已保存的正式任务关系及导航。真实供应方执行、跨主机及生产验收继续按原交付矩阵记录。
