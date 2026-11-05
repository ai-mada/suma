# 已选通知批量处理验证

T-MULTICA-INBOX-BATCH。Windows、Go 1.26.10、Vite 6.1.0、Chrome 和隔离 SQLite。验证使用独立构建与临时数据库。

- `go test ./internal/platform -run '^Test(NotificationInboxBatch|NotificationInbox|NotificationOrganizationInbox|TaskRunFailureInbox)' -count=1` 通过 21 项。覆盖跨项目只读成员的个人批量操作、原请求重放、撤权后拒绝回执、无变化项保留版本、真实版本冲突整批回滚及单条通知回归。

- `go test ./cmd/suma-contracts -run '^TestNotification.*InboxContract' -count=1` 及生成校验通过。1052 个模型、472 条 API 路径。结构比较确认新增一个浏览器批量入口和四个 DTO，旧接口与类型保持一致。并行仓库提供方源代码在隔离编译中沿用提交版本。
- `node node_modules/vitest/vitest.mjs run src/NotificationInbox.test.tsx src/notification-inbox-model.test.ts` 通过 43 项。覆盖显式勾选、准确版本与顺序、原回执重试、状态刷新和冲突后的重选。TypeScript、双语词条和独立 Vite 生产构建通过，10381 对词条、生产硬编码 0。合并冲突提示后，收件箱组件原 30 项通过复验。构建保留既有体积警告。
- Chrome 最终通过 8 项，零运行错误。使用真实项目成员会话和两个项目通知验证键盘选择、提交选中项、未选记录保留、同请求回执重放、第二项实际冲突后的整批回滚及明确重读。核查桌面和 390px 窄屏。
- 首次浏览器通过 7 项。目视检查发现通用冲突与批量提示重复，合并后以相同后端重验 8 项，核对单一提示和刷新动作。保留两次结果。
- 报告 `.cache/inbox-batch/browser-997oeL/result.json`，SHA256 `d3b8ebf29d3517d258d6b328eb92dad3557b7d217fdcbf273a8bc1a0e69baecb`。报告和原始截图保留在任务忽略目录，临时构建、浏览器数据和隔离数据库按摘要盘点后清理，共 308 个文件，见 `.cache/inbox-batch/scratch-cleanup.json`。
- 本阶段验证浏览器的已选项批量处理。完整通知来源、外部投递、其他数据库和多控制面规模验收继续按原计划推进。
