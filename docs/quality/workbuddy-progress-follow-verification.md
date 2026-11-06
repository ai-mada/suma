# WorkBuddy 团队进度自动刷新阶段验证

阶段 T-WORKBUDDY-PROGRESS-FOLLOW，速码 1.0.0。 [交互](../design/workbuddy-cloud-task.md#持续跟踪已保存进度) · [合同](../../contracts/workbuddy-cloud-status-ui.md#自动刷新已保存进度)

- Chrome 157 中文深色 390px 连接隔离 Go HTTP／SQLite。项目查看者在准确任务和等待输入筛选下用键盘开启，等候一次 20 秒周期，确认数据与筛选一致、最近读取时间更新、焦点保留，最后触摸关闭。两次读取均为 GET，供应方调用增量与业务写入均为零。
- 受影响的 Web 测试 32 项通过，定向 DOM 验证默认关闭、可见性暂停、单一在途读取、分页暂停、手动恢复、读取失败时旧链接锁定，以及权限失效后清除记录。TypeScript 和中英文词条通过。

```powershell
node node_modules/vitest/vitest.mjs run --no-cache --minWorkers 1 --maxWorkers 1 src/WorkBuddyProgressWorkspace.test.tsx
node node_modules/typescript/bin/tsc --noEmit -p tsconfig.json
node scripts/check-i18n.mjs
node scripts/check-docs.mjs
```

自动刷新覆盖平台持久观察。真实供应方账号、跨物理节点和生产规模仍按独立阶段验收。
