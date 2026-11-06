# 用量证据工作台验收

状态：T-USAGE-EVIDENCE-279 已通过本机定向验证。版本：1.0.0。范围：项目用量尝试、证据分页和工作台呈现。

## 操作

在「设置 → 用量与额度」的尝试记录下展开「用量证据」。历史区分原始执行回执、后续执行回执、人工用量确认和执行结果核对。显示 UTC 时间、用量覆盖状态及已报告计数，保留零值和未知的区别。按需展开摘要，使用「加载更早的证据」续页。账本汇总与原始观察分别呈现。 [用量合同](../../contracts/usage-accounting.md)

服务端在项目读取权限事务内核对原尝试和游标，用原账本索引读取至多一页。返回固定证据字段，原运行输出、提示词、内部确认正文及凭据不进入读接口。客户端仅在展开时请求，切换范围取消旧读取。权限失效时清除已加载证据。 [架构](../architecture/usage-accounting.md)

参考 Multica 的任务用量可追溯方向。速码将已保存的执行观察、人工结算及其账本摘要分开展示。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/pkg/db/queries/task_usage.sql#L1-L23)

## 验收

- 10 项 Go 顶层检查及 5 项子场景通过。真实 HTTP 与 SQLite 读取原始回执、核对、人工确认及后续回执，检查准确分页、权限撤销、私有内容边界和零值。
- 7 项 Web 组件检查通过。覆盖按需展开、继续读取、切换尝试、撤权和现有用量操作。生成合同通过一致性检查。
- Chrome 使用带模拟接口的完整用量工作台，验证中文 390px、英文 1280px、键盘展开与续页、UTC 时分秒，以及汇总 14 与回执观察 25 并存。浏览器用例无写请求。
- 分页沿现有存档时间键和 ID 稳定排序。历史时间戳在同一秒内的分数位长不同，可能与真实先后顺序有细微差异。差额调整、分布式数据库和实际厂商账单仍待交付或验证。

```powershell
go test -p 2 ./internal/platform -run '^(TestUsageAttemptFactsHTTP|TestUsageAccounting|TestAuthorityUsage)' -count=1
# 在 apps/web 下运行
pnpm exec vitest run src/UsageAttemptFacts.test.tsx src/UsageWorkspace.test.tsx
```
