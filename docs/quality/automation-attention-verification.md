# 自动化触发排障验收

阶段 T-AUTOMATION-ATTENTION-285，版本 1.0.0。 [交互](../design/automation-workspace.md#筛选需要处理的触发) · [合同](../../contracts/automation-events.md#按需处理状态查询触发记录)

- Windows 和真实 SQLite HTTP。使用持久触发、运行和节点记录验证过滤发生在分页前，查询总量一致。范围不符、重复或非法筛选拒绝。关联外项目运行的异常记录不会提供运行链接。节点恢复后，后续筛选反映其最新状态。原触发记录、运行状态和待核对分别保留。
- 工作台新增交互回归验证带原账号、组织的读请求，筛选及续页，拒绝迟到的旧筛选响应，以及 403 后清除记录、计数和游标。相关合同、界面和国际化测试通过。
- 隔离 Chrome 使用实际工作台及固定 API 响应。英文浅色 1366px 和中文深色 390px 均通过键盘切换筛选，准确显示待核对提示和失败运行入口，续页使用需处理范围的游标。切回全部重新读取第一页，浏览过程中无写请求或脚本错误。

```powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationOccurrence|TestAutomationAndStreamContracts)' -count=1 -v
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/AutomationOccurrences.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
```

本阶段交付规则内的只读排障。自动暂停与告警通知需独立策略。生产负载和其他数据库仍待验证。
