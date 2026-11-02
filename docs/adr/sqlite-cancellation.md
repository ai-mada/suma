# SQLite取消后的连接与文件释放

状态：采纳；负责人：主集成／存储与质量；版本1.0.0；关联T-TEST-DISPATCH-BINDINGS-216。

发布用例使用真实开发Worker和独立测试Worker后，取消轮询偶发留下数据库文件占用，并使后续请求等待锁。测试宿主关闭后活动HTTP请求为0、数据库池也可能显示0连接；这些指标不足以证明SQLite底层statement已释放。

定位到原modernc.org/sqlite v1.36.3的查询取消路径：已有Rows的原生statement所有权已转移，取消分支却直接丢弃Rows。采用官方稳定v1.61.1及其准确要求的libc v1.79.0，复用上游修复，不创建驱动fork、第二套连接清理或绕过取消。官方[版本变更](https://github.com/modernc-org/sqlite/blob/v1.61.1/CHANGELOG.md)记录v1.40.1的Rows取消泄漏修复及v1.42.2的误中断后续查询修复；[准确依赖](https://github.com/modernc-org/sqlite/blob/v1.61.1/go.mod)与[查询实现](https://github.com/modernc-org/sqlite/blob/v1.61.1/stmt.go)构成采用依据。

原业务事务、当前授权、busy_timeout恢复、候选固定、审批及数据Schema保持同一实现。八项依赖升级和原许可摘要见[依赖增量](../delivery/sqlite-cancellation-dependencies.json)。初始化版本没有增加历史兼容分支。

回归使用真实SQLite函数与取消屏障，公开driver接口仅在同一次Conn.Raw回调有效期内使用；随后检查原连接checkpoint、另一物理连接写入及checkpoint、关闭后的Windows文件改名。原始失败及修正后的旧锁定版本RED均保留，区别夹具API生命周期错误。最终结果、受影响平台检查与未运行范围见[限定验证](../quality/test-dispatch-bindings-verification.md)。

这项决定不替代其他操作系统、OceanBase／PostgreSQL、长时间压力、完整服务包或生产验收。
