# 修复轮次的自动化来源验收

阶段 T-REPAIR-AUTOMATION-BASIS-293，版本 1.0.0。 [使用指南](../development/issue-repair.md#自动化故障的修复依据) · [接口合同](../../contracts/issue-repair.md#修复轮次的自动化来源)

- SQLite 和 HTTP 核验准确失败问题可在准备修复时固定原运行、触发和暂停依据。规则及来源后来变更，浏览器读取和重放仍保留原快照。普通问题的可编辑来源字段不能伪造依据。项目 API 读取和重放省略浏览器专属字段；重放没有创建额外修复任务或启动运行。
- 16 项 Web 定向用例通过。Windows Chrome 使用真实控制面和独立 SQLite，由后台投影失败问题。英文 1366px 显式填写条件并提交准备修复，创建待办任务且关单仍受限。规则改版后，中文深色 390px 打开本轮快照，用键盘进入原暂停证据。浏览器只发出一次明确的修复准备写入。
- 生成合同、TypeScript、中英词条和文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationPauseInboxHTTP|TestIssueRepairHTTPClosedInputAndExactOriginalOperation|TestIssueRepairGenericBypassAndCanonicalPreparation|TestIssueRepairContract|TestFailureDiagnosisIssueSource)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/IssueRepairPanel.test.tsx src/FailureDiagnosisLauncher.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

本阶段验收覆盖证据固化、任务准备和原证据导航。跨节点负载、外部通知及 OceanBase 实机验收仍待完成。
