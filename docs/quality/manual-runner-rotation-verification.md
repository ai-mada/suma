# 手动执行节点凭证轮换验证

T-MANUAL-RUNNER-ROTATION。Windows、Go 1.26.10、Vite 6.1.0、Chrome、隔离 SQLite。控制面、Web 静态文件、临时数据库和浏览器证据均在 `.cache/manual-runner-rotation/`，与活动预览隔离。

- `go test ./internal/platform -run '^(TestManualRunnerRotation|TestReleaseWorkerCredentialOnceOnlyAndRevocation|TestDesktopRunnerRenewalWaitsForOriginalExecutionAndEffectResources)' -count=1 -v` 通过 5 个顶层及 4 个子用例。覆盖过期节点、原持有人、授权变化、机器托管凭证排除、在途任务和工具操作、供应商未决效果、一次性凭证、原回执及原子回滚。
- `go test ./cmd/suma-contracts -run '^TestManualRunnerRotationContract' -count=1` 通过。`go run ./cmd/suma-contracts -check` 核对 1020 个模型、460 条 API 路径与 TypeScript。
- `npm.cmd --prefix D:\codes\ai-dev-os\suma\apps\web run test -- src/ToolWorkerIdentityList.test.tsx src/api-signal.test.ts src/api-context.test.tsx --reporter=verbose` 通过 36 项，其中节点管理 8 项、5 项新增。覆盖一次性显示与复制、原请求在恢复时首次成功、提交后回执恢复和明确冲突。TypeScript 检查通过，国际化检查通过 10097 对词条和 0 项生产硬编码。首次尝试遇到 Windows 测试目录映射错误，使用绝对 npm 目录后完成。
- Web 生产构建使用 `vite build --outDir ../../.cache/manual-runner-rotation/web --emptyOutDir --logLevel warn`，完成并报告现有大资源包警告。
- 实际 Chrome 通过 8 项断言。原凭证先能上报库存，用户在工具管理确认后只发起一次浏览器轮换，控件默认遮盖新凭证且浏览器恢复存储中无凭证。使用完全相同的请求和操作编号验证回执恢复。原凭证随后被拒绝，新凭证成功为同一节点上报库存。中文桌面和 390px 英文深色确认截图均已目视，取消无写入，浏览器错误为零。
- 浏览器回执 `.cache/manual-runner-rotation/browser-Zmia6H/result.json`，SHA256 `0041981b173ad43764da0514a7301209ce13e1e1991aeaefa4bf677b11b1ff80`。报告和截图不含原始凭证。
- 实际轮换和库存上报使用本机隔离控制面及真实 HTTP。机器进程停止由操作者确认，本阶段使用测试节点且未启动真实 Runner。过期凭证的未封存制品继续阻止轮换，安全处置入口待补。远程配对、桌面和共享池按各自的授权流程处理。
