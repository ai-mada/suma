# 用量证据纳秒排序验收

状态：T-USAGE-EVIDENCE-ORDER-280。本机定向验证。版本：1.0.0。范围：项目用量证据的时间排序与分页边界。

排序把账本 UTC 时间的小数补齐至九位。按时间倒序，时间相同按事实 ID 倒序，继续读取使用同一组键。保留原时间文本和原游标 ID。数据库在项目、尝试及时间范围内排序证据键，最多读取一页加一条用于判断是否还有下一页的原件。 [用量合同](../../contracts/usage-accounting.md) · [架构](../architecture/usage-accounting.md)

真实 HTTP 与 SQLite 回归使用同一秒的四条证据。旧查询把整秒记录排在小数秒之前。新查询以每页一条返回两条 `.11Z`（ID 倒序）、一条 `.1Z` 和整秒记录，游标跨过相同时间与不同小数精度。沿用的用例同时核对项目权限、原件字段和账本结果。PostgreSQL、OceanBase 及生产负载仍待实测。

```powershell
go test -p 2 ./internal/platform -run '^(TestUsageAttemptFactsHTTP|TestUsageAccountingLateStopPreservesManualConfirmation)$' -count=1 -v
```
