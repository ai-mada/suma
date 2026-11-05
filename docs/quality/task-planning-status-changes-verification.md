# 批量调整任务计划状态验证

T-MULTICA-TASK-PLANNING-STATUS。Windows、Go 1.26.10、Vite 6.1.0、Chrome 和隔离 SQLite。

- `go test ./internal/platform -run '^TestTaskPlanningStatusesHTTP' -count=1 -v` 通过 5 项。覆盖当前执行及原运行待核对、失败和取消后的资格、成功运行转验收、正式验收保护、仍活动的历史运行和补偿、预览后原执行变化、跨项目及版本冲突整批零写入、原回执和真实双运行并发。
- `go test ./cmd/suma-contracts -run '^TestTask(PlanningStatus|PriorityChange|OwnerChanges|IterationAssignment)PublicContract$' -count=1` 及 `go run ./cmd/suma-contracts -check` 通过。生成 1039 个模型、467 条路径，逐项确认既有 API 路径和模型保持一致。
- `npm --prefix apps/web run test -- src/TaskPlanningStatusChanges.test.tsx src/write-action-recovery.test.tsx` 通过 13 项。TypeScript 和双语检查通过，10278 对词条，生产硬编码为 0。隔离生产构建通过，保留既有大资源包警告。
- Chrome 复验通过 11 项。用真实 DAG 的等待审批运行验证混合批次阻塞，确认浏览器只读预览。回到列表调整选择，再跨看板执行同一批次。HTTP 200 已提交后丢弃响应，整页刷新恢复原正文和回执。两项各增一个版本，同值项及未选任务保留，原运行状态和版本未变。中文桌面与 390px 英文深色已目视，预览和取消不发出写入。首轮夹具在服务端回执到达前判断失败，调整为等待真实响应后再核对。
- 浏览器回执 `.cache/task-planning-status-changes/browser-0qdbn6/result.json`，SHA256 `fc22991f38ff92713b43ee46d2e0195935424f9dee6b8885914adb2494a515d1`。三张截图和原报告保留在忽略目录。已核对 747 个临时文件摘要后清理，回执见 `.cache/task-planning-status-changes/scratch-cleanup.json`。
- 本阶段覆盖 Web 计划状态和当前任务所属运行。CLI、其他批量操作、物理多机、其他数据库和生产验收继续按原计划推进。
