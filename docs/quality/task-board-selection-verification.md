# 任务看板批量选择验证

T-MULTICA-BOARD-SELECTION。Windows、Vite 6.1.0、Chrome、任务隔离 SQLite。Web 构建输出到 `.cache/task-board-selection/web`。后端沿用上一阶段已验证的控制面二进制，当前阶段修改 Web 选择和交互。

- `npm.cmd run test -- src/TaskCollection.test.tsx` 27 项通过。覆盖看板勾选与详情独立、16 项上限、版本核对期间暂停勾选、列表与看板现有查询行为。
- `TaskViews.test.tsx`、`TaskIterationAssignments.test.tsx`、`TaskOwnerChanges.test.tsx` 分别通过 6、6、19 项。新增的 `TaskCollectionRecords.test.tsx -t 'shares exact loaded selections'` 通过，覆盖布局保留、版本变化、筛选清空和焦点。
- `TaskCollectionRecords.test.tsx` 整文件连续运行在当前源码和 `6306b4d` 基线均出现挂起。范围隔离与撤权两项已执行通过，排序及保存视图用例单独运行在两份源码均通过。组合运行的挂起仍待定位。
- `npm.cmd run typecheck` 通过。 `npm.cmd run check:i18n` 通过 10078 对词条、0 项生产硬编码。 `vite build --outDir ../../.cache/task-board-selection/web --emptyOutDir --logLevel warn` 完成，保留现有大资源包警告。
- 实际 Chrome 通过 9 项断言。中文桌面跨泳道勾选后切换列表／看板，原迭代命令收到准确的两项版本并成功写入一次，界面和正式读取均显示目标迭代。390px 英文深色确认滚动时操作栏仍可触达、主要操作和卡片勾选有 44px 触控区域，清除选择恢复焦点。两张图均目视，未观察到页面错误和额外业务写入。
- 浏览器回执 `.cache/task-board-selection/browser-ePPB3C/result.json`，SHA256 `55eb9e02127457ce2948930c66692adfdf8e0c54d20279ee392c5739cf485874`。批量状态、优先级、全结果集选中和真实跨设备离线验收沿原计划。
