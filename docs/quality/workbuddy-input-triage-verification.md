# WorkBuddy 等待输入分诊验证

T-WORKBUDDY-INPUT-TRIAGE · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157.0.8103.38。隔离控制面及合同基线为 84ec1de。

- 平台、解码和生成器共 14 项定向检查通过。覆盖平台等待和已保存供应方状态一致、最近观察失败、正式任务改绑、40 条有界扫描和跨筛选游标拒绝。共享目录的并行诊断改动曾干扰生成器检查，在隔离基线复验通过。
- CLI 4 项顶层检查通过。网页 36 项进度与路由检查、42 项原节点和协作回归通过。校验响应范围、旧记录拒绝、当前正式任务关联、精准尝试和按原权限保留操作。
- 390px 中文深色 Chrome 使用正式 App、真实 HTTP 和临时 SQLite。项目只读成员按任务筛选、打开原等待尝试、返回保留条件并查看空态。原授权人打开同一尝试，从等待提示进入并聚焦继续协作，只读取原协作视图。0 次写入，供应方创建与查询计数不变。
- 11090 对中英文词条检查通过。共享工作区整站类型检查命中并行 FailureDiagnosisLauncher 的合同字段错误。将两份并行前端源码仅在编译读取时替换为 84ec1de 基线后，整站类型检查通过。工作区其他代码未改动。
- OpenAPI 生成及复核通过。1100 个模型 Schema、496 条 API 路径。供应方状态来自受控测试夹具，真实供应方输入、物理多节点和生产验收仍分别待验。

    go test -ldflags '-s -w' ./internal/workbuddycloudcontract ./internal/platform ./cmd/suma-contracts -run '^Test(DecodeCloudProgress|WorkBuddyCloudProgress)' -count=1 -timeout=100s -v
    go test ./internal/cli -run '^TestWorkBuddyProgressListCLI' -count=1 -v
    go run -ldflags '-s -w' ./cmd/suma-contracts --check
    vitest run src/WorkBuddyProgressWorkspace.test.tsx src/task-delivery-navigation-model.test.ts
    vitest run src/WorkBuddyCloudTaskWorkspace.test.tsx src/WorkBuddyCloudTaskPanel.test.tsx src/WorkBuddyCloudTurns.test.tsx src/workbuddy-cloud-runner.test.tsx
    go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestWorkBuddyInputTriageActual$' -count=1 -timeout=115s -v
    node apps/web/scripts/check-i18n.mjs
    node <阶段目录>/check-type.cjs
    node scripts/check-docs.mjs

[交互](../design/workbuddy-cloud-task.md#等待输入筛选) · [接口](../../contracts/workbuddy-cloud-status-ui.md#项目进度列表)
