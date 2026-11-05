# 批量调整任务优先级验证

T-MULTICA-TASK-PRIORITY。Windows、Go 1.26.10、Vite 6.1.0、Chrome 和隔离 SQLite。

- `go test ./internal/platform -run '^TestTaskPriorityChangesHTTP' -count=1 -v` 通过 3 项及 6 个子用例。覆盖真实 DAG 和正式 Review 后的验收保留、已验收依赖项不受影响、自定义原值、混合空变更、当前权限下的原回执及权限撤回。项目、版本或状态冲突与活动写入失败均整批回滚。
- `go test ./cmd/suma-contracts -run '^TestTask(PriorityChange|OwnerChanges|IterationAssignment)PublicContract$' -count=1` 与 `go run ./cmd/suma-contracts -check` 通过。生成 1032 个模型和 465 条路径，既有路径与模型逐项比较一致。
- `npm --prefix apps/web run test -- src/TaskPriorityChanges.test.tsx src/write-action-recovery.test.tsx` 通过 13 项。TypeScript 与双语检查通过，10236 对词条，生产硬编码为 0。首次实际目视发现数量词条的占位符语法有误，修正后复跑 5 项直接相关 Web 测试。最终隔离 Web 生产构建通过，保留既有大资源包警告。
- 最终实际 Chrome 通过 12 项断言。列表选择三项后切换到按优先级分组的看板，键盘打开逐项预览。首次 HTTP 200 已提交后模拟响应中断，整页刷新再恢复同一正文和回执。两项各增加一个版本，同值和未选任务原样保留，刷新后看板准确归组。中文 1366px 和英文深色 390px 确认显示真实数量，取消及预览零写入，无渲染错误。
- 复验报告 `.cache/task-priority-changes/browser-8SXI7Q/result.json`，SHA256 `fef01940f49658d8d662ad3c406e64342ff68fd1e517fb80dac871281a25e706`。保留两轮报告与代表截图，232 个构建及隔离临时文件按摘要清理，回执见 `.cache/task-priority-changes/scratch-cleanup.json`。
- 本阶段覆盖当前 Web 项目写权限及显式选择。其他批量动作、CLI 入口、物理多机、其他数据库及生产验收仍按原计划推进。
