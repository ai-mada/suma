# WorkBuddy 进度与任务交付验证

T-WORKBUDDY-PROGRESS-TASKS 于 2026-11-06 在 Windows、Go 1.26.10、SQLite 和 Chrome 156.0.8078.80 完成定向验证。Go 检查使用 `8a1f35c` 的隔离快照及阶段文件。网页在临时本机端口连接同一 Go 控制面。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts ./internal/cli -run '^(TestWorkBuddyCloudProgress|TestWorkBuddyProgressListCLI|TestWorkBuddyCloudNodeLocatesFormalTaskDelivery)' -count=1 -v` 通过。覆盖正式来源、任务改名、改绑、解绑、标题不可用、已删除与独立运行、读权限及错误关联。CLI 拒绝孤立、重复及携带私有字段的关联。单节点进度响应保持原字段。
- `go run -ldflags '-s -w' ./cmd/suma-contracts --check` 通过，核对 OpenAPI 和 TypeScript 合同。Web 的 `WorkBuddyProgressWorkspace.test.tsx` 和 `task-delivery-navigation-model.test.ts` 共 21 项通过，`node node_modules/typescript/bin/tsc --noEmit` 与 10,948 个语言键检查通过。
- `go test -ldflags '-s -w' -overlay <阶段验证夹具> ./internal/platform -run '^TestWorkBuddyProgressTasksActual$' -count=1 -timeout=120s -v` 通过。正式派发后用英文亮色桌面键盘打开该次交付并返回「待处理」。通过本机候选流程完成改绑后，用中文暗色 390px 触屏打开当前交付并返回「待处理」。只读项目成员沿当前授权访问。编译的 CLI 使用项目凭据分别读取两个阶段，供应方调用计数与 SQLite 业务记录在读取阶段保持一致。

两种浏览器界面和结构化回执保存在阶段证据目录。供应方 HTTP 由测试夹具提供，生产 WorkBuddy 账号派发及供应方状态联调仍待验证。
