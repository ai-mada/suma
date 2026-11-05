# 未封存发布制品恢复验证

T-ARTIFACT-UPLOAD-RECOVERY。Windows、Go 1.26.10、Vite 6.1.0、Chrome 和隔离 SQLite。D 盘空间不足导致首轮生成和构建中断。随后在 `E:\tmp\cx\suma-a233` 独立构建，并将浏览器证据保存到项目忽略目录。

- `go test ./internal/platform -run '^TestManualWorkerUploadRecovery' -count=1 -v` 通过 3 项。覆盖真实 DAG 终态、过期凭证、原围栏、跨绑定拒绝、未决结果、分页、保留字节及声明配额、事务故障回滚、后续凭证轮换和原回执。较早的相关回归通过 8 个顶层及 4 个子用例，覆盖上传、封存及现有凭证流程。
- `go test ./cmd/suma-contracts -run '^TestManualRunner(RotationContract|UploadRecoveryKeepsScoped)' -count=1` 通过，重新生成并用 `go run ./cmd/suma-contracts -check` 核对 1024 个模型和 462 条 API 路径。重读后核对精确围栏、显式确认和安全响应范围。
- Web 定向检查通过 22 项，覆盖 `WorkerArtifactUploadRecovery.test.tsx`、`ToolWorkerIdentityList.test.tsx` 与 `write-action-recovery.test.tsx`。确认焦点和冲突处理更新后，6 项新用例再次通过。TypeScript 与国际化检查通过，10174 对词条、0 项生产硬编码。Web 生产构建完成，保留现有大资源包警告。
- 实际 Chrome 通过 9 项断言。隔离 DAG 创建并结束原尝试，真实 HTTP 接收一个文件，保留一个未上传文件。确认前轮换被拒，界面确认后得到同一结束回执，保留已收字节并拒绝新的写入，用户另行确认后为同一节点取得新凭证。中文桌面和 390px 英文深色界面已目视，确认焦点可达，无浏览器错误。
- 浏览器回执 `.cache/artifact-upload-recovery/browser-8HlPTw/result.json`，SHA256 `844759f1377238ea7172de575c42fde730bb8fa7cd8978c0ca2742e696249754`。包含两张代表截图，报告和截图不含凭证。此轮使用实际 DAG 和上传接口配合受控失败结果，真实构建工具和原进程停止由各自验收覆盖。
- 未知结果、非终态尝试和未完成的候选代码传递继续各自核对。此命令保留历史文件与声明配额，存储清理策略另行管理。
构建与浏览器临时目录清理前核对了证据副本，并为 215 个文件记录摘要。清理回执保存在 `.cache/artifact-upload-recovery/scratch-cleanup.json`。
