# 自动化运行回溯验收

阶段 T-AUTOMATION-RUN-PAUSE-TRACE-290，版本 1.0.0。 [交互设计](../design/automation-workspace.md#运行详情的自动化回溯) · [接口合同](../../contracts/automation-events.md#从运行回溯原触发与暂停)

- SQLite 和 HTTP 核验实际参与暂停的两条失败运行均能回溯原决定，较早未计入的运行只有自己的原触发。当前规则修订或运行状态变化后仍保留原回链。证据歧义、触发损坏或异项目运行隐藏暂停回链，保留可核对的原运行来源。当前项目读取授权继续生效。
- 6 项 Web 定向用例通过，覆盖独立导航、离开保护及异常回链。Windows Chrome 连接真实控制面和独立 SQLite。规则修订后，英文 1366px 用键盘从运行详情打开原触发。中文深色 390px 用键盘打开造成暂停的不同触发并找到该运行，界面无横向溢出。两个入口均无业务写入。
- 生成合同、TypeScript、中英词条和文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationRunOrigin|TestAutomationPauseInboxHTTP|TestAutomationAndStreamContracts)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/RunAutomationOrigin.test.tsx src/AutomationOccurrences.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

回溯以已有暂停证据为依据。外部渠道、跨节点负载和 OceanBase 实机验收仍待完成。
