# 自动化暂停依据验收

阶段 T-AUTOMATION-PAUSE-EVIDENCE-289，版本 1.0.0。 [交互设计](../design/automation-workspace.md#暂停依据与关联运行) · [接口合同](../../contracts/automation-events.md#暂停决策证据)

- SQLite 和 HTTP 核验两条真实失败运行达到阈值后，同事务保存规则暂停、队列阻塞、原触发证据与通知。注入通知写入失败时证据也回滚，重复调度不产生第二份。同批受阻记录和历史列表保留原投影。
- 规则修订、阈值及关联运行状态变更后，原证据保持一致。关联运行项目或触发绑定异常时隐藏证据。错误的项目范围仍返回 404。
- 4 项 Web 定向用例通过。Windows Chrome 使用真实控制面和独立 SQLite，英文 1366px 从另一项目的组织收件箱打开原触发，用键盘打开列表中的准确失败运行。中文深色 390px 显示两条失败运行和原修订，无横向溢出。查看证据与运行未产生业务写入。
- 生成合同、TypeScript、中英词条和文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationFailurePause|TestAutomationOccurrenceDetailHTTP|TestAutomationPauseInboxHTTP|TestAutomationAndStreamContracts)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/AutomationOccurrences.test.tsx src/AutomationFailurePolicy.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

早期暂停保留原原因和修订，新证据只来自实际暂停事务。外部渠道、跨节点负载和 OceanBase 实机验收仍待完成。
