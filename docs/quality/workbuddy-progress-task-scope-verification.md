# WorkBuddy 按任务查看云记录验证

T-WORKBUDDY-PROGRESS-TASK-SCOPE · Windows · Go 1.26.10 · Chrome 157.0.8103.38 · SQLite。隔离 Go 快照基线 `3de10f8` 叠加本阶段源码。网页通过隔离本机端口读取该控制面。

- `go test -ldflags '-s -w' ./internal/platform ./cmd/suma-contracts ./internal/cli ./internal/workbuddycloudcontract -run '^(TestWorkBuddyCloudProgress|TestWorkBuddyProgress|TestWorkBuddyCloudNodeLocatesFormalTaskDelivery|TestDecodeCloudProgressTaskScope)' -count=1 -v` 通过。覆盖任务缺失、跨项目、改绑／解绑、筛选游标隔离、空批次续查和拒绝不匹配的任务关联。
- `go run -ldflags '-s -w' ./cmd/suma-contracts --check` 通过。OpenAPI 与 TypeScript 使用生成合同。Web 的 `WorkBuddyProgressWorkspace.test.tsx`、`TaskDeliveryPanel.test.tsx` 和 `task-delivery-navigation-model.test.ts` 共 44 项通过，TypeScript 与双语词条检查通过。
- `go test -ldflags '-s -w' -overlay <阶段夹具> ./internal/platform -run '^TestWorkBuddyProgressTaskScopeActual$' -count=1 -timeout=120s -v` 通过。项目只读成员在英文桌面用键盘、中文暗色 390px 用触屏，分别完成行内进入任务范围、保留待处理条件、清除范围、从任务交付再次进入和浏览器返回。改绑后仍指向原 WorkBuddy 执行。编译后的 CLI 用项目凭证验证两个阶段。读取期间供应方调用和业务记录无变化。

阶段证据目录保留截图、脚本及结构化回执。供应方状态来自受控 HTTP 夹具。真实账号派发、生产规模和跨数据库验证仍单独记录。任务筛选沿提交索引以最多 40 条为一批核对正式来源，查完前可继续翻页。

[交互](../design/workbuddy-cloud-task.md#团队进度工作台) · [接口](../../contracts/workbuddy-cloud-status-ui.md#项目进度列表) · [命令行](../operations/task-dispatch-options.md#终端运行跟踪)
