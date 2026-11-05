# 批量任务 CLI 验证

T-MULTICA-TASK-BATCH-CLI。Windows、Go 1.26.10、真实 `suma.exe`、控制面和隔离 SQLite。

- `go test ./internal/platform -run '^Test(TaskBatchCLIUsesCurrentProjectAuthority|TaskPriorityChangesHTTP|TaskPlanningStatusesHTTP)' -count=1 -v` 通过 9 项及 6 个子用例。覆盖当前开发者、只读预览、精确版本、浏览器原视图、同一账号跨入口恢复，以及错误项目、只读角色、撤权、Runner 和 Agent 拒绝。原 DAG 和正式验收保护回归通过。
- `go test ./internal/cli -run '^Test(TaskBatchCLI|TaskOwnersCLI|ProjectContext)' -count=1 -v` 通过 13 项及 44 个子用例。覆盖精确文件与标准输入、稳定操作编号、响应范围及有序对象、未知提交和只读传输诊断。
- 任务批量相关的四组 API 合同测试与 `go run ./cmd/suma-contracts -check` 通过。生成 1042 个模型、468 条路径。独立结构比较确认只新增优先级预览及三个模型，既有改动限于三个批量入口的凭证声明。
- 实际 CLI 通过 10 项断言。验证中文帮助、错误输入零请求、优先级只读预览、控制面提交后丢失响应、同文件恢复、浏览器会话同回执、等待审批的真实运行阻塞整批、独立计划状态变更和跨项目拒绝。首轮夹具用对象键顺序比较回执，改用结构比较并保留任务数组顺序后通过。
- 报告 `.cache/task-batch-cli/cli-vn8FxS/result.json`，SHA256 `d849db5630ea23216fea3d9aa15ea629a421344baebac2f948847e1df31366ef`。两轮报告保留在忽略目录，16 个隔离构建和临时数据文件核对摘要后清理，回执见 `.cache/task-batch-cli/scratch-cleanup.json`。
- 本阶段涉及现有 CLI、项目写权限及 Web 共用的 API。其他批量动作、跨物理节点与生产规模验收仍按原计划推进。
