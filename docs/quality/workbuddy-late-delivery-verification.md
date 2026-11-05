# WorkBuddy 迟到完成与原尝试交付验证

T-WORKBUDDY-LATE-DELIVERY-240 · 速码 1.0.0 · 2026-11-05。 [派发与观察合同](../../contracts/workbuddy-task-dispatch.md#迟到的云端完成观察) · [架构设计](../suma-architecture-design.html#workbuddy-local-candidate-239)

在原运行节点刷新 WorkBuddy 任务状态。本人授权仍有效且原尝试可推进时，时间线显示“WorkBuddy 云端已完成 · 等待交付证据”。任务的“交付与质量”随即可以定位原尝试，再沿现有终端候选流程处理代码、独立测试与人工验收。

| 范围 | 实际结果 |
| --- | --- |
| Windows、Go 1.26.10、SQLite、Suma HTTP、回环 WorkBuddy | 首次 Runner 等待输入或等待确认后，查询到原云任务完成，原节点继续等待并更新交付原因。任务详情返回同一运行、节点和尝试。原尝试结果、执行代次和验收状态保留。 |
| 异常与重放 | 停止未确认、节点代次已变化时仅保存供应方观察。SQLite 故障注入拒绝事件写入后，交付状态和命令回执一起回滚。同一键可再次查询并提交一次有效更新。已提交键不再查询供应方，新键查询同一完成状态不会再次更新运行或派发。 |
| 候选交接 | `TestWorkspaceTerminalCandidateWorkBuddyOrigin` 通过，复核现有本机候选原来源约束。 |
| Web | 云任务工作区及 Runner 状态组件两套定向测试共 18 项通过，Web TypeScript 检查通过，10,328 对中英文词条通过占位符、复数和标记检查。本次复用已有刷新与跳转，时间线增加中英文事件标题。 |

查询接口的 `completed` 是对应云任务的生命周期状态。正式 Git 候选、测试证据和人工验收分别处理。原任务与事务更新的设计参考 Multica 固定提交的守卫及重放处理。 [WorkBuddy Open API](https://open.workbuddy.cn/docs/openapi#查询云端任务) · [Multica 状态更新](https://github.com/multica-ai/multica/blob/e8b6a97643ccc35efbcf2a4675c8d088a288ed6e/server/internal/service/task.go#L4639-L4676)

本轮验证使用回环供应方和 SQLite。真实 WorkBuddy 账号、可信云端 Git 导入、OceanBase 并发及生产验收仍待验证。此次界面核验为组件级，沿用前序阶段已验证的原运行定位交互。
