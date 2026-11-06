# 自动化失败暂停收件箱验收

阶段 T-AUTOMATION-PAUSE-INBOX-287，版本 1.0.0。 [交互设计](../design/notification-inbox.md#自动化暂停的处理入口) · [接口合同](../../contracts/notification-inbox.md#自动化暂停通知)

- SQLite 和 HTTP 回归使用不同的规则创建者、审批启用人和触发者。只向原审批启用人写入一条通知。注入通知写入故障后，规则、触发队列、序号和通知一并回滚，未产生第三个运行。重试成功且重复调度不增写。
- 核对项目及组织未读数、项目权限变更和同账号跨组织隔离。修改规则后仍返回原修订与名称，原触发不再匹配时隐藏跳转入口。
- 59 项 Web 定向用例通过。Windows Chrome 使用真实控制面和独立 SQLite，英文 1366px 从另一项目的组织收件箱键盘进入原规则，通知保持未读且浏览器未写入业务数据。中文深色 390px 从项目收件箱显式标为已读，组织未读数降为 0，再成功进入原规则。页面无脚本错误或横向溢出。
- 生成合同、TypeScript、中英文词条及文档结构检查通过。

~~~powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^(TestAutomationPauseInbox|TestAutomationFailurePause|TestNotificationInbox|TestNotificationOrganization)' -count=1
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/NotificationInbox.test.tsx src/NotificationInboxHeader.test.tsx src/notification-inbox-navigation.test.ts src/notification-inbox-model.test.ts --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
~~~

当前覆盖项目与组织站内收件箱。外部消息渠道、跨进程通知规模及 OceanBase 实机验收仍待验证。
