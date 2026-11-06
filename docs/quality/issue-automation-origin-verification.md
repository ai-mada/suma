# 失败问题的自动化来源回溯验收

阶段 T-ISSUE-AUTOMATION-ORIGIN-292，版本 1.0.0。 [交互设计](../design/automation-workspace.md#从问题分诊返回暂停依据) · [接口合同](../../contracts/failure-diagnosis.md#自动化问题的来源回溯)

- SQLite 和 HTTP 核验同项目的正式失败问题可回溯准确运行、原触发和暂停依据，规则修订后仍保留历史链接。普通问题、损坏的自动化来源和跨项目访问不会暴露额外证据。同项目 API 凭据只取得原有运行关联。
- 8 项 Web 定向用例通过。Windows Chrome 使用真实控制面和独立 SQLite，由后台投影失败问题。英文 1366px 从暂停证据经问题详情用键盘回到原触发。中文深色 390px 同样从问题详情回到准确的受阻触发，证据中仍可打开原问题。两种界面保留运行入口，浏览器没有发出业务写入。
- 生成合同、TypeScript、中英词条和文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationPauseInboxHTTP|TestAutomationRunOrigin|TestFailureDiagnosisIssueSource|TestAutomationAndStreamContracts)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/FailureDiagnosisLauncher.test.tsx src/RunAutomationOrigin.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

回溯以当前项目权限及可核对的历史来源为依据。外部通知、分布式负载和 OceanBase 实机验收仍待完成。
