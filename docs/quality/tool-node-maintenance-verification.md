# 执行节点权限与凭证维护验证

T-TOOL-NODE-MAINTENANCE。Windows、Go 1.26.10、SQLite、Vite 6.1.0、Chrome。工具管理的配置、检测和凭证维护均指向同一节点。凭证的有效期、在线状态、工具可用性和实际派发分别核对。

- `go test ./internal/platform -count=1 -v -run 'Test(ReleaseWorkerList|ManualRunnerRotation|ReleaseWorkerCredentialOnceOnlyAndRevocation|AuthorityDomainCHumanReadsRejectRevokedCredential|AuthorityDomainCProjectAPIReadScopeIsPreserved)'` 最终通过 8 个不同顶层用例及 13 个子例。覆盖空列表、只读能力字段、轮换持有人、API 令牌、当前降权、撤权、跨项目、跨租户和既有凭证生命周期。首轮组合用例错误地以项目管理员签发 API 令牌，服务端按原策略拒绝；改用组织所有者签发后单独复验通过。
- `go test ./cmd/suma-contracts -run '^TestRelease(RoutesWireAndAuthorityBoundaries|PagingAndExactDecisionSchemas)$' -count=1` 两项通过。并行仓库阶段落地后重建并运行 `suma-contracts -check`，验证 1055 个模型、473 条路径及 TypeScript。结构比对确认只有节点列表 GET 响应及其新增 DTO 变化。
- `node node_modules/vitest/vitest.mjs run src/ToolWorkerIdentityList.test.tsx src/ToolWorkspace.test.tsx src/ToolReadiness.test.tsx src/ToolQuickConnect.test.tsx src/RunnerPairingWorkspace.test.tsx src/ToolWorkspaceManagement.test.tsx src/ToolWorkspace-recovery-navigation.test.tsx src/ToolWorkspace-experience.test.tsx --maxWorkers 2 --minWorkers 1` 覆盖 63 个不同用例，首轮 61 项通过，修正夹具及已有高级替换路径的过时断言后定向复验通过。基线独立重现了替换审核旧断言失败。TypeScript 最终通过，双语 10387 对词条及生产硬编码 0。
- 隔离 Chrome 和 SQLite 最终 9 项通过、零页面或运行错误。实际项目管理员与只读成员查看同一节点；后者直接调用登记和撤销均被拒绝。键盘展开、取消回焦及降权后关闭旧确认并把焦点移到可见标题均通过。恢复权限后，明确确认只发送一次带原版本的撤销，节点进入历史并移除配置、检测和凭证操作。复核中文明亮 1366px、英文暗色 390px 和 WorkBuddy 入口可见。控制面基于 `60fa16a` 隔离并行仓库改动构建；前端使用最终阶段源码单独构建。第一轮改为等待真实回焦，第二轮通过 8 项，修复窄屏标题定位后最终通过 9 项。
- 最终报告 `.cache/tool-node-maintenance/browser-evidence-4YQNo7/result.json`，SHA256 `3cc3f589ccbd000d9564da1b6ffc5d39e873e81f92ac2d7d4105539eb5ff8597`。保留三轮报告及原图。Chrome 正常关闭，隔离控制进程结束并释放端口。已盘点并清理 304 个浏览器构建与运行临时文件及 18 个合同临时文件，收据分别保留在 `browser-cleanup.json` 和 `contract/scratch-cleanup.json`。

本阶段的实际验收范围为节点凭证与页面交互。真实厂商授权、工具运行、物理多机和其他数据库继续按各自计划验证。
