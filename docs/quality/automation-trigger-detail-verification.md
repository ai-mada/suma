# 自动化精确触发详情验收

阶段 T-AUTOMATION-TRIGGER-DETAIL-288，版本 1.0.0。 [交互设计](../design/automation-workspace.md#精确触发详情) · [接口合同](../../contracts/automation-events.md)

- 真实 SQLite 和 HTTP 验证历史页外读取、原修订和阻塞原因、同范围关联运行与待核对节点、异项目运行屏蔽、规则与项目隔离、项目授权撤销、缓存禁用及闭合查询。自动化关注筛选和暂停收件箱定向回归同时通过。
- 45 项 Web 用例通过。切换目标会终止旧请求，范围不匹配的响应不能显示，明确区分重试和 404，关闭后恢复焦点。收件箱保留显式已读及归档动作。
- Windows Chrome 连接独立的真实控制面和 SQLite。英文浅色 1366px 用键盘从另一项目的组织收件箱直接打开原触发，在规则已修订时仍显示原修订、阻塞原因和当前规则标题，刷新可恢复同一记录。中文深色 390px 从项目收件箱打开，内容无横向溢出。显式已读后未读数变为 0，打开或刷新无业务写入。
- OpenAPI、TypeScript、中英词条和文档链接检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationOccurrenceDetailHTTP|TestAutomationOccurrenceAttentionHTTP|TestAutomationPauseInboxHTTP|TestAutomationAndStreamContracts)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/AutomationOccurrences.test.tsx src/NotificationInbox.test.tsx src/notification-inbox-navigation.test.ts --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

本阶段覆盖站内触发记录定位。外部消息渠道、分布式规模和 OceanBase 实机验收仍待完成。
