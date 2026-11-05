# 共享任务视图在线核对验证

T-MULTICA-SHARED-VIEW-LIVE。Windows、Go 1.26.10、Vite 6.1.0、Chrome、隔离 SQLite。因 D 盘余量不足，临时构建和浏览器状态放在本阶段 E 盘目录。

- `go test ./internal/platform -run '^TestSharedTaskView' -count=1 -v` 通过 17 项及 22 个子用例。新测试覆盖精确只读合同、当前身份与管理资格、其他账号的私有变更不影响标识、分页之外的更新及归档、重放稳定、撤权及机器身份拒绝。
- `go test ./cmd/suma-contracts -run '^Test(SharedTaskView|TaskViewColumns)' -count=1`、生成与 `go run ./cmd/suma-contracts -check` 通过。1044 个模型、469 条路径。独立结构比对确认只新增同步接口和对应模型，既有 API 保持一致。
- `node node_modules/vitest/vitest.mjs run src/SharedTaskViews.test.tsx`（apps/web）通过 18 项。覆盖条件未自动应用、过时状态恢复、隐藏或断网暂停、单读取、分页外视图、当前项目撤权、编辑期间同版本权限降级。TypeScript 与双语检查通过，10295 对词条，生产硬编码 0。隔离生产构建通过，保留既有包体积警告。
- Chrome 最终通过 8 项断言。真实所有者与受邀项目只读成员，两个独立浏览器上下文。验证自动发现另一账号更新、当前过滤保留、显式应用、断网及重连、归档退出选择、撤权移除私有目录。中文桌面和 390px 英文深色截图已目视。前两轮分别修正浏览器夹具的桌面筛选展开步骤和筛选控件定位；最后一轮完整通过。
- 报告 `.cache/shared-view-live/browser-imd4kh/result.json`，SHA256 `3b2a82892f4bff54eeff42881711cea241cf3c948edbb27d556a6e5a0a0cfb5e`。保留浏览器报告与截图，E 盘任务目录的 309 个文件记录摘要后清理。回执见 `.cache/shared-view-live/scratch-cleanup.json`。
- 本阶段覆盖在线读取与显式应用。未知写入的原请求保留在当前面板。跨页面重载的写入恢复、离线编辑合并、其他数据库、物理多设备和生产验收仍按原计划推进。
