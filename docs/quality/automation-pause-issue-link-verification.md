# 自动化暂停证据与问题跟进验收

阶段 T-AUTOMATION-PAUSE-ISSUE-LINK-291，版本 1.0.0。 [交互设计](../design/automation-workspace.md#查看失败运行对应的问题) · [接口合同](../../contracts/automation-events.md#从暂停依据查看问题)

- SQLite 和 HTTP 核验正式失败事件、系统回执及当前项目问题共同建立链接。普通手填元数据、损坏回执、跨项目或不可用的问题均不会产生链接。变更现有问题的可编辑元数据不影响经原回执验证的关联。读取不修改暂停证据、问题、投影回执和活动记录。
- 6 项 Web 定向用例通过。Windows Chrome 使用真实控制面和独立 SQLite，由后台实际投影两条失败问题。英文 1366px 与中文深色 390px 均从原暂停证据用键盘打开准确问题详情，保留运行入口。查看前后问题列表一致，浏览器没有发出写入。
- 生成合同、TypeScript、中英词条和文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationPauseInboxHTTP|TestAutomationRunOrigin|TestFailureDiagnosisIssueSource|TestFailureIssueKey|TestAutomationAndStreamContracts)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/AutomationOccurrences.test.tsx src/RunAutomationOrigin.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

问题链接在现有投影通过核对后显示。外部通知、分布式负载和 OceanBase 实机验收仍待完成。
