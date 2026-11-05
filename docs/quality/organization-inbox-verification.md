# 组织内跨项目收件箱验证

T-MULTICA-ORGANIZATION-INBOX。Windows、Go 1.26.10、Vite 6.1.0、Chrome 与隔离 SQLite。验证使用任务独立构建和临时数据库。

- 组织收件箱、新旧通知来源与权限的 Go 定向命令 `go test ./internal/platform -run '^Test(NotificationOrganizationInbox|NotificationInbox|TaskConversationMentionInbox|TaskHandoffInbox|TaskSubscription)' -count=1` 通过 37 项。覆盖跨项目纳秒排序、平序键、全范围计数、当前个人状态、来源核对、游标撤权、跨组织隔离及项目 API／Runner／Agent 拒绝。首轮新增夹具使用了会覆盖项目 ID 的历史辅助函数，改用准确项目的入口后复验。
- `go test ./cmd/suma-contracts -run '^TestNotification.*InboxContract' -count=1` 与生成合同核对通过。1047 个模型、470 条 API 路径。结构比对确认仅增加一个组织读取路径及两个 DTO，既有 API 和模型保持一致。
- Web 的通知界面和读取模型 37 项、通知及交付定位 9 项、共享写入恢复 8 项检查通过。Web TypeScript、双语词条及隔离生产构建通过，词条 10350 对、生产硬编码 0。构建保留既有包体积警告。
- Chrome 最终通过 11 项。使用真实的所有者和受邀人员，通过正式任务分配和讨论事务生成通知。验证当前项目和组织范围、真实计数和来源项目、另一项目的明确已读动作、准确任务／提及定位、撤权后旧游标拒绝和重新读取、分页及无额外写入。检查 1366px 桌面和 390px 深色界面。
- 前两轮分别修正夹具中的同秒事件顺序假设、HTTP 状态读取方式。两份结果保留，最终使用同一产品构建通过。
- 报告 `.cache/organization-inbox/browser-3ruE7W/result.json`，SHA256 `7b6485bc8b0fb85078a6c647f5af5aa8354ab85757cba9e89612f873691f29ae`。报告和原始截图保留在任务忽略目录；经摘要盘点并清理 296 个临时文件，见 `.cache/organization-inbox/scratch-cleanup.json`。
- 本阶段是浏览器组织内聚合。完整通知种类、外部消息送达、多机和其他数据库的规模与部署验收继续按原计划推进。
