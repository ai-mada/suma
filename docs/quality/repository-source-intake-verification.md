# WorkBuddy 待交付仓库来源审批验证

T-REPOSITORY-SOURCE-INTAKE-244 · 速码 1.0.0 · 2026-11-05。 [接口合同](../../contracts/workbuddy-task-dispatch.md#批准并保留确切仓库来源) · [部署配置](../deployment/repository-integration.md#启用登记分支-git-包导出)

| 范围 | 实际证据 |
| --- | --- |
| 控制面、SQLite、真实 Git 裸仓库和回环提供方 | 原 WorkBuddy 尝试待交付，项目审批人通过 HTTP 确认确切提交及基线。原 Git 包保留在受限目录，公开回执包含摘要和字节数。查询期间原执行代次变化会拒绝保留。同键恢复原回执，不再次调用提供方。查看者及开发者被拒绝。 |
| 仓库提供方 | 真实 Git 验证成对元数据按实际分支读取，独立核对凭据可用。拒绝自行指定基线及使用导出凭据查询元数据。 |
| 二进制传输 | 12 个响应场景涵盖摘要不符、超限、截断、重定向及提供方状态。回调、Runner、核对和包导出凭据各自独立。 |
| Runner | 原候选与来源包共享 Git 导入校验。实际提供方导出的包成功导入仅含基线的独立仓库，摘要或头提交不符被拒绝。原候选合成回归通过。 |
| 接口生成 | 定向 Go 合同测试通过。OpenAPI 和 TypeScript 增量为一个路由及两个类型，生成器一致性检查通过。 |

设计参考 Multica 为评审固定 PR HEAD 的做法。速码的留存回执同时绑定原任务与执行代次、管理员基线及包摘要。 [Multica 评审绑定](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L1241-L1265)

本阶段提供审批 API 和受限留存。WorkBuddy 状态由回环夹具建立，未接入真实账号。工作台审批交互、绑定正式候选运行及不同 Runner 的同提交测试仍待完成。测试仅在 Windows、SQLite 和隔离 Git 环境执行。
