# 故障问题投影的持久重试

速码 1.0.0，`T-FAILURE-PROJECTION-RETRY-270`。关联需求为自动问题回流、后台恢复及跨节点调度。 [架构](../architecture/failure-diagnosis.md)

参考 Multica 以正式失败记录串行核对恢复事件的做法。速码在既有故障投影上分配首次处理和到期重试名额，并按失败事件序号串行保存重试与确认。 [Multica 固定参考](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L6589-L6615)

## 阶段验证

- Windows、Go 1.26.10、实际 SQLite。8 项定向测试通过。回归触发 52 个真实 DAG 失败事件，其中 51 个遇到隔离数据库的写入阻断。服务实例重建后，后到的健康事件仍生成问题，已阻断事件保留独立重试记录。解除阻断并让指定事件到期后，只生成一条问题和一份确认，过时代次不能推迟或重新挂起该事件。
- 同轮验证覆盖原问题已提交但尚未确认的重放、子流程超时、通知与问题同事务、撤权及索引投影初始化。OceanBase、PostgreSQL 和真实多节点并发仍待验证。证据保存在本机忽略目录 `.cache/q270/`。
- 补充的托管发布失败集成用例未完成。当前变更与移除本阶段补丁的对照运行，都在准备发布候选时等待执行节点超时，均未进入故障投影断言。

该用例随后在 T271 的本机运行通过，详见[后续验收](failure-issue-key-reservation-verification.md)。
