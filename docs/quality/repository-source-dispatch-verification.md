# WorkBuddy 仓库来源派发与独立测试

T-REPOSITORY-SOURCE-DISPATCH-245 · 速码 1.0.0 · 2026-11-05。 [派发合同](../../contracts/workbuddy-task-dispatch.md#派发已批准仓库来源的候选和独立测试)

| 验证 | 实际结果 |
| --- | --- |
| Windows、SQLite、实际 Git、回环提供方和 Suma HTTP | 已审批来源进入正式候选计划和新任务运行。两个项目 Runner 使用只含基线的独立仓库。生产 Runner 隔离导入原提交并封存原包；另一 Runner 领取同一传递及提交，执行实际 JUnit。验收前置检查拦截提前通过；隔离测试账号用正式 Review 验收新运行，原 WorkBuddy 等待记录保留。 |
| 授权、重放与完整性 | 普通开发者不能从通用启动入口绕过审批。同键恢复原运行和版本。错误 Runner 或过期执行代次只能得到错误响应；不同包摘要不能创建候选传递。成功结果必须引用当前尝试真实封存的原包。 |
| Runner 回归 | 选定的工具命令未执行；实际 Git 导入不污染管理员仓库。既有普通项目和 Pool 候选传递回归通过。 |
| 接口合同 | 派发预览、普通 Runner 与 Pool 下载的定向生成合同测试通过。只纳入本阶段三个路由、一个请求类型及现有启动权限说明。 |

设计参考 Multica 把评审请求固定到关联 PR HEAD 的做法，速码沿现有的派发计划、DAG、候选传递及验收实现。 [Multica 评审绑定](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L1241-L1265)

本次通过 API 验证了项目 Runner 的完整链路。工作台的来源审批和派发交互、此新路径的真实 Pool 跨主机验证、真实 WorkBuddy 作者归属及生产验收仍待完成。当前生产导入的管理员基线对象闭包和来源 Git 包分别受 64 MiB 上限约束。
