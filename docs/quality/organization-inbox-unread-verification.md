# 组织收件箱顶栏未读提示验收

阶段 T-ORGANIZATION-INBOX-UNREAD-282，版本 1.0.0。[读取合同](../../contracts/notification-inbox.md#组织未读摘要) · [交互设计](../design/notification-inbox.md#顶栏未读提示)

- Windows、Go 1.26.10 和隔离 SQLite 上，组织收件箱及未读摘要的四个服务端用例、两个接口合同用例通过。覆盖当前账号、项目成员角色、撤权、归档、恢复、已读以及非浏览器凭证。摘要响应只包含账号、组织和计数，并声明禁止缓存。
- Web 的 31 个现有收件箱用例及 3 个顶栏用例通过。顶栏用例覆盖无项目账号打开组织收件箱、身份切换后忽略晚到响应、拒绝错范围和异常响应、中英文标签及长计数的完整可读名称。
- 本机隔离 Chrome 加载真实工作台组件和样式，使用隔离 API 固定数据。英文浅色 1366px 和中文深色 390px 均通过键盘打开组织收件箱，打开入口无写请求。窄屏标为已读后重读计数并清除徽标。两种布局的按钮均在视口内，未观察到脚本错误。

```powershell
go test -p 2 ./internal/platform ./cmd/suma-contracts -run '^TestNotificationOrganization(Inbox|UnreadSummary)' -count=1 -v
go run -p 2 ./cmd/suma-contracts -check
pnpm --filter @suma/web test src/NotificationInboxHeader.test.tsx src/NotificationInbox.test.tsx --minWorkers 1 --maxWorkers 2
pnpm --filter @suma/web typecheck
pnpm --filter @suma/web check:i18n
node scripts/check-docs.mjs
```

数据库和浏览器证据分开记录。本阶段浏览器使用固定响应，跨端实时送达、生产性能、PostgreSQL 与 OceanBase 尚待实测。
