# 修复任务交接验收

阶段 T-TASK-REPAIR-HANDOFF-294，版本 1.0.0。 [使用指南](../development/issue-repair.md#从任务接手修复) · [接口合同](../../contracts/task-delivery-chain.md#修复任务的接手依据)

- SQLite 和 HTTP 核验尚未派发的真实修复任务可读取同项目原问题和冻结的自动化证据。可编辑任务数据不能伪造绑定。跨项目读取被拒绝，项目 API 保留授权内的修复轮次和回归范围，省略浏览器专属证据。
- 已绑定的回归范围保留准确计划修订和用例，问题关闭及重开后原任务仍可辨认原轮次。回归绑定、独立测试结果和正式验收分别核对。
- 任务交付及工作台的 22 项前端定向测试通过。Windows Chrome 连接独立真实控制面与 SQLite。英文 1366px 从未派发任务打开原问题，中文深色 390px 从 IDE 任务上下文打开原暂停依据。IDE 只读取一次任务上下文，浏览器无业务写入。
- 生成合同、TypeScript、中英文词条与文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationPauseInboxHTTP|TestIssueRepairOriginalCaseGateCloseReopenAndReceipts|TestTaskDeliveryHTTPUndispatchedReadOnlySnapshot|TestTaskDeliveryBoundedReadContract|TestIssueRepairHTTPClosedInputAndExactOriginalOperation)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/TaskDeliveryPanel.test.tsx src/WorkbenchTaskContext.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

本阶段覆盖修复来源的只读交接。跨节点负载、外部通知及 OceanBase 实机验收仍待完成。
