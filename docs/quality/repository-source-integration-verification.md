# 已验收仓库候选集成验证

T-REPOSITORY-SOURCE-INTEGRATION-248 · 速码 1.0.0 · 2026-11-06。负责人为平台集成。本地集成和界面组件检查已通过。前序候选与测试证据见[共享执行池仓库候选交接](repository-source-pool-verification.md)，受管仓库的权限、恢复及 Git 事务见[集成合同](../../contracts/repository-integration.md)和[部署配置](../deployment/repository-integration.md)。

## 旅程与用户操作

从任务的「交付与质量」打开「查看仓库集成」／“Open repository integration”。入口以当前任务及运行的正式验收为依据，打开同一任务的仓库集成面板并将焦点移至该面板的展开按钮。面板重新读取当前可用候选、目标和权限。导航只读取信息。选取候选、选择目标、准备、批准和执行由用户分别完成。

准备显示固定的候选提交、目标基线和实际文件变更。批准绑定当前计划版本、候选依据摘要及准备回执摘要。执行由持有项目授权的 Pool Runner 领取，并由独立 Git Provider 在管理员配置的隔离目标上推进 Git 引用。应用完成后，以原操作回执核对目标提交和来源包摘要。

## 验收项目

| 范围 | 要核对的证据 | 结果 |
| --- | --- | --- |
| 原始候选 | 复用 Pool 集成测试中的同一 WorkBuddy 来源夹具、独立 Runner 的 JUnit 结果和正式验收。仓库选项提供当前任务版本、验收运行、候选 Git 提交和密封包摘要。 | 通过，两个 Windows Runner 进程各自执行来源封存和独立测试 |
| 真实准备与审批 | 独立裸仓库从源基线开始。实际 Git 准备给出快进及变更文件。批准前执行和使用旧版本执行分别被拒绝，目标引用仍指向基线。 | 通过，基线直到授权应用前保持原值 |
| 原操作应用与恢复 | 实际 Runner 请求带当前 Pool 授权。准备回执不能替代应用回执。原结果重传后计划只确认一次。控制面回执与 Git 回执引用、回执对象、目标提交和来源包摘要一致。 | 通过，Git 与控制面的原操作回执一致 |
| 界面与国际化 | 检查中英文入口、当前验收限定、键盘焦点和任务内定位。打开面板只发出读取请求。受管操作使用原有独立权限与确认。 | 通过，组件测试覆盖中英文、焦点与零写入 |

后端使用现有 `TestTaskDeliveryRepositoryDispatchPoolActualIndependentQuality`，通过 `assertActualRepositoryDeliveryIntegration` 延长同一候选的验证链。夹具运行实际 Git、SQLite、回环 HTTP 和受管 Git Provider。来源封存和独立测试由两个实际 `suma-runner` 进程完成，受管准备和应用由同一 Pool 内独立登记的 Runner 适配器执行。测试耗时 17.16 秒。浏览器组件检查共 18 项，另通过 TypeScript 与双语目录检查。厂商在线账号、跨主机、远程托管写入与生产发布属于各自的验收范围。应用源码版本、应用范围构建证明和部署结果使用各自的证据。

## Multica 参考

参考本地 `D:\codes\github\multica` 固定提交 `e8b6a97643ccc35efbcf2a4675c8d088a288ed6e`。Multica 为评审任务绑定当前关联 PR 的 HEAD，避免旧提交的待处理评审覆盖新提交。查询同时考虑 GitHub 和自托管 VCS。 [评审提交解析](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L1241-L1265) · [评审查询](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/pkg/db/queries/github.sql#L197-L210)

PR 事件触发 GitHub 权威 CI 与可合并状态刷新。关联 PR 的状态决定任务是否可按规则推进。速码本阶段复用固定提交和状态核对思路，受管 Git 写入走速码已有独立授权、审批与回执协议。 [GitHub 事件入口](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/handler/github.go#L1314-L1335) · [关联 PR 状态规则](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/handler/pr_auto_complete.go#L224-L239)
