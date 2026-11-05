# 仓库来源交付工作台验证

T-REPOSITORY-SOURCE-WORKBENCH-246 · 速码 1.0.0 · 2026-11-05。 [交互设计](../design/repository-source-workbench.md)

| 验证 | 实际结果 |
| --- | --- |
| 实际 Git、回环提供方、SQLite 与 Suma HTTP | 审批人查询到固定来源、可信基线、原尝试和不同 Worker 档案。提供方元数据返回 503 时，已留存回执仍可用于预览和启动。普通开发者及非法查询被拒绝。两个实际项目 Runner 封存并测试同一提交，隔离验收通过。 |
| Web 组件 | 12 项通过，包含提供方不可用时选用留存来源、任务已换绑时恢复丢失的启动回执。TypeScript 和中英文词条检查通过。 |
| Chrome | 390 像素中英文及 1280 像素中文均无横向溢出。键盘完成批准、预览和独立确认启动。错误审批回执通过原操作键恢复，权限撤销与过期响应阻止新写入。共 24 个业务请求，4 个 POST 含 1 次原操作重放。 |

复用现有任务交付面板、操作回执协议及候选运行路由。确切提交的评审依据参考 Multica 固定 PR HEAD 的设计。 [Multica 评审绑定](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L1241-L1265)

Chrome 使用真实速码组件与隔离模拟响应。真实 Git、HTTP 和 Runner 由独立集成测试覆盖。真实 WorkBuddy 账号、跨主机 Pool 及生产部署验收仍待验证。
