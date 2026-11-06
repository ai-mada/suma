# 故障回流增量扫描验收

状态：T-FAILURE-PROJECTION-FRONTIER-274 已通过本机定向验证。版本：1.0.0。范围：控制面 DAG 失败投影。

## 行为与边界

- 首次处理按持久序号读取固定范围。空闲循环通过索引判断范围是否为空。
- 只有持久化确认或重试记录成功，扫描位点才会越过该事件。记账失败时，后续候选仍可处理，位点保留在缺口之前。
- 退避到期的事件独立从重试账本领取，序号落在位点之前也能处理。重复投影按原问题回执恢复。
- DAG 写入以事务中的全局序号锁保证事件可见顺序，位点更新只能前进。

## 验证

Windows 本机使用真实 SQLite 执行了 10 项定向检查。验证了索引范围扫描、记账失败期间继续处理后续事件、重启后的缺口恢复、位点前的到期重试以及旧写入者无法回退位点。现有故障回流、收件箱、知识投影初始化和执行历史分页用例通过。

可复现命令如下。

```powershell
go test -p 2 ./internal/platform ./internal/dag -run '^(TestFailureProjection|TestTaskRunFailureInbox|TestDeletedSourceRejectsLateIndexHitAndProjectionSchemaIsInstalled|TestAttemptToolHistoryProgressRetriesAllPagesAndRestart)' -count=1 -v
```

PostgreSQL、OceanBase、跨物理节点及长期负载仍待实际验收。本阶段范围限于控制面与存储查询。

设计见[失败诊断架构](../architecture/failure-diagnosis.md)。
