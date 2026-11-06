# 自动故障问题回执键保护

速码 1.0.0，`T-FAILURE-ISSUE-KEY-RESERVATION-271`。关联需求为外部执行、问题自动回流、重试与验收。 [架构](../architecture/failure-diagnosis.md)

通用项目命令和候选制品传输与系统故障投影共用回执表。两个入口均在写入前拒绝故障投影的保留前缀及大小写变体，防止其他命令占用尚未投影的运行键。OceanBase 在这两类原始键入口限制为可见 ASCII，针对 Unicode 排序规则别名。 [OceanBase 字符集与排序规则](https://docs.oceanbase.com/oceanbase-database/V4.6.0-character-set-mysql-compatible-mode)

## 阶段验证

- Windows、Go 1.26.10，4 项定向测试通过。真实 SQLite 和 HTTP 测试使用相同项目内的测试计划及 Runner 制品传输，核对保留键返回 400、无部分写入及普通键正常提交。随后触发真实 DAG 审批失败，重复投影只生成一条问题和一份确认，问题转需求仍幂等。
- 同轮覆盖 52 个真实失败事件的持久重试，以及验收退回的正式问题。OceanBase ASCII 规则通过逻辑单测，实机排序规则与跨节点仍待验证。现有冲突回执保留原证据。证据在本机忽略目录 `.cache/q271/`。
- 单独重跑上阶段未进入断言的托管发布集成用例，通过实际本机 Runner、SQLite、受控发布流程和问题回流。故障记录保留准确部署操作与运行，重复投影和重复转需求均保持唯一。
