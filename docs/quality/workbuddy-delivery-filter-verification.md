# WorkBuddy 待交付筛选验证

T-WORKBUDDY-DELIVERY-FILTER · Windows amd64 · Go 1.26.10 · SQLite · Chrome 157.0.8103.38。隔离控制面和合同以 13ae6aa 及本阶段代码为输入。

- 12 项平台、共享解码与生成器定向检查通过。覆盖已暂停、已完成、改绑、解绑、独立提交及撤权后的排除，验证任务范围和空批次续查。改绑后原云运行的等待标记仍保留在历史中，待交付筛选不会将它作为当前工作。
- CLI 4 项顶层检查通过。网页进度和深链共 33 项定向检查通过，涵盖当前关联、准确尝试、错误响应拒绝，以及加载更多失败后用原游标重试。完整 Web 类型检查及 11080 对中英文词条检查通过。
- 390px 中文深色 Chrome 在真实 App 中使用独立项目只读成员身份，完成待交付筛选、组合准确任务范围、打开原运行节点与尝试、浏览器返回保留范围和无匹配空态。浏览器 0 写入，WorkBuddy 创建及查询计数不变，运行、任务及提交记录无新增。
- 生成合同经复核，包含 1100 个模型 Schema 和 496 条 API 路径。供应方状态来自受控测试夹具，真实账号执行、物理多机与生产验收独立记录。

    go test -ldflags '-s -w' ./internal/workbuddycloudcontract ./internal/platform ./cmd/suma-contracts -run '^Test(DecodeCloudProgress|WorkBuddyCloudProgress)' -count=1 -timeout=100s -v
    go test ./internal/cli -run '^TestWorkBuddyProgressListCLI' -count=1 -v
    go run -ldflags '-s -w' ./cmd/suma-contracts --check
    vitest run src/WorkBuddyProgressWorkspace.test.tsx src/task-delivery-navigation-model.test.ts
    tsc --noEmit -p apps/web/tsconfig.json
    node apps/web/scripts/check-i18n.mjs
    go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestWorkBuddyDeliveryFilterActual$' -count=1 -timeout=115s -v
    node scripts/check-docs.mjs

[交互](../design/workbuddy-cloud-task.md#待交付筛选) · [接口](../../contracts/workbuddy-cloud-status-ui.md#项目进度列表)
