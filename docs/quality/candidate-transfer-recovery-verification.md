# 未封存候选代码传递验证

T-CANDIDATE-TRANSFER-RECOVERY。Windows、Go 1.26.10、Vite 6.1.0、Chrome 和隔离 SQLite。

- `go test ./internal/platform -run 'TestManualWorker(CandidateRecovery|UploadRecovery)|TestManualRunnerRotation|TestDesktopRunnerRenewal|TestCandidateTransfer' -count=1` 通过。覆盖真实授权与过期凭证、原尝试及围栏核对、分页越过已封存记录、异常结果拒绝、保留字节、恢复回执与凭证轮换。
- `go test ./cmd/suma-contracts -run '^TestManualRunner(CandidateRecovery|UploadRecovery|RotationContract)' -count=1` 与 `go run ./cmd/suma-contracts -check` 通过。生成的 1028 个模型、464 条 API 路径与 TypeScript 合同一致。
- `npm --prefix apps/web run test -- src/WorkerArtifactUploadRecovery.test.tsx src/ToolWorkerIdentityList.test.tsx src/write-action-recovery.test.tsx` 通过 26 项。TypeScript 和国际化检查通过，10203 对词条，生产代码硬编码检查为 0。隔离 Web 生产构建通过，保留既有包体积警告。
- Chrome 实测通过 9 项。创建真实 Git bundle，用原尝试身份经 HTTP 上传。隔离 DAG 将尝试明确置为失败，确认前轮换被拒，用户结束传递后原回执可重放，原字节 SHA256 不变，原机器封存被拒，另行确认后同一节点签发新凭证。目视中文桌面和 390px 英文深色。复验移动确认控件尺寸、键盘焦点和布局，无浏览器错误。
- 浏览器回执 `.cache/candidate-transfer-recovery/browser-rmsp1E/result.json`，SHA256 `7a77d9acd45b2348efba3215269ec8352ac4797d2c326edcf8f6353990bca0d3`。两张截图和报告不含凭证。为 299 个隔离构建与临时文件记录摘要后完成清理，回执保存在 `.cache/candidate-transfer-recovery/scratch-cleanup.json`。
- 本轮使用真实候选协议和 DAG，终态来自受控夹具。原尝试仍运行、结果未知或依据矛盾时保留阻塞。共享资源池、远程配对、桌面托管和第三方真实进程留在各自验证范围内。
